package version

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: version][perf] API read latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	ginkgo.Serial,
	func() {
		var h *helper.Helper
		var channelID string
		var versionID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating parent channel")
			ch, err := h.Client.CreateChannelFromPayload(ctx, h.TestDataPath("payloads/channels/channel-request.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(ch.Id).NotTo(BeNil(), "channel ID should be set")
			channelID = *ch.Id

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestResource(ctx, h.Entities.MustGet("Channel"), "", channelID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup channel %s: %v\n", channelID, err)
				}
			})

			ginkgo.By("creating version under channel")
			version, err := h.Client.CreateVersionFromPayload(ctx, channelID, h.TestDataPath("payloads/versions/version-request.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(version.Id).NotTo(BeNil(), "version ID should be set")
			versionID = *version.Id
		})

		ginkgo.It("should read a version within acceptable latency", func(ctx context.Context) {
			helper.MeasureMedianLatency("GET /channels/{parent_id}/versions/{id}", h.Entities.MustGet("Version").Thresholds.Read, helper.DefaultSamples,
				func(int) {
					_, err := h.Client.GetVersion(ctx, channelID, versionID)
					Expect(err).NotTo(HaveOccurred())
				},
			)
		})
	},
)
