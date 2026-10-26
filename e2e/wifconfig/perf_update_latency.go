package wifconfig

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: wifconfig][perf] Update latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	ginkgo.Serial,
	func() {
		var h *helper.Helper
		var wifConfigID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			wifConfig, err := h.Client.CreateWifConfigFromPayload(
				ctx,
				h.TestDataPath("payloads/wifconfigs/wifconfig-request.json"),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(wifConfig.ID).NotTo(BeNil(), "wifconfig ID should be set")
			wifConfigID = *wifConfig.ID

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestResource(ctx, h.Entities.MustGet("WifConfig"), "", wifConfigID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup wifconfig %s: %v\n", wifConfigID, err)
				}
			})
		})

		ginkgo.It("should update a wifconfig within acceptable latency", func(ctx context.Context) {
			helper.MeasureMedianLatency(
				"PATCH /wifconfigs/{id}",
				h.Cfg.APILatencyThreshold(h.Entities.MustGet("WifConfig").Thresholds.Update),
				helper.DefaultSamples,
				func(i int) {
					_, err := h.Client.PatchWifConfig(ctx, wifConfigID, client.ResourcePatchRequest{
						Spec: map[string]any{
							"projectId": fmt.Sprintf("updated-project-%d", i),
							"version":   "4.18",
						},
					})
					Expect(err).NotTo(HaveOccurred())
				},
			)
		})
	},
)
