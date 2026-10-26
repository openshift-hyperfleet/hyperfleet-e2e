package adapter

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // Gomega matchers are designed to be used with dot import
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

const (
	// remoteConfigMapName is the ConfigMap the remote adapter task writes into helper.RemoteNamespace.
	remoteConfigMapName = "cluster-config"

	// remoteHoldFinalizer holds the remote ConfigMap in deletion. The applier's server-side
	// apply does not own metadata.finalizers, so it leaves this finalizer alone.
	remoteHoldFinalizer = "e2e.hyperfleet.io/hold"
)

// Strategic merge patches that add and remove remoteHoldFinalizer without touching
// other finalizers. Removing a finalizer that is not there is a no-op.
var (
	addHoldFinalizerPatch    = fmt.Appendf(nil, `{"metadata":{"finalizers":[%q]}}`, remoteHoldFinalizer)
	removeHoldFinalizerPatch = fmt.Appendf(
		nil,
		`{"metadata":{"$deleteFromPrimitiveList/finalizers":[%q]}}`,
		remoteHoldFinalizer,
	)
)

// The remote adapter (h.Cfg.Desire.Adapter) delivers through the desire store and the
// applier instead of writing to Kubernetes itself. The tier suites check its status
// like any other required adapter; this suite checks the objects the applier wrote for
// it, that an unchanged desire is not rewritten, and that a delete waits for the remote
// objects to be gone.
var _ = ginkgo.Describe("[Suite: adapter][remote-transport] Adapter Framework - Remote Transport through the Applier",
	ginkgo.Ordered,
	ginkgo.Label(labels.Tier0),
	func() {
		var (
			h          *helper.Helper
			adapter    string // h.Cfg.Desire.Adapter
			clusterID  string // the cluster the later specs use: the last one created
			createdIDs []string
			generation int32
		)

		ginkgo.BeforeAll(func() {
			h = helper.New()
			adapter = h.Cfg.Desire.Adapter
			// The tier suites check only the adapters in adapters.cluster. Requiring the
			// remote adapter there keeps them from silently dropping the remote path.
			Expect(h.Entities.MustGet("Cluster").RequiredAdapters).To(ContainElement(adapter),
				"adapters.cluster must include the remote adapter %q (desire.adapter)", adapter)

			// Registered here rather than in the create spec: in an Ordered container a
			// DeferCleanup made inside an It runs when that It ends, which would delete the
			// cluster before the delete spec. From BeforeAll it runs after the last spec,
			// and finds the cluster already gone (404) when the delete spec passed.
			// It cleans up every cluster the create spec made: with --flake-attempts a
			// retried create spec runs again without BeforeAll, and its first cluster
			// would otherwise leak.
			ginkgo.DeferCleanup(func(ctx context.Context) {
				for _, id := range createdIDs {
					if err := h.CleanupTestResource(ctx, h.Entities.MustGet("Cluster"), "", id); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", id, err)
					}
				}
			})
		})

		// Fails when delivery through the applier, or the status it reports back, breaks.
		// The adapter has no Kubernetes access of its own, so the objects on the cluster were
		// created by the applier, and its Applied/Available/Health conditions are built from
		// the live objects the applier reports.
		ginkgo.It("should create a cluster's resources through the applier and report their status",
			func(ctx context.Context) {
				ginkgo.By("creating a cluster")
				cluster, err := h.Client.CreateClusterFromPayload(
					ctx,
					h.TestDataPath("payloads/clusters/cluster-request.json"),
				)
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.ID).NotTo(BeNil(), "cluster ID should be generated")
				clusterID = *cluster.ID
				createdIDs = append(createdIDs, clusterID) // deleted by the cleanup registered in BeforeAll
				generation = cluster.Generation
				Expect(generation).To(Equal(int32(1)), "a new cluster should have generation 1")
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)

				ginkgo.By("waiting for the cluster to become Reconciled")
				Eventually(
					h.Poll(ctx, h.Entities.MustGet("Cluster"), "", clusterID),
					h.Entities.MustGet("Cluster").Timeouts.Reconciled,
					h.Cfg.Polling.Interval,
				).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				ginkgo.By(
					"verifying the remote adapter reports Applied, Available and Health True at the cluster generation",
				)
				Eventually(
					h.PollAdapterStatuses(ctx, h.Entities.MustGet("Cluster"), "", clusterID),
					h.Cfg.Timeouts.Adapter.Processing,
					h.Cfg.Polling.Interval,
				).
					Should(helper.HaveAllAdaptersAtGeneration([]string{adapter}, generation))

				ginkgo.By("verifying the applier created both objects")
				wantGeneration := strconv.Itoa(int(generation))
				remote := helper.RemoteNamespace(clusterID)
				namespace, err := h.GetNamespace(ctx, remote)
				Expect(err).NotTo(HaveOccurred())
				Expect(namespace.Labels).To(HaveKeyWithValue(client.KeyClusterID, clusterID))
				Expect(namespace.Annotations).To(HaveKeyWithValue(client.KeyGeneration, wantGeneration))
				configMap, err := h.GetConfigMap(ctx, remote, remoteConfigMapName)
				Expect(err).NotTo(HaveOccurred())
				Expect(configMap.Annotations).To(HaveKeyWithValue(client.KeyGeneration, wantGeneration))
				Expect(configMap.Data).To(HaveKeyWithValue("cluster_id", clusterID))

				// The applier re-applies every desire on each pass. An unchanged server-side
				// apply is a no-op, so the ConfigMap must not be rewritten while the cluster
				// generation stays the same.
				ginkgo.By("verifying the applier does not rewrite the ConfigMap while the generation is unchanged")
				resourceVersion := configMap.ResourceVersion
				Consistently(func(g Gomega) {
					current, err := h.K8sClient.FetchConfigMap(ctx, remote, remoteConfigMapName)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(current.ResourceVersion).To(Equal(resourceVersion),
						"ConfigMap %s/%s was rewritten at an unchanged generation", remote, remoteConfigMapName)
				}, 3*h.Cfg.Polling.Interval, h.Cfg.Polling.Interval).Should(Succeed())
			})

		// The adapter must see the applier confirm the delete before it reports Finalized,
		// and the API hard-deletes the cluster as soon as it does. A finalizer on the remote
		// ConfigMap holds the delete open, so the spec can observe the wait; once it is
		// released, the check after the 404 is one-shot: a Namespace still there means the
		// adapter finalized early, a product finding, not a reason to retry.
		ginkgo.It("should hold the cluster delete until the applier confirms the remote objects are gone",
			func(ctx context.Context) {
				remote := helper.RemoteNamespace(clusterID)
				configMaps := h.K8sClient.CoreV1().ConfigMaps(remote)

				ginkgo.By("holding the remote ConfigMap with a finalizer")
				// A finalizer left behind would keep the Namespace in deletion for good. Registered
				// before the patch: a patch can be applied on the server and still return an error
				// (e.g. a timeout), and removing a finalizer that is not there is a no-op.
				ginkgo.DeferCleanup(func(ctx context.Context) {
					_, err := configMaps.Patch(ctx, remoteConfigMapName, types.StrategicMergePatchType,
						removeHoldFinalizerPatch, metav1.PatchOptions{})
					if err != nil && !apierrors.IsNotFound(err) {
						ginkgo.GinkgoWriter.Printf("Warning: failed to remove finalizer %s from ConfigMap %s/%s: %v\n",
							remoteHoldFinalizer, remote, remoteConfigMapName, err)
					}
				})
				_, err := configMaps.Patch(ctx, remoteConfigMapName, types.StrategicMergePatchType,
					addHoldFinalizerPatch, metav1.PatchOptions{})
				Expect(err).NotTo(HaveOccurred(), "failed to add finalizer %s to ConfigMap %s/%s",
					remoteHoldFinalizer, remote, remoteConfigMapName)

				ginkgo.By("soft-deleting the cluster")
				deleted, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed")
				Expect(deleted.DeletedTime).NotTo(BeNil(), "soft-deleted cluster should have deleted_time set")

				ginkgo.By("waiting for the applier to delete the held ConfigMap")
				Eventually(func(g Gomega) {
					configMap, fetchErr := h.K8sClient.FetchConfigMap(ctx, remote, remoteConfigMapName)
					g.Expect(fetchErr).NotTo(HaveOccurred())
					g.Expect(configMap.DeletionTimestamp).NotTo(BeNil(),
						"ConfigMap %s/%s should be marked for deletion", remote, remoteConfigMapName)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				// The Finalized reason and observed generation are not checked here: the
				// adapter's reports during the hold carry Available=Unknown, which the API
				// discards, so the API still shows the pre-delete status.
				ginkgo.By("verifying the cluster is not finalized while the ConfigMap is held")
				Consistently(func(g Gomega) {
					status, pollErr := h.PollHTTPStatus(ctx, h.Entities.MustGet("Cluster"), "", clusterID)()
					g.Expect(pollErr).NotTo(HaveOccurred())
					g.Expect(status).To(Equal(http.StatusOK), "cluster %s should not be hard-deleted", clusterID)

					namespace, fetchErr := h.K8sClient.FetchNamespace(ctx, remote)
					g.Expect(fetchErr).NotTo(HaveOccurred())
					g.Expect(namespace.DeletionTimestamp).To(BeNil(),
						"Namespace %s should not be deleted before its ConfigMap is confirmed deleted", remote)

					statuses, statusesErr := h.Client.GetClusterStatuses(ctx, clusterID)
					g.Expect(statusesErr).NotTo(HaveOccurred())
					i := slices.IndexFunc(
						statuses.Items,
						func(s client.AdapterStatus) bool { return s.Adapter == adapter },
					)
					g.Expect(i).NotTo(Equal(-1), "adapter %s should be present in statuses", adapter)
					g.Expect(h.HasAdapterCondition(statuses.Items[i].Conditions,
						client.ConditionTypeFinalized, client.AdapterConditionStatusTrue)).To(BeFalse(),
						"adapter %s should not report Finalized=True while the ConfigMap is held", adapter)
				}, 3*h.Cfg.Polling.Interval, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("releasing the finalizer")
				_, err = configMaps.Patch(ctx, remoteConfigMapName, types.StrategicMergePatchType,
					removeHoldFinalizerPatch, metav1.PatchOptions{})
				Expect(err).NotTo(HaveOccurred(), "failed to remove finalizer %s from ConfigMap %s/%s",
					remoteHoldFinalizer, remote, remoteConfigMapName)

				ginkgo.By("waiting for the remote adapter to finalize and the cluster to be hard-deleted")
				// Hard-delete runs inside the status POST that computes Reconciled=True from
				// the required adapters' Finalized=True, so the 404 proves the adapter
				// finalized; Finalized=True itself may never be observable.
				Eventually(
					h.PollHTTPStatus(ctx, h.Entities.MustGet("Cluster"), "", clusterID),
					h.Entities.MustGet("Cluster").Timeouts.Deleted,
					h.Cfg.Polling.Interval,
				).
					Should(Equal(http.StatusNotFound))

				ginkgo.By("checking once, without retry, that the remote Namespace is gone")
				if namespace, err := h.K8sClient.CoreV1().Namespaces().Get(ctx, remote, metav1.GetOptions{}); err == nil {
					ginkgo.Fail(fmt.Sprintf("Namespace %s still exists (phase %s) after the cluster was hard-deleted",
						remote, namespace.Status.Phase))
				} else {
					Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Get Namespace %s: want NotFound, got %v", remote, err)
				}
			})
	},
)
