package channel

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: channel][perf] Create latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	ginkgo.Serial,
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
		})

		ginkgo.It("should create a channel within acceptable latency", func(ctx context.Context) {
			helper.MeasureMedianLatency(
				"POST /channels",
				h.Cfg.APILatencyThreshold(h.Entities.MustGet("Channel").Thresholds.Create),
				helper.DefaultSamples,
				func(int) {
					channel, err := h.Client.CreateChannelFromPayload(
						ctx,
						h.TestDataPath("payloads/channels/channel-request.json"),
					)
					if channel != nil && channel.ID != nil {
						h.DeferCleanup(h.Entities.MustGet("Channel"), "", *channel.ID)
					}
					Expect(err).NotTo(HaveOccurred())
					Expect(channel.ID).NotTo(BeNil(), "channel ID should be set")
				},
			)
		})
	},
)
