package e2e

import (
	"log"

	"github.com/onsi/ginkgo/v2"

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

var _ = ginkgo.BeforeSuite(func() {
	cfg := GetSuiteConfig()
	if cfg == nil {
		log.Fatalf("Suite config not initialized")
	}

	if err := logger.Init(&cfg.Log, "dev"); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	cfg.Display()

	// Get run ID (optional - empty string if not set)
	runID, err := helper.GetE2ETestRunID()
	if err != nil {
		log.Fatalf("Failed to get run ID: %v", err)
	}
	helper.SetRunID(runID)

	if runID != "" {
		logger.Info("starting hyperfleet-e2e test suite",
			"run_id", runID,
			"message", "each test creates temporary resources")
	} else {
		logger.Info("starting hyperfleet-e2e test suite",
			"message", "each test creates temporary resources (no run-id set)")
	}
})

var _ = ginkgo.AfterSuite(func() {
	runID := helper.GetRunID()
	helper.ClearSuiteConfig()
	logger.Info("test suite completed", "run_id", runID)
})
