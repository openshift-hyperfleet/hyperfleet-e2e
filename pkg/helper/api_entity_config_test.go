package helper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
)

const clusterKind = "Cluster"

// installFakeHelmScript puts an executable helm stand-in first on PATH. Tests use
// it to exercise the real exec path without a cluster or a Helm installation.
func installFakeHelmScript(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	// #nosec G306 -- the stand-in must be executable.
	if err := os.WriteFile(filepath.Join(dir, "helm"), []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// writeValuesFile marshals deployed entities as Helm values and points the
// stand-in helm at them. The stand-in also writes to stderr, proving the reader
// keeps warnings out of the YAML it parses.
func writeValuesFile(t *testing.T, deployed []apiEntity) {
	t.Helper()
	var values apiValues
	values.Config.Entities = deployed
	data, err := yaml.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "values.yaml")
	// #nosec G306 -- a private test fixture read by the helm stand-in.
	if writeErr := os.WriteFile(path, data, 0600); writeErr != nil {
		t.Fatal(writeErr)
	}
	installFakeHelmScript(t, "cat \"$HELM_TEST_VALUES\"\necho 'warning: stderr is not YAML' >&2\n")
	t.Setenv("HELM_TEST_VALUES", path)
}

func writeRawValues(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "values.yaml")
	// #nosec G306 -- a private test fixture read by the helm stand-in.
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	installFakeHelmScript(t, "cat \"$HELM_TEST_VALUES\"\n")
	t.Setenv("HELM_TEST_VALUES", path)
}

// requireDriftError fails unless err is non-nil and its message contains every
// fragment. The release and namespace context stays at the call sites that
// expect it, because the early input errors do not include it.
func requireDriftError(t *testing.T, err error, fragments ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("drift accepted")
	}
	for _, want := range fragments {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%s", want, err)
		}
	}
}

func testExpectedEntities() []descriptor.EntityDescriptor {
	return []descriptor.EntityDescriptor{
		{Kind: "Parent", Plural: "parents"},
		{Kind: "CascadeChild", Plural: "cascadechildren", ParentKind: "Parent", OnParentDelete: "cascade"},
		{Kind: "Cluster", Plural: "clusters"},
	}
}

func matchingDeployedEntities() []apiEntity {
	return []apiEntity{
		{Kind: clusterKind, Plural: "clusters"},
		{Kind: "CascadeChild", Plural: "cascadechildren", ParentKind: "Parent", OnParentDelete: "cascade"},
		{Kind: "Parent", Plural: "parents"},
	}
}

func TestValidateAPIEntityConfigAcceptsReorderedDeploymentWithExtraKinds(t *testing.T) {
	deployed := append(matchingDeployedEntities(),
		apiEntity{Kind: "Extra", Plural: "extras"},
	)
	writeValuesFile(t, deployed)

	if err := ValidateAPIEntityConfig(t.Context(), "e2e-ns", testExpectedEntities()); err != nil {
		t.Fatalf("matching deployment rejected: %v", err)
	}
}

func TestValidateAPIEntityConfigAcceptsCanonicalDescriptorTable(t *testing.T) {
	rows := config.EntityDescriptors()
	deployed := make([]apiEntity, 0, len(rows))
	for _, row := range rows {
		deployed = append(deployed, apiEntity{
			Kind:           row.Kind,
			Plural:         row.Plural,
			ParentKind:     row.ParentKind,
			OnParentDelete: row.OnParentDelete,
		})
	}
	slices.Reverse(deployed)
	writeValuesFile(t, deployed)

	if err := ValidateAPIEntityConfig(t.Context(), "e2e-ns", rows); err != nil {
		t.Fatalf("canonical descriptor table rejected: %v", err)
	}
}

func TestValidateAPIEntityConfigReportsDrift(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func([]apiEntity) []apiEntity
		want      []string
		wantCount int
	}{
		{
			name: "missing expected kind", wantCount: 1,
			mutate: func(entities []apiEntity) []apiEntity {
				return slices.DeleteFunc(entities, func(e apiEntity) bool { return e.Kind == "Parent" })
			},
			want: []string{"Parent: missing from config.entities"},
		},
		{
			name: "wrong plural", wantCount: 1,
			mutate: func(entities []apiEntity) []apiEntity {
				entities[0].Plural = "cluster"
				return entities
			},
			want: []string{`Cluster: plural: deployed "cluster", expected "clusters"`},
		},
		{
			name: "kind with surrounding whitespace is not normalized", wantCount: 1,
			mutate: func(entities []apiEntity) []apiEntity {
				entities[0].Kind = " Cluster "
				return entities
			},
			want: []string{"Cluster: missing from config.entities"},
		},
		{
			name: "wrong parent kind", wantCount: 1,
			mutate: func(entities []apiEntity) []apiEntity {
				entities[1].ParentKind = clusterKind
				return entities
			},
			want: []string{`CascadeChild: parent_kind: deployed "Cluster", expected "Parent"`},
		},
		{
			name: "omitted child policy does not pass", wantCount: 1,
			mutate: func(entities []apiEntity) []apiEntity {
				entities[1].OnParentDelete = ""
				return entities
			},
			want: []string{`CascadeChild: on_parent_delete: deployed "", expected "cascade"`},
		},
		{
			name: "wrong child policy", wantCount: 1,
			mutate: func(entities []apiEntity) []apiEntity {
				entities[1].OnParentDelete = "restrict"
				return entities
			},
			want: []string{`CascadeChild: on_parent_delete: deployed "restrict", expected "cascade"`},
		},
		{
			name: "root with parent metadata", wantCount: 2,
			mutate: func(entities []apiEntity) []apiEntity {
				entities[2].ParentKind = clusterKind
				entities[2].OnParentDelete = "cascade"
				return entities
			},
			want: []string{
				`Parent: parent_kind: deployed "Cluster", expected ""`,
				`Parent: on_parent_delete: deployed "cascade", expected ""`,
			},
		},
		{
			name: "duplicate kind", wantCount: 1,
			mutate: func(entities []apiEntity) []apiEntity {
				return append(entities, apiEntity{Kind: clusterKind, Plural: "clusters"})
			},
			want: []string{"Cluster: duplicate kind in config.entities"},
		},
		{
			name: "duplicate plural", wantCount: 1,
			mutate: func(entities []apiEntity) []apiEntity {
				return append(entities, apiEntity{Kind: "Impostor", Plural: "clusters"})
			},
			want: []string{`Impostor: plural "clusters" is already used by kind Cluster`},
		},
		{
			name: "entry without kind", wantCount: 1,
			mutate: func(entities []apiEntity) []apiEntity {
				return append(entities, apiEntity{Plural: "orphans"})
			},
			want: []string{"config.entities has an entry without a kind"},
		},
		{
			name: "entry without plural", wantCount: 2,
			mutate: func(entities []apiEntity) []apiEntity {
				entities[0].Plural = ""
				return entities
			},
			want: []string{"Cluster: plural is empty in config.entities"},
		},
		{
			name: "no entities at all", wantCount: 1,
			mutate: func([]apiEntity) []apiEntity { return nil },
			want:   []string{"config.entities is missing or empty in the deployed Helm values"},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			deployed := tt.mutate(matchingDeployedEntities())
			writeValuesFile(t, deployed)
			err := ValidateAPIEntityConfig(t.Context(), "e2e-ns", testExpectedEntities())
			requireDriftError(t, err, tt.want...)
			if count := fmt.Sprintf("(%d difference(s))", tt.wantCount); !strings.Contains(err.Error(), count) {
				t.Errorf("error missing %q:\n%s", count, err)
			}
			for _, fragment := range []string{`release "hyperfleet-api"`, `namespace "e2e-ns"`} {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("error missing context %q:\n%s", fragment, err)
				}
			}
		})
	}
}

func TestValidateAPIEntityConfigIsDeterministic(t *testing.T) {
	deployed := matchingDeployedEntities()
	deployed[1].ParentKind = clusterKind
	deployed[1].OnParentDelete = "restrict"
	deployed[2].Plural = "parent"
	writeValuesFile(t, deployed)

	first := ValidateAPIEntityConfig(t.Context(), "e2e-ns", testExpectedEntities())
	second := ValidateAPIEntityConfig(t.Context(), "e2e-ns", testExpectedEntities())
	if first == nil || second == nil {
		t.Fatal("drift accepted")
	}
	if first.Error() != second.Error() {
		t.Fatalf("non-deterministic report:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	cascade := strings.Index(first.Error(), "CascadeChild: parent_kind")
	policy := strings.Index(first.Error(), "CascadeChild: on_parent_delete")
	if cascade < 0 || policy < 0 || cascade > policy {
		t.Fatalf("field order changed:\n%s", first)
	}
}

func TestValidateAPIEntityConfigRejectsMalformedValues(t *testing.T) {
	writeRawValues(t, "config:\n  entities: not-a-list\n")
	requireDriftError(t, ValidateAPIEntityConfig(t.Context(), "e2e-ns", testExpectedEntities()),
		"failed to parse helm values")

	writeRawValues(t, "config:\n  entities: null\n")
	requireDriftError(t, ValidateAPIEntityConfig(t.Context(), "e2e-ns", testExpectedEntities()),
		"missing or empty")
}

func TestValidateAPIEntityConfigReportsHelmFailure(t *testing.T) {
	installFakeHelmScript(t, "echo 'Error: cluster unreachable' >&2\nexit 1\n")
	requireDriftError(t, ValidateAPIEntityConfig(t.Context(), "e2e-ns", testExpectedEntities()),
		`release "hyperfleet-api"`, `namespace "e2e-ns"`, "cluster unreachable")
}

// Real deployments carry entity fields the suite does not model: spec schema
// names, name length limits and references. The guard must keep reading them
// through the inline catch-all without turning them into drift.
func TestValidateAPIEntityConfigIgnoresUnmodeledEntityFields(t *testing.T) {
	rows := config.EntityDescriptors()
	deployed := make([]apiEntity, 0, len(rows))
	for _, row := range rows {
		deployed = append(deployed, apiEntity{
			Kind:           row.Kind,
			Plural:         row.Plural,
			ParentKind:     row.ParentKind,
			OnParentDelete: row.OnParentDelete,
			Rest: map[string]any{
				"spec_schema_name":    strings.ToLower(row.Kind),
				"require_spec_schema": true,
				"name_min_len":        3,
				"name_max_len":        63,
				"references": []any{
					map[string]any{"name": "target", "target_kind": "Target", "min": 0, "max": 1},
				},
			},
		})
	}
	writeValuesFile(t, deployed)

	if err := ValidateAPIEntityConfig(t.Context(), "e2e-ns", rows); err != nil {
		t.Fatalf("unmodeled entity fields reported as drift: %v", err)
	}
}

// The empty plural case counts as two differences on purpose: the structural
// line explains why the row is unusable and the field line names the expected
// value. Pin both so the report cannot silently change shape.
func TestAPIEntityDiffsPinsEmptyPluralShape(t *testing.T) {
	t.Parallel()
	deployed := matchingDeployedEntities()
	deployed[0].Plural = ""

	got := apiEntityDiffs(testExpectedEntities(), deployed)
	want := []string{
		"Cluster: plural is empty in config.entities",
		`Cluster: plural: deployed "", expected "clusters"`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("diffs = %#v, want %#v", got, want)
	}
}

// Structural lines come first, then the field lines in kind order, so the same
// drift always produces the same report.
func TestAPIEntityDiffsPinsReportOrder(t *testing.T) {
	t.Parallel()
	deployed := []apiEntity{
		{Kind: "Parent", Plural: "parents"},
		{Kind: "CascadeChild", Plural: "cascadechildren", ParentKind: clusterKind, OnParentDelete: "restrict"},
		{Kind: "Cluster", Plural: "cluster"},
	}

	got := apiEntityDiffs(testExpectedEntities(), deployed)
	want := []string{
		`CascadeChild: parent_kind: deployed "Cluster", expected "Parent"`,
		`CascadeChild: on_parent_delete: deployed "restrict", expected "cascade"`,
		`Cluster: plural: deployed "cluster", expected "clusters"`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("diffs = %#v, want %#v", got, want)
	}
}

func TestValidateAPIEntityConfigHonorsCancellation(t *testing.T) {
	installFakeHelmScript(t, "cat \"$HELM_TEST_VALUES\"\n")
	t.Setenv("HELM_TEST_VALUES", filepath.Join(t.TempDir(), "unused.yaml"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := ValidateAPIEntityConfig(ctx, "e2e-ns", testExpectedEntities())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestValidateAPIEntityConfigRequiresInputs(t *testing.T) {
	if err := ValidateAPIEntityConfig(t.Context(), "e2e-ns", nil); err == nil ||
		!strings.Contains(err.Error(), "descriptor table is empty") {
		t.Fatalf("empty expected table accepted: %v", err)
	}
	if err := ValidateAPIEntityConfig(t.Context(), "  ", testExpectedEntities()); err == nil ||
		!strings.Contains(err.Error(), "namespace is required") {
		t.Fatalf("empty namespace accepted: %v", err)
	}
}
