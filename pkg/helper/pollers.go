package helper

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
)

// Poll returns a polling function for use with Eventually.
func (h *Helper) Poll(
	ctx context.Context,
	entity descriptor.EntityDescriptor,
	parentID, resourceID string,
) func() (*client.Resource, error) {
	path := entity.ResourcePath(parentID, resourceID)
	return func() (*client.Resource, error) {
		return h.Client.GetResource(ctx, path)
	}
}

// PollAdapterStatuses returns a polling function for use with Eventually.
func (h *Helper) PollAdapterStatuses(
	ctx context.Context,
	entity descriptor.EntityDescriptor,
	parentID, resourceID string,
) func() (*client.AdapterStatusList, error) {
	path := entity.ResourcePath(parentID, resourceID)
	return func() (*client.AdapterStatusList, error) {
		return h.Client.GetResourceStatuses(ctx, path)
	}
}

// PollHTTPStatus returns a polling function for use with Eventually.
func (h *Helper) PollHTTPStatus(
	ctx context.Context,
	entity descriptor.EntityDescriptor,
	parentID, resourceID string,
) func() (int, error) {
	path := entity.ResourcePath(parentID, resourceID)
	return func() (int, error) {
		return h.Client.GetResourceHTTPStatus(ctx, path)
	}
}

// PollNamespacesByPrefix returns a polling function for namespace existence checks.
func (h *Helper) PollNamespacesByPrefix(ctx context.Context, prefix string) func() ([]string, error) {
	return func() ([]string, error) {
		return h.K8sClient.FindNamespacesByPrefix(ctx, prefix)
	}
}
