package auth

import (
	"context"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: auth][upgrade] JWT Upgrade Path Validation",
	ginkgo.Label(labels.Tier0, labels.Auth, labels.Upgrade),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(_ context.Context) {
			h = helper.New()
		})

		// This test validates all 4 JWT configuration items from the v1.0.0 upgrade guide
		// in a single end-to-end flow. Each assertion proves one config item is correctly applied:
		//
		//   1. jwt.enabled=true     -> unauthenticated GET returns 401 (not 200)
		//   2. identity_claim=sub   -> created_by contains system:serviceaccount: prefix
		//   3. Sentinel auth        -> cluster reaches Reconciled (Sentinel must publish events)
		//   4. jwk_cert_ca_file     -> authenticated request succeeds (JWKS fetch required)
		ginkgo.It("confirms JWT upgrade path config is applied in deployed system",
			func(ctx context.Context) {
				expected := h.ExpectedIdentity()
				if expected == "" {
					ginkgo.Skip("identity.expectedIdentity not configured - cannot validate upgrade path")
				}

				ginkgo.By("1. Verifying jwt.enabled=true: unauthenticated GET returns 401")
				resp, err := h.Client.GetClusters(ctx, nil, withoutAuth())
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()
				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"jwt.enabled=true should enforce authentication - unauthenticated request must return 401")

				ginkgo.By("2. Verifying identity_claim=sub: creating cluster and checking created_by")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "authenticated cluster creation should succeed (proves jwk_cert_ca_file works)")
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id

				h.DeferClusterCleanup(clusterID)

				Expect(cluster.CreatedBy).To(Equal(expected),
					"identity_claim=sub should resolve the configured service-account identity")

				ginkgo.By("3. Verifying Sentinel auth: cluster reaches Reconciled")
				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue),
						"Sentinel must authenticate to API and publish events for cluster to reach Reconciled")

				ginkgo.By("4. jwk_cert_ca_file validated implicitly: authenticated request above succeeded")
				// If jwk_cert_ca_file were misconfigured, the API could not fetch JWKS to validate
				// our token, and the CreateCluster call above would have returned 401.
				// No separate assertion needed - success of step 2 proves it.
			})
	},
)
