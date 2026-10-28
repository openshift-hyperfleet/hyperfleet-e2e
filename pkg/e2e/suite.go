package e2e

import (
	"log"
	"log/slog"

	"github.com/onsi/ginkgo/v2"

	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
)

var (
	// suiteConfig is loaded once in cmd layer before tests start
	suiteConfig *config.Config
)

// SetSuiteConfig sets the global suite configuration for both e2e and helper packages
func SetSuiteConfig(cfg *config.Config) {
	suiteConfig = cfg
	helper.SetSuiteConfig(cfg)
}

// GetSuiteConfig returns the global suite configuration
func GetSuiteConfig() *config.Config {
	return suiteConfig
}

var _ = ginkgo.SynchronizedBeforeSuite(
	// Process 1 only: mint the JWT token once and share it, so parallel
	// processes don't each burn a redundant TokenRequest call at startup.
	func(ctx ginkgo.SpecContext) []byte {
		cfg := GetSuiteConfig()
		if cfg == nil {
			log.Fatalf("Suite config not initialized")
		}

		// Fail before any spec runs when the deployed API does not serve the
		// entity configuration this suite's descriptor table expects. Entity
		// routing is independent of authentication, so this also runs for
		// AUTH_MODE=NONE. Ginkgo does not execute setup nodes in dry-run, so
		// listing specs stays cluster-free even though this hook is registered.
		if err := helper.ValidateAPIEntityConfig(ctx, cfg.Namespace, config.EntityDescriptors()); err != nil {
			ginkgo.Fail(err.Error())
		}

		if config.IsDryRun() || cfg.AuthMode == config.AuthModeNone || !cfg.Identity.TokenRequest.IsEnabled() {
			return nil
		}

		k8s, err := k8sclient.NewClient()
		if err != nil {
			log.Fatalf("Failed to create K8s client for token acquisition: %v", err)
		}
		token, err := k8s.CreateToken(
			ctx,
			cfg.Identity.TokenRequest.Namespace,
			cfg.Identity.TokenRequest.ServiceAccountName,
			cfg.Identity.TokenRequest.Audience,
			cfg.Identity.TokenRequest.ExpirationSeconds,
		)
		if err != nil {
			log.Fatalf("Failed to acquire JWT via TokenRequest: %v", err)
		}
		return []byte(token)
	},
	// Runs on every process: apply the token minted above (if any) and finish suite setup.
	func(ctx ginkgo.SpecContext, tokenBytes []byte) {
		cfg := GetSuiteConfig()
		if cfg == nil {
			log.Fatalf("Suite config not initialized")
		}

		if err := initLogging(&cfg.Log); err != nil {
			log.Fatalf("Failed to initialize logger: %v", err)
		}

		if ginkgo.GinkgoParallelProcess() == 1 {
			cfg.Display()
		}
		slog.Info("starting hyperfleet-e2e test suite - creating resources with", "run-id", cfg.RunID)

		if len(tokenBytes) > 0 {
			cfg.Identity.SetToken(string(tokenBytes))
			slog.Info("acquired JWT for suite",
				"service-account", cfg.Identity.TokenRequest.Namespace+"/"+cfg.Identity.TokenRequest.ServiceAccountName,
				"audience", cfg.Identity.TokenRequest.Audience,
				"expires-seconds", cfg.Identity.TokenRequest.ExpirationSeconds)
		}

		// Initialize adapter deployment list - for test tiers that deploy temporary adapters.
		adapterDeploymentList := helper.InitAdapterDeploymentList()
		helper.SetAdapterDeploymentList(adapterDeploymentList)

		slog.Info("starting hyperfleet-e2e test suite - each test creates temporary resources")
	},
)

var _ = ginkgo.SynchronizedAfterSuite(
	// Per-process: sweep Pub/Sub resources. Safe to call from every process
	// since each tracks its own AdapterDeploymentList in memory.
	func() {
		helper.CleanupPubSubResources()
	},
	// Process 1 only, after all processes finish: sweep Helm releases and labeled K8s
	// resources. Running this once prevents sweeping resources that belong to specs still
	// executing on other processes.
	func() {
		helper.CleanupKubeResources()
		helper.ClearSuiteConfig()
		slog.Info("test suite completed")
	},
)
