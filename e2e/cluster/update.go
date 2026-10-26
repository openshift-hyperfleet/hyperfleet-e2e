package cluster

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][update] Cluster Update Lifecycle",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled at generation 1")
			cluster, err := h.Client.CreateClusterFromPayload(
				ctx,
				h.TestDataPath("payloads/clusters/cluster-request.json"),
			)
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
			Expect(cluster.ID).NotTo(BeNil(), "cluster ID should be generated")
			clusterID = *cluster.ID

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestResource(ctx, h.Entities.MustGet("Cluster"), "", clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
				}
			})

			Eventually(
				h.Poll(ctx, h.Entities.MustGet("Cluster"), "", clusterID),
				h.Entities.MustGet("Cluster").Timeouts.Reconciled,
				h.Cfg.Polling.Interval,
			).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))
		})

		ginkgo.It(
			"should update cluster via PATCH, trigger reconciliation, and reach Reconciled at new generation",
			func(ctx context.Context) {
				ginkgo.By("verifying cluster is at generation 1 before PATCH")
				clusterBefore, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())
				Expect(clusterBefore.Generation).To(Equal(int32(1)), "cluster should be at generation 1 before update")

				ginkgo.By("sending PATCH to update cluster spec")
				patchedCluster, err := h.Client.PatchClusterFromPayload(
					ctx,
					clusterID,
					h.TestDataPath("payloads/clusters/cluster-patch.json"),
				)
				Expect(err).NotTo(HaveOccurred(), "PATCH request should succeed")
				expectedGen := clusterBefore.Generation + 1
				Expect(patchedCluster.Generation).To(Equal(expectedGen), "generation should increment after PATCH")
				Expect(patchedCluster.Spec).To(HaveKey("dns"),
					"PATCH response should reflect updated spec fields")

				if expected := h.ExpectedIdentity(); expected != "" {
					Expect(patchedCluster.UpdatedBy).NotTo(BeNil(), "updated_by should be set after PATCH")
					Expect(
						*patchedCluster.UpdatedBy,
					).To(Equal(expected), "updated_by should match configured identity after PATCH")
				}

				ginkgo.By("waiting for all adapters to reconcile at new generation")
				Eventually(
					h.PollAdapterStatuses(ctx, h.Entities.MustGet("Cluster"), "", clusterID),
					h.Cfg.Timeouts.Adapter.Processing,
					h.Cfg.Polling.Interval,
				).
					Should(helper.HaveAllAdaptersAtGeneration(h.Entities.MustGet("Cluster").RequiredAdapters, expectedGen))

				ginkgo.By("verifying cluster reaches Reconciled=True at new generation")
				Eventually(func(g Gomega) {
					finalCluster, err := h.Client.GetCluster(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(finalCluster.Generation).
						To(Equal(expectedGen), "final cluster generation should match expected")
					g.Expect(finalCluster.Status).NotTo(BeNil(), "cluster status should be present")

					found := false
					for _, cond := range finalCluster.Status.Conditions {
						if cond.Type == client.ConditionTypeReconciled &&
							cond.Status == client.ResourceConditionStatusTrue {
							found = true
							g.Expect(cond.ObservedGeneration).
								To(Equal(expectedGen), "Reconciled condition observed_generation should match expected")
						}
					}
					g.Expect(found).To(BeTrue(), "cluster should have Reconciled=True")
				}, h.Entities.MustGet("Cluster").Timeouts.Reconciled, h.Cfg.Polling.Interval).Should(Succeed())
			},
		)

	},
)
