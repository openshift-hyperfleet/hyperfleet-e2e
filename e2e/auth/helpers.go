package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
)

// withoutAuth returns a RequestEditorFn that removes the Authorization header,
// overriding any token the client would normally inject.
func withoutAuth() openapi.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Del("Authorization")
		return nil
	}
}

// withBearerToken returns a RequestEditorFn that replaces the Authorization
// header with the given bearer token, overriding any token the client would
// normally inject.
func withBearerToken(token string) openapi.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
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

// craftUnconfiguredIssuerJWT creates a JWT with an issuer URL not in the API's
// configured issuer list. The token is structurally valid but from an unknown issuer.
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
