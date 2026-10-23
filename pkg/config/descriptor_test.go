package config

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
)

// loadEntities runs loadEntityCatalog on viper values set in code.
// viper is process-global, so callers must not run in parallel.
func loadEntities(t *testing.T, values map[string]any) (*descriptor.Catalog, error) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	for key, value := range values {
		viper.Set(key, value)
	}
	return loadEntityCatalog()
}

func TestEntityCatalogOverlaysConfigOnRowDefaults(t *testing.T) {
	catalog, err := loadEntities(t, map[string]any{
		"adapters.cluster":                 []string{"custom-cl"},
		"adapters.parent":                  []string{"p-success"},
		"timeouts.cluster.reconciled":      31 * time.Second,
		"timeouts.nodepool.deleted":        61 * time.Second,
		"timeouts.cascadechild.reconciled": 53 * time.Second,
		"timeouts.cascadechild.deleted":    47 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	cl := catalog.MustGet("Cluster")
	if !slices.Equal(cl.RequiredAdapters, []string{"custom-cl"}) {
		t.Errorf("Cluster adapters = %q, want the override", cl.RequiredAdapters)
	}
	if cl.Timeouts != (descriptor.EntityTimeouts{Reconciled: 31 * time.Second, Deleted: defaultEntityTimeouts.Deleted}) {
		t.Errorf("Cluster timeouts = %+v, want reconciled overridden and deleted at the default", cl.Timeouts)
	}
	np := catalog.MustGet("NodePool")
	if !slices.Equal(np.RequiredAdapters, []string{"np-configmap"}) {
		t.Errorf("NodePool adapters = %q, want the row default", np.RequiredAdapters)
	}
	if np.Timeouts != (descriptor.EntityTimeouts{Reconciled: defaultEntityTimeouts.Reconciled, Deleted: 61 * time.Second}) {
		t.Errorf("NodePool timeouts = %+v, want deleted overridden and reconciled at the default", np.Timeouts)
	}
	if p := catalog.MustGet("Parent"); !slices.Equal(p.RequiredAdapters, []string{"p-success"}) || p.Timeouts != defaultEntityTimeouts {
		t.Errorf("Parent = %q %+v, want the adapters override and default timeouts", p.RequiredAdapters, p.Timeouts)
	}
	if cc := catalog.MustGet("CascadeChild"); cc.Timeouts != (descriptor.EntityTimeouts{Reconciled: 53 * time.Second, Deleted: 47 * time.Second}) {
		t.Errorf("CascadeChild timeouts = %+v, want both overridden", cc.Timeouts)
	}

	cl.RequiredAdapters[0] = "test-only"
	if catalog.MustGet("Cluster").RequiredAdapters[0] != "custom-cl" {
		t.Fatal("descriptor shares mutable adapters with callers")
	}
}

func TestEntityCatalogRowDefaults(t *testing.T) {
	t.Parallel()
	catalog, err := descriptor.NewCatalog(EntityDescriptors())
	if err != nil {
		t.Fatal(err)
	}

	cl, np := catalog.MustGet("Cluster"), catalog.MustGet("NodePool")
	if !slices.Equal(cl.RequiredAdapters, []string{"cl-namespace", "cl-job", "cl-deployment", "cl-desire"}) ||
		!slices.Equal(np.RequiredAdapters, []string{"np-configmap"}) {
		t.Errorf("adapters = %q / %q, want the hyperfleet-infra release names", cl.RequiredAdapters, np.RequiredAdapters)
	}
	for kind, want := range map[string]descriptor.EntityTimeouts{
		"Cluster":      {Reconciled: 2 * time.Minute, Deleted: 2 * time.Minute},
		"NodePool":     {Reconciled: 2 * time.Minute, Deleted: 2 * time.Minute},
		"Parent":       {Reconciled: 2 * time.Minute, Deleted: 2 * time.Minute},
		"CascadeChild": {Reconciled: 2 * time.Minute, Deleted: 2 * time.Minute},
	} {
		if got := catalog.MustGet(kind).Timeouts; got != want {
			t.Errorf("%s timeouts = %+v, want %+v", kind, got, want)
		}
	}

	if cl.Thresholds.CreateReconciled != 90*time.Second || cl.Thresholds.UpdateReconciled != 90*time.Second ||
		cl.Thresholds.Deleted != 75*time.Second || cl.Thresholds.CascadeDeleted != 90*time.Second ||
		np.Thresholds.CreateReconciled != 45*time.Second || np.Thresholds.Deleted != 45*time.Second {
		t.Error("reconciliation thresholds changed")
	}
	for _, kind := range []string{"Cluster", "NodePool", "Channel", "Version", "WifConfig"} {
		e := catalog.MustGet(kind)
		if e.Thresholds.Read != 50*time.Millisecond || e.Thresholds.List != 50*time.Millisecond {
			t.Errorf("%s read thresholds changed", kind)
		}
	}
	for _, kind := range []string{"Channel", "WifConfig"} {
		e := catalog.MustGet(kind)
		if e.Thresholds.Create != 60*time.Millisecond || e.Thresholds.Update != 60*time.Millisecond || e.Thresholds.Delete != 60*time.Millisecond {
			t.Errorf("%s write thresholds changed", kind)
		}
	}
	v := catalog.MustGet("Version")
	if v.Thresholds.Create != 75*time.Millisecond || v.Thresholds.Update != 75*time.Millisecond || v.Thresholds.Delete != 75*time.Millisecond {
		t.Error("nested write thresholds changed")
	}

	for _, kind := range []string{"Parent", "CascadeChild"} {
		if e := catalog.MustGet(kind); len(e.RequiredAdapters) != 0 || e.Thresholds != (descriptor.EntityThresholds{}) {
			t.Errorf("fixture %s received invented adapters or thresholds", kind)
		}
	}
	for _, kind := range []string{"RestrictChild", "Referencer", "Target"} {
		e := catalog.MustGet(kind)
		if len(e.RequiredAdapters) != 0 || e.Timeouts != (descriptor.EntityTimeouts{}) || e.Thresholds != (descriptor.EntityThresholds{}) {
			t.Errorf("fixture %s received invented runtime settings", kind)
		}
	}
}

func TestEntityKeys(t *testing.T) {
	t.Parallel()
	keys := entityKeys()
	rows := EntityDescriptors()
	if len(keys) != len(rows) {
		t.Fatalf("got %d keys for %d rows", len(keys), len(rows))
	}
	seen := map[string]bool{}
	for i, key := range keys {
		if key != strings.ToLower(rows[i].Kind) {
			t.Errorf("key %q does not match row %q in table order", key, rows[i].Kind)
		}
		if seen[key] {
			t.Errorf("duplicate config key %q", key)
		}
		seen[key] = true
		// TimeoutsConfig owns these keys under timeouts, so no kind may use them.
		if slices.Contains(sharedTimeoutKeys(), key) {
			t.Errorf("config key %q collides with a TimeoutsConfig field", key)
		}
	}
}

func TestEntityRuntimeValidation(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]any
		wantErr string
	}{
		{
			name:    "negative timeout",
			values:  map[string]any{"timeouts.parent.deleted": "-1s"},
			wantErr: `entity "Parent" timeouts must not be negative`,
		},
		{
			name:    "adapters on a kind without a reconciled timeout",
			values:  map[string]any{"adapters.channel": []string{"ch-adapter"}},
			wantErr: `entity "Channel" has required adapters but no reconciled timeout`,
		},
		{
			name:    "empty adapters list",
			values:  map[string]any{"adapters.cluster": []string{}},
			wantErr: "adapters.cluster is set but names no adapter",
		},
		{
			name:    "empty adapters string",
			values:  map[string]any{"adapters.cluster": " , "},
			wantErr: "adapters.cluster is set but names no adapter",
		},
		{
			name:    "unknown timeout field",
			values:  map[string]any{"timeouts.cluster.reconcile": "5m"},
			wantErr: `unknown field "reconcile" under timeouts.cluster; valid fields: deleted, reconciled`,
		},
		{
			name:    "timeout that is not a duration",
			values:  map[string]any{"timeouts.nodepool.deleted": "soon"},
			wantErr: "timeouts.nodepool.deleted:",
		},
		{
			name:    "timeout without a unit",
			values:  map[string]any{"timeouts.cluster.reconciled": "300"},
			wantErr: "missing unit in duration",
		},
		{
			name:    "timeout as a bare number",
			values:  map[string]any{"timeouts.cluster.reconciled": 300},
			wantErr: "duration 300 has no unit",
		},
		{
			name: "valid overrides",
			values: map[string]any{
				"adapters.channel":            []string{"ch-adapter"},
				"timeouts.channel.reconciled": time.Minute,
				"timeouts.cluster.deleted":    "3m",
			},
		},
		{
			name:   "trailing comma in an adapters list",
			values: map[string]any{"adapters.nodepool": "np-a, np-b,"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadEntities(t, tt.values)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestEntityCatalogRoutes(t *testing.T) {
	t.Parallel()
	catalog, err := descriptor.NewCatalog(EntityDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ kind, parentKind, collection, policy string }{
		{"Parent", "", "parents", ""},
		{"CascadeChild", "Parent", "parents/p/cascadechildren", "cascade"},
		{"RestrictChild", "Parent", "parents/p/restrictchildren", "restrict"},
		{"Referencer", "", "referencers", ""},
		{"Target", "", "targets", ""},
		{"Cluster", "", "clusters", ""},
		{"NodePool", "Cluster", "clusters/p/nodepools", "cascade"},
		{"Channel", "", "channels", ""},
		{"Version", "Channel", "channels/p/versions", "restrict"},
		{"WifConfig", "", "wifconfigs", ""},
	}
	if len(EntityDescriptors()) != len(cases) {
		t.Fatal("update the contract cases when adding a descriptor")
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			e := catalog.MustGet(tc.kind)
			parentID := ""
			if tc.parentKind != "" {
				parentID = "p"
			}
			if e.ParentKind != tc.parentKind || e.OnParentDelete != tc.policy ||
				e.CollectionPath(parentID) != tc.collection || e.ResourcePath(parentID, "id") != tc.collection+"/id" {
				t.Fatalf("incorrect routing metadata: %+v", e)
			}
		})
	}
}

func TestAdditionalDescriptorNeedsOnlyOneRow(t *testing.T) {
	t.Parallel()
	rows := append(EntityDescriptors(), descriptor.EntityDescriptor{
		Kind: "Widget", Plural: "widgets", ParentKind: "Parent", OnParentDelete: "cascade",
		RequiredAdapters: []string{"widget-success"},
		Timeouts:         descriptor.EntityTimeouts{Reconciled: time.Minute, Deleted: time.Minute},
		Thresholds:       descriptor.EntityThresholds{Read: time.Second},
		Cleanup:          descriptor.CleanupPolicy{WaitForHardDelete: true},
	})
	catalog, err := descriptor.NewCatalog(rows)
	if err != nil {
		t.Fatal(err)
	}
	widget := catalog.MustGet("Widget")
	if widget.ResourcePath("parent", "id") != "parents/parent/widgets/id" ||
		len(widget.RequiredAdapters) != 1 || widget.RequiredAdapters[0] != "widget-success" ||
		widget.Timeouts.Deleted != time.Minute || widget.Timeouts.Reconciled != time.Minute ||
		widget.Thresholds.Read != time.Second || !widget.Cleanup.WaitForHardDelete {
		t.Fatalf("additional descriptor lost its row settings: %+v", widget)
	}
}

func TestEntityLogAttrsKeepFlatKeys(t *testing.T) {
	t.Parallel()
	catalog, err := descriptor.NewCatalog(EntityDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	attrs := (&Config{Entities: catalog}).entityLogAttrs()
	got := map[string]any{}
	for i := 0; i+1 < len(attrs); i += 2 {
		got[attrs[i].(string)] = attrs[i+1]
	}
	// Prow log searches use these names, so keep them for cluster and nodepool.
	for key, want := range map[string]any{
		"adapters_cluster":            []string{"cl-namespace", "cl-job", "cl-deployment", "cl-desire"},
		"adapters_nodepool":           []string{"np-configmap"},
		"timeout_cluster_reconciled":  2 * time.Minute,
		"timeout_cluster_deleted":     2 * time.Minute,
		"timeout_nodepool_reconciled": 2 * time.Minute,
		"timeout_nodepool_deleted":    2 * time.Minute,
	} {
		if !reflect.DeepEqual(got[key], want) {
			t.Errorf("%s = %v, want %v", key, got[key], want)
		}
	}
	if _, ok := got["adapters_channel"]; ok {
		t.Error("logged a row with no adapters or timeouts")
	}
}
