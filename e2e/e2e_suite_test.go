// e2e_suite_test.go is the ginkgo CLI entry point for parallel E2E execution.
//
// This file intentionally uses _test.go so the ginkgo CLI can discover and
// compile it as a test binary. All E2E spec files remain plain .go (compiled
// into the main binary via blank imports in e2e.go). This file is never part
// of the production binary - go build ignores _test.go files.
//
// The blank imports in e2e.go (same package) register all spec suites, and
// importing pkg/e2e registers the BeforeSuite/SynchronizedAfterSuite hooks.
//
// Usage:
//
//	ginkgo --procs=4 --label-filter=tier0 ./e2e
package e2e

import (
	"log"
	"os"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/cmd/hyperfleet-e2e/cliconfig"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"

	// Import pkg/e2e to register BeforeSuite and SynchronizedAfterSuite hooks.
	// The package name collides with this package, so we use an alias.
	pkge2e "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/e2e"
)

func TestMain(m *testing.M) {
	// Bootstrap configuration from env vars and config file.
	// The ginkgo CLI handles all test-execution flags (--label-filter,
	// --flake-attempts, --timeout, --junit-report, --procs) directly,
	// so we only load HyperFleet-specific config here.
	if err := cliconfig.LoadConfig(""); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration validation failed: %v", err)
	}

	pkge2e.SetSuiteConfig(cfg)

	os.Exit(m.Run())
}

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "HyperFleet E2E Suite")
}
