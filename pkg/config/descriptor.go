package config

import (
	"cmp"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
)

// entityKeys returns the config key of every kind in the table, in table order.
// A key names the kind under adapters.<key> and timeouts.<key>, and in the env vars
// HYPERFLEET_ADAPTERS_<KEY> and HYPERFLEET_TIMEOUTS_<KEY>_RECONCILED/_DELETED.
func entityKeys() []string {
	var keys []string
	for _, row := range EntityDescriptors() {
		keys = append(keys, entityKey(row.Kind))
	}
	return keys
}

// entityKey is the kind in lower case, because viper lowercases every config key.
func entityKey(kind string) string {
	return strings.ToLower(kind)
}

// defaultEntityTimeouts is what CI runs the reconciling kinds with.
var defaultEntityTimeouts = descriptor.EntityTimeouts{Reconciled: 2 * time.Minute, Deleted: 2 * time.Minute}

// EntityDescriptors returns a copy of the single descriptor table, with no config applied. Each row holds routing,
// cleanup and the default runtime settings; config only overrides adapters and
// timeouts by kind. Adding a descriptor needs one row here and no config field,
// default constant, env alias, helper or kind-dispatch code.
//
// Parent and CascadeChild wait for hard-delete, so they carry default timeouts.
// Their adapter release names do not exist yet (HYPERFLEET-1729). Registration
// alone does not make a fixture runnable or contact its API route.
func EntityDescriptors() []descriptor.EntityDescriptor {
	return []descriptor.EntityDescriptor{
		{Kind: "Parent", Plural: "parents",
			Timeouts: defaultEntityTimeouts,
			Cleanup:  descriptor.CleanupPolicy{WaitForHardDelete: true, IgnoreNotFound: true}},
		{Kind: "CascadeChild", Plural: "cascadechildren", ParentKind: "Parent", OnParentDelete: "cascade",
			Timeouts: defaultEntityTimeouts,
			Cleanup:  descriptor.CleanupPolicy{WaitForHardDelete: true, IgnoreNotFound: true}},
		{Kind: "RestrictChild", Plural: "restrictchildren", ParentKind: "Parent", OnParentDelete: "restrict",
			Cleanup: descriptor.CleanupPolicy{IgnoreNotFound: true}},
		{Kind: "Referencer", Plural: "referencers", Cleanup: descriptor.CleanupPolicy{IgnoreNotFound: true}},
		{Kind: "Target", Plural: "targets", Cleanup: descriptor.CleanupPolicy{IgnoreNotFound: true}},
		{
			Kind: "Cluster", Plural: "clusters",
			// The release names hyperfleet-infra gives the cluster adapters.
			RequiredAdapters: []string{"cl-namespace", "cl-job", "cl-deployment", DefaultDesireAdapter},
			Timeouts:         defaultEntityTimeouts,
			// Reconciliation thresholds are calibrated from Prow tier1-nightly
			// baselines (hyperfleet-dev-prow), with a ~1.5x margin to absorb CI
			// run-to-run variance.
			//
			// The update, delete and cascade-delete thresholds are raised ahead of
			// a rebaseline. Since the e2e API requires cl-desire, the remote adapter needs
			// extra sentinel rounds to see the applier's result: one on update, two on
			// delete (ConfigMap, then Namespace, then Finalized). Specs poll every 10s, so
			// readings land on 10s steps: update read 70s on kind, and every reconciled
			// cluster in the 2026-10-14 tier1-nightly took 50.2s from delete to hard-delete.
			// Recalibrate once Maestro is gone and the perf specs poll finer than 10s.
			Thresholds: descriptor.EntityThresholds{
				Read: thresholdAPIRead, List: thresholdAPIList,
				CreateReconciled: 90 * time.Second, // baseline ~60s
				UpdateReconciled: 90 * time.Second, // was 60s, baseline ~40s before cl-desire
				Deleted:          75 * time.Second, // was 60s, baseline ~40s before cl-desire
				CascadeDeleted:   90 * time.Second, // was 75s, baseline ~50s before cl-desire
			},
			Cleanup: descriptor.CleanupPolicy{WaitForHardDelete: true, IgnoreNotFound: true},
		},
		{
			Kind: "NodePool", Plural: "nodepools", ParentKind: "Cluster", OnParentDelete: "cascade",
			// The release name hyperfleet-infra gives the nodepool adapter.
			RequiredAdapters: []string{"np-configmap"},
			Timeouts:         defaultEntityTimeouts,
			// Calibrated from Prow tier1-nightly baselines (hyperfleet-dev-prow), with
			// a ~2.25x margin: the lower baselines make the same absolute jitter as
			// Cluster a higher percentage.
			Thresholds: descriptor.EntityThresholds{
				Read: thresholdAPIRead, List: thresholdAPIList,
				CreateReconciled: 45 * time.Second, // baseline ~20s
				Deleted:          45 * time.Second, // baseline ~20s
			},
			Cleanup: descriptor.CleanupPolicy{WaitForHardDelete: true},
		},
		{
			Kind: "Channel", Plural: "channels",
			Thresholds: descriptor.EntityThresholds{
				Read: thresholdAPIRead, List: thresholdAPIList,
				Create: thresholdAPICreate, Update: thresholdAPIUpdate, Delete: thresholdAPIDelete,
			},
			Cleanup: descriptor.CleanupPolicy{IgnoreNotFound: true, ChildKinds: []string{"Version"}},
		},
		{
			Kind: "Version", Plural: "versions", ParentKind: "Channel", OnParentDelete: "restrict",
			Thresholds: descriptor.EntityThresholds{
				Read: thresholdAPIRead, List: thresholdAPIList,
				Create: thresholdAPINestedWrite, Update: thresholdAPINestedWrite, Delete: thresholdAPINestedWrite,
			},
			Cleanup: descriptor.CleanupPolicy{IgnoreNotFound: true},
		},
		{
			Kind: "WifConfig", Plural: "wifconfigs",
			Thresholds: descriptor.EntityThresholds{
				Read: thresholdAPIRead, List: thresholdAPIList,
				Create: thresholdAPICreate, Update: thresholdAPIUpdate, Delete: thresholdAPIDelete,
			},
			Cleanup: descriptor.CleanupPolicy{IgnoreNotFound: true, IgnoreConflict: true},
		},
	}
}

// timeoutFields maps each field name under timeouts.<key> to its EntityTimeouts field.
func timeoutFields(t *descriptor.EntityTimeouts) map[string]*time.Duration {
	return map[string]*time.Duration{"reconciled": &t.Reconciled, "deleted": &t.Deleted}
}

// sharedTimeoutKeys returns the keys under timeouts that TimeoutsConfig owns, such
// as adapter, so that they are not taken for kinds.
func sharedTimeoutKeys() []string {
	var keys []string
	for field := range reflect.TypeFor[TimeoutsConfig]().Fields() {
		key, _, _ := strings.Cut(field.Tag.Get(TagMapstructure), ",")
		keys = append(keys, strings.ToLower(key))
	}
	return keys
}

// loadEntityCatalog overlays the adapters.<key> and timeouts.<key> values viper
// holds (config file, env vars) on the table defaults and builds the catalog:
//   - an adapters key that is set replaces the row's list and must name an adapter;
//   - a non-zero timeout replaces that one field.
//
// It fails on a kind or timeout field that is not in the table, such as the typo
// timeouts.nodepools, and on a scalar or list where a map belongs, such as
// timeouts.cluster: 5m. Viper would otherwise ignore both.
func loadEntityCatalog() (*descriptor.Catalog, error) {
	rows := EntityDescriptors()
	keys := entityKeys()
	shared := sharedTimeoutKeys()
	for _, section := range []string{"adapters", "timeouts"} {
		values, err := viperMap(section)
		if err != nil {
			return nil, err
		}
		for _, key := range slices.Sorted(maps.Keys(values)) {
			if !slices.Contains(keys, key) && (section != "timeouts" || !slices.Contains(shared, key)) {
				return nil, fmt.Errorf("unknown kind %q under %s; valid kinds: %s", key, section, strings.Join(keys, ", "))
			}
		}
	}

	for i := range rows {
		row := &rows[i]
		key := entityKey(row.Kind)
		if path := "adapters." + key; viper.IsSet(path) {
			if row.RequiredAdapters = viperStringSlice(path); len(row.RequiredAdapters) == 0 {
				return nil, fmt.Errorf("%s is set but names no adapter; remove it to keep the default", path)
			}
		}
		fields := timeoutFields(&row.Timeouts)
		values, err := viperMap("timeouts." + key)
		if err != nil {
			return nil, err
		}
		for _, name := range slices.Sorted(maps.Keys(values)) {
			if fields[name] == nil {
				return nil, fmt.Errorf("unknown field %q under timeouts.%s; valid fields: %s",
					name, key, strings.Join(slices.Sorted(maps.Keys(fields)), ", "))
			}
		}
		for name, field := range fields {
			// UnmarshalKey decodes like the other durations in Config: a value
			// needs a unit, so "300" or 300 fails instead of becoming 300ns.
			path := "timeouts." + key + "." + name
			var timeout time.Duration
			if err := viper.UnmarshalKey(path, &timeout, rejectBareDurations); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			*field = cmp.Or(timeout, *field)
		}
	}
	return descriptor.NewCatalog(rows)
}

// viperMap reads the map at path, or nil when it is not set. It fails on a scalar
// or a list, such as timeouts.cluster: 5m, which GetStringMap would turn into an
// empty map and so hide from the key checks.
func viperMap(path string) (map[string]any, error) {
	switch value := viper.Get(path).(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return value, nil
	default:
		return nil, fmt.Errorf("%s must be a map, got %v", path, value)
	}
}

// viperStringSlice reads a string list. A string (as an env var gives) is split
// on commas and whitespace, so "a,b", "a, b" and "a b" all name a and b;
// GetStringSlice alone splits on whitespace only and would turn "a, b" into "a,"
// and "b". Each name is trimmed and blank names, as a trailing comma leaves, dropped.
func viperStringSlice(path string) []string {
	values := viper.GetStringSlice(path)
	if raw, ok := viper.Get(path).(string); ok {
		values = strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	}
	var names []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			names = append(names, value)
		}
	}
	return names
}
