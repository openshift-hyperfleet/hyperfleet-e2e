# Architecture

HyperFleet E2E is a Ginkgo-based black-box testing framework for validating HyperFleet cluster lifecycle management.

## Design Principles

1. **Ephemeral resource management** - Tests create and cleanup temporary resources
2. **Configuration-driven execution** - Behavior controlled through config files, env vars, and CLI flags
3. **Structured logging** - All operations tracked with component, cluster ID, and error context
4. **Generic HTTP client** - Type-safe API client using hand-written types matching the generic resource API contract

## Core Packages

```text
pkg/
├── client/             - HyperFleet API client (generic HTTP)
│   └── kubernetes/     - Kubernetes client (client-go)
├── config/             - Configuration loading and validation
├── descriptor/         - Entity metadata, catalog and path construction
├── e2e/                - Test execution engine (Ginkgo)
├── helper/             - Test helpers (pollers, matchers, resource management)
├── labels/             - Test label definitions
└── util/               - Shared utility functions
```

## Resource Management

HyperFleet E2E creates ephemeral resources per test for complete isolation.

**Resource Lifecycle**: Per-test creation and cleanup
- Each test creates its own cluster via API
- Full isolation between tests
- Automatic cleanup after completion
- Supports parallel execution

**Workflow**:
```text
Test starts
  → Create new Helper instance (helper.New())
  → CreateTestResource() creates cluster via API
  → Poll for cluster Reconciled condition (pollers + matchers)
  → Execute test assertions
  → CleanupTestResource() deletes via the API; adapters remove downstream resources
Test ends
```

**Example Configuration**:
```yaml
api:
  url: https://api.hyperfleet.example.com
timeouts:
  cluster:
    reconciled: 5m   # Example override; the default is defaultEntityTimeouts in pkg/config/descriptor.go
```

### pkg/config

**Purpose**: Configuration loading, validation, and management

**Key Features**:
- Multi-source configuration loading (file, environment variables, CLI flags)
- Detailed validation with helpful error messages
- Configuration priority enforcement

**Configuration Priority Chain**:
```text
CLI Flags (highest priority)
  ↓
Environment Variables (HYPERFLEET_* prefix)
  ↓
Config File (configs/config.yaml)
  ↓
Built-in Defaults (lowest priority)
```

**Key Types**:
- `Config` - Top-level configuration struct
- `APIConfig`, `TimeoutsConfig`, `PollingConfig`, `LogConfig` - Nested configuration sections

**Key Functions**:
- `Load()` - Load and validate configuration
- `Validate()` - Validate configuration requirements
- `Display()` - Log merged configuration using structured logging

### pkg/client

**Purpose**: Generic HTTP client for the HyperFleet API with test-friendly methods

**Key Features**:
- Generic HTTP response handler
- Structured error handling
- Convenience methods for common operations (cluster, nodepool, channel, version, wifconfig)
- Auth injection via `http.RoundTripper`

**Key Types**:
- `HyperFleetClient` - Main client struct
- `Resource` / `ResourceList` - Generic resource types matching the API contract
- `AdapterStatus` / `AdapterStatusList` - Adapter status types

**Key Methods**:

*Clusters*:
- `CreateCluster(ctx, req)` / `CreateClusterFromPayload(ctx, path)` - Create cluster
- `GetCluster(ctx, clusterID)` - Fetch cluster details
- `ListClusters(ctx)` - List all clusters
- `DeleteCluster(ctx, clusterID)` - Soft-delete cluster
- `PatchCluster(ctx, clusterID, req)` / `PatchClusterFromPayload(ctx, clusterID, path)` - Update cluster
- `GetClusterStatuses(ctx, clusterID)` - Fetch adapter statuses

*NodePools*:
- `CreateNodePool(ctx, clusterID, req)` / `CreateNodePoolFromPayload(ctx, clusterID, path)` - Create nodepool
- `GetNodePool(ctx, clusterID, npID)` - Fetch nodepool details
- `ListNodePools(ctx, clusterID)` - List nodepools for a cluster
- `DeleteNodePool(ctx, clusterID, npID)` - Soft-delete nodepool
- `PatchNodePool(ctx, clusterID, npID, req)` / `PatchNodePoolFromPayload(ctx, clusterID, npID, path)` - Update nodepool
- `GetNodePoolStatuses(ctx, clusterID, npID)` - Fetch adapter statuses

### pkg/helper

**Purpose**: Test helper utilities — resource management, pollers, matchers, K8s verification

**Key Features**:
- Per-test helper instance creation (`New()`)
- Resource lifecycle management (create, cleanup)
- Pollers for async assertions with `Eventually`
- Custom Gomega matchers for resource and adapter conditions
- Kubernetes resource verification (namespaces, deployments, jobs, configmaps)
- Adapter deployment/uninstall via Helm

**Key Types**:
- `Helper` - Main struct with `Cfg`, `Entities`, `Client`, `K8sClient`, `AdapterDeploymentList`

**Key Methods**:

*Resource Management* (`resource.go`):
- `CreateTestResource(ctx, entity, parentID, payloadPath)` - Create a temporary resource and return its ID
- `CleanupTestResource(ctx, entity, parentID, resourceID)` - Delete through the API using the descriptor's cleanup policy
- `DeferCleanup(entity, parentID, resourceID)` - Register warning-only cleanup in the current Ginkgo node

*Pollers* (`pollers.go`) — thin functions returning current state for use with `Eventually`:
- `Poll(ctx, entity, parentID, resourceID)` - Returns `(*Resource, error)`
- `PollAdapterStatuses(ctx, entity, parentID, resourceID)` - Returns `(*AdapterStatusList, error)`
- `PollHTTPStatus(ctx, entity, parentID, resourceID)` - Returns the HTTP status code
- `PollNamespacesByPrefix(ctx, prefix)` - Returns `([]string, error)`

Use an empty `parentID` for root resources. Child descriptors derive their nested
path from their parent's plural. Resolve a known kind with
`h.Entities.MustGet("Cluster")`; use `Lookup` for runtime input that can be unknown.

The catalog is built after configuration has been resolved. `pkg/descriptor`
owns the type, catalog and path construction, with no config or helper imports.
The single table is `EntityDescriptors` in `pkg/config/descriptor.go`. Each row
combines routing, cleanup and the default runtime settings: required adapters,
`Timeouts` and perf `Thresholds`. There is no second kind switch, config field,
default constant or env alias to update when adding a row. Nested paths are
derived from parent plural, parent ID and plural.

`config.Load()` overlays the kind-keyed config on the row defaults, builds and
validates the catalog once, and stores it in `Config.Entities`; `helper.New()`
uses the same catalog. Config keys are the kind in lower case, env vars the kind
in upper case:

- `adapters.<kind>` or `HYPERFLEET_ADAPTERS_<KIND>` (comma- or space-separated) replaces the
  row's adapter list. Blank names are dropped, and the list must name at least
  one adapter; remove the key to keep the default. `API_ADAPTERS_CLUSTER` and `API_ADAPTERS_NODEPOOL` are
  aliases for the cluster and nodepool keys until the per-kind suites go.
- `timeouts.<kind>.reconciled` / `.deleted`, or
  `HYPERFLEET_TIMEOUTS_<KIND>_RECONCILED` / `_DELETED`, replaces one field; a
  missing or zero value keeps the row default. Values decode like every other
  duration in the config: a value needs a unit (`90s`, `5m`), and a bare YAML
  number other than zero fails. The keys `TimeoutsConfig` owns, such as `timeouts.adapter.processing`,
  are shared, not per kind.
- Thresholds are code in the row and have no config key.

`Load()` fails before the suite starts on a YAML key that names no table kind
(for example the typo `timeouts.nodepools`), an unknown field under
`timeouts.<kind>`, a scalar or list where a map belongs (such as
`timeouts.cluster: 5m`), a timeout that is not a duration (such as `"300"` or `300`, with
no unit), and an empty adapter list. A bare number fails for the shared durations too, such as
`polling.interval`. Env vars
for an unknown kind are not checked. `descriptor.NewCatalog` then checks each
row (`EntityDescriptor.Validate`), so `Load()` also fails on a negative timeout
or a row with adapters and no reconciled timeout. A row that waits for
hard-delete needs a positive deleted timeout, but config cannot remove one, so
only a table row can fail that check. Startup logs
`adapters_<kind>`, `timeout_<kind>_reconciled` and `timeout_<kind>_deleted` for
each row with adapters or timeouts in the "Loaded configuration" entry.

The suite's startup drift guard is `helper.ValidateAPIEntityConfig`; see
[pkg/e2e](#pkge2e) for when it runs.

Parent and CascadeChild are adapter-backed fixtures that `e2e/capability` runs
on: each row names the success adapter hyperfleet-infra deploys for its plural
(`parents-success`, `cascadechildren-success`), and both wait for hard-delete
with the Cluster timeouts. RestrictChild, Referencer and Target register routing
and cleanup only. The specs take Parent and CascadeChild by the role constants
`config.KindParent` and `config.KindCascadeChild`. Fixture thresholds stay unset
until HYPERFLEET-1734.

Cleanup policy preserves existing behavior: Cluster and NodePool wait for 404,
Channel explicitly deletes eligible Versions before itself, and WifConfig still
tolerates conflicts during teardown. These are cleanup choices, not API deletion
semantics. The API and adapters own downstream resource deletion.
Hard-delete polling retains the existing deadline-between-requests loop and sleep
interval. Requests use the caller's context; the cleanup timeout does not impose
a new request deadline. Resource IDs are concatenated as before and passed to the
client/API without new helper validation.

*Custom Matchers* (`matchers.go`) — reusable Gomega matchers:
- `HaveResourceCondition(condType, status)` - Matches `*Resource` with given condition
- `HaveAllAdaptersWithCondition(adapters, condType, status)` - All required adapters have condition
- `HaveAllAdaptersAtGeneration(adapters, gen)` - All adapters at generation with Applied/Available/Health=True

*Condition Validation* (`validation.go`):
- `HasResourceCondition(conditions, condType, status)` - Synchronous condition check
- `HasAdapterCondition(conditions, condType, status)` - Synchronous adapter condition check
- `AllConditionsTrue(conditions, condTypes)` - All specified conditions are True
- `AdapterNameToConditionType(adapterName)` - Convert adapter name to condition type string

*Kubernetes Verification* (`k8s.go`):
- `VerifyNamespaceActive(ctx, name, labels, annotations)` - Namespace exists and Active
- `VerifyDeploymentAvailable(ctx, ns, labels, annotations)` - Deployment is Available
- `VerifyJobComplete(ctx, ns, labels, annotations)` - Job has completed
- `VerifyConfigMap(ctx, ns, labels, annotations)` - ConfigMap exists with expected metadata

*Adapter Operations* (`adapter.go`):
- `DeployAdapter(ctx, opts)` - Deploy adapter via Helm upgrade --install; `opts.Entity` supplies its descriptor
- `UninstallAdapter(ctx, releaseName, namespace)` - Uninstall adapter via Helm

### pkg/e2e

**Purpose**: Test execution engine, Ginkgo configuration, and process-wide structured logging

**Key Features**:
- Ginkgo suite configuration
- JUnit report generation
- Test filtering (labels, focus, skip)
- Suite timeout management
- Direct use of Go's `log/slog` throughout the framework
- Shared `hyperfleet-logger` handler, which adds `component`, `version`, and `hostname` to every record
- Ginkgo log handler that adds `test_case` for log records emitted during a spec
- Configurable output format (text, JSON), level (debug, info, warn, error), and output (stdout, stderr)

**Key Functions**:
- `RunTests(ctx)` - Main entry point for test execution
- Configures Ginkgo reporters, timeouts, and filters
- Handles suite-level setup and teardown
- `initLogging(cfg)` - Configures the default `slog` logger with the shared handler and Ginkgo context

**Startup drift guard** (`suite.go`): before any spec runs, process 1 of
`SynchronizedBeforeSuite` calls `helper.ValidateAPIEntityConfig`. It reads
`config.entities` from the deployed `hyperfleet-api` Helm release in `NAMESPACE`
and compares every row of the descriptor table by `kind`, `plural`, `parent_kind`
and `on_parent_delete`. A missing row or a changed field fails the suite with a
deterministic field-level diff instead of letting specs fail later against routes
or deletion semantics the API does not serve. Extra deployed kinds are allowed.
The check is read-only, runs for `AUTH_MODE=NONE` too, and needs `helm` plus a
`KUBECONFIG` that can read the release namespace. Ginkgo does not execute setup
nodes in `--dry-run`, so listing specs stays cluster-free.

## Configuration Priority

Configuration values are applied in this priority order:

```text
┌──────────────────────────────────────────────────────────────────┐
│ 1. CLI Flags (highest priority)                                  │
│    Example: --api-url, --log-level                               │
└──────────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────────┐
│ 2. Environment Variables                                         │
│    Example: HYPERFLEET_API_URL, HYPERFLEET_LOG_LEVEL             │
└──────────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────────┐
│ 3. Config File                                                   │
│    Example: configs/config.yaml                                  │
└──────────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────────┐
│ 4. Built-in Defaults (lowest priority)                           │
│    Example: timeout: 30m, poll: 10s                              │
└──────────────────────────────────────────────────────────────────┘
```

**Example**: If you set `HYPERFLEET_API_URL=https://env.example.com` and also pass `--api-url=https://flag.example.com`, the CLI flag value (`https://flag.example.com`) wins.

## Test Execution Flow

```text
CLI Invoked (hyperfleet-e2e test)
  ↓
┌─────────────────────────────────────┐
│ Load Configuration                  │
│ • Read config file                  │
│ • Apply environment variables       │
│ • Apply CLI flags                   │
│ • Validate configuration            │
└─────────────────────────────────────┘
  ↓
┌─────────────────────────────────────┐
│ Initialize Logger                   │
│ • Setup structured logging          │
│ • Display merged configuration      │
└─────────────────────────────────────┘
  ↓
┌─────────────────────────────────────┐
│ Configure Ginkgo                    │
│ • Apply label filters               │
│ • Setup JUnit reporter              │
│ • Configure timeouts                │
└─────────────────────────────────────┘
  ↓
┌─────────────────────────────────────┐
│ Run Test Suites                     │
│ • Discover all e2e/*/*.go            │
│ • Execute matched tests             │
│ • Collect results                   │
└─────────────────────────────────────┘
  ↓
┌─────────────────────────────────────┐
│ Generate Reports                    │
│ • JUnit XML (if configured)         │
│ • Console output                    │
│ • Exit code (0 = pass, 1 = fail)    │
└─────────────────────────────────────┘
```

## API Client Design

The framework uses hand-written Go types (`Resource`, `AdapterStatus`, `ResourceCondition`, etc.) that mirror the generic resource API contract. With the migration to the generic resource layer, the API uses a single `Resource` type with `map[string]any` spec for all entities (clusters, nodepools, channels, versions, wifconfigs).

### Client Architecture

- **Types**: Defined in `pkg/client/resource.go` - `Resource`, `ResourceCondition`, `AdapterStatus`, etc.
- **Generic CRUD**: `CreateResource`, `GetResource`, `ListResources`, `PatchResource`, `DeleteResource`
- **Entity convenience methods**: `GetCluster`, `CreateNodePool`, etc. delegate to generic CRUD with entity-specific paths
- **Auth**: `WithAuthorizationToken` wraps `http.RoundTripper` and injects a mode-specific scheme on API-host requests; `WithBearerToken` remains a compatibility wrapper.

**Example**:
```go
// Create client with the scheme selected by AUTH_MODE.
cl, err := client.NewHyperFleetClient(apiURL, nil, client.WithAuthorizationToken(token, cfg.MachineAuthorizationScheme()))
if err != nil {
    log.Fatalf("create client: %v", err)
}
cluster, err := cl.GetCluster(ctx, clusterID)
```
