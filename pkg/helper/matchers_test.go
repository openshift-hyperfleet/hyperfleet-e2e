package helper

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/pointer"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
)

func TestHaveAuditIdentity(t *testing.T) {
	expected := "system:serviceaccount:hyperfleet:hyperfleet-e2e-sa"
	other := "someone-else@example.com"

	tests := []struct {
		actual    any
		name      string
		wantMatch bool
		wantErr   bool
	}{
		{
			name:      "Resource with matching identity",
			actual:    &client.Resource{CreatedBy: pointer.ToPtr(expected)},
			wantMatch: true,
		},
		{
			name:      "Resource with mismatched identity",
			actual:    &client.Resource{CreatedBy: pointer.ToPtr(other)},
			wantMatch: false,
		},
		{
			name:    "nil *Resource",
			actual:  (*client.Resource)(nil),
			wantErr: true,
		},
		{
			name:      "Resource with nil CreatedBy",
			actual:    &client.Resource{CreatedBy: nil},
			wantMatch: false,
		},
		{
			name:    "unsupported type",
			actual:  "not-a-cluster",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := HaveAuditIdentity(expected)
			matched, err := matcher.Match(tt.actual)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got match=%v", matched)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if matched != tt.wantMatch {
				t.Errorf("got match=%v, want %v", matched, tt.wantMatch)
			}

			if !matched {
				msg := matcher.FailureMessage(tt.actual)
				if msg == "" {
					t.Error("FailureMessage returned empty string")
				}
			}
		})
	}
}

func TestHaveRFC9457Error(t *testing.T) {
	makeResp := func(contentType, body string) *http.Response {
		return &http.Response{
			Header: http.Header{"Content-Type": []string{contentType}},
			Body:   io.NopCloser(strings.NewReader(body)),
		}
	}

	tests := []struct {
		actual    any
		name      string
		code      string
		wantMatch bool
		wantErr   bool
	}{
		{
			name: "matching error code",
			code: "HYPERFLEET-AUT-001",
			actual: makeResp(
				"application/problem+json",
				`{"type":"https://api.hyperfleet.io/errors/authentication-required",`+
					`"title":"Authentication Required","status":401,"code":"HYPERFLEET-AUT-001"}`,
			),
			wantMatch: true,
		},
		{
			name:   "malformed content type",
			code:   "HYPERFLEET-AUT-001",
			actual: makeResp("application/problem+json; invalid", `{"code":"HYPERFLEET-AUT-001"}`),
		},
		{
			name: "mismatched error code",
			code: "HYPERFLEET-AUT-001",
			actual: makeResp(
				"application/problem+json",
				`{"type":"https://api.hyperfleet.io/errors/invalid-credentials",`+
					`"title":"Invalid Credentials","status":401,"code":"HYPERFLEET-AUT-002"}`,
			),
			wantMatch: false,
		},
		{
			name:      "wrong content type",
			code:      "HYPERFLEET-AUT-001",
			actual:    makeResp("application/json", `{"error":"unauthorized"}`),
			wantMatch: false,
		},
		{
			name: "no code field",
			code: "HYPERFLEET-AUT-001",
			actual: makeResp(
				"application/problem+json",
				`{"type":"about:blank","title":"Unauthorized","status":401}`,
			),
			wantMatch: false,
		},
		{
			name:    "wrong type",
			code:    "HYPERFLEET-AUT-001",
			actual:  "not-a-response",
			wantErr: true,
		},
		{
			name:    "nil response",
			code:    "HYPERFLEET-AUT-001",
			actual:  (*http.Response)(nil),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := HaveRFC9457Error(tt.code)
			matched, err := matcher.Match(tt.actual)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got match=%v", matched)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if matched != tt.wantMatch {
				t.Errorf("got match=%v, want %v", matched, tt.wantMatch)
			}

			if !matched {
				msg := matcher.FailureMessage(tt.actual)
				if msg == "" {
					t.Error("FailureMessage returned empty string")
				}
			}
		})
	}
}
