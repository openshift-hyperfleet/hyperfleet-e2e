package wifconfig

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: wifconfig][perf] Create latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	ginkgo.Serial,
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
		})

		ginkgo.It("should create a wifconfig within acceptable latency", func(ctx context.Context) {
			helper.MeasureMedianLatency(
				"POST /wifconfigs",
				h.Cfg.APILatencyThreshold(h.Entities.MustGet("WifConfig").Thresholds.Create),
				helper.DefaultSamples,
				func(int) {
					wifConfig, err := h.Client.CreateWifConfigFromPayload(
						ctx,
						h.TestDataPath("payloads/wifconfigs/wifconfig-request.json"),
					)
					if wifConfig != nil && wifConfig.ID != nil {
						h.DeferCleanup(h.Entities.MustGet("WifConfig"), "", *wifConfig.ID)
					}
					Expect(err).NotTo(HaveOccurred())
					Expect(wifConfig.ID).NotTo(BeNil(), "wifconfig ID should be set")
				},
			)
		})
	},
)
