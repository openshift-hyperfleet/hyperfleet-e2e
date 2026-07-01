package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/samber/lo"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

func (c *HyperFleetClient) CreateWifConfig(ctx context.Context, req openapi.WifConfigCreateRequest) (*openapi.WifConfig, error) {
	logger.Info("creating wifconfig", "name", req.Name)

	resp, err := c.PostWifConfig(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create wifconfig %q: %w", req.Name, err)
	}

	wifConfig, err := handleHTTPResponse[openapi.WifConfig](resp, http.StatusCreated, "create wifconfig")
	if err != nil {
		return nil, err
	}

	logger.Info("wifconfig created", "wifconfig_id", lo.FromPtr(wifConfig.Id), "name", req.Name)
	return wifConfig, nil
}

func (c *HyperFleetClient) GetWifConfig(ctx context.Context, wifConfigID string) (*openapi.WifConfig, error) {
	resp, err := c.GetWifConfigById(ctx, wifConfigID, &openapi.GetWifConfigByIdParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to get wifconfig: %w", err)
	}
	return handleHTTPResponse[openapi.WifConfig](resp, http.StatusOK, "get wifconfig")
}

func (c *HyperFleetClient) ListWifConfigs(ctx context.Context, search string) (*openapi.WifConfigList, error) {
	params := &openapi.GetWifConfigsParams{}
	if search != "" {
		params.Search = &search
	}
	resp, err := c.GetWifConfigs(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list wifconfigs: %w", err)
	}
	return handleHTTPResponse[openapi.WifConfigList](resp, http.StatusOK, "list wifconfigs")
}

func (c *HyperFleetClient) DeleteWifConfig(ctx context.Context, wifConfigID string) (*openapi.WifConfig, error) {
	logger.Info("deleting wifconfig", "wifconfig_id", wifConfigID)

	resp, err := c.DeleteWifConfigById(ctx, wifConfigID)
	if err != nil {
		return nil, fmt.Errorf("delete wifconfig %q: %w", wifConfigID, err)
	}

	wifConfig, err := handleHTTPResponse[openapi.WifConfig](resp, http.StatusAccepted, "delete wifconfig")
	if err != nil {
		return nil, err
	}

	logger.Info("wifconfig deleted", "wifconfig_id", wifConfigID)
	return wifConfig, nil
}

func (c *HyperFleetClient) PatchWifConfig(ctx context.Context, wifConfigID string, req openapi.WifConfigPatchRequest) (*openapi.WifConfig, error) {
	logger.Info("patching wifconfig", "wifconfig_id", wifConfigID)

	resp, err := c.PatchWifConfigById(ctx, wifConfigID, req)
	if err != nil {
		return nil, fmt.Errorf("patch wifconfig %q: %w", wifConfigID, err)
	}

	wifConfig, err := handleHTTPResponse[openapi.WifConfig](resp, http.StatusOK, "patch wifconfig")
	if err != nil {
		return nil, err
	}

	logger.Info("wifconfig patched", "wifconfig_id", wifConfigID, "generation", wifConfig.Generation)
	return wifConfig, nil
}

func (c *HyperFleetClient) CreateWifConfigFromPayload(ctx context.Context, payloadPath string) (*openapi.WifConfig, error) {
	logger.Debug("loading wifconfig payload", "payload_path", payloadPath)

	req, err := loadPayloadFromFile[openapi.WifConfigCreateRequest](payloadPath)
	if err != nil {
		logger.Error("failed to load payload", "payload_path", payloadPath, "error", err)
		return nil, err
	}

	return c.CreateWifConfig(ctx, *req)
}
