package wifconfig

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: wifconfig][perf] Delete latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	ginkgo.Serial,
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
		})

		ginkgo.It("should delete a wifconfig within acceptable latency", func(ctx context.Context) {
			wifConfigIDs := make([]string, helper.DefaultSamples)
			for i := range wifConfigIDs {
				wifConfig, err := h.Client.CreateWifConfigFromPayload(
					ctx,
					h.TestDataPath("payloads/wifconfigs/wifconfig-request.json"),
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(wifConfig.ID).NotTo(BeNil(), "wifconfig ID should be set")
				wifConfigIDs[i] = *wifConfig.ID
				h.DeferCleanup(h.Entities.MustGet("WifConfig"), "", *wifConfig.ID)
			}

			helper.MeasureMedianLatency(
				"DELETE /wifconfigs/{id}",
				h.Cfg.APILatencyThreshold(h.Entities.MustGet("WifConfig").Thresholds.Delete),
				len(wifConfigIDs),
				func(i int) {
					deleted, err := h.Client.DeleteWifConfig(ctx, wifConfigIDs[i])
					Expect(err).NotTo(HaveOccurred())
					Expect(deleted.DeletedTime).NotTo(BeNil(), "deleted wifconfig should have deleted_time set")
				},
			)
		})
	},
)
