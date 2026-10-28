package helper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPatchEntityRequiredAdapters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		vals     apiValues
		name     string
		kind     string
		wantErr  string
		adapters []string
	}{
		{
			name: "patches matching entity",
			vals: apiValues{
				Config: struct {
					Rest     map[string]any `yaml:",inline"`
					Entities []apiEntity    `yaml:"entities"`
				}{
					Entities: []apiEntity{
						{Kind: "Cluster", RequiredAdapters: []string{"old-adapter"}},
					},
				},
			},
			kind:     "Cluster",
			adapters: []string{"new-a", "new-b"},
		},
		{
			name: "entity kind not found",
			vals: apiValues{
				Config: struct {
					Rest     map[string]any `yaml:",inline"`
					Entities []apiEntity    `yaml:"entities"`
				}{
					Entities: []apiEntity{
						{Kind: "NodePool"},
					},
				},
			},
			kind:     "Cluster",
			adapters: []string{"a"},
			wantErr:  `entity with kind "Cluster" not found`,
		},
		{
			name:     "empty entities",
			vals:     apiValues{},
			kind:     "Cluster",
			adapters: []string{"a"},
			wantErr:  `entity with kind "Cluster" not found`,
		},
		{
			name: "multiple entities patches correct one",
			vals: apiValues{
				Config: struct {
					Rest     map[string]any `yaml:",inline"`
					Entities []apiEntity    `yaml:"entities"`
				}{
					Entities: []apiEntity{
						{Kind: "NodePool", RequiredAdapters: []string{"np-adapter"}},
						{Kind: "Cluster", RequiredAdapters: []string{"old"}},
					},
				},
			},
			kind:     "Cluster",
			adapters: []string{"new-a", "new-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := patchEntityRequiredAdapters(&tt.vals, tt.kind, tt.adapters)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify the patched entity and that non-matching entities are unchanged
			found := false
			for _, e := range tt.vals.Config.Entities {
				if e.Kind == tt.kind {
					found = true
					if len(e.RequiredAdapters) != len(tt.adapters) {
						t.Fatalf("patched adapters len = %d, want %d", len(e.RequiredAdapters), len(tt.adapters))
					}
					for i, want := range tt.adapters {
						if e.RequiredAdapters[i] != want {
							t.Errorf("adapter[%d] = %v, want %v", i, e.RequiredAdapters[i], want)
						}
					}
				}
			}
			if !found {
				t.Fatal("patched entity not found after successful patch")
			}

			// Assert non-matching entities were not modified
			for _, e := range tt.vals.Config.Entities {
				if e.Kind == tt.kind {
					continue
				}
				if e.Kind == "NodePool" {
					if len(e.RequiredAdapters) != 1 || e.RequiredAdapters[0] != "np-adapter" {
						t.Errorf(
							"non-target entity %s was modified: RequiredAdapters = %v, want [np-adapter]",
							e.Kind,
							e.RequiredAdapters,
						)
					}
				}
			}
		})
	}
}

// A read-modify-write of the Helm values must not add parent fields to a root
// entity: the API chart's schema only allows cascade or restrict for
// on_parent_delete, so an empty string would fail the upgrade.
func TestAPIEntityMarshalOmitsEmptyParentFields(t *testing.T) {
	t.Parallel()
	const values = `config:
  entities:
    - kind: Cluster
      plural: clusters
      required_adapters: [cl-namespace]
    - kind: NodePool
      plural: nodepools
      parent_kind: Cluster
      on_parent_delete: cascade
      required_adapters: [np-configmap]
`
	var vals apiValues
	if err := yaml.Unmarshal([]byte(values), &vals); err != nil {
		t.Fatal(err)
	}
	out, err := yaml.Marshal(vals)
	if err != nil {
		t.Fatal(err)
	}

	var decoded struct {
		Config struct {
			Entities []map[string]any `yaml:"entities"`
		} `yaml:"config"`
	}
	if err := yaml.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Config.Entities) != 2 {
		t.Fatalf("entities = %d, want 2:\n%s", len(decoded.Config.Entities), out)
	}
	for _, key := range []string{"parent_kind", "on_parent_delete"} {
		if _, exists := decoded.Config.Entities[0][key]; exists {
			t.Errorf("root entity gained %q on round-trip:\n%s", key, out)
		}
	}
	child := decoded.Config.Entities[1]
	if child["parent_kind"] != "Cluster" || child["on_parent_delete"] != "cascade" {
		t.Errorf("child parent fields lost on round-trip:\n%s", out)
	}
}

// assertUnmodeledEntityFields checks the applied YAML directly, so a dropped
// inline catch-all fails here instead of only on a live release.
func assertUnmodeledEntityFields(t *testing.T, applied []byte) {
	t.Helper()
	var values map[string]any
	if err := yaml.Unmarshal(applied, &values); err != nil {
		t.Fatal(err)
	}
	entities := values["config"].(map[string]any)["entities"].([]any)
	cluster := entities[0].(map[string]any)
	for field, want := range map[string]any{
		"spec_schema_name":    "cluster",
		"require_spec_schema": true,
		"name_min_len":        3,
		"name_max_len":        63,
	} {
		if cluster[field] != want {
			t.Errorf("cluster %s = %v, want %v", field, cluster[field], want)
		}
	}
	references, ok := entities[1].(map[string]any)["references"].([]any)
	if !ok || len(references) != 1 {
		t.Fatalf("nodepool references lost on round-trip: %v", entities[1])
	}
	reference, ok := references[0].(map[string]any)
	if !ok {
		t.Fatalf("reference is not a mapping: %v", references[0])
	}
	for field, want := range map[string]any{"name": "cluster", "target_kind": "Cluster", "min": 1, "max": 1} {
		if reference[field] != want {
			t.Errorf("reference %s = %v, want %v", field, reference[field], want)
		}
	}
}

// Exercise the real get/patch/upgrade/ready/restore call chain with a local Helm
// stand-in. No Kubernetes or Helm installation is used by these tests.
func TestUpgradeAndRestoreDescriptorAdapters(t *testing.T) {
	const originalValues = `image:
  tag: keep-this
config:
  server:
    jwt:
      enabled: true
  entities:
    - kind: Cluster
      plural: clusters
      required_adapters: [cl-base]
      spec_schema_name: cluster
      require_spec_schema: true
      name_min_len: 3
      name_max_len: 63
    - kind: NodePool
      plural: nodepools
      parent_kind: Cluster
      on_parent_delete: cascade
      required_adapters: [np-base]
      references:
        - name: cluster
          target_kind: Cluster
          min: 1
          max: 1
`
	dir := t.TempDir()
	input := filepath.Join(dir, "input.yaml")
	applied := filepath.Join(dir, "applied.yaml")
	if err := os.WriteFile(input, []byte(originalValues), 0600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
set -eu
case "$1 $2" in
  "get values")
    cat "$HELM_TEST_INPUT"
    echo 'warning: this is stderr, not YAML' >&2
    ;;
  "upgrade hyperfleet-api")
    while [ "$#" -gt 0 ]; do
      if [ "$1" = "--values" ]; then cp "$2" "$HELM_TEST_APPLIED"; exit 0; fi
      shift
    done
    exit 2
    ;;
  *) exit 3 ;;
esac
`
	// #nosec G306 -- this private test fixture must be executable by exec.CommandContext.
	if err := os.WriteFile(filepath.Join(dir, "helm"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HELM_TEST_INPUT", input)
	t.Setenv("HELM_TEST_APPLIED", applied)
	readyCalls := 0
	h := newResourceTestHelper(t, func(w http.ResponseWriter, r *http.Request) {
		readyCalls++
		if r.Method != http.MethodGet || r.URL.Path != "/api/hyperfleet/v1/nodepools" {
			t.Errorf("readiness still tied to the wrong entity: %s %s", r.Method, r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"items":[]}`)
	})
	e := h.Entities.MustGet("NodePool")
	e.RequiredAdapters = []string{"np-base"}
	baseline := h.GetAPIRequiredAdapters(e)
	updated := append(append([]string{}, baseline...), "np-crash")
	if err := h.UpgradeAPIRequiredAdapters(t.Context(), e, "chart", "namespace", updated); err != nil {
		t.Fatal(err)
	}
	assertApplied := func(want []string) {
		t.Helper()
		// #nosec G304 -- applied is a fixed filename inside this test's t.TempDir().
		data, err := os.ReadFile(applied)
		if err != nil {
			t.Fatal(err)
		}
		// Compare raw YAML maps, not apiValues. Decoding both sides through the
		// same struct would let a loss in the inline catch-all cancel out, and
		// the fixture carries entity fields this package does not model.
		var got, before map[string]any
		if err := yaml.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal([]byte(originalValues), &before); err != nil {
			t.Fatal(err)
		}
		entities := before["config"].(map[string]any)["entities"].([]any)
		patched := make([]any, len(want))
		for i, adapter := range want {
			patched[i] = adapter
		}
		entities[1].(map[string]any)["required_adapters"] = patched
		if !reflect.DeepEqual(got, before) {
			t.Fatalf("Helm round-trip changed unrelated values:\n%s", data)
		}
		assertUnmodeledEntityFields(t, data)
	}
	assertApplied(updated)
	if err := h.RestoreAPIRequiredAdaptersWithRetry(t.Context(), e, "chart", "namespace", baseline, 1); err != nil {
		t.Fatal(err)
	}
	assertApplied([]string{"np-base"})
	if readyCalls != 2 || e.RequiredAdapters[0] != "np-base" {
		t.Fatalf("readiness calls=%d, baseline=%v", readyCalls, e.RequiredAdapters)
	}
	missing := e
	missing.Kind = "NotDeployed"
	if err := h.UpgradeAPIRequiredAdapters(t.Context(), missing, "chart", "namespace", nil); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing entity silently accepted: %v", err)
	}
	assertApplied([]string{"np-base"})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := h.RestoreAPIRequiredAdaptersWithRetry(ctx, e, "chart", "namespace", baseline, 2); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf("restore lost cancellation: %v", err)
	}
}
