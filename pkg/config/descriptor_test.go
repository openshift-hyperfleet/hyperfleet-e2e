package config

import (
	"testing"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
)

func TestEntityCatalogPreservesResolvedSettings(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Adapters: AdaptersConfig{Cluster: []string{"custom-cl"}, NodePool: []string{"custom-np"}},
		Timeouts: TimeoutsConfig{
			Cluster:  ClusterTimeouts{Reconciled: 31 * time.Second, Deleted: 47 * time.Second},
			NodePool: NodePoolTimeouts{Reconciled: 53 * time.Second, Deleted: 61 * time.Second},
		},
	}
	catalog, err := cfg.EntityCatalog()
	if err != nil {
		t.Fatal(err)
	}
	cl, np := catalog.MustGet("Cluster"), catalog.MustGet("NodePool")
	if cl.RequiredAdapters[0] != "custom-cl" || np.RequiredAdapters[0] != "custom-np" ||
		cl.Timeouts.Reconciled != cfg.Timeouts.Cluster.Reconciled || cl.Timeouts.Deleted != cfg.Timeouts.Cluster.Deleted ||
		np.Timeouts.Reconciled != cfg.Timeouts.NodePool.Reconciled || np.Timeouts.Deleted != cfg.Timeouts.NodePool.Deleted {
		t.Fatal("resolved adapters or timeouts changed")
	}
	cl.RequiredAdapters[0] = "test-only"
	if cfg.Adapters.Cluster[0] != "custom-cl" || catalog.MustGet("Cluster").RequiredAdapters[0] != "custom-cl" {
		t.Fatal("descriptor leaked mutable adapters")
	}
	if cl.Thresholds.CreateReconciled != ThresholdClusterCreateReconciled ||
		cl.Thresholds.UpdateReconciled != ThresholdClusterUpdateReconciled ||
		cl.Thresholds.Deleted != ThresholdClusterDeleted || cl.Thresholds.CascadeDeleted != ThresholdClusterCascadeDeleted ||
		np.Thresholds.CreateReconciled != ThresholdNodePoolCreateReconciled || np.Thresholds.Deleted != ThresholdNodePoolDeleted {
		t.Fatal("reconciliation thresholds changed")
	}
	for _, kind := range []string{"Cluster", "NodePool", "Channel", "Version", "WifConfig"} {
		e := catalog.MustGet(kind)
		if e.Thresholds.Read != ThresholdAPIRead || e.Thresholds.List != ThresholdAPIList {
			t.Fatalf("%s read thresholds changed", kind)
		}
	}
	for _, kind := range []string{"Channel", "WifConfig"} {
		e := catalog.MustGet(kind)
		if e.Thresholds.Create != ThresholdAPICreate || e.Thresholds.Update != ThresholdAPIUpdate || e.Thresholds.Delete != ThresholdAPIDelete {
			t.Fatalf("%s write thresholds changed", kind)
		}
	}
	v := catalog.MustGet("Version")
	if v.Thresholds.Create != ThresholdAPINestedWrite || v.Thresholds.Update != ThresholdAPINestedWrite || v.Thresholds.Delete != ThresholdAPINestedWrite {
		t.Fatal("nested write thresholds changed")
	}
	for _, tc := range []struct {
		kind    string
		deleted time.Duration
	}{
		{"Parent", DefaultFixtureDeletedTimeout},
		{"CascadeChild", DefaultFixtureDeletedTimeout},
		{"RestrictChild", 0},
		{"Referencer", 0},
		{"Target", 0},
	} {
		e := catalog.MustGet(tc.kind)
		if len(e.RequiredAdapters) != 0 || e.Timeouts != (descriptor.EntityTimeouts{Deleted: tc.deleted}) || e.Thresholds != (descriptor.EntityThresholds{}) {
			t.Fatalf("fixture %s has unexpected runtime settings: %+v", tc.kind, e)
		}
	}
}

func TestEntityCatalogRoutes(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	cfg.applyDefaults()
	catalog, err := cfg.EntityCatalog()
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
	if len(cfg.entityDescriptors()) != len(cases) {
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
	cfg := &Config{}
	cfg.applyDefaults()
	rows := append(cfg.entityDescriptors(), descriptor.EntityDescriptor{
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
