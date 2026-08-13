package auth

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: auth][renewal] JWT Token Renewal",
	ginkgo.Serial, // Serial: scales down sentinel replicas, blocking all concurrent specs
	ginkgo.Label(labels.Tier1, labels.Auth, labels.Slow),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(_ context.Context) {
			h = helper.New()
		})

		// sentinelSelector matches the Sentinel deployment(s), mirroring e2e/cluster/delete_edge_cases.go.
		const sentinelSelector = "app.kubernetes.io/instance in (sentinel-clusters,clusters)"

		// Restarting Sentinel between cycles forces a fresh token, so renewal is actually proven
		// rather than assumed from tokenCacheTtl coincidentally lapsing during the test.
		ginkgo.It("Sentinel continues publishing events across token refresh boundary",
			ginkgo.Label(labels.Disruptive),
			func(ctx context.Context) {
				ginkgo.By("creating a cluster and waiting for initial Reconciled state")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "cluster creation should succeed")
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id

				h.DeferClusterCleanup(clusterID)

				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				ginkgo.By("verifying cluster is at generation 1")
				clusterBefore, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())
				Expect(clusterBefore.Generation).To(Equal(int32(1)))

				ginkgo.By("restarting Sentinel to force a fresh token acquisition")
				Expect(h.ScaleDeploymentBySelector(ctx, h.Cfg.Namespace, sentinelSelector, 0)).To(Succeed())
				Expect(h.ScaleDeploymentBySelector(ctx, h.Cfg.Namespace, sentinelSelector, 1)).To(Succeed())

				ginkgo.By("patching cluster to trigger new reconciliation (forces Sentinel to re-publish)")
				patched, err := h.Client.PatchClusterFromPayload(ctx, clusterID, h.TestDataPath("payloads/clusters/cluster-patch.json"))
				Expect(err).NotTo(HaveOccurred(), "PATCH should succeed")
				expectedGen := patched.Generation

				ginkgo.By("waiting for all adapters to reconcile at new generation")
				Eventually(h.PollClusterAdapterStatuses(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
					Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.Cluster, expectedGen))

				ginkgo.By("verifying cluster reaches Reconciled at new generation")
				Eventually(func(g Gomega) {
					c, err := h.Client.GetCluster(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(c.Generation).To(Equal(expectedGen))

					found := false
					for _, cond := range c.Status.Conditions {
						if cond.Type == client.ConditionTypeReconciled && cond.Status == client.ResourceConditionStatusTrue {
							found = true
							g.Expect(cond.ObservedGeneration).To(Equal(expectedGen))
						}
					}
					g.Expect(found).To(BeTrue(), "cluster should have Reconciled=True at new generation")
				}, h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).Should(Succeed())
			})

		// Unlike Sentinel, these adapters' deployment names aren't resolvable here, so this still
		// relies on tokenCacheTtl/rotation lapsing naturally rather than a forced restart.
		ginkgo.It("Adapter continues reporting status across token refresh boundary",
			func(ctx context.Context) {
				ginkgo.By("creating a cluster and waiting for initial Reconciled state")
				clusterID, err := h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "cluster creation should succeed")

				h.DeferClusterCleanup(clusterID)

				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				ginkgo.By("verifying at least one adapter has reported")
				statusesBefore, err := h.Client.GetClusterStatuses(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())
				Expect(statusesBefore.Items).NotTo(BeEmpty())

				ginkgo.By("patching cluster to trigger adapter re-reconciliation")
				patched, err := h.Client.PatchClusterFromPayload(ctx, clusterID, h.TestDataPath("payloads/clusters/cluster-patch.json"))
				Expect(err).NotTo(HaveOccurred())
				expectedGen := patched.Generation

				ginkgo.By("waiting for adapters to report at new generation")
				Eventually(h.PollClusterAdapterStatuses(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
					Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.Cluster, expectedGen))

				ginkgo.By("verifying adapters successfully reported (proving token renewal worked)")
				statusesAfter, err := h.Client.GetClusterStatuses(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())
				for _, s := range statusesAfter.Items {
					Expect(s.ObservedGeneration).To(Equal(expectedGen),
						"adapter %s should have observed the new generation (proving JWT-authenticated PUT succeeded after potential token refresh)",
						s.Adapter)
				}
			})
	},
)
