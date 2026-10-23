package config

import "time"

// Performance thresholds for API read operations. These and the write
// thresholds below are shared: the descriptor table in descriptor.go puts them
// in each row's Thresholds, next to the per-kind reconciliation thresholds, and
// specs read them from the row.
// All API reads share the same threshold since payload size
// does not meaningfully impact latency at current scales.
// In Prow, each sample also goes through the public LoadBalancer and the
// gateway, so most of it is network time. Baselines are Prow medians from
// tier1-nightly, 2026-09-16 to 2026-10-16 (see
// hyperfleet/docs/performance-baselines.md in the architecture repo).
// The read threshold is ~1.4x the baseline.
const (
	thresholdAPIRead = 50 * time.Millisecond // baseline ~36ms
	thresholdAPIList = 50 * time.Millisecond // baseline ~36ms
)

// Performance thresholds for generic (non-reconciling) resource write operations.
// Covers Channel, Version, and WifConfig create/update/delete. None of these kinds
// carry RequiredAdapters, so writes complete synchronously with no reconciliation
// to await — unlike Cluster/NodePool, there is no create/update-to-reconciled variant.
//
// These apply to root writes (Channel, WifConfig), with a ~1.5x margin over
// the Prow baseline. The measurement is the same as for reads above.
const (
	thresholdAPICreate = 60 * time.Millisecond // baseline ~38ms
	thresholdAPIUpdate = 60 * time.Millisecond // baseline ~38ms
	thresholdAPIDelete = 60 * time.Millisecond // baseline ~38ms
)

// Performance threshold for nested Version writes (/channels/{parent_id}/versions).
// Before each write, the API also reads the parent Channel. On update and
// delete, it also reads the Version (checkOwnership in hyperfleet-api).
// 75ms is above the highest Prow median seen (74.3ms). Recalibrate after
// HYPERFLEET-1766 removes the extra reads. HYPERFLEET-1734 replaces these specs.
const thresholdAPINestedWrite = 75 * time.Millisecond // baseline ~43ms
