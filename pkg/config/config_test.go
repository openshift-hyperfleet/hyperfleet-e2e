package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/cmd/hyperfleet-e2e/common"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
)

// shippedConfig is the configs/config.yaml the image ships and CI runs with.
var shippedConfig = filepath.Join("..", "..", "configs", "config.yaml")

// loadConfig runs the same loading path as the CLI: common.LoadConfig binds the
// HYPERFLEET_* env vars, then config.Load unmarshals, applies defaults and validates.
// viper is process-global, so these tests must not run in parallel.
func loadConfig(t *testing.T, configFile string, env map[string]string) *config.Config {
	t.Helper()

	cfg, err := loadConfigResult(t, configFile, env)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

// loadConfigResult is loadConfig for cases where config.Load is expected to fail.
func loadConfigResult(t *testing.T, configFile string, env map[string]string) (*config.Config, error) {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)

	// Isolate from the developer's shell: an empty value counts as unset for viper.
	// env/env.local exports the API_ADAPTERS_* aliases.
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, config.EnvPrefix+"_") || strings.HasPrefix(name, "API_ADAPTERS_") || name == "AUTH_MODE" {
			t.Setenv(name, "")
		}
	}
	t.Setenv("HYPERFLEET_API_URL", "http://hyperfleet-api.invalid:8000")
	t.Setenv("NAMESPACE", "unit-test")
	t.Setenv("RUN_ID", "unit-test")
	t.Setenv("BROKER_TYPE", "rabbitmq")
	for key, value := range env {
		t.Setenv(key, value)
	}

	if err := common.LoadConfig(configFile); err != nil {
		return nil, fmt.Errorf("LoadConfig(%q): %w", configFile, err)
	}
	return config.Load()
}

// entity returns the resolved catalog row of a kind.
func entity(t *testing.T, cfg *config.Config, kind string) descriptor.EntityDescriptor {
	t.Helper()
	return cfg.Entities.MustGet(kind)
}

// envVar returns the HYPERFLEET_* env var viper reads for a config path.
func envVar(path string) string {
	return config.EnvVar(strings.ToUpper(strings.ReplaceAll(path, ".", "_")))
}

// defaultEntity returns a kind's row with no config at all.
func defaultEntity(t *testing.T, kind string) descriptor.EntityDescriptor {
	t.Helper()
	catalog, err := descriptor.NewCatalog(config.EntityDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	return catalog.MustGet(kind)
}

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
	return path
}

func TestDesireAdapterKey(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string // written to a temp file when set
		configPath string // an existing file, used as-is when set
		env        map[string]string
		want       string
	}{
		{
			name: "default",
			want: config.DefaultDesireAdapter,
		},
		{
			name: "env var overrides the default",
			env:  map[string]string{"HYPERFLEET_DESIRE_ADAPTER": "cl-desire-env"},
			want: "cl-desire-env",
		},
		{
			name:       "config file sets the key",
			configYAML: "desire:\n  adapter: cl-desire-file\n",
			want:       "cl-desire-file",
		},
		{
			name:       "env var overrides the config file",
			configYAML: "desire:\n  adapter: cl-desire-file\n",
			env:        map[string]string{"HYPERFLEET_DESIRE_ADAPTER": "cl-desire-env"},
			want:       "cl-desire-env",
		},
		{
			name:       "shipped configs/config.yaml",
			configPath: shippedConfig,
			want:       "cl-desire",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFile := tt.configPath
			if tt.configYAML != "" {
				configFile = writeConfigFile(t, tt.configYAML)
			}
			cfg := loadConfig(t, configFile, tt.env)

			if got := cfg.Desire.Adapter; got != tt.want {
				t.Errorf("Desire.Adapter = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuthMode(t *testing.T) {
	tests := []struct {
		name         string
		configYAML   string
		env          map[string]string
		want         string
		wantScheme   string
		wantSA       string
		wantNS       string
		wantIdentity string
	}{
		{
			name:         "default",
			want:         config.AuthModeEdgeAPI,
			wantScheme:   config.AuthorizationSchemeServiceAccount,
			wantSA:       config.DefaultE2ERunnerServiceAccountName,
			wantNS:       "unit-test",
			wantIdentity: "system:serviceaccount:unit-test:" + config.DefaultE2ERunnerServiceAccountName,
		},
		{
			name:         "bare AUTH_MODE overrides default",
			env:          map[string]string{"AUTH_MODE": config.AuthModeAPI},
			want:         config.AuthModeAPI,
			wantScheme:   config.AuthorizationSchemeBearer,
			wantSA:       config.DefaultE2ERunnerServiceAccountName,
			wantNS:       "unit-test",
			wantIdentity: "system:serviceaccount:unit-test:" + config.DefaultE2ERunnerServiceAccountName,
		},
		{
			name:         "bare AUTH_MODE overrides config file",
			configYAML:   "auth_mode: API\n",
			env:          map[string]string{"AUTH_MODE": config.AuthModeEdge},
			want:         config.AuthModeEdge,
			wantScheme:   config.AuthorizationSchemeServiceAccount,
			wantSA:       config.DefaultE2ERunnerServiceAccountName,
			wantNS:       "unit-test",
			wantIdentity: "system:serviceaccount:unit-test:" + config.DefaultE2ERunnerServiceAccountName,
		},
		{
			name:       "config file sets mode",
			configYAML: "auth_mode: NONE\n",
			want:       config.AuthModeNone,
			wantScheme: config.AuthorizationSchemeBearer,
		},
		{
			name:         "HYPERFLEET_AUTH_MODE alias is accepted",
			env:          map[string]string{"HYPERFLEET_AUTH_MODE": config.AuthModeAPI},
			want:         config.AuthModeAPI,
			wantScheme:   config.AuthorizationSchemeBearer,
			wantSA:       config.DefaultE2ERunnerServiceAccountName,
			wantNS:       "unit-test",
			wantIdentity: "system:serviceaccount:unit-test:" + config.DefaultE2ERunnerServiceAccountName,
		},
		{
			name:         "HYPERFLEET_AUTH_MODE beats AUTH_MODE",
			env:          map[string]string{"HYPERFLEET_AUTH_MODE": config.AuthModeAPI, "AUTH_MODE": config.AuthModeEdge},
			want:         config.AuthModeAPI,
			wantScheme:   config.AuthorizationSchemeBearer,
			wantSA:       config.DefaultE2ERunnerServiceAccountName,
			wantNS:       "unit-test",
			wantIdentity: "system:serviceaccount:unit-test:" + config.DefaultE2ERunnerServiceAccountName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFile := ""
			if tt.configYAML != "" {
				configFile = writeConfigFile(t, tt.configYAML)
			}
			cfg := loadConfig(t, configFile, tt.env)
			if cfg.AuthMode != tt.want {
				t.Errorf("AuthMode = %q, want %q", cfg.AuthMode, tt.want)
			}
			if got := cfg.MachineAuthorizationScheme(); got != tt.wantScheme {
				t.Errorf("MachineAuthorizationScheme() = %q, want %q", got, tt.wantScheme)
			}
			if got := cfg.Identity.TokenRequest.ServiceAccountName; got != tt.wantSA {
				t.Errorf("TokenRequest.ServiceAccountName = %q, want %q", got, tt.wantSA)
			}
			if got := cfg.Identity.TokenRequest.Namespace; got != tt.wantNS {
				t.Errorf("TokenRequest.Namespace = %q, want %q", got, tt.wantNS)
			}
			if got := cfg.Identity.ExpectedIdentity; got != tt.wantIdentity {
				t.Errorf("Identity.ExpectedIdentity = %q, want %q", got, tt.wantIdentity)
			}
		})
	}
}

func TestAuthModeRejectsUnsupportedValue(t *testing.T) {
	_, err := loadConfigResult(t, "", map[string]string{"AUTH_MODE": "INVALID"})
	if err == nil {
		t.Fatal("Load accepted unsupported AUTH_MODE")
	}
	if !strings.Contains(err.Error(), "auth_mode must be one of") {
		t.Errorf("Load error = %q, want unsupported AUTH_MODE validation error", err)
	}
}

func TestAuthenticatedModeRequiresNamespace(t *testing.T) {
	_, err := loadConfigResult(t, "", map[string]string{"NAMESPACE": ""})
	if err == nil {
		t.Fatal("Load accepted authenticated mode without NAMESPACE")
	}
	if !strings.Contains(err.Error(), "NAMESPACE is required when AUTH_MODE=EDGE+API") {
		t.Errorf("Load error = %q, want missing NAMESPACE validation error", err)
	}
}

func TestAuthModeRejectsCustomRunnerServiceAccount(t *testing.T) {
	_, err := loadConfigResult(t, "", map[string]string{
		"HYPERFLEET_IDENTITY_TOKENREQUEST_SERVICEACCOUNTNAME": "other-runner",
	})
	if err == nil {
		t.Fatal("Load accepted a nonstandard E2E runner ServiceAccount")
	}
	if !strings.Contains(err.Error(), "serviceAccountName is fixed") {
		t.Errorf("Load error = %q, want fixed runner ServiceAccount validation error", err)
	}
}

func TestEntityTimeoutKeys(t *testing.T) {
	t.Run("row defaults with no config file and with the shipped one", func(t *testing.T) {
		for _, configFile := range []string{"", shippedConfig} {
			cfg := loadConfig(t, configFile, nil)
			for _, row := range config.EntityDescriptors() {
				if got, want := entity(t, cfg, row.Kind).Timeouts, row.Timeouts; got != want {
					t.Errorf("config file %q: %s timeouts = %+v, want the row default %+v", configFile, row.Kind, got, want)
				}
			}
		}
	})

	t.Run("an env var resolves for every kind in the table", func(t *testing.T) {
		env := map[string]string{}
		want := map[string]descriptor.EntityTimeouts{}
		for i, row := range config.EntityDescriptors() {
			timeouts := descriptor.EntityTimeouts{
				Reconciled: time.Duration(i+1)*time.Minute + 7*time.Second,
				Deleted:    time.Duration(i+1)*time.Minute + 11*time.Second,
			}
			key := strings.ToLower(row.Kind)
			env[envVar("timeouts."+key+".reconciled")] = timeouts.Reconciled.String()
			env[envVar("timeouts."+key+".deleted")] = timeouts.Deleted.String()
			want[row.Kind] = timeouts
		}
		cfg := loadConfig(t, "", env)
		for kind, timeouts := range want {
			if got := entity(t, cfg, kind).Timeouts; got != timeouts {
				t.Errorf("%s timeouts = %+v, want %+v", kind, got, timeouts)
			}
		}
	})

	t.Run("a file key overrides one field and keeps the other default", func(t *testing.T) {
		cfg := loadConfig(t, writeConfigFile(t, "timeouts:\n  parent:\n    reconciled: 7m\n"), nil)
		want := descriptor.EntityTimeouts{Reconciled: 7 * time.Minute, Deleted: defaultEntity(t, "Parent").Timeouts.Deleted}
		if got := entity(t, cfg, "Parent").Timeouts; got != want {
			t.Errorf("Parent timeouts = %+v, want %+v", got, want)
		}
	})

	t.Run("a zero value keeps the row default", func(t *testing.T) {
		cfg := loadConfig(t, writeConfigFile(t, "timeouts:\n  nodepool:\n    reconciled: 0\n    deleted: 0s\n"), nil)
		if got, want := entity(t, cfg, "NodePool").Timeouts, defaultEntity(t, "NodePool").Timeouts; got != want {
			t.Errorf("NodePool timeouts = %+v, want the row default %+v", got, want)
		}
	})

	t.Run("env var overrides the config file", func(t *testing.T) {
		file := writeConfigFile(t, "timeouts:\n  cluster:\n    reconciled: 5m\n    deleted: 4m\n")
		cfg := loadConfig(t, file, map[string]string{"HYPERFLEET_TIMEOUTS_CLUSTER_RECONCILED": "3m"})
		want := descriptor.EntityTimeouts{Reconciled: 3 * time.Minute, Deleted: 4 * time.Minute}
		if got := entity(t, cfg, "Cluster").Timeouts; got != want {
			t.Errorf("Cluster timeouts = %+v, want %+v", got, want)
		}
	})

	t.Run("timeouts.adapter.processing keeps its env var", func(t *testing.T) {
		cfg := loadConfig(t, shippedConfig, map[string]string{"HYPERFLEET_TIMEOUTS_ADAPTER_PROCESSING": "3m"})
		if got := cfg.Timeouts.Adapter.Processing; got != 3*time.Minute {
			t.Errorf("Timeouts.Adapter.Processing = %v, want 3m", got)
		}
	})
}

func TestEntityAdapterKeys(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string // written to a temp file when set
		configPath string // an existing file, used as-is when set
		env        map[string]string
		kind       string
		want       []string
	}{
		{
			name: "env var splits and trims a comma-separated list",
			env:  map[string]string{"HYPERFLEET_ADAPTERS_PARENT": "a, b"},
			kind: "Parent",
			want: []string{"a", "b"},
		},
		{
			name: "env var splits a whitespace-separated list",
			env:  map[string]string{"HYPERFLEET_ADAPTERS_PARENT": "a b\tc"},
			kind: "Parent",
			want: []string{"a", "b", "c"},
		},
		{
			name: "API_ADAPTERS_CLUSTER alias",
			env:  map[string]string{"API_ADAPTERS_CLUSTER": "alias-a,alias-b"},
			kind: "Cluster",
			want: []string{"alias-a", "alias-b"},
		},
		{
			name: "API_ADAPTERS_NODEPOOL alias",
			env:  map[string]string{"API_ADAPTERS_NODEPOOL": "alias-np"},
			kind: "NodePool",
			want: []string{"alias-np"},
		},
		{
			name: "HYPERFLEET_ADAPTERS_CLUSTER beats API_ADAPTERS_CLUSTER",
			env:  map[string]string{"HYPERFLEET_ADAPTERS_CLUSTER": "env-cl", "API_ADAPTERS_CLUSTER": "alias-cl"},
			kind: "Cluster",
			want: []string{"env-cl"},
		},
		{
			name:       "config file sets a kind's list",
			configYAML: "adapters:\n  nodepool:\n    - file-np\n",
			kind:       "NodePool",
			want:       []string{"file-np"},
		},
		{
			name:       "env var overrides the config file",
			configYAML: "adapters:\n  nodepool:\n    - file-np\n",
			env:        map[string]string{"HYPERFLEET_ADAPTERS_NODEPOOL": "env-np"},
			kind:       "NodePool",
			want:       []string{"env-np"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFile := tt.configPath
			if tt.configYAML != "" {
				configFile = writeConfigFile(t, tt.configYAML)
			}
			cfg := loadConfig(t, configFile, tt.env)
			if got := entity(t, cfg, tt.kind).RequiredAdapters; !slices.Equal(got, tt.want) {
				t.Errorf("%s adapters = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}

	t.Run("row defaults with no config file and with the shipped one", func(t *testing.T) {
		for _, configFile := range []string{"", shippedConfig} {
			cfg := loadConfig(t, configFile, nil)
			for _, row := range config.EntityDescriptors() {
				if got := entity(t, cfg, row.Kind).RequiredAdapters; !slices.Equal(got, row.RequiredAdapters) {
					t.Errorf("config file %q: %s adapters = %q, want the row default %q", configFile, row.Kind, got, row.RequiredAdapters)
				}
			}
		}
	})

	t.Run("the desire adapter is a Cluster default", func(t *testing.T) {
		if !slices.Contains(defaultEntity(t, "Cluster").RequiredAdapters, config.DefaultDesireAdapter) {
			t.Errorf("Cluster defaults do not contain %q", config.DefaultDesireAdapter)
		}
	})

	t.Run("an env var resolves for every kind in the table", func(t *testing.T) {
		// A kind with adapters needs a reconciled timeout, so set one as well.
		env := map[string]string{}
		for _, row := range config.EntityDescriptors() {
			key := strings.ToLower(row.Kind)
			env[envVar("adapters."+key)] = key + "-a, " + key + "-b"
			env[envVar("timeouts."+key+".reconciled")] = "1m"
		}
		cfg := loadConfig(t, "", env)
		for _, row := range config.EntityDescriptors() {
			key := strings.ToLower(row.Kind)
			want := []string{key + "-a", key + "-b"}
			if got := entity(t, cfg, row.Kind).RequiredAdapters; !slices.Equal(got, want) {
				t.Errorf("%s adapters = %q, want %q", row.Kind, got, want)
			}
		}
	})
}

func TestUnknownEntityKeyFails(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		wantErr    string
	}{
		{
			name:       "timeouts key that is not a kind",
			configYAML: "timeouts:\n  nodepools:\n    reconciled: 1m\n",
			wantErr:    `unknown kind "nodepools" under timeouts`,
		},
		{
			name:       "adapters key that is not a kind",
			configYAML: "adapters:\n  widget:\n    - w\n",
			wantErr:    `unknown kind "widget" under adapters`,
		},
		{
			name:       "empty adapters list",
			configYAML: "adapters:\n  cluster: []\n",
			wantErr:    "adapters.cluster is set but names no adapter",
		},
		{
			name:       "timeout without a unit",
			configYAML: "timeouts:\n  cluster:\n    reconciled: \"300\"\n",
			wantErr:    "timeouts.cluster.reconciled:",
		},
		{
			name:       "kind timeouts as a scalar",
			configYAML: "timeouts:\n  cluster: 5m\n",
			wantErr:    "timeouts.cluster must be a map, got 5m",
		},
		{
			name:       "kind timeouts as a list",
			configYAML: "timeouts:\n  cluster: [5m]\n",
			wantErr:    "timeouts.cluster must be a map, got [5m]",
		},
		{
			name:       "adapters as a list",
			configYAML: "adapters: [cl-namespace]\n",
			wantErr:    "adapters must be a map, got [cl-namespace]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadConfigResult(t, writeConfigFile(t, tt.configYAML), nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// A bare YAML number where a duration belongs fails like a string without a unit,
// for the per-kind timeouts and the shared durations alike, instead of becoming
// nanoseconds.
func TestBareNumberDurationFails(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		wantErr    string
	}{
		{
			name:       "kind timeout",
			configYAML: "timeouts:\n  cluster:\n    reconciled: 300\n",
			wantErr:    "timeouts.cluster.reconciled:",
		},
		{
			name:       "kind timeout as a float",
			configYAML: "timeouts:\n  nodepool:\n    deleted: 1.5\n",
			wantErr:    "timeouts.nodepool.deleted:",
		},
		{
			name:       "polling interval",
			configYAML: "polling:\n  interval: 300\n",
			wantErr:    "duration 300 has no unit",
		},
		{
			name:       "adapter processing timeout",
			configYAML: "timeouts:\n  adapter:\n    processing: 300\n",
			wantErr:    "duration 300 has no unit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadConfigResult(t, writeConfigFile(t, tt.configYAML), nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "has no unit") {
				t.Fatalf("Load error = %v, want it to contain %q and %q", err, tt.wantErr, "has no unit")
			}
		})
	}
}
