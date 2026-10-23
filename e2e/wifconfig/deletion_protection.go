package wifconfig

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: wifconfig][delete-restrict] WIF Config Deletion Protection",
	ginkgo.Label(labels.Tier1, labels.Negative),
	func() {
		var h *helper.Helper
		var wifConfigID string
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating a WIF config")
			wifConfig, err := h.Client.CreateWifConfigFromPayload(ctx, h.TestDataPath("payloads/wifconfigs/wifconfig-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create wifconfig")
			Expect(wifConfig.Id).NotTo(BeNil(), "wifconfig ID should be generated")
			wifConfigID = *wifConfig.Id
			ginkgo.GinkgoWriter.Printf("Created wifconfig ID: %s, Name: %s\n", wifConfigID, wifConfig.Name)

			ginkgo.By("creating a cluster that references the WIF config")
			cluster, err := h.Client.CreateResourceFromPayloadWith(ctx, client.ClustersPath,
				h.TestDataPath("payloads/clusters/cluster-request.json"),
				map[string]any{"references": map[string]any{"wif_config": []map[string]any{{"id": wifConfigID, "kind": "WifConfig"}}}},
			)
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster referencing wifconfig")
			Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
			clusterID = *cluster.Id
			ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s (references wifconfig %s)\n", clusterID, cluster.Name, wifConfigID)

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestResource(ctx, h.Entities.MustGet("WifConfig"), "", wifConfigID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup wifconfig %s: %v\n", wifConfigID, err)
				}
			})
			h.DeferCleanup(h.Entities.MustGet("Cluster"), "", clusterID)
		})

		ginkgo.It("should return 409 when deleting wifconfig referenced by a cluster", func(ctx context.Context) {
			ginkgo.By("attempting to delete wifconfig while cluster references it")
			_, err := h.Client.DeleteWifConfig(ctx, wifConfigID)
			var httpErr *client.HTTPError
			Expect(errors.As(err, &httpErr)).To(BeTrue(), "error should be HTTPError")
			Expect(httpErr.StatusCode).To(Equal(http.StatusConflict),
				"deleting wifconfig with referencing cluster should return 409")
		})

		ginkgo.It("should allow wifconfig deletion after referencing cluster is deleted", func(ctx context.Context) {
			ginkgo.By("deleting the cluster")
			_, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "failed to delete cluster")

			ginkgo.By("waiting for cluster hard-delete")
			Eventually(h.PollHTTPStatus(ctx, h.Entities.MustGet("Cluster"), "", clusterID), h.Entities.MustGet("Cluster").Timeouts.Deleted, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound), "cluster should be hard-deleted")

			ginkgo.By("deleting the wifconfig after cluster is gone")
			deleted, err := h.Client.DeleteWifConfig(ctx, wifConfigID)
			Expect(err).NotTo(HaveOccurred(), "wifconfig deletion should succeed after cluster is deleted")
			Expect(deleted.DeletedTime).NotTo(BeNil(), "deleted wifconfig should have deleted_time set")
		})
	},
)

var _ = ginkgo.Describe("[Suite: wifconfig][negative] Cluster Creation With Invalid WIF Config Reference",
	ginkgo.Label(labels.Tier1, labels.Negative),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
		})

		ginkgo.It("should reject cluster creation with non-existent wifconfig reference", func(ctx context.Context) {
			nonExistentID := uuid.NewString()

			ginkgo.By("attempting to create a cluster referencing a non-existent wifconfig")
			_, err := h.Client.CreateResourceFromPayloadWith(ctx, client.ClustersPath,
				h.TestDataPath("payloads/clusters/cluster-request.json"),
				map[string]any{"references": map[string]any{"wif_config": []map[string]any{{"id": nonExistentID, "kind": "WifConfig"}}}},
			)
			var httpErr *client.HTTPError
			Expect(errors.As(err, &httpErr)).To(BeTrue(), "error should be HTTPError")
			Expect(httpErr.StatusCode).To(Equal(http.StatusBadRequest),
				"creating cluster with non-existent wifconfig reference should return 400")
		})
	},
)
