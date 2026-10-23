package cluster

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][perf] API read latency by entity size",
	ginkgo.Label(labels.Tier1, labels.Performance),
	// Serial: the ms-scale latency assertion needs CPU contention-free timing,
	// and the Eventually(...Reconciled...) wait needs reconciliation itself
	// free of concurrent create/reconcile load elsewhere in the suite.
	ginkgo.Serial,
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
		})

		sizes := []struct {
			name    string
			payload string
		}{
			{"small", "payloads/clusters/cluster-request-small.json"},
			{"medium", "payloads/clusters/cluster-request.json"},
			{"large", "payloads/clusters/cluster-request-large.json"},
		}

		for _, size := range sizes {
			ginkgo.It("should read a "+size.name+" cluster within acceptable latency", func(ctx context.Context) {
				ginkgo.By("creating a " + size.name + " cluster")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath(size.payload))
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be set")
				clusterID := *cluster.Id

				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := h.CleanupTestResource(ctx, h.Entities.MustGet("Cluster"), "", clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
					}
				})

				ginkgo.By("waiting for cluster to reach Reconciled before read")
				Eventually(h.Poll(ctx, h.Entities.MustGet("Cluster"), "", clusterID), h.Entities.MustGet("Cluster").Timeouts.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				helper.MeasureMedianLatency("GET /clusters/{id} ("+size.name+" entity)", h.Entities.MustGet("Cluster").Thresholds.Read, helper.DefaultSamples,
					func(int) {
						_, err := h.Client.GetCluster(ctx, clusterID)
						Expect(err).NotTo(HaveOccurred())
					},
				)
			})
		}
	},
)
