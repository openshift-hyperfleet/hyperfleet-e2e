package nodepool

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: nodepool][update] Labels-Only PATCH",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var clusterID string
		var nodepoolID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			var err error
			clusterID, err = h.CreateTestResource(
				ctx,
				h.Entities.MustGet("Cluster"),
				"",
				h.TestDataPath("payloads/clusters/cluster-request.json"),
			)
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

			Eventually(
				h.Poll(ctx, h.Entities.MustGet("Cluster"), "", clusterID),
				h.Entities.MustGet("Cluster").Timeouts.Reconciled,
				h.Cfg.Polling.Interval,
			).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

			ginkgo.By("creating nodepool and waiting for Reconciled at generation 1")
			np, err := h.Client.CreateNodePoolFromPayload(
				ctx,
				clusterID,
				h.TestDataPath("payloads/nodepools/nodepool-request.json"),
			)
			Expect(err).NotTo(HaveOccurred(), "failed to create nodepool")
			Expect(np.ID).NotTo(BeNil())
			nodepoolID = *np.ID

			Eventually(
				h.Poll(ctx, h.Entities.MustGet("NodePool"), clusterID, nodepoolID),
				h.Entities.MustGet("NodePool").Timeouts.Reconciled,
				h.Cfg.Polling.Interval,
			).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))
		})

		ginkgo.It(
			"should bump generation and trigger reconciliation from a labels-only PATCH",
			func(ctx context.Context) {
				ginkgo.By("capturing state before labels-only PATCH")
				npBefore, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
				Expect(err).NotTo(HaveOccurred())
				specBefore := npBefore.Spec
				parentBefore, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())

				ginkgo.By("sending labels-only PATCH to nodepool (preserving existing labels)")
				newLabels := make(map[string]string)
				for k, v := range npBefore.Labels {
					newLabels[k] = v
				}
				newLabels["env"] = "staging"
				newLabels["pool-type"] = "gpu"
				patchedNP, err := h.Client.PatchNodePool(ctx, clusterID, nodepoolID, client.ResourcePatchRequest{
					Labels: newLabels,
				})
				Expect(err).NotTo(HaveOccurred(), "labels-only PATCH should succeed")
				Expect(patchedNP.Generation).To(Equal(int32(2)),
					"generation should increment after labels-only PATCH")

				ginkgo.By("waiting for all nodepool adapters to reconcile at generation 2")
				Eventually(
					h.PollAdapterStatuses(ctx, h.Entities.MustGet("NodePool"), clusterID, nodepoolID),
					h.Cfg.Timeouts.Adapter.Processing,
					h.Cfg.Polling.Interval,
				).
					Should(helper.HaveAllAdaptersAtGeneration(h.Entities.MustGet("NodePool").RequiredAdapters, int32(2)))

				ginkgo.By("verifying nodepool reaches Reconciled=True and LastKnownReconciled=True")
				Eventually(
					h.Poll(ctx, h.Entities.MustGet("NodePool"), clusterID, nodepoolID),
					h.Entities.MustGet("NodePool").Timeouts.Reconciled,
					h.Cfg.Polling.Interval,
				).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))
				Eventually(
					h.Poll(ctx, h.Entities.MustGet("NodePool"), clusterID, nodepoolID),
					h.Entities.MustGet("NodePool").Timeouts.Reconciled,
					h.Cfg.Polling.Interval,
				).
					Should(helper.HaveResourceCondition(client.ConditionTypeLastKnownReconciled, client.ResourceConditionStatusTrue))

				finalNP, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
				Expect(err).NotTo(HaveOccurred())
				Expect(finalNP.Labels).NotTo(BeEmpty(), "nodepool should have labels")
				Expect(finalNP.Labels["env"]).To(Equal("staging"))
				Expect(finalNP.Labels["pool-type"]).To(Equal("gpu"))
				Expect(finalNP.Spec).To(Equal(specBefore),
					"spec should be unchanged after labels-only PATCH")

				ginkgo.By("verifying parent cluster generation is unchanged")
				parentCluster, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())
				Expect(parentCluster.Generation).To(Equal(parentBefore.Generation),
					"nodepool labels PATCH should not affect cluster generation")

				hasParentReconciled := h.HasResourceCondition(
					parentCluster.Status.Conditions,
					client.ConditionTypeReconciled,
					client.ResourceConditionStatusTrue,
				)
				Expect(hasParentReconciled).To(BeTrue(), "parent cluster should remain Reconciled=True")
			},
		)

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil || clusterID == "" {
				return
			}
			ginkgo.By("cleaning up cluster " + clusterID)
			if err := h.CleanupTestResource(ctx, h.Entities.MustGet("Cluster"), "", clusterID); err != nil {
				ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
			}
		})
	},
)
