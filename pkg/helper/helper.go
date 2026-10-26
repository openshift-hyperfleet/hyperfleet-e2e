package helper

import (
	"path/filepath"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
)

// Helper provides utility functions for e2e tests
type Helper struct {
	Entities              *descriptor.Catalog
	Cfg                   *config.Config
	Client                *client.HyperFleetClient
	K8sClient             *k8sclient.Client
	AdapterDeploymentList *AdapterDeploymentList
}

// TestDataPath resolves a relative path within the testdata directory
// This ensures testdata paths work correctly whether invoked via go test or the e2e binary
func (h *Helper) TestDataPath(relativePath string) string {
	return filepath.Join(h.Cfg.TestDataDir, relativePath)
}

// ExpectedIdentity returns the configured expected audit identity value.
// Returns empty string if not configured, signaling callers to skip audit assertions.
func (h *Helper) ExpectedIdentity() string {
	return h.Cfg.Identity.ExpectedIdentity
}
