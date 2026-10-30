package capability

import (
	"context"
	"net/http"
	"slices"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe(
	"[Capability: adapter-backed-lifecycle] Adapter-backed Resource Lifecycle",
	ginkgo.Label(labels.Tier0, labels.Capability),
	func() {
		for _, kind := range []string{config.KindParent, config.KindCascadeChild} {
			ginkgo.It("should create, update and delete "+kind+" through its adapters", func(ctx context.Context) {
				h := helper.New()
				entity := h.Entities.MustGet(kind)
				interval := h.Cfg.Polling.Interval

				var parent descriptor.EntityDescriptor
				var parentID string
				if entity.ParentKind != "" {
					ginkgo.By("creating the parent " + entity.ParentKind + " and waiting for Reconciled")
					parent = h.Entities.MustGet(entity.ParentKind)
					parentID = *createFixture(ctx, h, parent, "").ID
					Eventually(h.Poll(ctx, parent, "", parentID), parent.Timeouts.Reconciled, interval).
						Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))
				}

				ginkgo.By("creating the " + entity.Kind)
				created := createFixture(ctx, h, entity, parentID)
				id := *created.ID
				path := entity.ResourcePath(parentID, id)
				Expect(created.Name).NotTo(BeEmpty(), "%s name should be present", entity.Kind)
				Expect(created.Generation).To(Equal(int32(1)), "a new %s should start at generation 1", entity.Kind)
				if expected := h.ExpectedIdentity(); expected != "" {
					Expect(created).To(helper.HaveAuditIdentity(expected))
				}

				ginkgo.By("verifying every required adapter executed")
				Eventually(func(g Gomega) {
					statuses, err := h.Client.GetResourceStatuses(ctx, path)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get %s statuses", entity.Kind)
					expectAdaptersExecuted(g, entity, statuses)
				}, entity.Timeouts.Reconciled, interval).Should(Succeed())

				ginkgo.By("waiting for Reconciled=True at generation 1")
				Eventually(h.Poll(ctx, entity, parentID, id), entity.Timeouts.Reconciled, interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				reconciled, err := h.Client.GetResource(ctx, path)
				Expect(err).NotTo(HaveOccurred(), "failed to get reconciled %s", entity.Kind)
				Expect(reconciled.Generation).To(Equal(int32(1)), "%s should be at generation 1 before update", entity.Kind)
				Expect(reconciled).To(
					helper.HaveResourceCondition(client.ConditionTypeLastKnownReconciled, client.ResourceConditionStatusTrue),
					"%s should have LastKnownReconciled=True condition", entity.Kind)
				for _, condition := range reconciled.Status.Conditions {
					if condition.Type == client.ConditionTypeReconciled ||
						condition.Type == client.ConditionTypeLastKnownReconciled {
						Expect(condition.ObservedGeneration).To(Equal(int32(1)),
							"%s condition %s should have observed_generation=1 for new creation request",
							entity.Kind, condition.Type)
					}
				}
				for _, adapterName := range entity.RequiredAdapters {
					expectedCondType := h.AdapterNameToConditionType(adapterName)
					Expect(reconciled).To(
						helper.HaveResourceCondition(expectedCondType, client.ResourceConditionStatusTrue),
						"%s should have %s=True condition for adapter %s", entity.Kind, expectedCondType, adapterName)
				}

				ginkgo.By("verifying the adapter's namespace is Active")
				// The success adapter role names its namespace after the resource ID.
				Eventually(func() error {
					return h.VerifyNamespaceActive(ctx, id,
						map[string]string{
							"hyperfleet.io/resource-id":    id,
							"hyperfleet.io/resource-name":  created.Name,
							"e2e.hyperfleet.io/managed-by": "test-framework",
						},
						map[string]string{"hyperfleet.io/generation": "1"},
					)
				}, entity.Timeouts.Reconciled, interval).Should(Succeed())

				var parentBefore *client.Resource
				if entity.ParentKind != "" {
					parentBefore, err = h.Client.GetResource(ctx, parent.ResourcePath("", parentID))
					Expect(err).NotTo(HaveOccurred())
				}

				ginkgo.By("sending PATCH to update the spec")
				patched, err := h.Client.PatchResourceFromPayload(
					ctx, path, h.TestDataPath("payloads/fixtures/fixture-patch.json"))
				Expect(err).NotTo(HaveOccurred(), "PATCH request should succeed")
				expectedGen := reconciled.Generation + 1
				Expect(patched.Generation).To(Equal(expectedGen), "generation should increment after PATCH")
				Expect(patched.Spec).To(HaveKeyWithValue("revision", BeNumerically("==", 2)),
					"PATCH response should reflect the updated spec")
				if expected := h.ExpectedIdentity(); expected != "" {
					Expect(patched.UpdatedBy).NotTo(BeNil(), "updated_by should be set after PATCH")
					Expect(*patched.UpdatedBy).To(Equal(expected), "updated_by should match configured identity after PATCH")
				}

				ginkgo.By("waiting for every adapter to reconcile the new generation")
				Eventually(h.PollAdapterStatuses(ctx, entity, parentID, id), entity.Timeouts.Reconciled, interval).
					Should(helper.HaveAllAdaptersAtGeneration(entity.RequiredAdapters, expectedGen))

				ginkgo.By("verifying Reconciled=True at the new generation")
				Eventually(func(g Gomega) {
					updated, getErr := h.Client.GetResource(ctx, path)
					g.Expect(getErr).NotTo(HaveOccurred())
					g.Expect(updated.Generation).To(Equal(expectedGen), "final generation should match expected")

					found := false
					for _, cond := range updated.Status.Conditions {
						if cond.Type == client.ConditionTypeReconciled && cond.Status == client.ResourceConditionStatusTrue {
							found = true
							g.Expect(cond.ObservedGeneration).
								To(Equal(expectedGen), "Reconciled condition observed_generation should match expected")
						}
					}
					g.Expect(found).To(BeTrue(), "%s should have Reconciled=True", entity.Kind)
				}, entity.Timeouts.Reconciled, interval).Should(Succeed())

				if entity.ParentKind != "" {
					ginkgo.By("verifying the parent is unaffected by the update")
					expectParentUnchanged(ctx, h, parent, parentID, parentBefore)
				}

				ginkgo.By("soft-deleting the " + entity.Kind)
				deleted, err := h.Client.DeleteResource(ctx, path)
				Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
				Expect(deleted.DeletedTime).NotTo(BeNil(), "soft-deleted %s should have deleted_time set", entity.Kind)
				Expect(deleted.Generation).To(Equal(expectedGen+1), "generation should increment after soft-delete")
				if expected := h.ExpectedIdentity(); expected != "" {
					Expect(deleted.DeletedBy).NotTo(BeNil(), "deleted_by should be set on soft-delete")
					Expect(*deleted.DeletedBy).To(Equal(expected), "deleted_by should match configured identity")
				}

				ginkgo.By("waiting for the adapters to finalize and the " + entity.Kind + " to be hard-deleted")
				Eventually(finalizedOrGone(ctx, h, entity, parentID, id), entity.Timeouts.Deleted, interval).
					Should(Succeed())
				Eventually(h.PollHTTPStatus(ctx, entity, parentID, id), entity.Timeouts.Deleted, interval).
					Should(Equal(http.StatusNotFound))

				ginkgo.By("verifying the adapter's namespace is cleaned up")
				Eventually(h.PollNamespacesByPrefix(ctx, id), entity.Timeouts.Deleted, interval).
					Should(BeEmpty())

				if entity.ParentKind != "" {
					ginkgo.By("verifying the parent is unaffected by the delete")
					parentAfter := expectParentUnchanged(ctx, h, parent, parentID, parentBefore)
					Expect(parentAfter.DeletedTime).To(BeNil(), "parent %s should not have deleted_time", parent.Kind)
				}
			})
		}
	},
)

func expectAdaptersExecuted(g Gomega, entity descriptor.EntityDescriptor, statuses *client.AdapterStatusList) {
	g.Expect(statuses.Items).NotTo(BeEmpty(), "at least one adapter should have executed")
	g.Expect(statuses).To(helper.HaveAllAdaptersAtGeneration(entity.RequiredAdapters, 1))

	for _, adapter := range statuses.Items {
		if !slices.Contains(entity.RequiredAdapters, adapter.Adapter) {
			continue
		}
		g.Expect(adapter.CreatedTime).NotTo(BeZero(), "adapter %s should have valid created_time", adapter.Adapter)
		g.Expect(adapter.LastReportTime).NotTo(BeZero(),
			"adapter %s should have valid last_report_time", adapter.Adapter)

		for _, condition := range adapter.Conditions {
			// architecture adapter-lifecycle-delete-design.md: when not deleting, Finalized=False
			// "reason/message is meaningless".
			if condition.Type != client.ConditionTypeFinalized ||
				condition.Status != client.AdapterConditionStatusFalse {
				g.Expect(condition.Reason).NotTo(BeNil(),
					"adapter %s condition %s should have non-nil reason", adapter.Adapter, condition.Type)
				g.Expect(*condition.Reason).NotTo(BeEmpty(),
					"adapter %s condition %s should have non-empty reason", adapter.Adapter, condition.Type)
				g.Expect(condition.Message).NotTo(BeNil(),
					"adapter %s condition %s should have non-nil message", adapter.Adapter, condition.Type)
				g.Expect(*condition.Message).NotTo(BeEmpty(),
					"adapter %s condition %s should have non-empty message", adapter.Adapter, condition.Type)
			}
			g.Expect(condition.LastTransitionTime).NotTo(BeZero(),
				"adapter %s condition %s should have valid last_transition_time", adapter.Adapter, condition.Type)
		}
	}
}

func expectParentUnchanged(
	ctx context.Context,
	h *helper.Helper,
	parent descriptor.EntityDescriptor,
	parentID string,
	before *client.Resource,
) *client.Resource {
	ginkgo.GinkgoHelper()
	after, err := h.Client.GetResource(ctx, parent.ResourcePath("", parentID))
	Expect(err).NotTo(HaveOccurred(), "parent %s should still exist", parent.Kind)
	Expect(after.Generation).To(Equal(before.Generation), "parent %s generation should remain unchanged", parent.Kind)
	Expect(after).To(
		helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue),
		"parent %s should remain Reconciled=True", parent.Kind)
	return after
}
