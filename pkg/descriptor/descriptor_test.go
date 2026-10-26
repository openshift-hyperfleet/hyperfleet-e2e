package descriptor

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPathsPreserveIDConcatenation(t *testing.T) {
	t.Parallel()
	catalog, err := NewCatalog([]EntityDescriptor{
		{Kind: "Root", Plural: "roots"},
		{Kind: "Child", Plural: "children", ParentKind: "Root", OnParentDelete: "cascade"},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, child := catalog.MustGet("Root"), catalog.MustGet("Child")
	for _, id := range []string{"id", "", "..", "a/b", "id?search=x", "id#fragment"} {
		if got := root.ResourcePath("", id); got != "roots/"+id {
			t.Errorf("root path changed for ID %q: %q", id, got)
		}
		if got := child.ResourcePath(id, id); got != "roots/"+id+"/children/"+id {
			t.Errorf("nested path changed for ID %q: %q", id, got)
		}
	}
	if _, err := catalog.Lookup("Unknown"); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestCatalogRejectsInvalidDefinitions(t *testing.T) {
	t.Parallel()
	root := EntityDescriptor{Kind: "Root", Plural: "roots"}
	child := EntityDescriptor{Kind: "Child", Plural: "children", ParentKind: "Root", OnParentDelete: "cascade"}
	cases := []struct {
		name string
		want string
		rows []EntityDescriptor
	}{
		{name: "empty kind", rows: []EntityDescriptor{{Plural: "roots"}}, want: "kind"},
		{name: "bad plural", rows: []EntityDescriptor{{Kind: "Root", Plural: "a/b"}}, want: "plural"},
		{name: "duplicate kind", rows: []EntityDescriptor{root, root}, want: "duplicate entity kind"},
		{
			name: "duplicate plural",
			rows: []EntityDescriptor{root, {Kind: "Other", Plural: "roots"}},
			want: "duplicate entity plural",
		},
		{name: "missing parent", rows: []EntityDescriptor{child}, want: "unknown parent"},
		{
			name: "wrong parent plural",
			rows: []EntityDescriptor{
				root,
				{
					Kind:           "Child",
					Plural:         "children",
					ParentKind:     "Root",
					ParentPlural:   "wrong",
					OnParentDelete: "cascade",
				},
			},
			want: "parent plural",
		},
		{
			name: "parent cycle",
			rows: []EntityDescriptor{
				{Kind: "Root", Plural: "roots", ParentKind: "Child", OnParentDelete: "cascade"},
				child,
			},
			want: "cyclic",
		},
		{
			name: "root policy",
			rows: []EntityDescriptor{{Kind: "Root", Plural: "roots", OnParentDelete: "cascade"}},
			want: "parent metadata",
		},
		{
			name: "missing policy",
			rows: []EntityDescriptor{root, {Kind: "Child", Plural: "children", ParentKind: "Root"}},
			want: "deletion policy",
		},
		{
			name: "unrelated cleanup child",
			rows: []EntityDescriptor{
				{Kind: "Other", Plural: "others", Cleanup: CleanupPolicy{ChildKinds: []string{"Child"}}},
				root,
				child,
			},
			want: "does not belong",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewCatalog(tc.rows)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCatalogValidatesRuntimeSettings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		wantErr  string // empty when the row is valid
		adapters []string
		timeouts EntityTimeouts
		wait     bool
	}{
		{name: "hard-delete with zero deleted timeout", wait: true, wantErr: "positive deleted timeout"},
		{name: "hard-delete with positive deleted timeout", wait: true, timeouts: EntityTimeouts{Deleted: time.Second}},
		{name: "no hard-delete polling and no timeouts"},
		{
			name: "negative deleted timeout", timeouts: EntityTimeouts{Deleted: -time.Second},
			wantErr: "timeouts must not be negative",
		},
		{
			name: "negative reconciled timeout", timeouts: EntityTimeouts{Reconciled: -time.Second},
			wantErr: "timeouts must not be negative",
		},
		{
			name: "adapters without a reconciled timeout", adapters: []string{"a"},
			wantErr: "required adapters but no reconciled timeout",
		},
		{
			name: "adapters with a reconciled timeout", adapters: []string{"a"},
			timeouts: EntityTimeouts{Reconciled: time.Second},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewCatalog([]EntityDescriptor{{
				Kind: "Fixture", Plural: "fixtures",
				RequiredAdapters: tc.adapters,
				Timeouts:         tc.timeouts,
				Cleanup:          CleanupPolicy{WaitForHardDelete: tc.wait},
			}})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "Fixture") || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to name Fixture and contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestCatalogOwnsMutableSettings(t *testing.T) {
	t.Parallel()
	rows := []EntityDescriptor{
		{
			Kind:             "Root",
			Plural:           "roots",
			RequiredAdapters: []string{"success"},
			Timeouts:         EntityTimeouts{Reconciled: time.Second},
			Cleanup:          CleanupPolicy{ChildKinds: []string{"Child"}},
		},
		{Kind: "Child", Plural: "children", ParentKind: "Root", OnParentDelete: "restrict"},
	}
	catalog, err := NewCatalog(rows)
	if err != nil {
		t.Fatal(err)
	}
	rows[0].RequiredAdapters[0] = "changed-input"
	rows[0].Cleanup.ChildKinds[0] = "changed-input"
	first := catalog.MustGet("Root")
	first.RequiredAdapters[0] = "changed-result"
	first.Cleanup.ChildKinds[0] = "changed-result"
	second := catalog.MustGet("Root")
	if !reflect.DeepEqual(second.RequiredAdapters, []string{"success"}) ||
		!reflect.DeepEqual(second.Cleanup.ChildKinds, []string{"Child"}) {
		t.Fatalf("catalog settings were mutated: %+v", second)
	}
}
