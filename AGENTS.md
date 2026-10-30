# HyperFleet E2E — Agent Instructions

<!--
Maintainers: this file is loaded into every agent session (CLAUDE.md imports it), so every line costs context.
- Keep it under 200 lines. For each line ask: "would removing this cause the agent to make a mistake?" If not, cut it.
- Keep only what an agent cannot learn by reading the code: commands, non-obvious conventions, gotchas.
  Do not list functions, files, or enum values here; they go stale. Point to the file instead.
- Link to docs/ rather than copying from it.
- Use IMPORTANT on one rule at most; when many lines are emphasized, none stands out.
-->

Black-box E2E tests for HyperFleet. Specs call the HyperFleet API, create ephemeral resources, check adapter execution and the K8s resources that result, then clean up. Go 1.26, Ginkgo v2, Gomega, and the generic HTTP client in `pkg/client`.

Spec suites live in `e2e/<suite>/`, one directory per suite. Each suite is registered by a blank import in `e2e/e2e.go`; add one there when you create a suite.

## Verification

Run `make check` before you call work done, then `make build`.

- `make check`: `generate`, `fmt-check`, `vet`, `lint`, unit tests, `verify-tools` (fails if `tools/go.mod` drifted)
- `make fmt`: pinned `golangci-lint fmt ./...` applies `gofmt` and `goimports`; `fmt-check` checks the same formatters without rewriting files
- `make gofmt`: alias for `make fmt` (used by the `hyperfleet-gofmt` pre-commit hook)
- `make go-vet`: alias for `make vet` (used by the `hyperfleet-go-vet` pre-commit hook)
- `make install-hooks`: installs the pre-commit hooks (`.pre-commit-config.yaml`: secret scanning, commitlint, Go tooling, file hygiene)
- `make lint`: verifies `.golangci.yml` against the v2 schema, then runs golangci-lint pinned in `tools/go.mod`
- `make test`: unit tests for `./pkg/...` and `./cmd/...`, with race detection and coverage. E2E specs need a live environment; see `docs/setup.md`
- `make list-tests`: dry run that lists specs by tier. Use it to check labels and names without a cluster; dry runs skip the startup entity drift check

## Where to look

- Writing specs, full conventions: `docs/development.md`
- Architecture and package layout: `docs/architecture.md`
- Environment setup (kind, GCP): `docs/setup.md`
- Running tests, Prow jobs, label-to-CI mapping: `docs/runbook.md`
- CI failures and log locations: `docs/debugging.md`
- Test case documents and templates: `test-design/` (start at `test-design/README.md`)
- Which layer a test belongs in (unit / integration / E2E): [test placement strategy](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/docs/e2e-testing/test-placement-strategy.md)
- Config defaults, struct, file: `pkg/config/defaults.go`, `pkg/config/config.go`, `configs/config.yaml`. Per-kind adapters, timeouts and thresholds: the descriptor table in `pkg/config/descriptor.go`
- Pollers and matchers: `pkg/helper/pollers.go`, `pkg/helper/matchers.go`. Reuse these before writing new ones
- Logging standard: [HyperFleet logging specification](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/standards/logging-specification.md)

## Writing E2E specs

### Files and names

- IMPORTANT: spec files end in `.go`, never `_test.go`. Specs are compiled into the `hyperfleet-e2e` binary, not run by `go test`. The only `_test.go` file in `e2e/` is `e2e/e2e_suite_test.go`, the ginkgo CLI entry point for `make e2e-ci`. Put no spec code there.
- Path: `e2e/<suite>/descriptive-name.go`. The package name matches the directory.
- Describe name: `[Suite: <suite>][<category>] Description`, for example `[Suite: cluster][baseline] Cluster Resource Type Lifecycle`. Match the categories already used in that suite.
- Capability specs in `e2e/capability/` use `[Capability: <name>] Description`, carry `labels.Capability`, and take each fixture kind by its role constant, for example `h.Entities.MustGet(config.KindCascadeChild)`.

### Labels (from `pkg/labels`, never string literals)

- Every spec carries exactly one severity label: `labels.Tier0` (blocks release), `labels.Tier1` (important), `labels.Tier2` (edge case, can defer).
- `make test` enforces this: `pkg/labels/validate_test.go` parses every spec in `e2e/`.
- Optional labels are in `pkg/labels/labels.go`.

### Waiting on async state

Use `Eventually` with a poller and a matcher in the spec itself. Do not write `WaitFor*` helpers that hide `Eventually`.

```go
Eventually(h.Poll(ctx, h.Entities.MustGet("Cluster"), "", clusterID), h.Entities.MustGet("Cluster").Timeouts.Reconciled, h.Cfg.Polling.Interval).
    Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))
```

- For a one-off compound check, use `Eventually(func(g Gomega) { g.Expect(...) }).Should(Succeed())`. Inside the closure, call `g.Expect`, not bare `Expect`.
- Take per-kind timeouts from the descriptor (`h.Entities.MustGet(kind).Timeouts.Reconciled`, `.Deleted`), and `h.Cfg.Timeouts.Adapter.Processing` and `h.Cfg.Polling.Interval` from config. Never hardcode durations.
- Mark major steps with `ginkgo.By()`, but never inside an `Eventually` closure.

### Cleanup

- Register `ginkgo.DeferCleanup` right after you create each resource. Use `h.DeferCleanup(entity, parentID, resourceID)` for warning-only cleanup.
- Exception: in an `Ordered` container where a later spec uses a resource an earlier spec created, register the cleanup in `BeforeAll`. A `DeferCleanup` made inside an `It` runs when that `It` ends, before the later specs run. Say so in a comment, as `e2e/adapter/adapter_with_remote_transport.go` does.

### Payloads

- Resolve payload paths with `h.TestDataPath("payloads/...")`. Never hardcode a `testdata/` prefix, because CI overrides `TESTDATA_DIR`.
- Payloads in `testdata/payloads/` are Go templates. The available variables are the `templateVars` struct in `pkg/client/payload.go`.

### Audit identity (JWT)

When a spec checks `created_by` / `deleted_by`, guard the check. Audit checks are skipped when no identity is configured:

```go
if expected := h.ExpectedIdentity(); expected != "" {
    Expect(cluster).To(helper.HaveAuditIdentity(expected))
}
```

## Gotchas

- Every live run starts with a read-only drift check: `helm get values hyperfleet-api --all -n "$NAMESPACE"` must match the descriptor table, or the suite fails in `SynchronizedBeforeSuite` before any spec runs. Set `NAMESPACE` to the namespace that holds the API release and keep `helm`/`KUBECONFIG` able to read it. See `docs/debugging.md#startup-entity-drift-check`.

- Resolve a known descriptor after `helper.New()` with `h.Entities.MustGet("Cluster")`; use `Lookup` for runtime kind names. Generic resource helpers take a descriptor and parent ID (empty for roots).

- Adapter names come from the descriptor row at runtime: `h.Entities.MustGet("Cluster").RequiredAdapters`. Never hardcode them. The row in `pkg/config/descriptor.go` holds the hyperfleet-infra release names as defaults. YAML (`adapters.<kind>`, `timeouts.<kind>`) and env (`HYPERFLEET_ADAPTERS_<KIND>`, `HYPERFLEET_TIMEOUTS_<KIND>_RECONCILED`) override them by kind, lower case in YAML and upper case in env; an unknown kind in YAML or an empty adapter list fails config loading.
- Config priority: CLI flags > `HYPERFLEET_*` env vars > `configs/config.yaml` > `pkg/config/defaults.go` (per-kind values: the descriptor rows in `pkg/config/descriptor.go`). Config file path: `--config` > `HYPERFLEET_CONFIG` > `./configs/config.yaml`.
- `helper.New()` calls `log.Fatalf` when the suite config is nil; call `helper.SetSuiteConfig` first. `Config.Validate()` returns an error rather than panicking. Outside `--dry-run` it requires `API.URL` and a `RunID` that is a valid K8s label value (`RUN_ID`), and it always checks `brokerType`.
- Logging uses `log/slog` directly. There is no custom logger wrapper.

## Boundaries

- Never import `e2e/*` packages from `pkg/`.
- Keep E2E specs to user journeys and critical operations. Field validation and single-component behavior belong in unit or integration tests (see the test placement strategy above).
