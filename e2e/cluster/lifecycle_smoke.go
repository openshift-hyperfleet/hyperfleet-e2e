package cluster

import (
	"context"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][baseline] Cluster Full Lifecycle Smoke",
	ginkgo.Label(labels.Tier0),
	func() {
		ginkgo.It("should complete create → Reconciled → delete → hard-delete → K8s cleanup in a single pass",
			func(ctx context.Context) {
				h := helper.New()

				ginkgo.By("creating a cluster")
				cluster, err := h.Client.CreateClusterFromPayload(
					ctx,
					h.TestDataPath("payloads/clusters/cluster-request.json"),
				)
				Expect(err).NotTo(HaveOccurred(), "create should succeed")
				Expect(cluster).NotTo(BeNil(), "create should return a cluster object")
				Expect(cluster.ID).NotTo(BeNil(), "cluster ID should be assigned")
				clusterID := *cluster.ID

				ginkgo.DeferCleanup(func(ctx context.Context) {
					if cleanupErr := h.CleanupTestResource(ctx, h.Entities.MustGet("Cluster"), "", clusterID); cleanupErr != nil {
						ginkgo.GinkgoWriter.Printf(
							"Warning: cleanup failed for cluster %s: %v\n",
							clusterID,
							cleanupErr,
						)
					}
				})

				ginkgo.By("waiting for Reconciled=True")
				Eventually(
					h.Poll(ctx, h.Entities.MustGet("Cluster"), "", clusterID),
					h.Entities.MustGet("Cluster").Timeouts.Reconciled,
					h.Cfg.Polling.Interval,
				).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				ginkgo.By("confirming reconciled state via GET")
				reconciledCluster, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())
				Expect(reconciledCluster).NotTo(BeNil(), "GET should return cluster object")
				Expect(reconciledCluster.Status.Conditions).NotTo(BeEmpty(), "GET should return status conditions")
				Expect(h.HasResourceCondition(reconciledCluster.Status.Conditions,
					client.ConditionTypeReconciled, client.ResourceConditionStatusTrue)).To(BeTrue())

				ginkgo.By("soft-deleting the cluster")
				deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "DELETE should return 202")
				Expect(deletedCluster).NotTo(BeNil(), "DELETE should return cluster object")
				Expect(deletedCluster.DeletedTime).NotTo(BeNil())

				ginkgo.By("waiting for hard-delete (404)")
				Eventually(
					h.PollHTTPStatus(ctx, h.Entities.MustGet("Cluster"), "", clusterID),
					h.Cfg.Timeouts.Adapter.Processing,
					h.Cfg.Polling.Interval,
				).
					Should(Equal(http.StatusNotFound))

				ginkgo.By("confirming K8s namespace is cleaned up")
				Eventually(
					h.PollNamespacesByPrefix(ctx, clusterID),
					h.Cfg.Timeouts.Adapter.Processing,
					h.Cfg.Polling.Interval,
				).
					Should(BeEmpty())
			})
	},
)
