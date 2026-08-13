package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const apiPrefix = "/api/hyperfleet/v1/"

// rawRequest makes an HTTP request to the clusters API endpoint.
// If token is non-empty, it's sent as a Bearer token; otherwise unauthenticated.
func rawRequest(ctx context.Context, apiURL, method, token string) (*http.Response, error) {
	fullURL := strings.TrimRight(apiURL, "/") + apiPrefix + "clusters"
	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return (&http.Client{Timeout: 30 * time.Second}).Do(req)
}

// craftJWT creates a self-signed JWT with arbitrary claims, signed with a random RSA key.
// Useful for testing rejection of tokens with invalid signatures, expired tokens, etc.
func craftJWT(claims jwt.MapClaims) (string, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("generating RSA key: %w", err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("signing JWT: %w", err)
	}
	return signed, nil
}

// craftExpiredJWT creates a self-signed JWT that has already expired.
// Random-key signed, so it can't isolate exp-checking from signature rejection (see AUT-002 note in jwt_enforcement.go).
func craftExpiredJWT() (string, error) {
	now := time.Now()
	return craftJWT(jwt.MapClaims{
		"iss": "https://expired-issuer.example.com",
		"sub": "system:serviceaccount:test:expired-sa",
		"aud": "hyperfleet-api",
		"exp": jwt.NewNumericDate(now.Add(-1 * time.Hour)),
		"iat": jwt.NewNumericDate(now.Add(-2 * time.Hour)),
	})
}

// craftInvalidSignatureJWT creates a structurally valid JWT signed with a random key
// that the API's JWKS endpoint won't recognize.
func craftInvalidSignatureJWT() (string, error) {
	now := time.Now()
	return craftJWT(jwt.MapClaims{
		"iss": "https://kubernetes.default.svc",
		"sub": "system:serviceaccount:hyperfleet:fake-sa",
		"aud": "hyperfleet-api",
		"exp": jwt.NewNumericDate(now.Add(1 * time.Hour)),
		"iat": jwt.NewNumericDate(now),
	})
}

// craftUnconfiguredIssuerJWT creates a JWT with an issuer URL not in the API's configured issuer list.
// Random-key signed, so it can't isolate iss-checking from signature rejection either.
func craftUnconfiguredIssuerJWT() (string, error) {
	now := time.Now()
	return craftJWT(jwt.MapClaims{
		"iss": "https://unconfigured-issuer.example.com",
		"sub": "user@unconfigured.example.com",
		"aud": "hyperfleet-api",
		"exp": jwt.NewNumericDate(now.Add(1 * time.Hour)),
		"iat": jwt.NewNumericDate(now),
	})
}
