package cluster

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][update] Rapid Update Coalescing",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled at generation 1")
			cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
			Expect(cluster.Id).NotTo(BeNil())
			clusterID = *cluster.Id

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.True))
		})

		ginkgo.It("should coalesce multiple rapid updates and reconcile to the latest generation", func(ctx context.Context) {
			ginkgo.By("sending three PATCH requests in rapid succession")
			patch1, err := h.Client.PatchCluster(ctx, clusterID, openapi.ClusterPatchRequest{
				Labels: &map[string]string{"update": "first"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(patch1.Generation).To(Equal(int32(2)))

			patch2, err := h.Client.PatchCluster(ctx, clusterID, openapi.ClusterPatchRequest{
				Labels: &map[string]string{"update": "second"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(patch2.Generation).To(Equal(int32(3)))

			patch3, err := h.Client.PatchCluster(ctx, clusterID, openapi.ClusterPatchRequest{
				Labels: &map[string]string{"update": "third"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(patch3.Generation).To(Equal(int32(4)))

			ginkgo.By("waiting for all adapters to reconcile at the final generation")
			Eventually(h.PollClusterAdapterStatuses(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.Cluster, int32(4)))

			ginkgo.By("verifying cluster reaches Reconciled=True at final generation")
			Eventually(func(g Gomega) {
				finalCluster, err := h.Client.GetCluster(ctx, clusterID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(finalCluster.Generation).To(Equal(int32(4)))

				found := false
				for _, cond := range finalCluster.Status.Conditions {
					if cond.Type == client.ConditionTypeReconciled && cond.Status == openapi.True {
						found = true
						g.Expect(cond.ObservedGeneration).To(Equal(int32(4)))
					}
				}
				g.Expect(found).To(BeTrue(), "cluster should have Reconciled=True")
			}, h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).Should(Succeed())
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil || clusterID == "" {
				return
			}
			ginkgo.By("cleaning up cluster " + clusterID)
			if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
				ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
			}
		})
	},
)
