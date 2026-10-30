package capability

import (
	"context"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
)

func createFixture(
	ctx context.Context,
	h *helper.Helper,
	entity descriptor.EntityDescriptor,
	parentID string,
) *client.Resource {
	ginkgo.GinkgoHelper()
	resource, err := h.Client.CreateResourceFromPayloadWith(
		ctx,
		entity.CollectionPath(parentID),
		h.TestDataPath("payloads/fixtures/fixture-request.json"),
		map[string]any{"kind": entity.Kind},
	)
	Expect(err).NotTo(HaveOccurred(), "failed to create %s", entity.Kind)
	Expect(resource.ID).NotTo(BeNil(), "%s ID should be generated", entity.Kind)
	h.DeferCleanup(entity, parentID, *resource.ID)
	ginkgo.GinkgoWriter.Printf("Created %s ID: %s, Name: %s\n", entity.Kind, *resource.ID, resource.Name)
	return resource
}

// The API hard-deletes a resource in the status write that finalizes its last
// adapter, so Finalized=True may never be observable; a 404 ends the check.
func finalizedOrGone(
	ctx context.Context,
	h *helper.Helper,
	entity descriptor.EntityDescriptor,
	parentID, resourceID string,
) func(g Gomega) {
	path := entity.ResourcePath(parentID, resourceID)
	return func(g Gomega) {
		_, err := h.Client.GetResource(ctx, path)
		if helper.IsHTTPStatus(err, http.StatusNotFound) {
			return
		}
		g.Expect(err).NotTo(HaveOccurred())
		statuses, err := h.Client.GetResourceStatuses(ctx, path)
		if helper.IsHTTPStatus(err, http.StatusNotFound) {
			return
		}
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(statuses).To(helper.HaveAllAdaptersWithCondition(
			entity.RequiredAdapters, client.ConditionTypeFinalized, client.AdapterConditionStatusTrue))
	}
}
