package cluster

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][delete] External K8s Resource Deletion",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var clusterID string

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

			ginkgo.By("confirming managed K8s namespaces exist")
			namespaces, err := h.K8sClient.FindNamespacesByPrefix(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(namespaces).NotTo(BeEmpty(), "managed namespaces should exist after Reconciled")
		})

		ginkgo.It(
			"should treat externally-deleted K8s resources as finalized and complete hard-delete",
			func(ctx context.Context) {
				ginkgo.By("externally deleting all managed K8s namespaces (bypass the API)")
				namespaces, err := h.K8sClient.FindNamespacesByPrefix(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())

				remote := helper.RemoteNamespace(clusterID)
				hasRemote := slices.Contains(namespaces, remote)
				for _, ns := range namespaces {
					if ns == remote {
						continue
					}
					deleteErr := h.K8sClient.DeleteNamespaceAndWait(ctx, ns)
					Expect(deleteErr).NotTo(HaveOccurred(), "failed to delete namespace %s", ns)
				}

				ginkgo.By("verifying all local namespaces are gone")
				Eventually(func() ([]string, error) {
					remaining, pollErr := h.PollNamespacesByPrefix(ctx, clusterID)()
					return slices.DeleteFunc(remaining, func(ns string) bool { return ns == remote }), pollErr
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(BeEmpty())

				// The applier re-applies every desire on each pass, so an externally deleted
				// remote object is recreated rather than staying gone. A new UID proves the
				// recreation without having to observe the short NotFound window; while the old
				// namespace is still Terminating, the GET returns its old UID.
				if hasRemote {
					original, fetchErr := h.K8sClient.FetchNamespace(ctx, remote)
					Expect(fetchErr).NotTo(HaveOccurred())
					ginkgo.GinkgoWriter.Printf("Remote namespace %s UID before delete: %s\n", remote, original.UID)
					deleteErr := h.K8sClient.CoreV1().Namespaces().Delete(ctx, remote, metav1.DeleteOptions{})
					Expect(deleteErr).NotTo(HaveOccurred(), "failed to delete namespace %s", remote)

					ginkgo.By("verifying the applier recreates the remote namespace")
					var recreated *corev1.Namespace
					Eventually(func(g Gomega) {
						ns, fetchErr := h.K8sClient.FetchNamespace(ctx, remote)
						g.Expect(fetchErr).NotTo(HaveOccurred())
						g.Expect(ns.UID).NotTo(Equal(original.UID), "namespace %s has not been recreated yet", remote)
						g.Expect(kubernetes.HasNamespacePhase(ns, corev1.NamespaceActive)).To(BeTrue(),
							"recreated namespace %s phase is %s, want Active", remote, ns.Status.Phase)
						recreated = ns
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())
					ginkgo.GinkgoWriter.Printf("Remote namespace %s UID after recreate: %s\n", remote, recreated.UID)
				}

				ginkgo.By("sending DELETE through the API")
				deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())
				Expect(deletedCluster.DeletedTime).NotTo(BeNil())

				ginkgo.By("verifying adapters report Finalized=True with Health=True")
				Eventually(func(g Gomega) {
					var httpErr *client.HTTPError
					statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
					if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
						return
					}
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(statuses).To(helper.HaveAllAdaptersWithCondition(
						h.Entities.MustGet("Cluster").RequiredAdapters, client.ConditionTypeFinalized, client.AdapterConditionStatusTrue))
					g.Expect(statuses).To(helper.HaveAllAdaptersWithCondition(
						h.Entities.MustGet("Cluster").RequiredAdapters, client.ConditionTypeHealth, client.AdapterConditionStatusTrue))
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("verifying cluster is hard-deleted")
				Eventually(
					h.PollHTTPStatus(ctx, h.Entities.MustGet("Cluster"), "", clusterID),
					h.Cfg.Timeouts.Adapter.Processing,
					h.Cfg.Polling.Interval,
				).
					Should(Equal(http.StatusNotFound))
			},
		)

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil || clusterID == "" {
				return
			}
			ginkgo.By("cleaning up cluster " + clusterID)
			if cluster, err := h.Client.GetCluster(ctx, clusterID); err == nil && cluster.DeletedTime == nil {
				if _, err := h.Client.DeleteCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: API delete failed for cluster %s: %v\n", clusterID, err)
				}
			}
			if err := h.CleanupTestResource(ctx, h.Entities.MustGet("Cluster"), "", clusterID); err != nil {
				ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
			}
		})
	},
)
