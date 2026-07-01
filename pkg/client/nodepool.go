package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/core"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

// CreateNodePool creates a new nodepool for the specified cluster.
func (c *HyperFleetClient) CreateNodePool(ctx context.Context, clusterID string, req openapi.NodePoolCreateRequest) (*openapi.NodePool, error) {
	logger.Info("creating nodepool", "cluster_id", clusterID, "name", req.Name)

	resp, err := c.Client.CreateNodePool(ctx, clusterID, req)
	if err != nil {
		logger.Error("failed to create nodepool", "cluster_id", clusterID, "name", req.Name, "error", err)
		return nil, fmt.Errorf("failed to create nodepool: %w", err)
	}

	nodepool, err := handleHTTPResponse[openapi.NodePool](resp, http.StatusCreated, "create nodepool")
	if err != nil {
		return nil, err
	}

	logger.Info("nodepool created", "cluster_id", clusterID, "nodepool_id", *nodepool.Id, "name", req.Name)
	return nodepool, nil
}

// GetNodePool retrieves a nodepool by ID.
func (c *HyperFleetClient) GetNodePool(ctx context.Context, clusterID, nodepoolID string) (*openapi.NodePool, error) {
	resp, err := c.GetNodePoolById(ctx, clusterID, nodepoolID)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodepool: %w", err)
	}
	return handleHTTPResponse[openapi.NodePool](resp, http.StatusOK, "get nodepool")
}

// ListNodePools retrieves all nodepools for a cluster.
func (c *HyperFleetClient) ListNodePools(ctx context.Context, clusterID string) (*openapi.NodePoolList, error) {
	resp, err := c.GetNodePoolsByClusterId(ctx, clusterID, &openapi.GetNodePoolsByClusterIdParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodepools: %w", err)
	}
	return handleHTTPResponse[openapi.NodePoolList](resp, http.StatusOK, "list nodepools")
}

// GetNodePoolStatuses retrieves all adapter statuses for a nodepool.
func (c *HyperFleetClient) GetNodePoolStatuses(ctx context.Context, clusterID, nodepoolID string) (*core.AdapterStatusList, error) {
	resp, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("clusters/%s/nodepools/%s/statuses", clusterID, nodepoolID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodepool statuses: %w", err)
	}
	return handleHTTPResponse[core.AdapterStatusList](resp, http.StatusOK, "get nodepool statuses")
}

// CreateNodePoolFromPayload creates a nodepool from a JSON payload file.
// The payload file should contain a NodePoolCreateRequest in JSON format.
func (c *HyperFleetClient) CreateNodePoolFromPayload(ctx context.Context, clusterID, payloadPath string) (*openapi.NodePool, error) {
	logger.Debug("loading nodepool payload", "cluster_id", clusterID, "payload_path", payloadPath)

	req, err := loadPayloadFromFile[openapi.NodePoolCreateRequest](payloadPath)
	if err != nil {
		logger.Error("failed to load payload", "cluster_id", clusterID, "payload_path", payloadPath, "error", err)
		return nil, err
	}

	return c.CreateNodePool(ctx, clusterID, *req)
}

// DeleteNodePool soft-deletes a nodepool by ID (sets deleted_time, returns 202).
func (c *HyperFleetClient) DeleteNodePool(ctx context.Context, clusterID, nodepoolID string) (*openapi.NodePool, error) {
	logger.Info("deleting nodepool", "cluster_id", clusterID, "nodepool_id", nodepoolID)

	resp, err := c.DeleteNodePoolById(ctx, clusterID, nodepoolID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete nodepool: %w", err)
	}

	nodepool, err := handleHTTPResponse[openapi.NodePool](resp, http.StatusAccepted, "delete nodepool")
	if err != nil {
		return nil, err
	}

	logger.Info("nodepool deleted", "cluster_id", clusterID, "nodepool_id", nodepoolID)
	return nodepool, nil
}

// PatchNodePool updates a nodepool via PATCH.
func (c *HyperFleetClient) PatchNodePool(ctx context.Context, clusterID, nodepoolID string, req openapi.NodePoolPatchRequest) (*openapi.NodePool, error) {
	logger.Info("patching nodepool", "cluster_id", clusterID, "nodepool_id", nodepoolID)

	resp, err := c.PatchNodePoolById(ctx, clusterID, nodepoolID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to patch nodepool: %w", err)
	}

	nodepool, err := handleHTTPResponse[openapi.NodePool](resp, http.StatusOK, "patch nodepool")
	if err != nil {
		return nil, err
	}

	logger.Info("nodepool patched", "cluster_id", clusterID, "nodepool_id", nodepoolID, "generation", nodepool.Generation)
	return nodepool, nil
}

// PatchNodePoolFromPayload patches a nodepool from a JSON payload file.
func (c *HyperFleetClient) PatchNodePoolFromPayload(ctx context.Context, clusterID, nodepoolID, payloadPath string) (*openapi.NodePool, error) {
	logger.Debug("loading nodepool patch payload", "payload_path", payloadPath)

	req, err := loadPayloadFromFile[openapi.NodePoolPatchRequest](payloadPath)
	if err != nil {
		return nil, err
	}

	return c.PatchNodePool(ctx, clusterID, nodepoolID, *req)
}

// ForceDeleteNodePool permanently removes a nodepool stuck in Finalizing from the database.
func (c *HyperFleetClient) ForceDeleteNodePool(ctx context.Context, clusterID, nodepoolID, reason string) error {
	logger.Info("force-deleting nodepool", "cluster_id", clusterID, "nodepool_id", nodepoolID, "reason", reason)

	resp, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("clusters/%s/nodepools/%s/force-delete", clusterID, nodepoolID), core.ForceDeleteRequest{Reason: reason})
	if err != nil {
		return fmt.Errorf("failed to force-delete nodepool: %w", err)
	}

	if err := handleHTTPNoBodyResponse(resp, http.StatusNoContent, "force-delete nodepool"); err != nil {
		return err
	}

	logger.Info("nodepool force-deleted", "cluster_id", clusterID, "nodepool_id", nodepoolID)
	return nil
}
