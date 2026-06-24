package adapter

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

const conditionTypeReady = "Ready"

var _ = ginkgo.Describe("[Suite: adapter][negative] Legacy adapter behavior rejected by v1 API contract",
	ginkgo.Label(labels.Tier0, labels.Negative),
	func() {
		ginkgo.It("should not have a resource-level Ready condition on a reconciled cluster",
			func(ctx context.Context) {
				h := helper.New()

				ginkgo.By("creating a cluster")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id

				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
					}
				})

				ginkgo.By("waiting for Reconciled=True")
				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

				ginkgo.By("verifying resource-level Ready condition does not exist")
				reconciledCluster, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())
				Expect(reconciledCluster.Status).NotTo(BeNil())

				for _, c := range reconciledCluster.Status.Conditions {
					Expect(c.Type).NotTo(Equal(conditionTypeReady),
						"v1.0.0 removed the Ready condition; no Ready condition should exist regardless of status")
				}

				ginkgo.By("confirming Reconciled and LastKnownReconciled are the correct v1.0.0 conditions")
				Expect(h.HasResourceCondition(reconciledCluster.Status.Conditions,
					client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)).To(BeTrue())
				Expect(h.HasResourceCondition(reconciledCluster.Status.Conditions,
					client.ConditionTypeLastKnownReconciled, openapi.ResourceConditionStatusTrue)).To(BeTrue())
			})
	},
)
