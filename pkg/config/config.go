package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
)

const (
	// TagMapstructure is the struct tag used by Viper for configuration mapping
	TagMapstructure = "mapstructure"

	// AuthModeConfigKey is the configuration key for AUTH_MODE.
	AuthModeConfigKey = "auth_mode"

	// AuthModeNone disables gateway and API authentication.
	AuthModeNone = "NONE"
	// AuthModeAPI makes the API validate credentials directly.
	AuthModeAPI = "API"
	// AuthModeEdge makes the gateway validate credentials.
	AuthModeEdge = "EDGE"
	// AuthModeEdgeAPI makes the gateway validate credentials and the API validate its wristband.
	AuthModeEdgeAPI = "EDGE+API"

	// AuthorizationSchemeBearer selects direct API JWT or human OIDC validation.
	AuthorizationSchemeBearer = "Bearer"
	// AuthorizationSchemeServiceAccount selects gateway Kubernetes TokenReview.
	AuthorizationSchemeServiceAccount = "ServiceAccount"

	// EnvPrefix is the prefix for all environment variables (without trailing underscore)
	// Viper automatically adds underscore when using SetEnvPrefix()
	EnvPrefix = "HYPERFLEET"

	// RedactedPlaceholder is used to mask sensitive information in logs
	RedactedPlaceholder = "**REDACTED**"

	// NotSetPlaceholder indicates a configuration value has not been set
	NotSetPlaceholder = "<not set>"
)

// Validate RunID is a valid Kubernetes label value and is not empty for resource cleanup tracking
var LabelValueRegex = regexp.MustCompile(`^[a-zA-Z0-9]([-a-zA-Z0-9]*[a-zA-Z0-9])?$`)

// EnvVar constructs an environment variable name with the HYPERFLEET prefix
// Example: EnvVar("LOG_LEVEL") returns "HYPERFLEET_LOG_LEVEL"
func EnvVar(name string) string {
	return EnvPrefix + "_" + name
}

// API config keys
var API = struct {
	// URL is the HyperFleet API base URL
	// Env: HYPERFLEET_API_URL
	URL string
}{
	URL: "api.url",
}

// Authentication config keys
var Authentication = struct {
	// Mode controls where API authentication is enforced.
	// Env: AUTH_MODE
	Mode string
}{
	Mode: AuthModeConfigKey,
}

// Tests config keys for Ginkgo test execution
var Tests = struct {
	// GinkgoLabelFilter is the label filter for Ginkgo tests
	// Env: GINKGO_LABEL_FILTER
	GinkgoLabelFilter string

	// GinkgoFocus is a regex to focus on specific tests
	// Env: GINKGO_FOCUS
	GinkgoFocus string

	// GinkgoSkip is a regex to skip specific tests
	// Env: GINKGO_SKIP
	GinkgoSkip string

	// SuiteTimeout is the timeout for the entire test suite (Go duration format: "2h", "90m", etc.)
	// Env: SUITE_TIMEOUT
	SuiteTimeout string

	// JUnitReportPath is the path to write JUnit XML report
	// Env: JUNIT_REPORT_PATH
	JUnitReportPath string

	// GinkgoDryRun lists matching specs without executing them
	// Env: GINKGO_DRY_RUN
	GinkgoDryRun string
	// FlakeAttempts is the number of attempts for a failed test (includes initial attempt)
	// FlakeAttempts=1 means no retries, FlakeAttempts=3 means up to 2 retries
	// Env: FLAKE_ATTEMPTS
	FlakeAttempts string
}{
	GinkgoLabelFilter: "tests.ginkgoLabelFilter",
	GinkgoFocus:       "tests.focus",
	GinkgoSkip:        "tests.ginkgoSkip",
	SuiteTimeout:      "tests.suiteTimeout",
	JUnitReportPath:   "tests.junitReportPath",
	GinkgoDryRun:      "tests.dryRun",
	FlakeAttempts:     "tests.flakeAttempts",
}

// Identity config keys
var Identity = struct {
	// ExpectedIdentity is the expected audit identity for test assertions
	// (e.g. the email the API resolves from the JWT claim)
	// Env: HYPERFLEET_IDENTITY_EXPECTEDIDENTITY
	ExpectedIdentity string

	// TokenRequestNamespace is the namespace of the service account
	// Env: HYPERFLEET_IDENTITY_TOKENREQUEST_NAMESPACE
	TokenRequestNamespace string

	// TokenRequestAudience is the audience for the requested token
	// Env: HYPERFLEET_IDENTITY_TOKENREQUEST_AUDIENCE
	TokenRequestAudience string

	// TokenRequestExpirationSeconds is the token lifetime in seconds
	// Env: HYPERFLEET_IDENTITY_TOKENREQUEST_EXPIRATIONSECONDS
	TokenRequestExpirationSeconds string
}{
	ExpectedIdentity:              "identity.expectedIdentity",
	TokenRequestNamespace:         "identity.tokenRequest.namespace",
	TokenRequestAudience:          "identity.tokenRequest.audience",
	TokenRequestExpirationSeconds: "identity.tokenRequest.expirationSeconds",
}

// Log config keys
var Log = struct {
	// Level is the minimum log level
	// Env: HYPERFLEET_LOG_LEVEL
	Level string

	// Format is the log output format
	// Env: HYPERFLEET_LOG_FORMAT
	Format string

	// Output is the log destination
	// Env: HYPERFLEET_LOG_OUTPUT
	Output string
}{
	Level:  "log.level",
	Format: "log.format",
	Output: "log.output",
}

// Desire config keys
var Desire = struct {
	// Adapter is the cluster adapter that delivers through the desire store; see DesireConfig.Adapter
	// Env: HYPERFLEET_DESIRE_ADAPTER
	Adapter string
}{
	Adapter: "desire.adapter",
}

// BrokerTypeGooglePubSub identifies the Google Pub/Sub broker backend.
const BrokerTypeGooglePubSub = "googlepubsub"

// AdapterDeploymentConfig contains configuration for deploying adapters via Helm in tests.
type AdapterDeploymentConfig struct {
	ChartRepo     string `yaml:"chartRepo" mapstructure:"chartRepo"`
	ChartRef      string `yaml:"chartRef" mapstructure:"chartRef"`
	ChartPath     string `yaml:"chartPath" mapstructure:"chartPath"`
	ImageRegistry string `yaml:"imageRegistry" mapstructure:"imageRegistry"`
	ImageRepo     string `yaml:"imageRepo" mapstructure:"imageRepo"`
	ImageTag      string `yaml:"imageTag" mapstructure:"imageTag"`
}

// APIDeploymentConfig contains configuration for the API Helm chart used in tier2 tests
// (e.g., upgrading required adapters config during crash recovery).
type APIDeploymentConfig struct {
	ChartRepo string `yaml:"chartRepo" mapstructure:"chartRepo"`
	ChartRef  string `yaml:"chartRef" mapstructure:"chartRef"`
	ChartPath string `yaml:"chartPath" mapstructure:"chartPath"`
}

// TokenRequestConfig contains configuration for K8s TokenRequest-based JWT acquisition.
// When enabled (ServiceAccountName is set), a JWT is acquired via the K8s TokenRequest API
// at suite startup and used for API authentication.
type TokenRequestConfig struct {
	// Fixed to the E2E runner identity for authenticated modes.
	ServiceAccountName string `yaml:"serviceAccountName" mapstructure:"serviceAccountName"`
	Namespace          string `yaml:"namespace" mapstructure:"namespace"`
	Audience           string `yaml:"audience" mapstructure:"audience"`
	ExpirationSeconds  int64  `yaml:"expirationSeconds" mapstructure:"expirationSeconds"`
}

// IsEnabled returns true when TokenRequest-based authentication is configured.
func (t TokenRequestConfig) IsEnabled() bool {
	return t.ServiceAccountName != ""
}

// IdentityConfig contains JWT authentication and audit identity configuration.
// token is acquired via TokenRequest at runtime and sent with the mode-appropriate scheme.
// ExpectedIdentity is used by test assertions to verify audit fields (created_by, deleted_by).
type IdentityConfig struct {
	token string // JWT machine token (acquired via TokenRequest, not user-configurable)
	// Expected audit identity for assertions.
	ExpectedIdentity string `yaml:"expectedIdentity" mapstructure:"expectedIdentity"`
	// K8s TokenRequest JWT acquisition.
	TokenRequest TokenRequestConfig `yaml:"tokenRequest" mapstructure:"tokenRequest"`
}

// Token returns the JWT bearer token.
func (c *IdentityConfig) Token() string {
	return c.token
}

// SetToken sets the JWT bearer token (called by TokenRequest acquisition).
func (c *IdentityConfig) SetToken(t string) {
	c.token = t
}

// DesireConfig contains settings for remote delivery
// (adapter -> desire store -> applier).
type DesireConfig struct {
	// Adapter is the name of the cluster adapter that delivers on a remote transport.
	// It is one of the Cluster row's required adapters, so the tier suites check it like any
	// other required adapter; the remote-transport suite asserts that and uses this name to
	// read the adapter's status and the objects the applier wrote for it.
	// The default matches the name hyperfleet-infra gives the adapter; change it only when
	// the stack deploys the adapter under another name.
	Adapter string `yaml:"adapter" mapstructure:"adapter"`
}

// Config represents the e2e test configuration
type Config struct {
	// Entities is the descriptor catalog with the adapters.<kind> and timeouts.<kind>
	// config overlaid, built and validated once by Load.
	Entities          *descriptor.Catalog     `yaml:"-" mapstructure:"-"`
	AdapterDeployment AdapterDeploymentConfig `yaml:"adapterDeployment" mapstructure:"adapterDeployment"`
	Log               LogConfig               `yaml:"log" mapstructure:"log"`
	APIDeployment     APIDeploymentConfig     `yaml:"apiDeployment" mapstructure:"apiDeployment"`
	BrokerType        string                  `yaml:"brokerType" mapstructure:"brokerType"`
	AuthMode          string                  `yaml:"auth_mode" mapstructure:"auth_mode"`
	API               APIConfig               `yaml:"api" mapstructure:"api"`
	RunID             string                  `yaml:"runId" mapstructure:"runId"`
	TestDataDir       string                  `yaml:"testDataDir" mapstructure:"testDataDir"`
	OutputDir         string                  `yaml:"outputDir" mapstructure:"outputDir"`
	Namespace         string                  `yaml:"namespace" mapstructure:"namespace"`
	Desire            DesireConfig            `yaml:"desire" mapstructure:"desire"`
	GCPProjectID      string                  `yaml:"gcpProjectId" mapstructure:"gcpProjectId"`
	Identity          IdentityConfig          `yaml:"identity" mapstructure:"identity"`
	Timeouts          TimeoutsConfig          `yaml:"timeouts" mapstructure:"timeouts"`
	Polling           PollingConfig           `yaml:"polling" mapstructure:"polling"`
}

// MachineAuthorizationScheme returns the Authorization scheme for Kubernetes ServiceAccount tokens.
func (c *Config) MachineAuthorizationScheme() string {
	if c.AuthMode == AuthModeEdge || c.AuthMode == AuthModeEdgeAPI {
		return AuthorizationSchemeServiceAccount
	}
	return AuthorizationSchemeBearer
}

// APILatencyThreshold returns the latency budget for a single API request in
// the configured authentication mode. The base thresholds are calibrated for
// the request path without gateway authentication; EDGE and EDGE+API add the
// Authorino authorization hop to every request, so those modes get the
// DefaultAuthLatencyAllowance on top of the base threshold.
func (c *Config) APILatencyThreshold(base time.Duration) time.Duration {
	if c.AuthMode == AuthModeEdge || c.AuthMode == AuthModeEdgeAPI {
		return base + DefaultAuthLatencyAllowance
	}
	return base
}

// AdapterAuthEnabled reports whether adapters should use projected API tokens.
// An unset mode preserves the legacy TokenRequest-driven behavior for callers
// that construct Config directly rather than loading it through config.Load.
func (c *Config) AdapterAuthEnabled() bool {
	if c.AuthMode == "" {
		return c.Identity.TokenRequest.IsEnabled()
	}
	return c.AuthMode != AuthModeNone
}

// APIConfig contains API-related configuration
type APIConfig struct {
	URL string `yaml:"url" mapstructure:"url"`
}

// TimeoutsConfig contains the shared timeouts. Every other key under timeouts
// is a lower-case kind, which Load overlays on the descriptor row (see Entities).
type TimeoutsConfig struct {
	Adapter AdapterTimeouts `yaml:"adapter" mapstructure:"adapter"`
}

// AdapterTimeouts contains adapter-related timeouts
type AdapterTimeouts struct {
	Processing time.Duration `yaml:"processing" mapstructure:"processing"`
}

// PollingConfig contains polling configuration
type PollingConfig struct {
	Interval time.Duration `yaml:"interval" mapstructure:"interval"`
}

// LogConfig contains logging configuration
type LogConfig struct {
	Level  string `yaml:"level" mapstructure:"level"`   // debug, info, warn, error
	Format string `yaml:"format" mapstructure:"format"` // text, json
	Output string `yaml:"output" mapstructure:"output"` // stdout, stderr
}

// Load loads configuration from viper with improved validation
func Load() (*Config, error) {
	cfg := &Config{}

	// Use Unmarshal (not UnmarshalExact) to allow runtime test parameters (tests.*)
	// to coexist with persistent configuration. Test parameters (label-filter, focus, skip)
	// are set via flags/env vars and should not appear in config files.
	if err := viper.Unmarshal(cfg, rejectBareDurations); err != nil {
		return nil, fmt.Errorf("configuration error: %w\nPlease check your config file", err)
	}

	entities, err := loadEntityCatalog()
	if err != nil {
		return nil, fmt.Errorf("loading entity catalog failed: %w", err)
	}
	cfg.Entities = entities

	// Apply defaults
	cfg.applyDefaults()

	// Validate with detailed errors
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Note: Display() is called after logger initialization in e2e.RunTests()
	// to ensure structured logging is properly configured

	return cfg, nil
}

// IsDryRun reports whether the CLI is listing tests without executing them.
func IsDryRun() bool {
	return viper.GetBool(Tests.GinkgoDryRun)
}

// rejectBareDurations makes a non-zero number fail where a duration belongs, as a
// string without a unit already does. Viper would decode YAML interval: 300 as
// 300ns. Zero stays allowed, since it keeps a default. It runs before viper's own
// hooks, which parse duration strings.
func rejectBareDurations(c *mapstructure.DecoderConfig) {
	durationType := reflect.TypeFor[time.Duration]()
	c.DecodeHook = mapstructure.ComposeDecodeHookFunc(
		func(from, to reflect.Type, data any) (any, error) {
			if to != durationType || from == durationType {
				return data, nil
			}
			switch from.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
				reflect.Float32, reflect.Float64:
				if !reflect.ValueOf(data).IsZero() {
					return nil, fmt.Errorf("duration %v has no unit; write it with one, such as %vs", data, data)
				}
			default:
				// Leave other types to the remaining decode hooks.
				return data, nil
			}
			return data, nil
		},
		c.DecodeHook,
	)
}

// BindEnvVars makes viper read HYPERFLEET_* env vars: api.url -> HYPERFLEET_API_URL.
// AutomaticEnv covers viper.Get, and ExperimentalBindStruct makes viper.Unmarshal
// take its keys from the Config fields, so an env var reaches a field that the
// config file does not set. viper.Reset clears the option, so call this after it.
func BindEnvVars() error {
	viper.SetEnvPrefix(EnvPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()
	viper.SetOptions(viper.ExperimentalBindStruct())
	// Aliases for adapters.cluster and adapters.nodepool, removed with the per-kind
	// suites in HYPERFLEET-1734. HYPERFLEET_ADAPTERS_<KIND> still wins.
	// Include the unprefixed auth_mode alias. HYPERFLEET_AUTH_MODE still wins.
	for _, binding := range [][]string{
		{"adapters.cluster", "API_ADAPTERS_CLUSTER"},
		{"adapters.nodepool", "API_ADAPTERS_NODEPOOL"},
		{Authentication.Mode, "AUTH_MODE"},
	} {
		if err := viper.BindEnv(binding...); err != nil {
			return fmt.Errorf("failed to bind environment for %s: %w", binding[0], err)
		}
	}
	return nil
}

// applyDefaults applies default values for unset fields.
// Per-kind adapters and timeouts default in the descriptor table instead.
func (c *Config) applyDefaults() {
	if c.AuthMode == "" {
		c.AuthMode = DefaultAuthMode
	}

	// Apply timeout defaults
	if c.Timeouts.Adapter.Processing == 0 {
		c.Timeouts.Adapter.Processing = DefaultAdapterProcessingTimeout
	}
	if c.Polling.Interval == 0 {
		c.Polling.Interval = DefaultPollInterval
	}

	// Apply log defaults
	if c.Log.Level == "" {
		c.Log.Level = DefaultLogLevel
	}
	if c.Log.Format == "" {
		c.Log.Format = DefaultLogFormat
	}
	if c.Log.Output == "" {
		c.Log.Output = DefaultLogOutput
	}

	if c.Desire.Adapter == "" {
		c.Desire.Adapter = DefaultDesireAdapter
	}

	// Apply general configuration defaults from environment variables or config file
	// Priority: config file values > environment variables > empty
	// If config file value is empty, fall back to environment variable

	// Namespace: from config file or NAMESPACE env var
	if c.Namespace == "" {
		c.Namespace = os.Getenv("NAMESPACE")
	}

	// GCPProjectID: from config file or GCP_PROJECT_ID env var
	if c.GCPProjectID == "" {
		c.GCPProjectID = os.Getenv("GCP_PROJECT_ID")
	}

	if c.BrokerType == "" {
		if envVal := os.Getenv("BROKER_TYPE"); envVal != "" {
			c.BrokerType = envVal
		} else {
			c.BrokerType = DefaultBrokerType
		}
	}

	// OutputDir: from config file, OUTPUT_DIR env var, or default to "output"
	if c.OutputDir == "" {
		if envVal := os.Getenv("OUTPUT_DIR"); envVal != "" {
			c.OutputDir = envVal
		} else {
			c.OutputDir = "output"
		}
	}

	// TestDataDir: from config file, TESTDATA_DIR env var, or default to "testdata"
	if c.TestDataDir == "" {
		if envVal := os.Getenv("TESTDATA_DIR"); envVal != "" {
			c.TestDataDir = envVal
		} else {
			c.TestDataDir = "testdata"
		}
	}

	// RunID: from config file or RUN_ID env var
	if c.RunID == "" {
		c.RunID = os.Getenv("RUN_ID")
	}

	// Apply adapter deployment values from environment variables or config file

	// ChartRepo: from ADAPTER_CHART_REPO env var or config file
	if c.AdapterDeployment.ChartRepo == "" {
		c.AdapterDeployment.ChartRepo = os.Getenv("ADAPTER_CHART_REPO")
	}

	// ChartRef: from ADAPTER_CHART_REF env var or config file
	if c.AdapterDeployment.ChartRef == "" {
		c.AdapterDeployment.ChartRef = os.Getenv("ADAPTER_CHART_REF")
	}

	// ChartPath: from ADAPTER_CHART_PATH env var or config file
	if c.AdapterDeployment.ChartPath == "" {
		c.AdapterDeployment.ChartPath = os.Getenv("ADAPTER_CHART_PATH")
	}

	// ImageRegistry: from config file or IMAGE_REGISTRY env var
	// If not set, envsubst will use the env var directly (if set in shell)
	// If still not set, values.yaml defaults will be used
	if c.AdapterDeployment.ImageRegistry == "" {
		c.AdapterDeployment.ImageRegistry = os.Getenv("IMAGE_REGISTRY")
	}

	// ImageRepo: from config file or ADAPTER_IMAGE_REPO env var
	if c.AdapterDeployment.ImageRepo == "" {
		c.AdapterDeployment.ImageRepo = os.Getenv("ADAPTER_IMAGE_REPO")
	}

	// ImageTag: from config file or ADAPTER_IMAGE_TAG env var
	if c.AdapterDeployment.ImageTag == "" {
		c.AdapterDeployment.ImageTag = os.Getenv("ADAPTER_IMAGE_TAG")
	}

	// Apply API deployment values from environment variables or config file
	// These are used by tier2 tests that need to upgrade API configuration
	if c.APIDeployment.ChartRepo == "" {
		c.APIDeployment.ChartRepo = os.Getenv("API_CHART_REPO")
	}
	// ChartRef: from config file, API_CHART_REF env var, or fallback to adapter ref (same release branch)
	if c.APIDeployment.ChartRef == "" {
		if envVal := os.Getenv("API_CHART_REF"); envVal != "" {
			c.APIDeployment.ChartRef = envVal
		} else {
			c.APIDeployment.ChartRef = c.AdapterDeployment.ChartRef
		}
	}
	if c.APIDeployment.ChartPath == "" {
		c.APIDeployment.ChartPath = os.Getenv("API_CHART_PATH")
	}

	// Authenticated E2E runs use the fixed, infra-owned machine identity by default.
	if c.AuthMode != AuthModeNone && !IsDryRun() && !c.Identity.TokenRequest.IsEnabled() {
		c.Identity.TokenRequest.ServiceAccountName = DefaultE2ERunnerServiceAccountName
	}

	// TokenRequest namespace defaults to the namespace where HyperFleet is installed.
	if c.Identity.TokenRequest.IsEnabled() {
		if c.Identity.TokenRequest.Namespace == "" {
			c.Identity.TokenRequest.Namespace = c.Namespace
		}
		if c.Identity.TokenRequest.ExpirationSeconds == 0 {
			c.Identity.TokenRequest.ExpirationSeconds = DefaultTokenRequestExpirationSeconds
		}
		if c.Identity.TokenRequest.Audience == "" {
			c.Identity.TokenRequest.Audience = DefaultTokenRequestAudience
		}
		if c.Identity.ExpectedIdentity == "" && c.Identity.TokenRequest.Namespace != "" {
			c.Identity.ExpectedIdentity = fmt.Sprintf(
				"system:serviceaccount:%s:%s",
				c.Identity.TokenRequest.Namespace,
				c.Identity.TokenRequest.ServiceAccountName,
			)
		}
	}
}

// Validate validates configuration with detailed error messages
func (c *Config) Validate() error {
	dryRun := IsDryRun()
	switch c.AuthMode {
	case AuthModeNone, AuthModeAPI, AuthModeEdge, AuthModeEdgeAPI:
	default:
		return fmt.Errorf(
			"configuration validation failed: auth_mode must be one of: NONE, API, EDGE, or EDGE+API. Got: %s",
			c.AuthMode,
		)
	}

	// Validate API URL requirement (not needed for dry-run listing)
	if !dryRun && c.API.URL == "" {
		return fmt.Errorf(`configuration validation failed:
  - Field 'Config.API.URL' is required
    Please provide API URL (in order of priority):
      • Flag: --api-url
      • Environment variable: HYPERFLEET_API_URL
      • Config file: api.url: <url>`)
	}

	if !dryRun {
		if c.RunID == "" {
			return fmt.Errorf(
				"RUN_ID must be set for resource cleanup tracking. Set runId in test config or set RUN_ID env variable",
			)
		}

		if len(c.RunID) > 63 || !LabelValueRegex.MatchString(c.RunID) {
			return fmt.Errorf("RunID %q is not a valid Kubernetes label value", c.RunID)
		}
	}
	// Validate broker type
	if c.BrokerType != "googlepubsub" && c.BrokerType != "rabbitmq" {
		return fmt.Errorf(
			"configuration validation failed: brokerType must be one of: googlepubsub, rabbitmq. Got: %s",
			c.BrokerType,
		)
	}

	// Validate that the shared timeouts and polling interval are positive
	if c.Timeouts.Adapter.Processing <= 0 {
		return fmt.Errorf(
			"configuration validation failed: timeouts.adapter.processing must be a positive duration, got %v",
			c.Timeouts.Adapter.Processing,
		)
	}
	if c.Polling.Interval <= 0 {
		return fmt.Errorf(
			"configuration validation failed: polling.interval must be a positive duration, got %v",
			c.Polling.Interval,
		)
	}

	// Load validated the per-kind adapters and timeouts when it built Entities.

	// Validate identity tokenRequest config
	if c.Identity.TokenRequest.IsEnabled() {
		if !dryRun && c.AuthMode != AuthModeNone && c.Namespace == "" {
			return fmt.Errorf("configuration validation failed: NAMESPACE is required when AUTH_MODE=%s", c.AuthMode)
		}
		if c.Identity.TokenRequest.Namespace == "" && !dryRun {
			return fmt.Errorf(
				"configuration validation failed: identity.tokenRequest.namespace is required when tokenRequest is enabled",
			)
		}
		if !dryRun && c.AuthMode != AuthModeNone {
			if c.Identity.TokenRequest.Namespace != c.Namespace {
				return fmt.Errorf(
					"configuration validation failed: identity.tokenRequest.namespace must match namespace for AUTH_MODE=%s",
					c.AuthMode,
				)
			}
			if c.Identity.TokenRequest.ServiceAccountName != DefaultE2ERunnerServiceAccountName {
				return fmt.Errorf(
					"configuration validation failed: identity.tokenRequest.serviceAccountName is fixed to %q "+
						"for E2E machine authentication",
					DefaultE2ERunnerServiceAccountName,
				)
			}
		}

		// Reject token lifetimes shorter than the suite timeout (CWE-613).
		// The token is acquired once before the suite; a shorter lifetime
		// causes deterministic 401 failures later.
		if !dryRun {
			suiteTimeout := 2 * time.Hour
			if t := viper.GetDuration(Tests.SuiteTimeout); t > 0 {
				suiteTimeout = t
			}
			tokenLifetime := time.Duration(c.Identity.TokenRequest.ExpirationSeconds) * time.Second
			if tokenLifetime > 0 && tokenLifetime < suiteTimeout {
				return fmt.Errorf(
					"configuration validation failed: identity.tokenRequest.expirationSeconds (%ds = %s) "+
						"is shorter than the suite timeout (%s) - the JWT will expire mid-run causing 401 failures. "+
						"Increase expirationSeconds or reduce SUITE_TIMEOUT",
					c.Identity.TokenRequest.ExpirationSeconds,
					tokenLifetime,
					suiteTimeout,
				)
			}
		}
	}
	if !dryRun && c.AuthMode != AuthModeNone && !c.Identity.TokenRequest.IsEnabled() {
		return fmt.Errorf(
			"configuration validation failed: identity.tokenRequest.serviceAccountName is required when AUTH_MODE=%s "+
				"because E2E clients use machine ServiceAccount tokens",
			c.AuthMode,
		)
	}

	return nil
}

// Display logs the merged configuration using structured logging.
// Each descriptor row with adapters or timeouts logs adapters_<key>,
// timeout_<key>_reconciled and timeout_<key>_deleted.
func (c *Config) Display() {
	slog.Info("Loaded configuration", append([]any{
		"api_url", redactURL(c.API.URL),
		"auth_mode", c.AuthMode,
		"namespace", c.Namespace,
		"gcp_project_id", c.GCPProjectID,
		"output_dir", c.OutputDir,
		"testdata_dir", c.TestDataDir,
		"run_id", c.RunID,
		"timeout_adapter_processing", c.Timeouts.Adapter.Processing,
		"polling_interval", c.Polling.Interval,
		"log_level", c.Log.Level,
		"log_format", c.Log.Format,
		"log_output", c.Log.Output,
		"adapter_chart_repo", redactURL(c.AdapterDeployment.ChartRepo),
		"adapter_chart_ref", valueOrNotSet(c.AdapterDeployment.ChartRef),
		"adapter_chart_path", valueOrNotSet(c.AdapterDeployment.ChartPath),
		"adapter_image_registry", valueOrNotSet(c.AdapterDeployment.ImageRegistry),
		"adapter_image_repo", valueOrNotSet(c.AdapterDeployment.ImageRepo),
		"adapter_image_tag", valueOrNotSet(c.AdapterDeployment.ImageTag),
		"api_chart_repo", redactURL(c.APIDeployment.ChartRepo),
		"api_chart_ref", valueOrNotSet(c.APIDeployment.ChartRef),
		"api_chart_path", valueOrNotSet(c.APIDeployment.ChartPath),
		"identity_expected", valueOrNotSet(c.Identity.ExpectedIdentity),
		"identity_token_request_sa", valueOrNotSet(c.Identity.TokenRequest.ServiceAccountName),
		"identity_token_request_ns", valueOrNotSet(c.Identity.TokenRequest.Namespace),
		"identity_token_request_audience", valueOrNotSet(c.Identity.TokenRequest.Audience),
		"desire_adapter", c.Desire.Adapter,
	}, c.entityLogAttrs()...)...)
}

// entityLogAttrs returns the resolved runtime settings of each row that has any,
// in table order.
func (c *Config) entityLogAttrs() []any {
	var attrs []any
	for _, def := range EntityDescriptors() {
		row := c.Entities.MustGet(def.Kind)
		if len(row.RequiredAdapters) == 0 && row.Timeouts == (descriptor.EntityTimeouts{}) {
			continue
		}
		key := entityKey(row.Kind)
		attrs = append(attrs,
			"adapters_"+key, row.RequiredAdapters,
			"timeout_"+key+"_reconciled", row.Timeouts.Reconciled,
			"timeout_"+key+"_deleted", row.Timeouts.Deleted,
		)
	}
	return attrs
}

// valueOrNotSet returns the value if non-empty, otherwise returns NotSetPlaceholder
func valueOrNotSet(value string) string {
	if value == "" {
		return NotSetPlaceholder
	}
	return value
}

// redactURL redacts credentials from URLs
func redactURL(rawURL string) string {
	if rawURL == "" {
		return NotSetPlaceholder
	}

	// Parse the URL to safely handle credentials
	u, err := url.Parse(rawURL)
	if err != nil {
		// If parsing fails, redact entirely for safety
		return RedactedPlaceholder
	}

	// If URL contains user credentials, redact them
	if u.User != nil {
		// Clear the User field and manually build the redacted URL
		u.User = nil
		redactedURL := u.String()

		// Insert RedactedPlaceholder after the scheme://
		if u.Scheme != "" {
			redactedURL = u.Scheme + "://" + RedactedPlaceholder + "@" + u.Host
			if u.Path != "" {
				redactedURL += u.Path
			}
			if u.RawQuery != "" {
				redactedURL += "?" + u.RawQuery
			}
			if u.Fragment != "" {
				redactedURL += "#" + u.Fragment
			}
		}
		return redactedURL
	}

	// Return the URL as-is if no credentials present
	return u.String()
}
