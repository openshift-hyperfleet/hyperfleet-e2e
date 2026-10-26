package channel

import (
	"context"
	"errors"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: channel][delete-restrict] Channel Deletion Blocked While Versions Exist",
	ginkgo.Label(labels.Tier1, labels.Negative),
	func() {
		var h *helper.Helper
		var channelID string
		var versionID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating channel with a version")
			ch, err := h.Client.CreateChannelFromPayload(ctx, h.TestDataPath("payloads/channels/channel-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create channel")
			Expect(ch).NotTo(BeNil(), "created channel should not be nil")
			Expect(ch.ID).NotTo(BeNil(), "channel ID should not be nil")
			channelID = *ch.ID

			ver, err := h.Client.CreateVersionFromPayload(
				ctx,
				channelID,
				h.TestDataPath("payloads/versions/version-request.json"),
			)
			Expect(err).NotTo(HaveOccurred(), "failed to create version")
			Expect(ver).NotTo(BeNil(), "created version should not be nil")
			Expect(ver.ID).NotTo(BeNil(), "version ID should not be nil")
			versionID = *ver.ID

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestResource(ctx, h.Entities.MustGet("Channel"), "", channelID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup channel %s: %v\n", channelID, err)
				}
			})
		})

		ginkgo.It("should return 409 when deleting channel with existing versions", func(ctx context.Context) {
			ginkgo.By("attempting to delete channel while version exists")
			_, err := h.Client.DeleteChannel(ctx, channelID)
			var httpErr *client.HTTPError
			Expect(errors.As(err, &httpErr)).To(BeTrue(), "error should be HTTPError")
			Expect(httpErr.StatusCode).To(Equal(http.StatusConflict),
				"deleting channel with existing versions should return 409")
		})

		ginkgo.It("should allow channel deletion after all versions are deleted", func(ctx context.Context) {
			ginkgo.By("deleting the version first")
			_, err := h.Client.DeleteVersion(ctx, channelID, versionID)
			Expect(err).NotTo(HaveOccurred(), "failed to delete version")

			ginkgo.By("deleting the channel after versions are gone")
			deleted, err := h.Client.DeleteChannel(ctx, channelID)
			Expect(err).NotTo(HaveOccurred(), "channel deletion should succeed after versions deleted")
			Expect(deleted.DeletedTime).NotTo(BeNil(), "deleted channel should have deleted_time set")
		})
	},
)
