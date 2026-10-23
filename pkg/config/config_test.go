package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/cmd/hyperfleet-e2e/common"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
)

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

func loadConfigResult(t *testing.T, configFile string, env map[string]string) (*config.Config, error) {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)

	// Isolate from the developer's shell: an empty value counts as unset for viper.
	for _, key := range []string{
		"HYPERFLEET_CONFIG",
		"HYPERFLEET_DESIRE_ADAPTER",
		"HYPERFLEET_AUTHMODE",
		"HYPERFLEET_AUTH_MODE",
		"HYPERFLEET_IDENTITY_EXPECTEDIDENTITY",
		"HYPERFLEET_IDENTITY_TOKENREQUEST_SERVICEACCOUNTNAME",
		"HYPERFLEET_IDENTITY_TOKENREQUEST_NAMESPACE",
		"AUTH_MODE",
	} {
		t.Setenv(key, "")
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
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return cfg, nil
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
			configPath: filepath.Join("..", "..", "configs", "config.yaml"),
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

func TestAPILatencyThreshold(t *testing.T) {
	const base = 50 * time.Millisecond

	tests := []struct {
		name string
		mode string
		want time.Duration
	}{
		{name: "API mode keeps the base threshold", mode: config.AuthModeAPI, want: base},
		{name: "NONE keeps the base threshold", mode: config.AuthModeNone, want: base},
		{name: "EDGE adds the auth latency allowance", mode: config.AuthModeEdge, want: base + config.DefaultAuthLatencyAllowance},
		{name: "EDGE+API adds the auth latency allowance", mode: config.AuthModeEdgeAPI, want: base + config.DefaultAuthLatencyAllowance},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.Config{AuthMode: tt.mode}
			if got := cfg.APILatencyThreshold(base); got != tt.want {
				t.Errorf("APILatencyThreshold(%v) with AUTH_MODE=%s = %v, want %v", base, tt.mode, got, tt.want)
			}
		})
	}
}
