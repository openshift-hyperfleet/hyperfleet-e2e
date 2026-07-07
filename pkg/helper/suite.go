package helper

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"sync"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
)

var (
	// suiteConfig is loaded once in cmd layer before tests start
	suiteConfig *config.Config
	configMutex sync.RWMutex
	runID       string
)

// SetSuiteConfig sets the global suite configuration for the test suite
func SetSuiteConfig(cfg *config.Config) {
	configMutex.Lock()
	defer configMutex.Unlock()
	suiteConfig = cfg
}

// GetSuiteConfig returns the global suite configuration
func GetSuiteConfig() *config.Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return suiteConfig
}

// ClearSuiteConfig clears the global suite configuration
func ClearSuiteConfig() {
	configMutex.Lock()
	defer configMutex.Unlock()
	suiteConfig = nil
}

// SetRunID sets the global run ID for the test suite
func SetRunID(id string) {
	configMutex.Lock()
	defer configMutex.Unlock()
	runID = id
}

// GetRunID returns the global run ID for the test suite
func GetRunID() string {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return runID
}

// maxRunIDLength is the maximum allowed length for a run ID.
// Kubernetes label values are limited to 63 characters.
const maxRunIDLength = 63

// labelValueRegex matches valid Kubernetes label values per
// https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
var labelValueRegex = regexp.MustCompile(`^[a-zA-Z0-9]([-_.a-zA-Z0-9]*[a-zA-Z0-9])?$`)

// GetE2ETestRunID returns the run identifier for this E2E test suite execution.
// It reads the RUN_ID environment variable (set by CI/prow to the namespace name).
// Returns empty string if RUN_ID is not set (run-id is optional).
func GetE2ETestRunID() (string, error) {
	id := os.Getenv("RUN_ID")
	if id == "" {
		// Run ID is optional - return empty string
		return "", nil
	}

	// Validate run ID format and length
	if len(id) > maxRunIDLength {
		return "", fmt.Errorf("RUN_ID %q is %d characters, exceeds the %d-character Kubernetes label value limit", id, len(id), maxRunIDLength)
	}
	if !labelValueRegex.MatchString(id) {
		return "", fmt.Errorf("RUN_ID %q contains characters invalid for a Kubernetes label value", id)
	}
	return id, nil
}

// New creates a helper instance for testing
// Creates a new helper per test
func New() *Helper {
	cfg := GetSuiteConfig()
	if cfg == nil {
		log.Fatalf("Suite config not initialized")
	}

	h, err := newHelper(cfg)
	if err != nil {
		log.Fatalf("Failed to create helper: %v", err)
	}
	return h
}

// newHelper creates a new Helper instance (internal use)
func newHelper(cfg *config.Config) (*Helper, error) {
	cl, err := client.NewHyperFleetClient(cfg.API.URL, nil)
	if err != nil {
		return nil, err
	}

	k8sClient, err := k8sclient.NewClient()
	if err != nil {
		return nil, err
	}

	return &Helper{
		Cfg:       cfg,
		Client:    cl,
		K8sClient: k8sClient,
		RunID:     GetRunID(),
		// MaestroClient is initialized lazily via GetMaestroClient() to avoid
		// unnecessary K8s API calls in test suites that don't use Maestro
	}, nil
}
