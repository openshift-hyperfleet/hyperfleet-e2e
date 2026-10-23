package auth

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

var _ = ginkgo.Describe("[Suite: auth][upgrade] Authentication Upgrade Path Validation",
	ginkgo.Label(labels.Tier0, labels.Auth, labels.Upgrade),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(_ context.Context) {
			h = helper.New()
			if h.Cfg.AuthMode == config.AuthModeNone {
				ginkgo.Skip("authentication upgrade-path test requires an authenticated AUTH_MODE")
			}
		})

		// This test validates the mode-appropriate authentication path in one flow:
		//
		//   1. Authentication is enforced -> unauthenticated GET is denied
		//   2. Machine identity is preserved -> created_by contains the ServiceAccount subject
		//   3. Sentinel auth works -> cluster reaches Reconciled
		//   4. The configured JWT trust path accepts the authenticated request
		ginkgo.It("confirms the authentication path is applied in the deployed system",
			func(ctx context.Context) {
				expected := h.ExpectedIdentity()
				if expected == "" {
					ginkgo.Skip("identity.expectedIdentity not configured - cannot validate upgrade path")
				}

				ginkgo.By("1. Verifying authentication rejects an unauthenticated GET")
				resp, err := rawRequest(ctx, h.Cfg.API.URL, http.MethodGet, "")
				Expect(err).NotTo(HaveOccurred(), "HTTP request should succeed at transport level")
				defer func() { _ = resp.Body.Close() }()
				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"unauthenticated request must return 401")
				if h.Cfg.AuthMode == config.AuthModeAPI {
					Expect(resp).To(helper.HaveRFC9457Error("HYPERFLEET-AUT-001"))
				}

				ginkgo.By("2. Verifying machine identity: creating cluster and checking created_by")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "authenticated cluster creation should succeed")
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id

				h.DeferCleanup(h.Entities.MustGet("Cluster"), "", clusterID)

				Expect(cluster.CreatedBy).To(HaveValue(Equal(expected)),
					"the selected authentication mode should preserve the ServiceAccount identity")

				ginkgo.By("3. Verifying Sentinel auth: cluster reaches Reconciled")
				Eventually(h.Poll(ctx, h.Entities.MustGet("Cluster"), "", clusterID), h.Entities.MustGet("Cluster").Timeouts.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue),
						"Sentinel must authenticate to API and publish events for cluster to reach Reconciled")

				ginkgo.By("4. The configured JWT trust path accepted the authenticated request")
				// API validates the cluster-issued token in API mode and the gateway
				// wristband in EDGE+API mode. EDGE mode validates through TokenReview.
			})
	},
)
