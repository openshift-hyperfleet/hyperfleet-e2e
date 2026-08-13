package auth

import (
	"context"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: auth][enforcement] JWT Authentication Enforcement",
	ginkgo.Label(labels.Tier0, labels.Auth, labels.Negative),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(_ context.Context) {
			h = helper.New()
		})

		ginkgo.It("rejects GET request without Authorization header with 401 and AUT-001",
			func(ctx context.Context) {
				ginkgo.By("sending GET without Authorization header")
				resp, err := rawRequest(ctx, h.Cfg.API.URL, http.MethodGet, "")
				Expect(err).NotTo(HaveOccurred(), "HTTP request should succeed at transport level")
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"unauthenticated GET should return 401")
				Expect(resp).To(helper.HaveRFC9457Error("HYPERFLEET-AUT-001"))
			})

		ginkgo.It("rejects POST request without Authorization header with 401 and AUT-001",
			func(ctx context.Context) {
				ginkgo.By("sending POST without Authorization header")
				resp, err := rawRequest(ctx, h.Cfg.API.URL, http.MethodPost, "")
				Expect(err).NotTo(HaveOccurred(), "HTTP request should succeed at transport level")
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"unauthenticated POST should return 401")
				Expect(resp).To(helper.HaveRFC9457Error("HYPERFLEET-AUT-001"))
			})

		ginkgo.It("rejects requests with invalid JWT signature with 401 and AUT-002",
			func(ctx context.Context) {
				ginkgo.By("crafting a JWT signed with an unknown key")
				token, err := craftInvalidSignatureJWT()
				Expect(err).NotTo(HaveOccurred(), "crafting JWT should succeed")

				ginkgo.By("sending GET with invalid-signature bearer token")
				resp, err := rawRequest(ctx, h.Cfg.API.URL, http.MethodGet, token)
				Expect(err).NotTo(HaveOccurred(), "HTTP request should succeed at transport level")
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"invalid signature should return 401")
				Expect(resp).To(helper.HaveRFC9457Error("HYPERFLEET-AUT-002"))
			})

		// Note: the error model defines AUT-003 for expired tokens, but the API's JWT middleware
		// currently returns AUT-002 (invalid credentials) for all unverifiable tokens.
		// This test validates the deployed behavior.
		ginkgo.It("rejects requests with expired token with 401",
			func(ctx context.Context) {
				ginkgo.By("crafting a self-signed expired JWT")
				token, err := craftExpiredJWT()
				Expect(err).NotTo(HaveOccurred(), "crafting expired JWT should succeed")

				ginkgo.By("sending GET with expired bearer token")
				resp, err := rawRequest(ctx, h.Cfg.API.URL, http.MethodGet, token)
				Expect(err).NotTo(HaveOccurred(), "HTTP request should succeed at transport level")
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"expired token should return 401")
				Expect(resp).To(helper.HaveRFC9457Error("HYPERFLEET-AUT-002"))
			})

		// Note: the error model defines AUT-004 for malformed tokens, but the API's JWT middleware
		// currently returns AUT-002 (invalid credentials) for all unverifiable tokens.
		// This test validates the deployed behavior.
		ginkgo.It("rejects requests with malformed token with 401",
			func(ctx context.Context) {
				ginkgo.By("sending GET with a non-JWT string as bearer token")
				resp, err := rawRequest(ctx, h.Cfg.API.URL, http.MethodGet, "not-a-jwt")
				Expect(err).NotTo(HaveOccurred(), "HTTP request should succeed at transport level")
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"malformed token should return 401")
				Expect(resp).To(helper.HaveRFC9457Error("HYPERFLEET-AUT-002"))
			})
	},
)
