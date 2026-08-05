package e2e

import (
	"log"

	"github.com/onsi/ginkgo/v2"

	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
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

var _ = ginkgo.BeforeSuite(func(ctx ginkgo.SpecContext) {
	cfg := GetSuiteConfig()
	if cfg == nil {
		log.Fatalf("Suite config not initialized")
	}

	if err := logger.Init(&cfg.Log, "dev"); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	cfg.Display()
	logger.Info("starting hyperfleet-e2e test suite - creating resources with", "run-id", cfg.RunID)

	if cfg.Identity.TokenRequest.IsEnabled() {
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
		cfg.Identity.SetToken(token)
		logger.Info("acquired JWT for suite",
			"service-account", cfg.Identity.TokenRequest.Namespace+"/"+cfg.Identity.TokenRequest.ServiceAccountName,
			"audience", cfg.Identity.TokenRequest.Audience,
			"expires-seconds", cfg.Identity.TokenRequest.ExpirationSeconds)
	}

	// Initialize adapter deployment list - for test tiers that deploy temporary adapters
	adapterDeploymentList := helper.InitAdapterDeploymentList()
	helper.SetAdapterDeploymentList(adapterDeploymentList)

	// Initialize adapter and api clones - setup no actual cloning happens

	// Initialize the gitClone for the adapter chart
	helper.AdapterGitClone = helper.NewGitClone(&helper.HelmChartCloneOptions{
		Component: "adapter",
		RepoURL:   cfg.AdapterDeployment.ChartRepo,
		Ref:       cfg.AdapterDeployment.ChartRef,
		RepoPath:  cfg.AdapterDeployment.ChartPath,
		WorkDir:   ".test-work",
	})
	// Initialize the gitClone for the api chart
	helper.APIGitClone = helper.NewGitClone(&helper.HelmChartCloneOptions{
		Component: "api",
		RepoURL:   cfg.APIDeployment.ChartRepo,
		Ref:       cfg.APIDeployment.ChartRef,
		RepoPath:  cfg.APIDeployment.ChartPath,
		WorkDir:   ".test-work",
	})

	logger.Info("starting hyperfleet-e2e test suite - each test creates temporary resources")
})

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
		logger.Info("test suite completed")
	},
)
