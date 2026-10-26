package nodepool

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: nodepool][update] NodePool Update Lifecycle",
	ginkgo.Label(labels.Tier0),
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

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if cleanupErr := h.CleanupTestResource(ctx, h.Entities.MustGet("Cluster"), "", clusterID); cleanupErr != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, cleanupErr)
				}
			})

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
			Expect(np.ID).NotTo(BeNil(), "nodepool ID should be generated")
			nodepoolID = *np.ID

			Eventually(
				h.Poll(ctx, h.Entities.MustGet("NodePool"), clusterID, nodepoolID),
				h.Entities.MustGet("NodePool").Timeouts.Reconciled,
				h.Cfg.Polling.Interval,
			).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))
		})

		ginkgo.It(
			"should update nodepool via PATCH, trigger reconciliation, and reach Reconciled at new generation",
			func(ctx context.Context) {
				ginkgo.By("capturing state before PATCH")
				npBefore, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
				Expect(err).NotTo(HaveOccurred())
				Expect(npBefore.Generation).To(Equal(int32(1)), "nodepool should be at generation 1 before update")
				parentBefore, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())

				ginkgo.By("sending PATCH to update nodepool spec")
				patchedNP, err := h.Client.PatchNodePoolFromPayload(
					ctx,
					clusterID,
					nodepoolID,
					h.TestDataPath("payloads/nodepools/nodepool-patch.json"),
				)
				Expect(err).NotTo(HaveOccurred(), "PATCH request should succeed")
				expectedGen := npBefore.Generation + 1
				Expect(patchedNP.Generation).To(Equal(expectedGen), "generation should increment after PATCH")
				Expect(patchedNP.Spec).To(HaveKeyWithValue("replicas", BeNumerically("==", 3)),
					"PATCH response should reflect updated replicas field")

				ginkgo.By("waiting for all nodepool adapters to reconcile at new generation")
				Eventually(
					h.PollAdapterStatuses(ctx, h.Entities.MustGet("NodePool"), clusterID, nodepoolID),
					h.Cfg.Timeouts.Adapter.Processing,
					h.Cfg.Polling.Interval,
				).
					Should(helper.HaveAllAdaptersAtGeneration(h.Entities.MustGet("NodePool").RequiredAdapters, expectedGen))

				ginkgo.By("verifying nodepool reaches Reconciled=True at new generation")
				Eventually(func(g Gomega) {
					finalNP, getErr := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
					g.Expect(getErr).NotTo(HaveOccurred())
					g.Expect(finalNP.Generation).
						To(Equal(expectedGen), "final nodepool generation should match expected")
					g.Expect(finalNP.Status).NotTo(BeNil(), "nodepool status should be present")

					found := false
					for _, cond := range finalNP.Status.Conditions {
						if cond.Type == client.ConditionTypeReconciled &&
							cond.Status == client.ResourceConditionStatusTrue {
							found = true
							g.Expect(cond.ObservedGeneration).
								To(Equal(expectedGen), "Reconciled condition observed_generation should match expected")
						}
					}
					g.Expect(found).To(BeTrue(), "nodepool should have Reconciled=True")
				}, h.Entities.MustGet("NodePool").Timeouts.Reconciled, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("verifying parent cluster generation is unchanged")
				parentCluster, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())
				Expect(
					parentCluster.Generation,
				).To(Equal(parentBefore.Generation), "nodepool update should not affect cluster generation")

				Expect(
					parentCluster,
				).To(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue),
					"parent cluster should remain Reconciled=True")
			},
		)

	},
)
