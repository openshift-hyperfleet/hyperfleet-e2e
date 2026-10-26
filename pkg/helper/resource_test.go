package helper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
)

func newResourceTestHelper(t *testing.T, handler http.HandlerFunc) *Helper {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	cl, err := client.NewHyperFleetClient(server.URL, server.Client(), client.WithBearerToken("test-token"))
	if err != nil {
		t.Fatal(err)
	}
	rows := config.EntityDescriptors()
	for i := range rows {
		if rows[i].Kind == "Cluster" || rows[i].Kind == "NodePool" {
			rows[i].Timeouts.Deleted = time.Second
		}
	}
	entities, err := descriptor.NewCatalog(rows)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Polling: config.PollingConfig{Interval: time.Millisecond}, Entities: entities}
	return &Helper{Cfg: cfg, Client: cl, Entities: entities}
}

func TestDescriptorPollers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ kind, parent, path string }{
		{"Cluster", "", "clusters/id"},
		{"NodePool", "parent", "clusters/parent/nodepools/id"},
		{"Version", "parent", "channels/parent/versions/id"},
		{"CascadeChild", "parent", "parents/parent/cascadechildren/id"},
		{"Widget", "", "widgets/id"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			calls := 0
			h := newResourceTestHelper(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				wantPath := "/api/hyperfleet/v1/" + tc.path
				if calls == 2 {
					wantPath += "/statuses"
				}
				if r.Method != http.MethodGet || r.URL.Path != wantPath ||
					r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("unexpected request: %s %s, auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
				}
				switch calls {
				case 1:
					_, _ = fmt.Fprint(w, `{"id":"id"}`)
				case 2:
					_, _ = fmt.Fprint(w, `{"items":[{"adapter":"success"}]}`)
				case 3:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			e := descriptor.EntityDescriptor{Kind: "Widget", Plural: "widgets"}
			if tc.kind != "Widget" {
				e = h.Entities.MustGet(tc.kind)
			}
			resource, err := h.Poll(t.Context(), e, tc.parent, "id")()
			if err != nil || resource == nil || resource.ID == nil || *resource.ID != "id" {
				t.Fatalf("resource=%+v, err=%v", resource, err)
			}
			statuses, err := h.PollAdapterStatuses(t.Context(), e, tc.parent, "id")()
			if err != nil || len(statuses.Items) != 1 || statuses.Items[0].Adapter != "success" {
				t.Fatalf("statuses=%+v, err=%v", statuses, err)
			}
			status, err := h.PollHTTPStatus(t.Context(), e, tc.parent, "id")()
			if err != nil || status != http.StatusNotFound || calls != 3 {
				t.Fatalf("status=%d, calls=%d, err=%v", status, calls, err)
			}
		})
	}
}

func TestPollersPassMissingParentToAPI(t *testing.T) {
	t.Parallel()
	calls := 0
	h := newResourceTestHelper(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		wantPath := "/api/hyperfleet/v1/clusters//nodepools/id"
		if calls == 2 {
			wantPath += "/statuses"
		}
		if r.URL.Path != wantPath {
			t.Errorf("path=%q, want=%q", r.URL.Path, wantPath)
		}
		w.WriteHeader(http.StatusNotFound)
	})
	e := h.Entities.MustGet("NodePool")
	if _, err := h.Poll(t.Context(), e, "", "id")(); !isHTTPStatus(err, http.StatusNotFound) {
		t.Errorf("Poll did not preserve the API error: %v", err)
	}
	if _, err := h.PollAdapterStatuses(t.Context(), e, "", "id")(); !isHTTPStatus(err, http.StatusNotFound) {
		t.Errorf("PollAdapterStatuses did not preserve the API error: %v", err)
	}
	if status, err := h.PollHTTPStatus(t.Context(), e, "", "id")(); err != nil || status != http.StatusNotFound {
		t.Errorf("PollHTTPStatus: status=%d, err=%v", status, err)
	}
	if calls != 3 {
		t.Fatalf("requests=%d, want=3", calls)
	}
}

func TestPollersPropagateErrors(t *testing.T) {
	t.Parallel()
	h := newResourceTestHelper(
		t,
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
	)
	e := h.Entities.MustGet("Cluster")
	if _, err := h.Poll(t.Context(), e, "", "id")(); !isHTTPStatus(err, http.StatusUnauthorized) {
		t.Fatalf("lost HTTP error: %v", err)
	}
	if _, err := h.PollAdapterStatuses(t.Context(), e, "", "id")(); !isHTTPStatus(err, http.StatusUnauthorized) {
		t.Fatalf("lost status HTTP error: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := h.PollHTTPStatus(ctx, e, "", "id")(); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost context cancellation: %v", err)
	}
}

func TestCreateTestResource(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, kind, parent, path, response, wantID string
		wantErr                                    bool
	}{
		{"root", "Cluster", "", "clusters", `{"id":"new-id"}`, "new-id", false},
		{"child", "Version", "parent", "channels/parent/versions", `{"id":"new-id"}`, "new-id", false},
		{"missing ID", "Cluster", "", "clusters", `{}`, "", true},
		{"empty ID", "Cluster", "", "clusters", `{"id":""}`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := filepath.Join(t.TempDir(), "payload.json")
			data := fmt.Appendf(nil, `{"kind":%q,"name":"test-{{.Random}}","spec":{}}`, tc.kind)
			if err := os.WriteFile(payload, data, 0600); err != nil {
				t.Fatal(err)
			}
			h := newResourceTestHelper(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/hyperfleet/v1/"+tc.path {
					t.Errorf("unexpected create: %s %s", r.Method, r.URL.Path)
				}
				var body client.ResourceCreateRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Kind != tc.kind ||
					!strings.HasPrefix(body.Name, "test-") ||
					strings.Contains(body.Name, "{{") {
					t.Errorf("payload was not rendered: %+v, %v", body, err)
				}
				w.WriteHeader(http.StatusCreated)
				_, _ = fmt.Fprint(w, tc.response)
			})
			id, err := h.CreateTestResource(t.Context(), h.Entities.MustGet(tc.kind), tc.parent, payload)
			if (err != nil) != tc.wantErr || (!tc.wantErr && id != tc.wantID) {
				t.Fatalf("id=%q, err=%v", id, err)
			}
		})
	}
}

func TestCleanupDeleteCompatibility(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		kind    string
		status  int
		wantErr bool
	}{
		{"Cluster", 404, false}, {"NodePool", 404, true},
		{"WifConfig", 404, false}, {"WifConfig", 409, false},
		{"Target", 409, true}, {"Cluster", 409, true}, {"WifConfig", 500, true},
		{"WifConfig", 202, false}, {"Referencer", 202, false},
	} {
		t.Run(fmt.Sprintf("%s/%d", tc.kind, tc.status), func(t *testing.T) {
			calls := 0
			h := newResourceTestHelper(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodDelete {
					t.Errorf("unexpected poll: %s", r.Method)
				}
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, `{}`)
			})
			e := h.Entities.MustGet(tc.kind)
			parent := ""
			if e.ParentKind != "" {
				parent = "parent"
			}
			err := h.CleanupTestResource(t.Context(), e, parent, "id")
			if (err != nil) != tc.wantErr || calls != 1 {
				t.Fatalf("calls=%d, err=%v, wantErr=%v", calls, err, tc.wantErr)
			}
		})
	}
}

func TestCleanupHardDeletion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		pollStatus int
		wantErr    bool
	}{
		{"eventual 404", 404, false}, {"poll failure", 500, true}, {"timeout", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			polls := 0
			h := newResourceTestHelper(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/hyperfleet/v1/widgets/id" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusAccepted)
					_, _ = fmt.Fprint(w, `{}`)
					return
				}
				polls++
				if polls == 1 {
					w.WriteHeader(http.StatusOK)
					return
				}
				w.WriteHeader(tc.pollStatus)
			})
			e := descriptor.EntityDescriptor{
				Kind:     "Widget",
				Plural:   "widgets",
				Cleanup:  descriptor.CleanupPolicy{WaitForHardDelete: true},
				Timeouts: descriptor.EntityTimeouts{Deleted: 100 * time.Millisecond},
			}
			err := h.CleanupTestResource(t.Context(), e, "", "id")
			if (err != nil) != tc.wantErr || polls < 2 {
				t.Fatalf("polls=%d, err=%v", polls, err)
			}
			if tc.name == "timeout" && (err == nil || !strings.Contains(err.Error(), "not hard-deleted within")) {
				t.Fatalf("lost timeout: %v", err)
			}
		})
	}
}

func TestFixtureCleanupWaitsForHardDeletion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ kind, parent, path string }{
		{"Parent", "", "/api/hyperfleet/v1/parents/id"},
		{"CascadeChild", "parent", "/api/hyperfleet/v1/parents/parent/cascadechildren/id"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			var methods []string
			polls := 0
			h := newResourceTestHelper(t, func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				if r.URL.Path != tc.path {
					t.Errorf("path=%q, want=%q", r.URL.Path, tc.path)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusAccepted)
					_, _ = fmt.Fprint(w, `{}`)
					return
				}
				polls++
				if polls == 1 {
					w.WriteHeader(http.StatusOK)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			})
			// Use the catalog's fixture timeout unchanged; cap a broken test with its context.
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if err := h.CleanupTestResource(ctx, h.Entities.MustGet(tc.kind), tc.parent, "id"); err != nil {
				t.Fatal(err)
			}
			want := []string{http.MethodDelete, http.MethodGet, http.MethodGet}
			if !reflect.DeepEqual(methods, want) {
				t.Fatalf("requests=%v, want=%v", methods, want)
			}
		})
	}
}

func TestCleanupPreservesZeroTimeoutAndInterval(t *testing.T) {
	t.Parallel()
	// Mutate descriptors after catalog validation to cover the legacy polling loop.
	for _, tc := range []struct {
		name              string
		timeout, interval time.Duration
		wantPolls         int
		wantErr           bool
	}{
		{"zero timeout still deletes", 0, time.Millisecond, 0, true},
		{"negative timeout still deletes", -time.Second, time.Millisecond, 0, true},
		{"zero interval still polls", time.Second, 0, 1, false},
		{"negative interval still polls", time.Second, -time.Millisecond, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deletes, polls := 0, 0
			h := newResourceTestHelper(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deletes++
					w.WriteHeader(http.StatusAccepted)
					_, _ = fmt.Fprint(w, `{}`)
					return
				}
				polls++
				w.WriteHeader(http.StatusNotFound)
			})
			h.Cfg.Polling.Interval = tc.interval
			e := h.Entities.MustGet("Cluster")
			e.Timeouts.Deleted = tc.timeout
			err := h.CleanupTestResource(t.Context(), e, "", "id")
			if (err != nil) != tc.wantErr || deletes != 1 || polls != tc.wantPolls {
				t.Fatalf("deletes=%d, polls=%d, err=%v", deletes, polls, err)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "not hard-deleted within") {
				t.Fatalf("unexpected timeout error: %v", err)
			}
		})
	}
}

type resourceRoundTripFunc func(*http.Request) (*http.Response, error)

func (f resourceRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestCleanupUsesCallerContextForRequests(t *testing.T) {
	t.Parallel()
	for _, cancelOnPoll := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%t", cancelOnPoll), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			wantDeadline, _ := ctx.Deadline()
			var methods []string
			httpClient := &http.Client{Transport: resourceRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				methods = append(methods, r.Method)
				if deadline, ok := r.Context().Deadline(); !ok || !deadline.Equal(wantDeadline) {
					t.Errorf("%s replaced the caller's deadline: %v", r.Method, deadline)
				}
				status := http.StatusAccepted
				if r.Method == http.MethodGet {
					if cancelOnPoll {
						cancel()
						return nil, r.Context().Err()
					}
					status = http.StatusNotFound
				}
				return &http.Response{
					StatusCode: status,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{}`)),
				}, nil
			})}
			cl, err := client.NewHyperFleetClient("http://test.invalid", httpClient)
			if err != nil {
				t.Fatal(err)
			}
			h := &Helper{Cfg: &config.Config{}, Client: cl}
			e := descriptor.EntityDescriptor{Kind: "Widget", Plural: "widgets",
				Timeouts: descriptor.EntityTimeouts{
					Deleted: time.Second,
				}, Cleanup: descriptor.CleanupPolicy{WaitForHardDelete: true}}
			err = h.CleanupTestResource(ctx, e, "", "id")
			if cancelOnPoll {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("lost caller cancellation: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(methods, []string{http.MethodDelete, http.MethodGet}) {
				t.Fatalf("unexpected requests: %v", methods)
			}
		})
	}
}

func TestCleanupChildrenCompatibility(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		wantErr      string
		listStatus   int
		childStatus  int
		parentStatus int
	}{
		{name: "success", listStatus: 200, childStatus: 202, parentStatus: 202},
		{name: "child already deleted", listStatus: 200, childStatus: 404, parentStatus: 202},
		{name: "parent gone during list", listStatus: 404, childStatus: 202, parentStatus: 202},
		{name: "list failure", listStatus: 500, childStatus: 202, parentStatus: 202, wantErr: "list Version"},
		{
			name: "child failure retained", listStatus: 200, childStatus: 500, parentStatus: 202,
			wantErr: "delete Version first",
		},
		{
			name: "child failure retained when parent gone", listStatus: 200, childStatus: 500, parentStatus: 404,
			wantErr: "delete Version first",
		},
		{name: "parent failure wins", listStatus: 200, childStatus: 500, parentStatus: 409, wantErr: "delete Channel parent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			h := newResourceTestHelper(t, func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/api/hyperfleet/v1/")
				calls = append(calls, r.Method+" "+path)
				switch {
				case r.Method == http.MethodGet:
					w.WriteHeader(tc.listStatus)
					_, _ = fmt.Fprint(
						w,
						`{"items":[{"id":"first"},{"id":"second"},{"id":"gone","deleted_time":"2026-01-01T00:00:00Z"},{}]}`,
					)
				case path == "channels/parent":
					w.WriteHeader(tc.parentStatus)
					_, _ = fmt.Fprint(w, `{}`)
				default:
					w.WriteHeader(tc.childStatus)
					_, _ = fmt.Fprint(w, `{}`)
				}
			})
			err := h.CleanupTestResource(t.Context(), h.Entities.MustGet("Channel"), "", "parent")
			if tc.wantErr == "" && err != nil ||
				tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("err=%v, want=%q", err, tc.wantErr)
			}
			want := []string{"GET channels/parent/versions"}
			if tc.listStatus == 200 {
				want = append(
					want,
					"DELETE channels/parent/versions/first",
					"DELETE channels/parent/versions/second",
					"DELETE channels/parent",
				)
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("requests=%v, want=%v", calls, want)
			}
		})
	}
}
