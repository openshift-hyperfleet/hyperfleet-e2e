package adapter

import (
	"context"
	"slices"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // Gomega matchers are designed to be used with dot import
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

// applierSelector selects the applier Deployment and its pods in the run namespace.
const applierSelector = "app.kubernetes.io/instance=hyperfleet-applier"

// With the applier down nothing serves the desires the remote adapter writes, so its read
// desires stay unsynced: the adapter runs healthy but cannot report its objects applied,
// and the cluster must not become Reconciled. Once the applier is back, the cluster must
// converge without any intervention.
var _ = ginkgo.Describe("[Suite: adapter][remote-transport][negative] Adapter reports undelivered desires while the applier is unavailable",
	ginkgo.Serial, // Serial: scales down the shared applier, blocking remote delivery for every concurrent spec
	ginkgo.Label(labels.Tier1, labels.Negative, labels.Disruptive),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func() {
			h = helper.New()
		})

		ginkgo.It("should keep the cluster unreconciled while the applier is down and converge once it is restored",
			func(ctx context.Context) {
				adapter := h.Cfg.Desire.Adapter
				ns := h.Cfg.Namespace
				// The applier runs in the run namespace; empty would mean "all namespaces".
				Expect(ns).NotTo(BeEmpty(), "NAMESPACE must name the run namespace")

				// Restore whatever replica count the environment runs, not a guess.
				deployments, err := h.K8sClient.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{LabelSelector: applierSelector})
				Expect(err).NotTo(HaveOccurred(), "failed to list applier Deployments")
				Expect(deployments.Items).To(HaveLen(1), "expected one applier Deployment matching %s in %s", applierSelector, ns)
				applierReplicas := int32(1) // the Deployment default when spec.replicas is unset
				if r := deployments.Items[0].Spec.Replicas; r != nil {
					applierReplicas = *r
				}
				Expect(applierReplicas).To(BeNumerically(">", 0), "the applier must be running before the spec scales it down")

				// Registered before the scale-down, so a scale that fails halfway is still
				// undone. Runs last (LIFO). Hard assertion: the applier must be restored.
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("restoring the applier")
					Expect(h.ScaleDeploymentBySelector(ctx, ns, applierSelector, applierReplicas)).
						To(Succeed(), "cleanup must restore the applier")
				})

				ginkgo.By("scaling the applier down to 0")
				Expect(h.ScaleDeploymentBySelector(ctx, ns, applierSelector, 0)).To(Succeed(), "failed to scale down the applier")

				// A pod still terminating does not count as available, but it could still
				// apply the new cluster's desires.
				ginkgo.By("waiting until no applier pod remains")
				Eventually(func(g Gomega) {
					pods, err := h.K8sClient.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: applierSelector})
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(pods.Items).To(BeEmpty(), "applier pods still present in namespace %s", ns)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				// Created after the scale-down: while Available is Unknown the API keeps only
				// an adapter's first report, so a cluster created earlier would keep showing
				// its healthy pre-outage status.
				ginkgo.By("creating a cluster while the applier is unavailable")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
				clusterID := *cluster.Id
				remote := helper.RemoteNamespace(clusterID)
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)

				// Registered second, so it runs first (LIFO), while the cleanup above has not
				// yet restored the applier. Without the applier the delete is never confirmed
				// and the cluster is never hard-deleted, so restore it before the cleanup.
				// Soft failure: warn-log only.
				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := h.ScaleDeploymentBySelector(ctx, ns, applierSelector, applierReplicas); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to restore the applier before cluster cleanup: %v\n", err)
					}
					if err := h.CleanupTestResource(ctx, h.Entities.MustGet("Cluster"), "", clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
					}
				})

				ginkgo.By("verifying the remote adapter runs healthy but does not report its objects applied")
				Eventually(func(g Gomega) {
					statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred())
					i := slices.IndexFunc(statuses.Items, func(s client.AdapterStatus) bool { return s.Adapter == adapter })
					g.Expect(i).NotTo(Equal(-1), "adapter %s should be present in statuses", adapter)
					conditions := statuses.Items[i].Conditions
					g.Expect(h.HasAdapterCondition(conditions, client.ConditionTypeHealth, client.AdapterConditionStatusTrue)).
						To(BeTrue(), "adapter %s should report Health=True: it ran, only delivery is missing", adapter)
					g.Expect(h.HasAdapterCondition(conditions, client.ConditionTypeApplied, client.AdapterConditionStatusTrue)).
						To(BeFalse(), "adapter %s must not report Applied=True with no applier", adapter)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("verifying the cluster stays unreconciled and nothing reaches the target")
				Consistently(func(g Gomega) {
					cl, err := h.Poll(ctx, h.Entities.MustGet("Cluster"), "", clusterID)()
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(cl).To(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusFalse),
						"cluster Reconciled should remain False while the applier is unavailable")

					_, err = h.K8sClient.CoreV1().Namespaces().Get(ctx, remote, metav1.GetOptions{})
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue(),
						"Namespace %s should not exist while the applier is unavailable, got err=%v", remote, err)
				}, 3*h.Cfg.Polling.Interval, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("restoring the applier")
				Expect(h.ScaleDeploymentBySelector(ctx, ns, applierSelector, applierReplicas)).
					To(Succeed(), "failed to restore the applier")

				ginkgo.By("verifying the cluster converges once the applier is back")
				Eventually(h.Poll(ctx, h.Entities.MustGet("Cluster"), "", clusterID), h.Entities.MustGet("Cluster").Timeouts.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				ginkgo.By("verifying the remote adapter reports Applied, Available and Health True at the cluster generation")
				Eventually(h.PollAdapterStatuses(ctx, h.Entities.MustGet("Cluster"), "", clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
					Should(helper.HaveAllAdaptersAtGeneration([]string{adapter}, cluster.Generation))

				_, err = h.K8sClient.FetchNamespace(ctx, remote)
				Expect(err).NotTo(HaveOccurred(), "Namespace %s should exist after the applier is restored", remote)
			})
	},
)
