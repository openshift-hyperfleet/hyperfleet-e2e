package nodepool

import (
	"context"
	"errors"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/core"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: nodepool][delete] NodePool Deletion Lifecycle",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var clusterID string
		var nodepoolID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if cluster, err := h.Client.GetCluster(ctx, clusterID); err == nil && cluster.DeletedTime == nil {
					if _, err := h.Client.DeleteCluster(ctx, clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: API delete failed for cluster %s: %v\n", clusterID, err)
					}
				}
				if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
				}
			})

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.True))

			ginkgo.By("creating nodepool and waiting for Reconciled")
			np, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create nodepool")
			Expect(np.Id).NotTo(BeNil(), "nodepool ID should be generated")
			nodepoolID = *np.Id

			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.True))
		})

		ginkgo.It("should complete full deletion lifecycle from soft-delete through hard-delete", func(ctx context.Context) {
			npBefore, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
			Expect(err).NotTo(HaveOccurred())
			parentBefore, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())

			ginkgo.By("soft-deleting the nodepool")
			deletedNP, err := h.Client.DeleteNodePool(ctx, clusterID, nodepoolID)
			Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
			Expect(deletedNP.DeletedTime).NotTo(BeNil(), "soft-deleted nodepool should have deleted_time set")
			Expect(deletedNP.Generation).To(Equal(npBefore.Generation+1), "generation should increment after soft-delete")

			ginkgo.By("waiting for nodepool adapters to finalize and nodepool to be hard-deleted")
			// Hard-delete executes atomically within the POST /adapter_statuses request that
			// computes Reconciled=True, so there is no observable window to see Finalized=True
			// on the statuses endpoint. Accept either Finalized=True OR 404 (already hard-deleted).
			Eventually(func(g Gomega) {
				var httpErr *client.HTTPError
				_, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
					return
				}
				g.Expect(err).NotTo(HaveOccurred())
				statuses, err := h.Client.GetNodePoolStatuses(ctx, clusterID, nodepoolID)
				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
					return
				}
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statuses).To(helper.HaveAllAdaptersWithCondition(
					h.Cfg.Adapters.NodePool, client.ConditionTypeFinalized, core.AdapterConditionStatusTrue))
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

			ginkgo.By("confirming nodepool is hard-deleted")
			Eventually(h.PollNodePoolHTTPStatus(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("verifying parent cluster is unaffected")
			parentCluster, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "parent cluster should still exist")
			Expect(parentCluster.DeletedTime).To(BeNil(), "parent cluster should not have deleted_time")
			Expect(parentCluster.Generation).To(Equal(parentBefore.Generation), "parent cluster generation should remain unchanged")

			Expect(parentCluster).To(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.True),
				"parent cluster should remain Reconciled=True")
		})

	},
)
