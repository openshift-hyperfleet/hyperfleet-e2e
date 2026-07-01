package helper

import (
	"context"
	"errors"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/core"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
)

// PollCluster returns a polling function for use with Eventually.
func (h *Helper) PollCluster(ctx context.Context, id string) func() (*openapi.Cluster, error) {
	return func() (*openapi.Cluster, error) {
		return h.Client.GetCluster(ctx, id)
	}
}

// PollNodePool returns a polling function for use with Eventually.
func (h *Helper) PollNodePool(ctx context.Context, clusterID, npID string) func() (*openapi.NodePool, error) {
	return func() (*openapi.NodePool, error) {
		return h.Client.GetNodePool(ctx, clusterID, npID)
	}
}

// PollClusterAdapterStatuses returns a polling function for cluster adapter status checks.
func (h *Helper) PollClusterAdapterStatuses(ctx context.Context, clusterID string) func() (*core.AdapterStatusList, error) {
	return func() (*core.AdapterStatusList, error) {
		return h.Client.GetClusterStatuses(ctx, clusterID)
	}
}

// PollNodePoolAdapterStatuses returns a polling function for nodepool adapter status checks.
func (h *Helper) PollNodePoolAdapterStatuses(ctx context.Context, clusterID, npID string) func() (*core.AdapterStatusList, error) {
	return func() (*core.AdapterStatusList, error) {
		return h.Client.GetNodePoolStatuses(ctx, clusterID, npID)
	}
}

// PollClusterHTTPStatus returns a polling function that yields the HTTP status code.
// 200 when cluster exists, 404 when gone. Useful for hard-delete assertions.
func (h *Helper) PollClusterHTTPStatus(ctx context.Context, id string) func() (int, error) {
	return func() (int, error) {
		_, err := h.Client.GetCluster(ctx, id)
		if err == nil {
			return http.StatusOK, nil
		}
		var httpErr *client.HTTPError
		if errors.As(err, &httpErr) {
			return httpErr.StatusCode, nil
		}
		return 0, err
	}
}

// PollNodePoolHTTPStatus returns a polling function that yields the HTTP status code.
// 200 when nodepool exists, 404 when gone. Useful for hard-delete assertions.
func (h *Helper) PollNodePoolHTTPStatus(ctx context.Context, clusterID, npID string) func() (int, error) {
	return func() (int, error) {
		_, err := h.Client.GetNodePool(ctx, clusterID, npID)
		if err == nil {
			return http.StatusOK, nil
		}
		var httpErr *client.HTTPError
		if errors.As(err, &httpErr) {
			return httpErr.StatusCode, nil
		}
		return 0, err
	}
}

// PollNamespacesByPrefix returns a polling function for namespace existence checks.
func (h *Helper) PollNamespacesByPrefix(ctx context.Context, prefix string) func() ([]string, error) {
	return func() ([]string, error) {
		return h.K8sClient.FindNamespacesByPrefix(ctx, prefix)
	}
}
