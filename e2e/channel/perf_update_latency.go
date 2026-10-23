package channel

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: channel][perf] Update latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	ginkgo.Serial,
	func() {
		var h *helper.Helper
		var channelID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			channel, err := h.Client.CreateChannelFromPayload(ctx, h.TestDataPath("payloads/channels/channel-request.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(channel.Id).NotTo(BeNil(), "channel ID should be set")
			channelID = *channel.Id

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestResource(ctx, h.Entities.MustGet("Channel"), "", channelID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup channel %s: %v\n", channelID, err)
				}
			})
		})

		ginkgo.It("should update a channel within acceptable latency", func(ctx context.Context) {
			helper.MeasureMedianLatency("PATCH /channels/{id}", h.Entities.MustGet("Channel").Thresholds.Update, helper.DefaultSamples,
				func(i int) {
					_, err := h.Client.PatchChannel(ctx, channelID, client.ResourcePatchRequest{
						Spec: map[string]any{
							"is_default":    true,
							"enabled_regex": fmt.Sprintf("^v%d\\..*$", i),
						},
					})
					Expect(err).NotTo(HaveOccurred())
				},
			)
		})
	},
)
