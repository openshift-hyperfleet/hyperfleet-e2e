package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/samber/lo"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

func (c *HyperFleetClient) CreateVersion(ctx context.Context, channelID string, req openapi.VersionCreateRequest) (*openapi.Version, error) {
	logger.Info("creating version", "channel_id", channelID, "name", req.Name)

	resp, err := c.PostVersion(ctx, channelID, req)
	if err != nil {
		return nil, fmt.Errorf("create version %q in channel %s: %w", req.Name, channelID, err)
	}

	version, err := handleHTTPResponse[openapi.Version](resp, http.StatusCreated, "create version")
	if err != nil {
		return nil, err
	}

	logger.Info("version created", "channel_id", channelID, "version_id", lo.FromPtr(version.Id), "name", req.Name)
	return version, nil
}

func (c *HyperFleetClient) GetVersion(ctx context.Context, channelID, versionID string) (*openapi.Version, error) {
	resp, err := c.GetVersionById(ctx, channelID, versionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	return handleHTTPResponse[openapi.Version](resp, http.StatusOK, "get version")
}

func (c *HyperFleetClient) ListVersions(ctx context.Context, channelID, search string) (*openapi.VersionList, error) {
	params := &openapi.GetVersionsByChannelIdParams{}
	if search != "" {
		params.Search = &search
	}
	resp, err := c.GetVersionsByChannelId(ctx, channelID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}
	return handleHTTPResponse[openapi.VersionList](resp, http.StatusOK, "list versions")
}

func (c *HyperFleetClient) DeleteVersion(ctx context.Context, channelID, versionID string) (*openapi.Version, error) {
	logger.Info("deleting version", "channel_id", channelID, "version_id", versionID)

	resp, err := c.DeleteVersionById(ctx, channelID, versionID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete version: %w", err)
	}

	version, err := handleHTTPResponse[openapi.Version](resp, http.StatusAccepted, "delete version")
	if err != nil {
		return nil, err
	}

	logger.Info("version deleted", "channel_id", channelID, "version_id", versionID)
	return version, nil
}

func (c *HyperFleetClient) PatchVersion(ctx context.Context, channelID, versionID string, req openapi.VersionPatchRequest) (*openapi.Version, error) {
	logger.Info("patching version", "channel_id", channelID, "version_id", versionID)

	resp, err := c.PatchVersionById(ctx, channelID, versionID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to patch version: %w", err)
	}

	version, err := handleHTTPResponse[openapi.Version](resp, http.StatusOK, "patch version")
	if err != nil {
		return nil, err
	}

	logger.Info("version patched", "channel_id", channelID, "version_id", versionID, "generation", version.Generation)
	return version, nil
}

func (c *HyperFleetClient) CreateVersionFromPayload(ctx context.Context, channelID, payloadPath string) (*openapi.Version, error) {
	logger.Debug("loading version payload", "channel_id", channelID, "payload_path", payloadPath)

	req, err := loadPayloadFromFile[openapi.VersionCreateRequest](payloadPath)
	if err != nil {
		logger.Error("failed to load payload", "channel_id", channelID, "payload_path", payloadPath, "error", err)
		return nil, err
	}

	return c.CreateVersion(ctx, channelID, *req)
}
