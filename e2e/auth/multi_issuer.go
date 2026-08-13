package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: auth][multi-issuer] JWT Identity and Issuer Validation",
	ginkgo.Label(labels.Tier0, labels.Auth),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(_ context.Context) {
			h = helper.New()
		})

		ginkgo.It("populates audit fields on write requests from JWT sub claim",
			func(ctx context.Context) {
				expected := h.ExpectedIdentity()
				if expected == "" {
					ginkgo.Skip("identity.expectedIdentity not configured - skipping audit field assertion")
				}

				ginkgo.By("creating a cluster and verifying created_by")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "cluster creation should succeed")
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id

				h.DeferClusterCleanup(clusterID)

				Expect(cluster.CreatedBy).To(HaveValue(Equal(expected)),
					"created_by should contain the JWT sub claim identity")

				ginkgo.By("patching the cluster and verifying updated_by")
				patched, err := h.Client.PatchClusterFromPayload(ctx, clusterID, h.TestDataPath("payloads/clusters/cluster-patch.json"))
				Expect(err).NotTo(HaveOccurred(), "cluster PATCH should succeed")
				Expect(patched.UpdatedBy).To(HaveValue(Equal(expected)),
					"updated_by should contain the JWT sub claim identity")

				ginkgo.By("waiting for cluster to reconcile before delete")
				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				ginkgo.By("deleting the cluster and verifying deleted_by")
				deleted, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "cluster DELETE should succeed")
				Expect(deleted.DeletedBy).To(HaveValue(Equal(expected)),
					"deleted_by should contain the JWT sub claim identity")
			})

		ginkgo.It("rejects tokens from unconfigured issuers with 401 and AUT-002",
			func(ctx context.Context) {
				ginkgo.By("crafting a JWT from an unconfigured issuer")
				token, err := craftUnconfiguredIssuerJWT()
				Expect(err).NotTo(HaveOccurred(), "crafting unconfigured-issuer JWT should succeed")

				ginkgo.By("sending request with unconfigured issuer token")
				resp, err := rawRequest(ctx, h.Cfg.API.URL, http.MethodGet, token)
				Expect(err).NotTo(HaveOccurred(), "HTTP request should succeed at transport level")
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"unconfigured issuer should return 401")
				Expect(resp).To(helper.HaveRFC9457Error("HYPERFLEET-AUT-002"))
			})

		ginkgo.It("accepts tokens from different service accounts within the configured issuer",
			func(ctx context.Context) {
				if !h.Cfg.Identity.TokenRequest.IsEnabled() {
					ginkgo.Skip("TokenRequest not configured - cannot acquire token for a different SA")
				}

				// Use a different SA name in the same namespace to prove the API accepts
				// tokens from any SA under the configured K8s issuer, not just the E2E SA.
				altSAName := h.Cfg.Identity.TokenRequest.ServiceAccountName + "-alt"
				ns := h.Cfg.Identity.TokenRequest.Namespace
				audience := h.Cfg.Identity.TokenRequest.Audience
				expiration := h.Cfg.Identity.TokenRequest.ExpirationSeconds

				ginkgo.By("provisioning the alternative service account")
				Expect(h.EnsureServiceAccount(ctx, ns, altSAName)).To(Succeed(),
					"provisioning alt SA should succeed (requires RBAC to create service accounts in %s)", ns)
				ginkgo.DeferCleanup(func(cleanupCtx context.Context) {
					_ = h.DeleteServiceAccount(cleanupCtx, ns, altSAName)
				})

				ginkgo.By("acquiring a token for the alternative service account")
				altToken, err := h.K8sClient.CreateToken(ctx, ns, altSAName, audience, expiration)
				Expect(err).NotTo(HaveOccurred(), "acquiring a token for the alt SA should succeed")

				ginkgo.By("creating a cluster with the alternative SA token")
				altClient, err := client.NewHyperFleetClient(h.Cfg.API.URL, nil, client.WithBearerToken(altToken))
				Expect(err).NotTo(HaveOccurred(), "creating alt client should succeed")

				cluster, err := altClient.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "cluster creation with alt SA token should succeed")
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id

				h.DeferClusterCleanup(clusterID)

				ginkgo.By("verifying audit field shows the alternative SA identity")
				expectedAltIdentity := fmt.Sprintf("system:serviceaccount:%s:%s", ns, altSAName)
				Expect(cluster.CreatedBy).To(HaveValue(Equal(expectedAltIdentity)),
					"created_by should reflect the alternative SA identity, not the primary E2E SA")
			})

		ginkgo.It("rejects tokens with an incorrect audience with 401 and AUT-002",
			func(ctx context.Context) {
				if !h.Cfg.Identity.TokenRequest.IsEnabled() {
					ginkgo.Skip("TokenRequest not configured - cannot mint a real token with a wrong audience")
				}

				ns := h.Cfg.Identity.TokenRequest.Namespace
				saName := h.Cfg.Identity.TokenRequest.ServiceAccountName
				expiration := h.Cfg.Identity.TokenRequest.ExpirationSeconds
				wrongAudience := h.Cfg.Identity.TokenRequest.Audience + "-invalid"

				ginkgo.By("minting a real token with an incorrect audience")
				token, err := h.K8sClient.CreateToken(ctx, ns, saName, wrongAudience, expiration)
				Expect(err).NotTo(HaveOccurred(), "acquiring a wrong-audience token should succeed")

				ginkgo.By("sending request with the wrong-audience token")
				resp, err := rawRequest(ctx, h.Cfg.API.URL, http.MethodGet, token)
				Expect(err).NotTo(HaveOccurred(), "HTTP request should succeed at transport level")
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"incorrect audience should return 401")
				Expect(resp).To(helper.HaveRFC9457Error("HYPERFLEET-AUT-002"))
			})

		ginkgo.It("confirms adapters authenticate with JWT when reporting cluster status",
			func(ctx context.Context) {
				ginkgo.By("creating a cluster and waiting for Reconciled")
				clusterID, err := h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "cluster creation should succeed")

				h.DeferClusterCleanup(clusterID)

				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				ginkgo.By("verifying adapter status reports were recorded")
				statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "getting cluster statuses should succeed")
				Expect(statuses.Items).NotTo(BeEmpty(), "at least one adapter should have reported")

				// AdapterStatus has no audit-identity field, so this only proves the
				// JWT-authenticated PUT was accepted, not which identity it used.
				for _, status := range statuses.Items {
					Expect(status.CreatedTime).NotTo(BeZero(),
						"adapter %s status should have created_time set (proving JWT-authenticated PUT succeeded)",
						status.Adapter)
				}
			})
	},
)
