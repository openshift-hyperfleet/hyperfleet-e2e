package config

import "time"

// Performance thresholds for API read operations.
// All API reads share the same threshold since payload size
// does not meaningfully impact latency at current scales.
// In Prow, each sample also goes through the public LoadBalancer and the
// gateway, so most of it is network time. Baselines are Prow medians from
// tier1-nightly, 2026-09-16 to 2026-10-16 (see
// hyperfleet/docs/performance-baselines.md in the architecture repo).
// The read threshold is ~1.4x the baseline.
const (
	ThresholdAPIRead = 50 * time.Millisecond // baseline ~36ms
	ThresholdAPIList = 50 * time.Millisecond // baseline ~36ms
)

// Performance thresholds for reconciliation operations.
// Calibrated from Prow tier1-nightly baselines (hyperfleet-dev-prow).
// Cluster thresholds use a ~1.5x margin over baseline to absorb CI
// run-to-run variance. NodePool thresholds use a ~2.25x margin because
// their lower baselines make the same absolute jitter a higher percentage.
//
// The cluster update, delete and cascade-delete thresholds are raised ahead of
// a rebaseline. Since the e2e API requires cl-desire, the remote adapter needs
// extra sentinel rounds to see the applier's result: one on update, two on
// delete (ConfigMap, then Namespace, then Finalized). Specs poll every 10s, so
// readings land on 10s steps: update read 70s on kind, and every reconciled
// cluster in the 2026-10-14 tier1-nightly took 50.2s from delete to hard-delete.
// Recalibrate once Maestro is gone and the perf specs poll finer than 10s.
const (
	ThresholdClusterCreateReconciled  = 90 * time.Second // baseline ~60s
	ThresholdClusterUpdateReconciled  = 90 * time.Second // was 60s, baseline ~40s before cl-desire
	ThresholdClusterDeleted           = 75 * time.Second // was 60s, baseline ~40s before cl-desire
	ThresholdClusterCascadeDeleted    = 90 * time.Second // was 75s, baseline ~50s before cl-desire
	ThresholdNodePoolCreateReconciled = 45 * time.Second // baseline ~20s
	ThresholdNodePoolDeleted          = 45 * time.Second // baseline ~20s
)

// Performance thresholds for generic (non-reconciling) resource write operations.
// Covers Channel, Version, and WifConfig create/update/delete. None of these kinds
// carry RequiredAdapters, so writes complete synchronously with no reconciliation
// to await — unlike Cluster/NodePool, there is no create/update-to-reconciled variant.
//
// These apply to root writes (Channel, WifConfig), with a ~1.5x margin over
// the Prow baseline. The measurement is the same as for reads above.
const (
	ThresholdAPICreate = 60 * time.Millisecond // baseline ~38ms
	ThresholdAPIUpdate = 60 * time.Millisecond // baseline ~38ms
	ThresholdAPIDelete = 60 * time.Millisecond // baseline ~38ms
)

// Performance threshold for nested Version writes (/channels/{parent_id}/versions).
// Before each write, the API also reads the parent Channel. On update and
// delete, it also reads the Version (checkOwnership in hyperfleet-api).
// 75ms is above the highest Prow median seen (74.3ms). Recalibrate after
// HYPERFLEET-1766 removes the extra reads. HYPERFLEET-1734 replaces these specs.
const ThresholdAPINestedWrite = 75 * time.Millisecond // baseline ~43ms

// DefaultAuthLatencyAllowance is added to API latency thresholds when the
// gateway authenticates requests (AUTH_MODE=EDGE or EDGE+API). Every request
// then traverses Envoy -> Authorino (ext_authz) -> Kubernetes TokenReview
// before reaching the API. The first EDGE+API nightly (2026-10-23) measured a
// uniform +10-25ms per request, up to +36ms on filtered cluster lists,
// against the 2026-10-22 API-mode run on the same cluster. See
// HYPERFLEET-1848.
const DefaultAuthLatencyAllowance = 30 * time.Millisecond
