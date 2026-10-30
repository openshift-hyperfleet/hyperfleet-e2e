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
	"[Capability: cascade-delete] Cascade Deletion Through Adapter-backed Children",
	ginkgo.Label(labels.Tier0, labels.Capability),
	func() {
		ginkgo.It("should cascade deletion from Parent to CascadeChild", func(ctx context.Context) {
			h := helper.New()
			child := h.Entities.MustGet(config.KindCascadeChild)
			parent := h.Entities.MustGet(child.ParentKind)
			interval := h.Cfg.Polling.Interval

			ginkgo.By("creating the " + parent.Kind + " and waiting for Reconciled")
			parentID := *createFixture(ctx, h, parent, "").ID
			Eventually(h.Poll(ctx, parent, "", parentID), parent.Timeouts.Reconciled, interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

			ginkgo.By("creating two " + child.Kind + " resources and waiting for both to reach Reconciled")
			childIDs := []string{
				*createFixture(ctx, h, child, parentID).ID,
				*createFixture(ctx, h, child, parentID).ID,
			}
			for _, childID := range childIDs {
				Eventually(h.Poll(ctx, child, parentID, childID), child.Timeouts.Reconciled, interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))
			}

			ginkgo.By("soft-deleting the " + parent.Kind)
			deleted, err := h.Client.DeleteResource(ctx, parent.ResourcePath("", parentID))
			Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
			Expect(deleted.DeletedTime).NotTo(BeNil(), "%s should have deleted_time set", parent.Kind)

			ginkgo.By("verifying cascade: each child is soft-deleted or already hard-deleted")
			for _, childID := range childIDs {
				Eventually(func(g Gomega) {
					resource, err := h.Client.GetResource(ctx, child.ResourcePath(parentID, childID))
					if helper.IsHTTPStatus(err, http.StatusNotFound) {
						return
					}
					g.Expect(err).NotTo(HaveOccurred(), "%s %s should be accessible or 404", child.Kind, childID)
					g.Expect(resource.DeletedTime).NotTo(BeNil(),
						"%s %s should have deleted_time set via cascade", child.Kind, childID)
				}, child.Timeouts.Deleted, interval).Should(Succeed())
			}

			ginkgo.By("waiting for each child to finalize and be hard-deleted")
			for _, childID := range childIDs {
				Eventually(finalizedOrGone(ctx, h, child, parentID, childID), child.Timeouts.Deleted, interval).
					Should(Succeed())
				Eventually(h.PollHTTPStatus(ctx, child, parentID, childID), child.Timeouts.Deleted, interval).
					Should(Equal(http.StatusNotFound))
			}

			ginkgo.By("waiting for the " + parent.Kind + " to be hard-deleted after its children")
			Eventually(h.PollHTTPStatus(ctx, parent, "", parentID), parent.Timeouts.Deleted, interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("verifying every adapter namespace is cleaned up")
			Eventually(h.PollNamespacesByPrefix(ctx, parentID), parent.Timeouts.Deleted, interval).
				Should(BeEmpty())
			for _, childID := range childIDs {
				Eventually(h.PollNamespacesByPrefix(ctx, childID), child.Timeouts.Deleted, interval).
					Should(BeEmpty())
			}
		})
	},
)
