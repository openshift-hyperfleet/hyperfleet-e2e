package common

import (
	"fmt"
	"os"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/spf13/viper"
)

// ConfigFile holds the --config flag value set by main package
var ConfigFile string

// LoadConfig loads configuration following HyperFleet standard
// Priority: flags > env vars > config file > defaults
func LoadConfig(configFlagValue string) error {
	// 1. Determine config file path
	configFile := getConfigFilePath(configFlagValue)

	// 2. Load config file FIRST if found
	if configFile != "" {
		viper.SetConfigFile(configFile)
		viper.SetConfigType("yaml")

		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read config file %s: %w", configFile, err)
		}
	}

	// 3. Enable HYPERFLEET_* environment variables and the API_ADAPTERS_* and
	// AUTH_MODE aliases; env vars override config file values
	config.BindEnvVars()

	return nil
}

// getConfigFilePath determines the config file path following standard priority
// 1. --config flag value
// 2. HYPERFLEET_CONFIG env var
// 3. ./configs/config.yaml (default for development)
func getConfigFilePath(configFlagValue string) string {
	// Priority 1: --config flag
	if configFlagValue != "" {
		return configFlagValue
	}

	// Priority 2: HYPERFLEET_CONFIG env var
	if configPath := os.Getenv(config.EnvPrefix + "_CONFIG"); configPath != "" {
		return configPath
	}

	// Priority 3: Default path (development)
	defaultPath := "./configs/config.yaml"
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath
	}

	// No config file found - continue with flags and env vars only
	return ""
}
