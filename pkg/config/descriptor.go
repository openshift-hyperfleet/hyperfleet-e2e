package config

import "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"

// EntityCatalog builds the suite catalog after configuration has been resolved.
// The single table below owns routing, cleanup and runtime settings together.
func (c *Config) EntityCatalog() (*descriptor.Catalog, error) {
	return descriptor.NewCatalog(c.entityDescriptors())
}

// entityDescriptors keeps the current config fields and thresholds in their rows
// until HYPERFLEET-1727 replaces them with kind-keyed config and env bindings.
// Adding a descriptor needs one row here and no helper or kind-dispatch code.
// Fixture rows have a default deletion timeout where cleanup requires it; their
// adapters, reconciliation timeouts and thresholds remain unset. Registration
// alone does not make a fixture runnable or contact its API route.
// TODO(HYPERFLEET-1727): Resolve fixture runtime settings through kind-keyed config
// and environment bindings, replacing the shared fixture deletion default.
func (c *Config) entityDescriptors() []descriptor.EntityDescriptor {
	return []descriptor.EntityDescriptor{
		{Kind: "Parent", Plural: "parents",
			Timeouts: descriptor.EntityTimeouts{Deleted: DefaultFixtureDeletedTimeout},
			Cleanup:  descriptor.CleanupPolicy{WaitForHardDelete: true, IgnoreNotFound: true}},
		{Kind: "CascadeChild", Plural: "cascadechildren", ParentKind: "Parent", OnParentDelete: "cascade",
			Timeouts: descriptor.EntityTimeouts{Deleted: DefaultFixtureDeletedTimeout},
			Cleanup:  descriptor.CleanupPolicy{WaitForHardDelete: true, IgnoreNotFound: true}},
		{Kind: "RestrictChild", Plural: "restrictchildren", ParentKind: "Parent", OnParentDelete: "restrict",
			Cleanup: descriptor.CleanupPolicy{IgnoreNotFound: true}},
		{Kind: "Referencer", Plural: "referencers", Cleanup: descriptor.CleanupPolicy{IgnoreNotFound: true}},
		{Kind: "Target", Plural: "targets", Cleanup: descriptor.CleanupPolicy{IgnoreNotFound: true}},
		{
			Kind: "Cluster", Plural: "clusters",
			RequiredAdapters: c.Adapters.Cluster,
			Timeouts:         descriptor.EntityTimeouts{Reconciled: c.Timeouts.Cluster.Reconciled, Deleted: c.Timeouts.Cluster.Deleted},
			Thresholds: descriptor.EntityThresholds{
				Read: ThresholdAPIRead, List: ThresholdAPIList,
				CreateReconciled: ThresholdClusterCreateReconciled, UpdateReconciled: ThresholdClusterUpdateReconciled,
				Deleted: ThresholdClusterDeleted, CascadeDeleted: ThresholdClusterCascadeDeleted,
			},
			Cleanup: descriptor.CleanupPolicy{WaitForHardDelete: true, IgnoreNotFound: true},
		},
		{
			Kind: "NodePool", Plural: "nodepools", ParentKind: "Cluster", OnParentDelete: "cascade",
			RequiredAdapters: c.Adapters.NodePool,
			Timeouts:         descriptor.EntityTimeouts{Reconciled: c.Timeouts.NodePool.Reconciled, Deleted: c.Timeouts.NodePool.Deleted},
			Thresholds: descriptor.EntityThresholds{
				Read: ThresholdAPIRead, List: ThresholdAPIList,
				CreateReconciled: ThresholdNodePoolCreateReconciled, Deleted: ThresholdNodePoolDeleted,
			},
			Cleanup: descriptor.CleanupPolicy{WaitForHardDelete: true},
		},
		{
			Kind: "Channel", Plural: "channels",
			Thresholds: descriptor.EntityThresholds{
				Read: ThresholdAPIRead, List: ThresholdAPIList,
				Create: ThresholdAPICreate, Update: ThresholdAPIUpdate, Delete: ThresholdAPIDelete,
			},
			Cleanup: descriptor.CleanupPolicy{IgnoreNotFound: true, ChildKinds: []string{"Version"}},
		},
		{
			Kind: "Version", Plural: "versions", ParentKind: "Channel", OnParentDelete: "restrict",
			Thresholds: descriptor.EntityThresholds{
				Read: ThresholdAPIRead, List: ThresholdAPIList,
				Create: ThresholdAPINestedWrite, Update: ThresholdAPINestedWrite, Delete: ThresholdAPINestedWrite,
			},
			Cleanup: descriptor.CleanupPolicy{IgnoreNotFound: true},
		},
		{
			Kind: "WifConfig", Plural: "wifconfigs",
			Thresholds: descriptor.EntityThresholds{
				Read: ThresholdAPIRead, List: ThresholdAPIList,
				Create: ThresholdAPICreate, Update: ThresholdAPIUpdate, Delete: ThresholdAPIDelete,
			},
			Cleanup: descriptor.CleanupPolicy{IgnoreNotFound: true, IgnoreConflict: true},
		},
	}
}
