package capability

import (
	"context"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe(
	"[Capability: recreate-after-hard-delete] Re-create After Hard-delete",
	ginkgo.Label(labels.Tier0, labels.Capability),
	func() {
		ginkgo.It("should re-create Parent with the same name after hard-delete", func(ctx context.Context) {
			h := helper.New()
			entity := h.Entities.MustGet(config.KindParent)
			interval := h.Cfg.Polling.Interval

			ginkgo.By("creating the first " + entity.Kind + " and waiting for Reconciled")
			first := createFixture(ctx, h, entity, "")
			firstID := *first.ID
			Eventually(h.Poll(ctx, entity, "", firstID), entity.Timeouts.Reconciled, interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

			ginkgo.By("deleting the first " + entity.Kind + " and waiting for hard-delete")
			_, err := h.Client.DeleteResource(ctx, entity.ResourcePath("", firstID))
			Expect(err).NotTo(HaveOccurred())
			Eventually(h.PollHTTPStatus(ctx, entity, "", firstID), entity.Timeouts.Deleted, interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("waiting for namespace cleanup from the first " + entity.Kind)
			Eventually(h.PollNamespacesByPrefix(ctx, firstID), entity.Timeouts.Deleted, interval).
				Should(BeEmpty())

			ginkgo.By("creating a new " + entity.Kind + " with the same name")
			second, err := h.Client.CreateResource(ctx, entity.CollectionPath(""), client.ResourceCreateRequest{
				Kind:   entity.Kind,
				Name:   first.Name,
				Labels: first.Labels,
				Spec:   first.Spec,
			})
			Expect(err).NotTo(HaveOccurred(), "creating %s with reused name should succeed", entity.Kind)
			Expect(second.ID).NotTo(BeNil())
			secondID := *second.ID
			h.DeferCleanup(entity, "", secondID)

			Expect(secondID).NotTo(Equal(firstID), "new %s should have a different ID than the deleted one", entity.Kind)
			Expect(second.Generation).To(Equal(int32(1)), "new %s should start at generation 1", entity.Kind)

			ginkgo.By("waiting for the new " + entity.Kind + " to reach Reconciled")
			Eventually(h.Poll(ctx, entity, "", secondID), entity.Timeouts.Reconciled, interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

			ginkgo.By("verifying the old " + entity.Kind + " is still gone")
			Expect(h.Client.GetResourceHTTPStatus(ctx, entity.ResourcePath("", firstID))).
				To(Equal(http.StatusNotFound), "old %s should remain 404 after recreate", entity.Kind)
		})
	},
)
