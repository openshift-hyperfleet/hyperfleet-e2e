package helm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

// HelmClient wraps Helm SDK functionality for E2E test cleanup operations
type HelmClient struct {
	settings     *cli.EnvSettings
	actionConfig *action.Configuration
	configOnce   sync.Once
	configErr    error
}

// NewHelmClient creates a new Helm client using default environment settings
func NewHelmClient(namespace string) *HelmClient {
	envSettings := cli.New()
	envSettings.SetNamespace(namespace)

	return &HelmClient{
		settings: envSettings,
	}
}

// initActionConfig initializes Helm action configuration once and caches it
// Subsequent calls return the cached config
func (c *HelmClient) initActionConfig() (*action.Configuration, error) {
	c.configOnce.Do(func() {
		actionConfig := new(action.Configuration)

		// Use the default Helm driver (secrets)
		helmDriver := ""

		// Initialize with REST client getter, namespace, and driver
		if err := actionConfig.Init(c.settings.RESTClientGetter(), c.settings.Namespace(), helmDriver, func(format string, v ...interface{}) {
			logger.Info(fmt.Sprintf(format, v...))
		}); err != nil {
			c.configErr = fmt.Errorf("failed to init Helm action config: %w", err)
			return
		}

		c.actionConfig = actionConfig
		logger.Info("initialized Helm action config", "namespace", c.settings.Namespace())
	})

	return c.actionConfig, c.configErr
}

// ListReleases lists all Helm releases across client namespace with the given label selector
// labelSelector uses Kubernetes label selector format (e.g., "e2e.hyperfleet.io/run-id=test-123")
// Returns a list of release names
func (c *HelmClient) ListReleasesBySelector(labelSelector string) ([]string, error) {
	// Initialize action config for all namespaces (empty string means all)
	actionConfig, err := c.initActionConfig()
	if err != nil {
		return nil, err
	}

	listClient := action.NewList(actionConfig)
	listClient.All = true               // List releases in all states (not just deployed)
	listClient.AllNamespaces = false    // Search only in the configured namespace
	listClient.SetStateMask()           // Set state mask to include all states
	listClient.Selector = labelSelector // Use Kubernetes label selector

	results, err := listClient.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to list Helm releases: %w", err)
	}

	releases := []string{}
	for _, rel := range results {
		// check that helm list is only listing releases in namespace
		if rel.Namespace != c.settings.Namespace() {
			logger.Warn("helm incorrectly listing releases outside namespace")
			continue
		}
		releases = append(releases, rel.Name)
		logger.Info("found Helm release", "release", rel.Name)
	}

	return releases, nil
}

// InstallRelease installs a Helm chart from a local path with values from a template file
// fileValues is a slice of "key=filepath" entries that will be loaded and set as values (like --set-file)
func (c *HelmClient) InstallRelease(ctx context.Context, releaseName string, chartPath string, releaseValues map[string]interface{},
	labels map[string]string) error {
	actionConfig, err := c.initActionConfig()
	if err != nil {
		return err
	}

	// Set up install action
	installClient := action.NewInstall(actionConfig)
	installClient.DryRunOption = "none"
	installClient.ReleaseName = releaseName
	installClient.Namespace = c.settings.Namespace()
	installClient.CreateNamespace = true
	installClient.Wait = true
	installClient.Timeout = 5 * time.Minute
	installClient.Labels = labels

	// Load the chart from local filesystem
	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart from %s: %w", chartPath, err)
	}

	// Install the chart with dedicated releaseValues
	release, err := installClient.RunWithContext(ctx, chart, releaseValues)
	if err != nil {
		return fmt.Errorf("failed to install release: %w", err)
	}

	logger.Info("successfully installed release",
		"name", release.Name,
		"version", release.Version,
		"namespace", release.Namespace)

	return nil
}

// UninstallRelease uninstalls the helm release. This workflow matches the way the adapters are currently installed.
// Future work can be done to move helm releases to be installed with helm sdk
func (c *HelmClient) UninstallRelease(releaseName string) error {
	actionConfig, err := c.initActionConfig()
	if err != nil {
		return err
	}

	uninstallClient := action.NewUninstall(actionConfig)
	uninstallClient.DeletionPropagation = "foreground" // "background" or "orphan"

	result, err := uninstallClient.Run(releaseName)
	if err != nil {
		return fmt.Errorf("failed to run uninstall action: %w", err)
	}
	if result != nil && result.Release != nil {
		logger.Info("helm uninstall completed",
			"name", result.Release.Name,
			"version", result.Release.Version,
			"namespace", result.Release.Namespace)
	}
	return nil
}
