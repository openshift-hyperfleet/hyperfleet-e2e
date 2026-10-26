package helper

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
)

// CreateTestResource creates a temporary resource from a payload and returns its ID.
func (h *Helper) CreateTestResource(
	ctx context.Context,
	entity descriptor.EntityDescriptor,
	parentID, payloadPath string,
) (string, error) {
	path := entity.CollectionPath(parentID)
	resource, err := h.Client.CreateResourceFromPayload(ctx, path, payloadPath)
	if err != nil {
		return "", err
	}
	if resource == nil {
		return "", fmt.Errorf("CreateResourceFromPayload returned nil")
	}
	if resource.ID == nil {
		return "", fmt.Errorf("created %s has no ID", entity.Kind)
	}
	return *resource.ID, nil
}

// CleanupTestResource deletes through the API, which owns adapter finalization
// and downstream cleanup. The descriptor preserves each suite's teardown policy;
// no-poll cleanup does not imply that the API cannot hard-delete the resource.
func (h *Helper) CleanupTestResource(
	ctx context.Context,
	entity descriptor.EntityDescriptor,
	parentID, resourceID string,
) error {
	path := entity.ResourcePath(parentID, resourceID)

	childErr, err := h.cleanupChildren(ctx, entity, resourceID)
	if err != nil {
		if isHTTPStatus(err, http.StatusNotFound) {
			return nil // the parent disappeared before its children could be listed
		}
		return err
	}

	slog.Info("deleting resource via API", "kind", entity.Kind, "resource_id", resourceID)
	if _, err := h.Client.DeleteResource(ctx, path); err != nil {
		if (entity.Cleanup.IgnoreNotFound && isHTTPStatus(err, http.StatusNotFound)) ||
			(entity.Cleanup.IgnoreConflict && isHTTPStatus(err, http.StatusConflict)) {
			return childErr
		}
		return fmt.Errorf("delete %s %s: %w", entity.Kind, resourceID, err)
	}
	if entity.Cleanup.WaitForHardDelete {
		if err := h.pollHardDeletion(ctx, entity, parentID, resourceID); err != nil {
			return err
		}
	}
	return childErr
}

// cleanupChildren keeps the existing explicit child-first teardown: list once,
// skip deleted/ID-less children, attempt every remaining delete, then let the
// caller delete the parent. A list failure aborts; a child-delete failure does not.
func (h *Helper) cleanupChildren(
	ctx context.Context,
	entity descriptor.EntityDescriptor,
	resourceID string,
) (deleteErr, listErr error) {
	for _, kind := range entity.Cleanup.ChildKinds {
		child, err := h.Entities.Lookup(kind)
		if err != nil {
			return nil, err
		}
		path := child.CollectionPath(resourceID)
		children, err := h.Client.ListResources(ctx, path, "")
		if err != nil {
			return deleteErr, fmt.Errorf("list %s for %s %s: %w", kind, entity.Kind, resourceID, err)
		}
		for _, resource := range children.Items {
			if resource.DeletedTime != nil || resource.ID == nil {
				continue
			}
			childPath := child.ResourcePath(resourceID, *resource.ID)
			_, err := h.Client.DeleteResource(ctx, childPath)
			if err != nil && !isHTTPStatus(err, http.StatusNotFound) {
				slog.Error(
					"failed to delete child during cleanup",
					"kind",
					kind,
					"resource_id",
					*resource.ID,
					"error",
					err,
				)
				if deleteErr == nil {
					deleteErr = fmt.Errorf("delete %s %s: %w", kind, *resource.ID, err)
				}
			}
		}
	}
	return deleteErr, nil
}

func (h *Helper) pollHardDeletion(
	ctx context.Context,
	entity descriptor.EntityDescriptor,
	parentID, resourceID string,
) error {
	// Preserve the existing loop: the deadline is checked between requests, and
	// each request uses the caller's context. The interval is an ordinary sleep.
	poll := h.PollHTTPStatus(ctx, entity, parentID, resourceID)
	deadline := time.Now().Add(entity.Timeouts.Deleted)
	for time.Now().Before(deadline) {
		status, err := poll()
		if err != nil {
			return fmt.Errorf("polling hard-delete for %s %s: %w", entity.Kind, resourceID, err)
		}
		if status == http.StatusNotFound {
			return nil
		}
		if status >= http.StatusBadRequest {
			return fmt.Errorf("unexpected HTTP %d waiting for %s %s hard-delete", status, entity.Kind, resourceID)
		}
		time.Sleep(h.Cfg.Polling.Interval)
	}
	return fmt.Errorf("%s %s not hard-deleted within %s", entity.Kind, resourceID, entity.Timeouts.Deleted)
}

func isHTTPStatus(err error, status int) bool {
	var httpErr *client.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == status
}

// DeferCleanup registers warning-only cleanup in the current Ginkgo node.
func (h *Helper) DeferCleanup(entity descriptor.EntityDescriptor, parentID, resourceID string) {
	ginkgo.DeferCleanup(func(ctx context.Context) {
		if err := h.CleanupTestResource(ctx, entity, parentID, resourceID); err != nil {
			ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup %s %s: %v\n", entity.Kind, resourceID, err)
		}
	})
}
