package test

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/cmd/hyperfleet-e2e/cliconfig"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/e2e"

	// Import test registry (which imports all test suites)
	_ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e"
)

var Cmd = &cobra.Command{
	Use:   "test",
	Short: "Run end to end tests",
	Long:  "Run end to end tests on HyperFleet System.",
	Args:  cobra.OnlyValidArgs,
	Run:   run,
}

var args struct {
	labelFilter   string
	focusTests    string
	skipTests     string
	junitReport   string
	dryRun        bool
	flakeAttempts int
}

func init() {
	pfs := Cmd.PersistentFlags()

	// Test control flags
	pfs.StringVar(&args.labelFilter, "label-filter", "",
		"Ginkgo label filter expression")
	pfs.StringVar(&args.focusTests, "focus", "",
		"Only run tests matching this regex")
	pfs.StringVar(&args.skipTests, "skip", "",
		"Skip tests matching this regex")
	pfs.StringVar(&args.junitReport, "junit-report", "",
		"Path to write JUnit XML report")
	pfs.BoolVar(&args.dryRun, "dry-run", false,
		"List matching specs without executing them")
	pfs.IntVar(&args.flakeAttempts, "flake-attempts", 1,
		"Number of attempts for flaky tests (1 = no retries, 3 = up to 2 retries)")
}

func run(cmd *cobra.Command, argv []string) {
	if err := cliconfig.LoadConfig(cliconfig.ConfigFile); err != nil {
		log.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	if err := bindFlagsAndEnv(cmd); err != nil {
		log.Printf("Error binding configuration: %v\n", err)
		os.Exit(1)
	}

	// Load and validate config (fast failure before entering Ginkgo)
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Configuration validation failed: %v\n", err)
		os.Exit(1)
	}

	e2e.SetSuiteConfig(cfg)

	exitCode := e2e.RunTests(cmd.Context())
	os.Exit(exitCode)
}

func bindFlagsAndEnv(cmd *cobra.Command) error {
	// Bind flags after config loading (osde2e pattern)
	pfs := cmd.Flags()
	for key, name := range map[string]string{
		config.Tests.GinkgoLabelFilter: "label-filter",
		config.Tests.GinkgoFocus:       "focus",
		config.Tests.GinkgoSkip:        "skip",
		config.Tests.JUnitReportPath:   "junit-report",
		config.Tests.GinkgoDryRun:      "dry-run",
		config.Tests.FlakeAttempts:     "flake-attempts",
	} {
		if err := viper.BindPFlag(key, pfs.Lookup(name)); err != nil {
			return fmt.Errorf("failed to bind flag %s: %w", name, err)
		}
	}

	// Bind parent command flags (api-url, logging flags)
	parentFlags := cmd.Parent().PersistentFlags()
	for key, name := range map[string]string{
		config.API.URL:    "api-url",
		config.Log.Level:  "log-level",
		config.Log.Format: "log-format",
		config.Log.Output: "log-output",
	} {
		if err := viper.BindPFlag(key, parentFlags.Lookup(name)); err != nil {
			return fmt.Errorf("failed to bind parent flag %s: %w", name, err)
		}
	}

	// Bind test environment variables
	for key, name := range map[string]string{
		config.Tests.GinkgoLabelFilter: "GINKGO_LABEL_FILTER",
		config.Tests.GinkgoFocus:       "GINKGO_FOCUS",
		config.Tests.GinkgoSkip:        "GINKGO_SKIP",
		config.Tests.JUnitReportPath:   "JUNIT_REPORT_PATH",
		config.Tests.SuiteTimeout:      "SUITE_TIMEOUT",
		config.Tests.GinkgoDryRun:      "GINKGO_DRY_RUN",
		config.Tests.FlakeAttempts:     "FLAKE_ATTEMPTS",
	} {
		if err := viper.BindEnv(key, name); err != nil {
			return fmt.Errorf("failed to bind environment for %s: %w", key, err)
		}
	}
	return nil
}
