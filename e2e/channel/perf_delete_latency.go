package channel

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: channel][perf] Delete latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	ginkgo.Serial,
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
		})

		ginkgo.It("should delete a channel within acceptable latency", func(ctx context.Context) {
			channelIDs := make([]string, helper.DefaultSamples)
			for i := range channelIDs {
				channel, err := h.Client.CreateChannelFromPayload(ctx, h.TestDataPath("payloads/channels/channel-request.json"))
				Expect(err).NotTo(HaveOccurred())
				Expect(channel.Id).NotTo(BeNil(), "channel ID should be set")
				channelIDs[i] = *channel.Id
				h.DeferCleanup(h.Entities.MustGet("Channel"), "", *channel.Id)
			}

			helper.MeasureMedianLatency("DELETE /channels/{id}", h.Cfg.APILatencyThreshold(config.ThresholdAPIDelete), len(channelIDs),
				func(i int) {
					deleted, err := h.Client.DeleteChannel(ctx, channelIDs[i])
					Expect(err).NotTo(HaveOccurred())
					Expect(deleted.DeletedTime).NotTo(BeNil(), "deleted channel should have deleted_time set")
				},
			)
		})
	},
)
