package adapter

import (
	"context"
	"net/http"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

const (
	conformanceNegativeAdapter = "conformance-test"
	conditionTypeReady         = "Ready"
)

// These tests verify deployment-level conformance: the deployed v1 stack
// correctly rejects pre-migration adapter behavior. While the assertions are
// HTTP status codes, the intent is end-to-end migration validation, not
// unit-level handler testing.
var _ = ginkgo.Describe("[Suite: adapter][negative] Legacy adapter behavior rejected by v1 API contract",
	ginkgo.Label(labels.Tier0, labels.Negative),
	func() {
		ginkgo.It("should reject POST to the statuses endpoint with 405",
			func(ctx context.Context) {
				h := helper.New()

				ginkgo.By("creating a cluster")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id

				ginkgo.DeferCleanup(func(ctx context.Context) {
					_ = h.CleanupTestCluster(ctx, clusterID)
				})

				ginkgo.By("sending a POST status report (old v0.2.0 method)")
				body := openapi.AdapterStatusCreateRequest{
					Adapter:            conformanceNegativeAdapter,
					ObservedGeneration: cluster.Generation,
					ObservedTime:       time.Now(),
					Conditions: []openapi.ConditionRequest{
						{Type: client.ConditionTypeApplied, Status: openapi.AdapterConditionStatusTrue},
						{Type: client.ConditionTypeAvailable, Status: openapi.AdapterConditionStatusTrue},
						{Type: client.ConditionTypeHealth, Status: openapi.AdapterConditionStatusTrue},
					},
				}
				resp, err := h.Client.PostClusterStatuses(ctx, clusterID, body)
				defer func() {
					if resp != nil {
						_ = resp.Body.Close()
					}
				}()
				Expect(err).NotTo(HaveOccurred())

				Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed),
					"POST to statuses should return 405; v1.0.0 only accepts PUT")
			})

		ginkgo.It("should reject PUT with missing mandatory conditions with 400",
			func(ctx context.Context) {
				h := helper.New()

				ginkgo.By("creating a cluster")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id

				ginkgo.DeferCleanup(func(ctx context.Context) {
					_ = h.CleanupTestCluster(ctx, clusterID)
				})

				ginkgo.By("sending a PUT with only a Ready condition (missing Applied, Available, Health)")
				resp, err := h.Client.PutClusterStatuses(ctx, clusterID, openapi.AdapterStatusCreateRequest{
					Adapter:            conformanceNegativeAdapter,
					ObservedGeneration: cluster.Generation,
					ObservedTime:       time.Now(),
					Conditions: []openapi.ConditionRequest{
						{Type: conditionTypeReady, Status: openapi.AdapterConditionStatusTrue},
					},
				})
				defer func() {
					if resp != nil {
						_ = resp.Body.Close()
					}
				}()
				Expect(err).NotTo(HaveOccurred())

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
					"missing mandatory conditions (Available, Applied, Health) should return 400")
			})

		ginkgo.It("should not have a resource-level Ready condition on a reconciled cluster",
			func(ctx context.Context) {
				h := helper.New()

				ginkgo.By("creating a cluster")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id

				ginkgo.DeferCleanup(func(ctx context.Context) {
					_ = h.CleanupTestCluster(ctx, clusterID)
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
