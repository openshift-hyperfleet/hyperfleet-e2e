package nodepool

import (
	"context"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: nodepool][baseline] NodePool Full Lifecycle Smoke",
	ginkgo.Label(labels.Tier0),
	func() {
		ginkgo.It(
			"should complete create → Reconciled → update → re-Reconciled → delete → hard-delete in a single pass",
			func(ctx context.Context) {
				h := helper.New()

				ginkgo.By("creating a parent cluster and waiting for Reconciled")
				clusterID, err := h.CreateTestResource(
					ctx,
					h.Entities.MustGet("Cluster"),
					"",
					h.TestDataPath("payloads/clusters/cluster-request.json"),
				)
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

				ginkgo.DeferCleanup(func(ctx context.Context) {
					if cleanupErr := h.CleanupTestResource(ctx, h.Entities.MustGet("Cluster"), "", clusterID); cleanupErr != nil {
						ginkgo.GinkgoWriter.Printf(
							"Warning: cleanup failed for cluster %s: %v\n",
							clusterID,
							cleanupErr,
						)
					}
				})

				Eventually(
					h.Poll(ctx, h.Entities.MustGet("Cluster"), "", clusterID),
					h.Entities.MustGet("Cluster").Timeouts.Reconciled,
					h.Cfg.Polling.Interval,
				).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				ginkgo.By("creating a nodepool")
				np, err := h.Client.CreateNodePoolFromPayload(
					ctx,
					clusterID,
					h.TestDataPath("payloads/nodepools/nodepool-request.json"),
				)
				Expect(err).NotTo(HaveOccurred(), "create should succeed")
				Expect(np).NotTo(BeNil(), "create should return nodepool object")
				Expect(np.ID).NotTo(BeNil(), "nodepool ID should be assigned")
				nodepoolID := *np.ID

				ginkgo.By("waiting for Reconciled=True at generation 1")
				Eventually(
					h.Poll(ctx, h.Entities.MustGet("NodePool"), clusterID, nodepoolID),
					h.Entities.MustGet("NodePool").Timeouts.Reconciled,
					h.Cfg.Polling.Interval,
				).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				ginkgo.By("PATCHing nodepool spec to trigger generation bump")
				patched, err := h.Client.PatchNodePool(ctx, clusterID, nodepoolID, client.ResourcePatchRequest{
					Spec: map[string]any{"lifecycle-test": "updated"},
				})
				Expect(err).NotTo(HaveOccurred(), "PATCH should succeed")
				Expect(patched).NotTo(BeNil(), "PATCH should return nodepool object")
				Expect(patched.Generation).To(Equal(int32(2)), "generation should increment after PATCH")

				ginkgo.By("waiting for Reconciled=True at generation 2")
				Eventually(h.PollAdapterStatuses(ctx, h.Entities.MustGet("NodePool"), clusterID, nodepoolID),
					h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
					Should(helper.HaveAllAdaptersAtGeneration(h.Entities.MustGet("NodePool").RequiredAdapters, int32(2)))

				Eventually(
					h.Poll(ctx, h.Entities.MustGet("NodePool"), clusterID, nodepoolID),
					h.Entities.MustGet("NodePool").Timeouts.Reconciled,
					h.Cfg.Polling.Interval,
				).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				ginkgo.By("soft-deleting the nodepool")
				deletedNP, err := h.Client.DeleteNodePool(ctx, clusterID, nodepoolID)
				Expect(err).NotTo(HaveOccurred(), "DELETE should return 202")
				Expect(deletedNP).NotTo(BeNil(), "DELETE should return nodepool object")
				Expect(deletedNP.DeletedTime).NotTo(BeNil())

				ginkgo.By("waiting for hard-delete (404)")
				Eventually(h.PollHTTPStatus(ctx, h.Entities.MustGet("NodePool"), clusterID, nodepoolID),
					h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
					Should(Equal(http.StatusNotFound))

				ginkgo.By("verifying parent cluster is unaffected")
				parentCluster, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "parent cluster should still exist")
				Expect(parentCluster).NotTo(BeNil(), "GET should return parent cluster object")
				Expect(parentCluster.DeletedTime).To(BeNil(), "parent cluster should not be deleted")
				Expect(parentCluster).To(helper.HaveResourceCondition(
					client.ConditionTypeReconciled, client.ResourceConditionStatusTrue),
					"parent cluster should remain Reconciled=True")
			},
		)
	},
)
