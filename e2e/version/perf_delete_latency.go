package version

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: version][perf] Delete latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	ginkgo.Serial,
	func() {
		var h *helper.Helper
		var channelID string

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
		})

		ginkgo.It("should delete a version within acceptable latency", func(ctx context.Context) {
			// No per-version cleanup: the channel's DeferCleanup above sweeps all its versions.
			versionIDs := make([]string, helper.DefaultSamples)
			for i := range versionIDs {
				version, err := h.Client.CreateVersionFromPayload(ctx, channelID, h.TestDataPath("payloads/versions/version-request.json"))
				Expect(err).NotTo(HaveOccurred())
				Expect(version.Id).NotTo(BeNil(), "version ID should be set")
				versionIDs[i] = *version.Id
			}

			helper.MeasureMedianLatency("DELETE /channels/{parent_id}/versions/{id}", h.Cfg.APILatencyThreshold(config.ThresholdAPINestedWrite), len(versionIDs),
				func(i int) {
					deleted, err := h.Client.DeleteVersion(ctx, channelID, versionIDs[i])
					Expect(err).NotTo(HaveOccurred())
					Expect(deleted.DeletedTime).NotTo(BeNil(), "deleted version should have deleted_time set")
				},
			)
		})
	},
)
