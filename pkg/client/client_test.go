package client

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type failingCloseBody struct {
	io.Reader
	closed bool
}

func (b *failingCloseBody) Close() error {
	b.closed = true
	return errors.New("response close failed")
}

func TestResponseCleanupPreservesResult(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
		body := &failingCloseBody{Reader: strings.NewReader(`{"name":"test"}`)}
		resource, err := handleHTTPResponse[Resource](
			&http.Response{StatusCode: status, Body: body}, http.StatusOK, "get resource",
		)
		if !body.closed {
			t.Fatal("response body was not closed")
		}
		if status == http.StatusOK {
			if err != nil || resource == nil || resource.Name != "test" {
				t.Fatalf("cleanup changed successful response: resource=%+v err=%v", resource, err)
			}
		} else {
			var httpErr *HTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != status {
				t.Fatalf("cleanup replaced the HTTP error: %v", err)
			}
		}
	}
	body := &failingCloseBody{Reader: strings.NewReader("")}
	if err := handleHTTPNoBodyResponse(
		&http.Response{StatusCode: http.StatusNoContent, Body: body}, http.StatusNoContent, "delete resource",
	); err != nil || !body.closed {
		t.Fatalf("no-body cleanup changed the result: closed=%v err=%v", body.closed, err)
	}
}

func TestWithBearerToken(t *testing.T) {
	t.Parallel()

	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c, err := NewHyperFleetClient(ts.URL, nil, WithBearerToken("test-token-123"))
	if err != nil {
		t.Fatalf("NewHyperFleetClient: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/test", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if gotAuth != "Bearer test-token-123" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-token-123")
	}
}

func TestWithAuthorizationTokenScheme(t *testing.T) {
	tests := []struct {
		name   string
		scheme string
		want   string
	}{
		{name: "ServiceAccount", scheme: "ServiceAccount", want: "ServiceAccount token-value"},
		{name: "empty scheme defaults to Bearer", want: "Bearer token-value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotAuth string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusOK)
			}))
			defer ts.Close()

			c, err := NewHyperFleetClient(ts.URL, nil, WithAuthorizationToken("token-value", tt.scheme))
			if err != nil {
				t.Fatalf("NewHyperFleetClient: %v", err)
			}

			req, err := http.NewRequest(http.MethodGet, ts.URL+"/test", nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			resp, err := c.httpClient.Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if gotAuth != tt.want {
				t.Errorf("Authorization header = %q, want %q", gotAuth, tt.want)
			}
		})
	}
}

func TestAuthTransportNilBase(t *testing.T) {
	t.Parallel()

	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	transport := &authTransport{base: nil, token: "fallback-token"}
	httpClient := &http.Client{Transport: transport}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/test", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if gotAuth != "Bearer fallback-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer fallback-token")
	}
}
