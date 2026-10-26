package cluster

import (
	"context"
	"net/url"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][perf] API list latency with filters and pagination",
	ginkgo.Label(labels.Tier1, labels.Performance),
	ginkgo.Serial,
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			cluster, err := h.Client.CreateClusterFromPayload(
				ctx,
				h.TestDataPath("payloads/clusters/cluster-request.json"),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.ID).NotTo(BeNil(), "cluster ID should be set")
			clusterID = *cluster.ID

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestResource(ctx, h.Entities.MustGet("Cluster"), "", clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
				}
			})
		})

		ginkgo.It("should list clusters with search filter within acceptable latency", func(ctx context.Context) {
			filter := "labels.environment='test'"
			helper.MeasureMedianLatency(
				"GET /clusters (search filter)",
				h.Cfg.APILatencyThreshold(h.Entities.MustGet("Cluster").Thresholds.List),
				helper.DefaultSamples,
				func(int) {
					_, err := h.Client.ListClustersWithParams(ctx, url.Values{"search": {filter}})
					Expect(err).NotTo(HaveOccurred())
				},
			)
		})

		ginkgo.It("should list clusters with page size limit within acceptable latency", func(ctx context.Context) {
			helper.MeasureMedianLatency(
				"GET /clusters (size=10)",
				h.Cfg.APILatencyThreshold(h.Entities.MustGet("Cluster").Thresholds.List),
				helper.DefaultSamples,
				func(int) {
					_, err := h.Client.ListClustersWithParams(ctx, url.Values{"size": {"10"}})
					Expect(err).NotTo(HaveOccurred())
				},
			)
		})

		ginkgo.It("should list clusters with pagination within acceptable latency", func(ctx context.Context) {
			helper.MeasureMedianLatency(
				"GET /clusters (page=1, size=10)",
				h.Cfg.APILatencyThreshold(h.Entities.MustGet("Cluster").Thresholds.List),
				helper.DefaultSamples,
				func(int) {
					_, err := h.Client.ListClustersWithParams(ctx, url.Values{"page": {"1"}, "size": {"10"}})
					Expect(err).NotTo(HaveOccurred())
				},
			)
		})
	},
)
