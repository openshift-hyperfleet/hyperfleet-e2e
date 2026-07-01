package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/samber/lo"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

func (c *HyperFleetClient) CreateChannel(ctx context.Context, req openapi.ChannelCreateRequest) (*openapi.Channel, error) {
	logger.Info("creating channel", "name", req.Name)

	resp, err := c.PostChannel(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create channel %q: %w", req.Name, err)
	}

	channel, err := handleHTTPResponse[openapi.Channel](resp, http.StatusCreated, "create channel")
	if err != nil {
		return nil, err
	}

	logger.Info("channel created", "channel_id", lo.FromPtr(channel.Id), "name", req.Name)
	return channel, nil
}

func (c *HyperFleetClient) GetChannel(ctx context.Context, channelID string) (*openapi.Channel, error) {
	resp, err := c.GetChannelById(ctx, channelID, &openapi.GetChannelByIdParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to get channel: %w", err)
	}
	return handleHTTPResponse[openapi.Channel](resp, http.StatusOK, "get channel")
}

func (c *HyperFleetClient) ListChannels(ctx context.Context, search string) (*openapi.ChannelList, error) {
	params := &openapi.GetChannelsParams{}
	if search != "" {
		params.Search = &search
	}
	resp, err := c.GetChannels(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list channels: %w", err)
	}
	return handleHTTPResponse[openapi.ChannelList](resp, http.StatusOK, "list channels")
}

func (c *HyperFleetClient) DeleteChannel(ctx context.Context, channelID string) (*openapi.Channel, error) {
	logger.Info("deleting channel", "channel_id", channelID)

	resp, err := c.DeleteChannelById(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete channel: %w", err)
	}

	channel, err := handleHTTPResponse[openapi.Channel](resp, http.StatusAccepted, "delete channel")
	if err != nil {
		return nil, err
	}

	logger.Info("channel deleted", "channel_id", channelID)
	return channel, nil
}

func (c *HyperFleetClient) PatchChannel(ctx context.Context, channelID string, req openapi.ChannelPatchRequest) (*openapi.Channel, error) {
	logger.Info("patching channel", "channel_id", channelID)

	resp, err := c.PatchChannelById(ctx, channelID, req)
	if err != nil {
		return nil, fmt.Errorf("failed to patch channel: %w", err)
	}

	channel, err := handleHTTPResponse[openapi.Channel](resp, http.StatusOK, "patch channel")
	if err != nil {
		return nil, err
	}

	logger.Info("channel patched", "channel_id", channelID, "generation", channel.Generation)
	return channel, nil
}

func (c *HyperFleetClient) CreateChannelFromPayload(ctx context.Context, payloadPath string) (*openapi.Channel, error) {
	logger.Debug("loading channel payload", "payload_path", payloadPath)

	req, err := loadPayloadFromFile[openapi.ChannelCreateRequest](payloadPath)
	if err != nil {
		logger.Error("failed to load payload", "payload_path", payloadPath, "error", err)
		return nil, err
	}

	return c.CreateChannel(ctx, *req)
}
