package adapter

import (
	"context"
	"os"
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
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/util"
)

const (
	// reapplyAdapterName is the test adapter this spec deploys. Its config
	// (testdata/adapter-configs/cl-desire-reapply) is the environment's remote
	// adapter with a delete expression the spec turns on and off through a
	// cluster label: the API never clears deleted_time, so a soft-deleted
	// cluster can never be wanted back, and the environment's remote adapter
	// deletes only on is_deleting.
	reapplyAdapterName = "cl-desire-reapply"

	// reapplyConfigMapName is the ConfigMap the adapter manages per cluster.
	reapplyConfigMapName = "reapply-config"

	// deleteConfigMapLabel is the cluster label that turns the ConfigMap's
	// delete expression on ("true") and off (any other value). The adapter
	// keeps the cluster's other labels intact by replacing the whole set.
	deleteConfigMapLabel = "e2e.hyperfleet.io/delete-configmap"

	// desireStoreClientPodLabel marks the adapter's pods as desire store
	// clients for the store's NetworkPolicy, which this spec patches.
	desireStoreClientPodLabel = "e2e.hyperfleet.io/desire-store-client"
)

// reapplyNamespace returns the namespace the adapter manages for a cluster.
func reapplyNamespace(clusterID string) string {
	return clusterID + "-reapply"
}

// The delete-then-reapply path: a remote object is deleted, held by a
// finalizer, and wanted back before the delete is confirmed. The store must
// refuse the new apply while the DeleteDesire is unconfirmed — the adapter
// reports Available=Unknown, the API discards those reports, and the adapter
// never reaches the new generation — and the object must be recreated with a
// new UID only after the applier confirms the delete.
var _ = ginkgo.Describe("[Suite: adapter][remote-transport] Adapter Framework - Delete then Reapply through the Applier",
	ginkgo.Serial,
	ginkgo.Label(labels.Tier1, labels.Adapter),
	func() {
		var (
			h                *helper.Helper
			adapterChartPath string
			apiChartPath     string
			baseDeployOpts   helper.AdapterDeploymentOptions
		)

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("Cloning the adapter Helm chart")
			var cleanupAdapterChart func() error
			var err error
			adapterChartPath, cleanupAdapterChart, err = h.CloneHelmChart(ctx, helper.HelmChartCloneOptions{
				Component: "adapter",
				RepoURL:   h.Cfg.AdapterDeployment.ChartRepo,
				Ref:       h.Cfg.AdapterDeployment.ChartRef,
				ChartPath: h.Cfg.AdapterDeployment.ChartPath,
				WorkDir:   helper.TestWorkDir,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to clone adapter Helm chart")
			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := cleanupAdapterChart(); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup adapter chart: %v\n", err)
				}
			})

			ginkgo.By("Cloning the API Helm chart")
			var cleanupAPIChart func() error
			apiChartPath, cleanupAPIChart, err = h.CloneHelmChart(ctx, helper.HelmChartCloneOptions{
				Component: "api",
				RepoURL:   h.Cfg.APIDeployment.ChartRepo,
				Ref:       h.Cfg.APIDeployment.ChartRef,
				ChartPath: h.Cfg.APIDeployment.ChartPath,
				WorkDir:   helper.TestWorkDir,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to clone API Helm chart")
			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := cleanupAPIChart(); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup API chart: %v\n", err)
				}
			})

			baseDeployOpts = helper.AdapterDeploymentOptions{
				Namespace: h.Cfg.Namespace,
				ChartPath: adapterChartPath,
				Entity:    h.Entities.MustGet("Cluster"),
			}
		})

		ginkgo.It("should keep a held remote object and not reapply it until the delete is confirmed",
			func(ctx context.Context) {
				releaseName := helper.GenerateAdapterReleaseName(h.Entities.MustGet("Cluster"), reapplyAdapterName)

				ginkgo.By("admitting the adapter to the desire store")
				// The store's Redis admits only the clients helmfile declares in
				// hyperfleet-infra so we must create a dedicated network policy.
				// It must exist before the adapter starts.
				// TODO(HYPERFLEET-1825): Delete network policy machinery once the desire store
				// runs on Postgres and is accessible via the gateway.
				restoreStorePolicy, err := h.AdmitDesireStoreClient(ctx,
					"redis-ingress-"+reapplyAdapterName,
					map[string]string{desireStoreClientPodLabel: "true"})
				Expect(err).NotTo(HaveOccurred(), "failed to admit the adapter to the desire store")
				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := restoreStorePolicy(ctx); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to restore the desire store NetworkPolicy: %v\n", err)
					}
				})

				ginkgo.By("deploying the reapply adapter")
				// ADAPTER_NAME feeds the envsubst expansion of the adapter's values.yaml.
				Expect(os.Setenv("ADAPTER_NAME", reapplyAdapterName)).To(Succeed(), "failed to set ADAPTER_NAME")
				ginkgo.DeferCleanup(func() { _ = os.Unsetenv("ADAPTER_NAME") })

				deployOpts := baseDeployOpts
				deployOpts.ReleaseName = releaseName
				deployOpts.AdapterName = reapplyAdapterName
				err = h.DeployAdapter(ctx, deployOpts)
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("uninstalling the reapply adapter")
					if err := h.UninstallAdapter(ctx, releaseName, h.Cfg.Namespace); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to uninstall adapter %s: %v\n", releaseName, err)
					}
					if h.Cfg.BrokerType == "googlepubsub" {
						if err := h.DeletePubSubResourcesForAdapter(ctx, deployOpts.Entity, reapplyAdapterName); err != nil {
							ginkgo.GinkgoWriter.Printf("Warning: failed to delete Pub/Sub resources for adapter %s: %v\n",
								reapplyAdapterName, err)
						}
					}
				})
				Expect(err).NotTo(HaveOccurred(), "failed to deploy adapter %s", reapplyAdapterName)

				ginkgo.By("adding the adapter to the API's required adapters")
				originalAdapters := h.GetAPIRequiredAdapters(deployOpts.Entity)
				updatedAdapters := append(append([]string{}, originalAdapters...), reapplyAdapterName)
				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := h.RestoreAPIRequiredAdaptersWithRetry(ctx, deployOpts.Entity, apiChartPath, h.Cfg.Namespace, originalAdapters, 3); err != nil {
						ginkgo.GinkgoWriter.Printf("CRITICAL: %v\n", err)
					}
				})
				Expect(h.UpgradeAPIRequiredAdapters(ctx, deployOpts.Entity, apiChartPath, h.Cfg.Namespace, updatedAdapters)).
					To(Succeed(), "failed to add %s to the required adapters", reapplyAdapterName)

				ginkgo.By("creating a cluster")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
				clusterID := *cluster.Id
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)
				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := h.CleanupTestResource(ctx, deployOpts.Entity, "", clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
					}
				})
				createGeneration := cluster.Generation
				Expect(createGeneration).To(Equal(int32(1)), "a new cluster should have generation 1")

				ginkgo.By("waiting for the cluster to become Reconciled")
				Eventually(h.Poll(ctx, h.Entities.MustGet("Cluster"), "", clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				ginkgo.By("verifying the adapter reports its objects applied at the initial generation")
				Eventually(h.PollAdapterStatuses(ctx, h.Entities.MustGet("Cluster"), "", clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
					Should(helper.HaveAllAdaptersAtGeneration([]string{reapplyAdapterName}, createGeneration))

				ginkgo.By("verifying the applier created the adapter's objects")
				remote := reapplyNamespace(clusterID)
				wantGeneration := strconv.Itoa(int(createGeneration))
				namespace, err := h.GetNamespace(ctx, remote)
				Expect(err).NotTo(HaveOccurred(), "Namespace %s should exist", remote)
				Expect(namespace.Labels).To(HaveKeyWithValue(client.KeyClusterID, clusterID))
				Expect(namespace.Annotations).To(HaveKeyWithValue(client.KeyGeneration, wantGeneration))
				configMap, err := h.GetConfigMap(ctx, remote, reapplyConfigMapName)
				Expect(err).NotTo(HaveOccurred(), "ConfigMap %s/%s should exist", remote, reapplyConfigMapName)
				Expect(configMap.Annotations).To(HaveKeyWithValue(client.KeyGeneration, wantGeneration))
				Expect(configMap.Data).To(HaveKeyWithValue("cluster_id", clusterID))
				heldUID := configMap.UID

				configMaps := h.K8sClient.CoreV1().ConfigMaps(remote)

				ginkgo.By("holding the adapter's ConfigMap with a finalizer")
				// A finalizer left behind would keep the Namespace in deletion for
				// good. Registered before the patch: a patch can be applied on the
				// server and still return an error, and removing a finalizer that is
				// not there is a no-op. The patches are shared with the other
				// remote-transport spec.
				ginkgo.DeferCleanup(func(ctx context.Context) {
					_, err := configMaps.Patch(ctx, reapplyConfigMapName, types.StrategicMergePatchType,
						removeHoldFinalizerPatch, metav1.PatchOptions{})
					if err != nil && !apierrors.IsNotFound(err) {
						ginkgo.GinkgoWriter.Printf("Warning: failed to remove finalizer %s from ConfigMap %s/%s: %v\n",
							remoteHoldFinalizer, remote, reapplyConfigMapName, err)
					}
				})
				_, err = configMaps.Patch(ctx, reapplyConfigMapName, types.StrategicMergePatchType,
					addHoldFinalizerPatch, metav1.PatchOptions{})
				Expect(err).NotTo(HaveOccurred(), "failed to add finalizer %s to ConfigMap %s/%s",
					remoteHoldFinalizer, remote, reapplyConfigMapName)

				ginkgo.By("turning the ConfigMap's delete expression on")
				current, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "failed to get cluster %s", clusterID)
				gateOn, err := h.Client.PatchCluster(ctx, clusterID, client.ResourcePatchRequest{
					Labels: util.MergeLabels(current.Labels, map[string]string{deleteConfigMapLabel: "true"}),
				})
				Expect(err).NotTo(HaveOccurred(), "failed to turn the delete expression on")
				gateOnGeneration := gateOn.Generation
				Expect(gateOnGeneration).To(Equal(createGeneration+1),
					"the label patch should bump the generation exactly once")

				ginkgo.By("waiting for the applier to delete the held ConfigMap")
				Eventually(func(g Gomega) {
					held, err := h.K8sClient.FetchConfigMap(ctx, remote, reapplyConfigMapName)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(held.DeletionTimestamp).NotTo(BeNil(),
						"ConfigMap %s/%s should be marked for deletion", remote, reapplyConfigMapName)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("turning the delete expression off again while the delete is held")
				current, err = h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "failed to get cluster %s", clusterID)
				gateOff, err := h.Client.PatchCluster(ctx, clusterID, client.ResourcePatchRequest{
					Labels: util.MergeLabels(current.Labels, map[string]string{deleteConfigMapLabel: "false"}),
				})
				Expect(err).NotTo(HaveOccurred(), "failed to turn the delete expression off")
				gateOffGeneration := gateOff.Generation
				Expect(gateOffGeneration).To(Equal(gateOnGeneration+1),
					"the label patch should bump the generation exactly once")

				ginkgo.By("waiting for the adapter to process the gate-off generation and report the rejected apply")
				// The task config reports Available=False, never Unknown, while
				// the mirrors cannot confirm the requested generation, so the
				// API accepts the report instead of discarding it. The stored
				// row reaching the gate-off generation with Available=False is
				// the direct proof that the adapter processed the event
				Eventually(func(g Gomega) {
					statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")
					i := slices.IndexFunc(statuses.Items, func(s client.AdapterStatus) bool {
						return s.Adapter == reapplyAdapterName
					})
					g.Expect(i).NotTo(Equal(-1), "adapter %s should be present in statuses", reapplyAdapterName)
					g.Expect(statuses.Items[i].ObservedGeneration).To(Equal(gateOffGeneration),
						"adapter %s should report at generation %d", reapplyAdapterName, gateOffGeneration)
					g.Expect(h.HasAdapterCondition(statuses.Items[i].Conditions,
						client.ConditionTypeAvailable, client.AdapterConditionStatusFalse)).To(BeTrue(),
						"adapter %s should report Available=False at generation %d while the delete is pending",
						reapplyAdapterName, gateOffGeneration)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("verifying the held ConfigMap keeps its UID and content, and the adapter does not reapply it")
				// The store refuses the new ApplyDesire while the DeleteDesire is
				// unconfirmed, so nothing is applied onto the terminating object
				// and the adapter keeps reporting Available=False at the
				// gate-off generation. Watched for half the adapter processing
				// budget (the stuck-state convention, like stuck_deletion), so
				// at least one full adapter cycle runs while the finalizer
				// still holds the evidence.
				Consistently(func(g Gomega) {
					held, err := h.K8sClient.FetchConfigMap(ctx, remote, reapplyConfigMapName)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(held.UID).To(Equal(heldUID),
						"ConfigMap %s/%s must keep its UID while the delete is pending", remote, reapplyConfigMapName)
					g.Expect(held.DeletionTimestamp).NotTo(BeNil(),
						"ConfigMap %s/%s must stay held in deletion", remote, reapplyConfigMapName)
					g.Expect(held.Annotations).To(HaveKeyWithValue(client.KeyGeneration, wantGeneration),
						"the held ConfigMap must keep the pre-delete content; generation %s means it was reapplied onto the terminating object", wantGeneration)

					statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred())
					i := slices.IndexFunc(statuses.Items, func(s client.AdapterStatus) bool {
						return s.Adapter == reapplyAdapterName
					})
					g.Expect(i).NotTo(Equal(-1), "adapter %s should be present in statuses", reapplyAdapterName)
					g.Expect(statuses.Items[i].ObservedGeneration).To(Equal(gateOffGeneration),
						"adapter %s's stored status must stay at generation %d while the delete is pending",
						reapplyAdapterName, gateOffGeneration)
					g.Expect(h.HasAdapterCondition(statuses.Items[i].Conditions,
						client.ConditionTypeAvailable, client.AdapterConditionStatusFalse)).To(BeTrue(),
						"adapter %s must keep reporting Available=False while the delete is pending", reapplyAdapterName)

					cl, err := h.Client.GetCluster(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(h.HasResourceCondition(cl.Status.Conditions,
						client.ConditionTypeReconciled, client.ResourceConditionStatusTrue)).To(BeFalse(),
						"cluster %s must not be Reconciled while the delete is pending", clusterID)
				}, h.Cfg.Timeouts.Adapter.Processing/2, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("releasing the finalizer")
				_, err = configMaps.Patch(ctx, reapplyConfigMapName, types.StrategicMergePatchType,
					removeHoldFinalizerPatch, metav1.PatchOptions{})
				Expect(err).NotTo(HaveOccurred(), "failed to remove finalizer %s from ConfigMap %s/%s",
					remoteHoldFinalizer, remote, reapplyConfigMapName)

				ginkgo.By("waiting for the adapter to reapply the ConfigMap and report at the new generation")
				Eventually(h.PollAdapterStatuses(ctx, h.Entities.MustGet("Cluster"), "", clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveAllAdaptersAtGeneration([]string{reapplyAdapterName}, gateOffGeneration))

				ginkgo.By("verifying the ConfigMap was recreated with a new UID and the new content")
				recreated, err := h.GetConfigMap(ctx, remote, reapplyConfigMapName)
				Expect(err).NotTo(HaveOccurred(), "ConfigMap %s/%s should exist again", remote, reapplyConfigMapName)
				Expect(recreated.UID).NotTo(Equal(heldUID),
					"ConfigMap %s/%s must be recreated with a new UID, not patched in place", remote, reapplyConfigMapName)
				Expect(recreated.DeletionTimestamp).To(BeNil(), "the recreated ConfigMap must not be terminating")
				Expect(recreated.Annotations).To(HaveKeyWithValue(client.KeyGeneration, strconv.Itoa(int(gateOffGeneration))),
					"the recreated ConfigMap should carry the new generation")
				Expect(recreated.Data).To(HaveKeyWithValue("cluster_id", clusterID))

				ginkgo.By("waiting for the cluster to become Reconciled")
				Eventually(h.Poll(ctx, h.Entities.MustGet("Cluster"), "", clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))
			})
	},
)
