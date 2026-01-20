
# Feature: NodePool Lifecycle Management

## Test Title: Create nodepool will succeed via API

### Description

This test case validates the core nodepool API operations including creating a new GCP nodepool via the Hyperfleet API for an existing cluster, verifying it appears in the nodepool list with the correct configuration, and monitoring adapter status transitions until the nodepool reaches Ready state.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Critical |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-01-10 |
| **Updated** | 2026-01-20 |


---

### Preconditions

1. Hyperfleet API server is running and accessible
2. Set the API gateway URL as an environment variable: `export API_URL=<your-api-gateway-url>`
3. A cluster has been created and is in Ready state (see [Create cluster will succeed via API](cluster_lifecycle.md#test-title-create-cluster-will-succeed-via-api))
4. Set the cluster ID as an environment variable: `export CLUSTER_ID=<cluster_id>`

---

### Test Steps

#### Step 1: Create NodePool via API

**Action:**
Send POST request to create a new GCP nodepool using the payload from [templates/create_nodepool_gcp.json](templates/create_nodepool_gcp.json):

```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/nodepools \
  -H "Content-Type: application/json" \
  -d @testcases/templates/create_nodepool_gcp.json
```

- <details>
  <summary>Payload example (click to expand)</summary>

See [templates/create_nodepool_gcp.json](templates/create_nodepool_gcp.json) for the complete nodepool creation payload.

Key fields in the payload:
- `kind`: "NodePool"
- `name`: "hp-gcp-nodepool-1"
- `labels`: workload, tier, and environment labels
- `spec.clusterName`: parent cluster name
- `spec.replicas`: number of nodes
- `spec.platform.type`: "gcp"
- `spec.platform.gcp.instanceType`: GCP instance type
- `spec.release.image`: OpenShift release image

</details>

**Expected Result:**
- Response status code is 201 (Created)
- <details>
  <summary>Response example (click to expand)</summary>

  ```json
  {
    "created_by": "system",
    "created_time": "2026-01-20T10:00:00.000000Z",
    "generation": 1,
    "href": "/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/abc123def456",
    "id": "abc123def456",
    "kind": "NodePool",
    "labels": {
      "workload": "gpu",
      "tier": "compute",
      "environment": "test"
    },
    "name": "hp-gcp-nodepool-1",
    "spec": {
      "clusterName": "hp-gcp-cluster-1",
      "replicas": 2,
      "platform": {
        "type": "gcp",
        "gcp": {
          "instanceType": "n1-standard-8"
        }
      },
      "release": {
        "image": "registry.redhat.io/openshift4/ose-cluster-version-operator:v4.14.0"
      }
    },
    "status": {
      "conditions": [],
      "last_transition_time": "0001-01-01T00:00:00Z",
      "last_updated_time": "0001-01-01T00:00:00Z",
      "observed_generation": 0,
      "phase": "NotReady"
    },
    "updated_by": "system",
    "updated_time": "2026-01-20T10:00:00.000000Z"
  }
  ```
  </details>
- Verify response fields:
  - `id` is automatically generated, non-empty, lowercase, and unique
  - `href` matches pattern `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}`
  - `kind` is "NodePool"
  - `name` is "hp-gcp-nodepool-1"
  - `labels` contains "workload": "gpu", "tier": "compute", and "environment": "test"
  - `created_by` and `updated_by` are populated (currently "system" as placeholder; will change when auth is introduced)
  - `created_time` and `updated_time` are populated and not default values (not "0001-01-01T00:00:00Z")
  - `generation` is 1
  - `status.phase` is "NotReady" (initial state in MVP phase)
  - `status.conditions` is empty array
  - `status.observed_generation` is 0
- All spec fields match the request payload

---

#### Step 2: Verify NodePool API response

**Action:**
Send GET request to retrieve the nodepool list and verify filtering capabilities:

1. List nodepools by name filter:
```bash
curl -G ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/nodepools --data-urlencode "search=name='<nodepool_name>'"
```

2. List nodepools by label filter:
```bash
curl -G ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/nodepools --data-urlencode "search=labels.workload='gpu'"
```

**Expected Result:**
- Response status code is 200 (OK)
- <details>
  <summary>Response example (click to expand)</summary>

  ```json
  {
    "items": [
      {
        "created_by": "system",
        "created_time": "2026-01-20T10:00:00.000000Z",
        "generation": 1,
        "href": "/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/abc123def456",
        "id": "abc123def456",
        "kind": "NodePool",
        "labels": {
          "workload": "gpu",
          "tier": "compute",
          "environment": "test"
        },
        "name": "hp-gcp-nodepool-1",
        "spec": {
          "clusterName": "hp-gcp-cluster-1",
          "replicas": 2,
          "platform": {
            "type": "gcp",
            "gcp": {
              "instanceType": "n1-standard-8"
            }
          },
          "release": {
            "image": "registry.redhat.io/openshift4/ose-cluster-version-operator:v4.14.0"
          }
        },
        "status": {
          "conditions": [
            {
              "created_time": "2026-01-20T10:00:15Z",
              "last_transition_time": "2026-01-20T10:00:15Z",
              "last_updated_time": "2026-01-20T10:05:00Z",
              "message": "NodePool validation passed",
              "observed_generation": 1,
              "reason": "ValidationPassed",
              "status": "True",
              "type": "ValidationAdapterSuccessful"
            }
          ],
          "last_transition_time": "2026-01-20T10:00:15Z",
          "last_updated_time": "2026-01-20T10:05:00Z",
          "observed_generation": 1,
          "phase": "NotReady"
        },
        "updated_by": "system",
        "updated_time": "2026-01-20T10:05:00Z"
      }
    ],
    "kind": "NodePoolList",
    "page": 1,
    "size": 1,
    "total": 1
  }
  ```
  </details>
- **NodePoolList metadata:**
  - `kind` is "NodePoolList"
  - `total` matches the number of nodepools matching the filter
  - `size` matches `total`
  - `page` is 1
- **Created nodepool appears in the `items` array**
- **Label filtering works correctly:**
  - Filtering by `labels.workload='gpu'` returns the created nodepool
  - Filtering by non-matching labels returns empty result
- **System default fields:**
  - `id` matches the ID from Step 1
  - `href` is "/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}"
  - `kind` is "NodePool"
  - `created_by` is populated (currently "system" as placeholder; will change when auth is introduced)
  - `created_time` is populated and not default value (not "0001-01-01T00:00:00Z")
  - `updated_by` is populated (currently "system" as placeholder; will change when auth is introduced)
  - `updated_time` is populated and not default value (not "0001-01-01T00:00:00Z")
  - `generation` is 1
  - `status.phase` is "NotReady" (initial state in MVP phase)
  - `status.conditions` array exists with required fields: `type`, `status`, `reason`, `message`, `created_time`, `last_transition_time`, `last_updated_time`, `observed_generation`
  - `status.observed_generation` matches nodepool generation
  - `status.last_transition_time` and `status.last_updated_time` are populated with the real values
- **NodePool request body configured parameters:**
  - All spec fields match the request payload

---

#### Step 3: Retrieve the specific nodepool and monitor its status

**Action:**
Send GET request to retrieve the specific nodepool:

```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/nodepools/{nodepool_id}
```

**Expected Result:**
- Response status code is 200 (OK)
- <details>
  <summary>Response example (click to expand)</summary>

  ```json
  {
    "created_by": "system",
    "created_time": "2026-01-20T10:00:00.000000Z",
    "generation": 1,
    "href": "/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/abc123def456",
    "id": "abc123def456",
    "kind": "NodePool",
    "labels": {
      "workload": "gpu",
      "tier": "compute",
      "environment": "test"
    },
    "name": "hp-gcp-nodepool-1",
    "spec": {
      "clusterName": "hp-gcp-cluster-1",
      "replicas": 2,
      "platform": {
        "type": "gcp",
        "gcp": {
          "instanceType": "n1-standard-8"
        }
      },
      "release": {
        "image": "registry.redhat.io/openshift4/ose-cluster-version-operator:v4.14.0"
      }
    },
    "status": {
      "conditions": [
        {
          "created_time": "2026-01-20T10:00:15Z",
          "last_transition_time": "2026-01-20T10:00:15Z",
          "last_updated_time": "2026-01-20T10:05:00Z",
          "message": "NodePool validation passed",
          "observed_generation": 1,
          "reason": "ValidationPassed",
          "status": "True",
          "type": "ValidationAdapterSuccessful"
        },
        {
          "created_time": "2026-01-20T10:00:15Z",
          "last_transition_time": "2026-01-20T10:05:00Z",
          "last_updated_time": "2026-01-20T10:10:00Z",
          "message": "NodePool resources provisioned successfully",
          "observed_generation": 1,
          "reason": "ProvisioningComplete",
          "status": "True",
          "type": "NodePoolAdapterSuccessful"
        }
      ],
      "last_transition_time": "2026-01-20T10:05:00Z",
      "last_updated_time": "2026-01-20T10:10:00Z",
      "observed_generation": 1,
      "phase": "NotReady"
    },
    "updated_by": "system",
    "updated_time": "2026-01-20T10:10:00Z"
  }
  ```
  </details>
- Response contains all nodepool metadata fields from Step 1
- NodePool status contains adapter information:
  - `status.conditions` array contains adapter status entries
  - **ValidationAdapterSuccessful** condition exists (example):
    - `type`: "ValidationAdapterSuccessful"
    - `status`: "True"
    - `reason`: "ValidationPassed"
    - `message`: "NodePool validation passed"
    - `created_time`, `last_transition_time`, `last_updated_time` populated and not default values
    - `observed_generation`: 1 (must match nodepool.generation)
- `updated_time` is more recent than `created_time`, indicating the nodepool has been processed by adapters

---

#### Step 4: Retrieve the NodePool Adapter Statuses

**Action:**
Send GET request to retrieve the adapter statuses for the nodepool:

```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/nodepools/{nodepool_id}/statuses
```

**Expected Result:**
- Response status code is 200 (OK)
- <details>
  <summary>Response example (click to expand)</summary>

  ```json
  {
    "items": [
      {
        "adapter": "validation-adapter",
        "conditions": [
          {
            "last_transition_time": "2026-01-20T10:00:15Z",
            "message": "Validation job applied successfully",
            "reason": "JobApplied",
            "status": "True",
            "type": "Applied"
          },
          {
            "last_transition_time": "2026-01-20T10:00:30Z",
            "message": "NodePool configuration validated successfully",
            "reason": "ValidationPassed",
            "status": "True",
            "type": "Available"
          },
          {
            "last_transition_time": "2026-01-20T10:00:15Z",
            "message": "All adapter operations completed successfully",
            "reason": "Healthy",
            "status": "True",
            "type": "Health"
          }
        ],
        "created_time": "2026-01-20T10:00:15Z",
        "data": {},
        "last_report_time": "2026-01-20T10:05:00Z",
        "observed_generation": 1
      },
      {
        "adapter": "nodepool-adapter",
        "conditions": [
          {
            "last_transition_time": "2026-01-20T10:01:00Z",
            "message": "NodePool resources applied successfully",
            "reason": "ResourcesApplied",
            "status": "True",
            "type": "Applied"
          },
          {
            "last_transition_time": "2026-01-20T10:05:00Z",
            "message": "NodePool nodes are provisioned and ready",
            "reason": "NodesReady",
            "status": "True",
            "type": "Available"
          },
          {
            "last_transition_time": "2026-01-20T10:01:00Z",
            "message": "All adapter operations completed successfully",
            "reason": "Healthy",
            "status": "True",
            "type": "Health"
          }
        ],
        "created_time": "2026-01-20T10:01:00Z",
        "data": {
          "nodes": {
            "ready": 2,
            "total": 2
          }
        },
        "last_report_time": "2026-01-20T10:10:00Z",
        "observed_generation": 1
      }
    ],
    "kind": "AdapterStatusList",
    "page": 1,
    "size": 2,
    "total": 2
  }
  ```
  </details>
- Response contains AdapterStatusList metadata:
  - `kind` is "AdapterStatusList"
  - `total` matches the number of deployed adapters (this number may vary as more adapters are deployed)
  - `size` matches `total`
  - `page` is 1
- Response contains `items` array with adapter status entries:
  - **validation-adapter** status exists:
    - `adapter`: Adapter name
    - `created_time` is populated and not default value
    - `last_report_time` is populated and recent
    - `observed_generation`: 1 (must match nodepool.generation)
    - `conditions` array contains three required condition types: **Applied**, **Available**, and **Health**
      - Each condition's `status` field is false at the beginning and will be "True" when that specific condition is satisfied
      - **Applied** condition:
        - `type`: "Applied"
        - `status`: "True" (when resources are successfully applied)
        - `reason`: "JobApplied"
        - `message`: "Validation job applied successfully"
        - `last_transition_time` is populated
      - **Available** condition:
        - `type`: "Available"
        - `status`: "True" (when resources are available and ready)
        - `reason`: "ValidationPassed"
        - `message`: "NodePool configuration validated successfully"
        - `last_transition_time` is populated
      - **Health** condition:
        - `type`: "Health"
        - `status`: "True" (when all adapter operations are healthy)
        - `reason`: "Healthy"
        - `message`: "All adapter operations completed successfully"
        - `last_transition_time` is populated
  - **nodepool-adapter** status exists with similar structure:
    - `adapter`: "nodepool-adapter"
    - `created_time` is populated and not default value
    - `last_report_time` is populated and recent
    - `observed_generation`: 1 (must match nodepool.generation)
    - `conditions` array contains three required condition types: **Applied**, **Available**, and **Health**
    - `data` contains node information:
      - `nodes.ready`: number of ready nodes
      - `nodes.total`: total number of nodes (should match spec.replicas)
      - additional fields like KSA will be added in future

---

#### Step 5: Verify NodePool Final State

**Action:**
Send GET request to retrieve the nodepool status and verify final state:

```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/nodepools/{nodepool_id} | jq .status
```

**Expected Result:**
- Response status code is 200 (OK)
- <details>
  <summary>Status response example (click to expand)</summary>

  ```json
  {
    "conditions": [
      {
        "created_time": "2026-01-20T10:00:15Z",
        "last_transition_time": "2026-01-20T10:00:30Z",
        "last_updated_time": "2026-01-20T10:10:00Z",
        "message": "NodePool validation passed",
        "observed_generation": 1,
        "reason": "ValidationPassed",
        "status": "True",
        "type": "ValidationAdapterSuccessful"
      },
      {
        "created_time": "2026-01-20T10:01:00Z",
        "last_transition_time": "2026-01-20T10:05:00Z",
        "last_updated_time": "2026-01-20T10:10:00Z",
        "message": "NodePool resources provisioned successfully",
        "observed_generation": 1,
        "reason": "ProvisioningComplete",
        "status": "True",
        "type": "NodePoolAdapterSuccessful"
      }
    ],
    "last_transition_time": "2026-01-20T10:05:00Z",
    "last_updated_time": "2026-01-20T10:10:00Z",
    "observed_generation": 1,
    "phase": "Ready"
  }
  ```
  </details>
- Verify nodepool final state:
  - **NodePool phase:**
    - `phase` is "Ready"
      - Real available adapters number == Expected adapter number → NodePool phase: Ready
      - Any adapter Available: False → NodePool phase: NotReady (MVP)
    - `observed_generation` is 1
    - `last_transition_time` is populated (when phase last changed)
    - `last_updated_time` is populated and more recent than creation time
  - **Adapter conditions:**
    - All adapter conditions have `status`: "True"
    - `conditions` array contains adapters information (e.g., "ValidationAdapterSuccessful" and "NodePoolAdapterSuccessful")
    - Each condition has valid `created_time`, `last_transition_time`, and `last_updated_time`

---

## Test Title: Resources should be created after nodepool creation

### Description

This test case validates that the nodepool adapters have created the expected resources in the deployment environment.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Critical |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-01-10 |
| **Updated** | 2026-01-20 |


---

### Preconditions

1. Hyperfleet API server is running and accessible
2. Set the API gateway URL as an environment variable: `export API_URL=<your-api-gateway-url>`
3. A cluster has been created and is in Ready state (see [Create cluster will succeed via API](cluster_lifecycle.md#test-title-create-cluster-will-succeed-via-api))
4. A nodepool has been created and processed by adapters (see [Create nodepool will succeed via API](#test-title-create-nodepool-will-succeed-via-api))
5. kubectl is configured to access the deployment Kubernetes cluster

---

### Test Steps

#### Step 1: Check resources in deployed environment

**Action:**
Verify Kubernetes resources created by the adapters:

1. List pods in the cluster namespace to verify nodepool-related pods:
```bash
kubectl get pods -n {cluster_id}
```

2. Check nodes in the hosted cluster (if accessible):
```bash
kubectl get nodes --kubeconfig=<hosted_cluster_kubeconfig>
```

**Expected Result:**

- **Adapter created resources:**
  - **Validation adapter** creates a validation job under the cluster namespace
  - **NodePool adapter** triggers node provisioning in the hosted cluster

- **Kubernetes Jobs verification:**
  - Kubernetes Jobs created by adapters complete successfully
  - Jobs should have `status.succeeded: 1`
  - Example command to verify Jobs:
  ```bash
  kubectl get jobs -n {cluster_id} | grep nodepool
  ```
  - Example output:
  ```text
  NAME                                    COMPLETIONS   DURATION   AGE
  nodepool-validator-abc123-1             1/1           30s        5m
  ```

- **Pods verification:**
  - Validation pod (created by Job) exists in the cluster namespace
  - Pod status should be "Completed" without errors
  - Pod should not have any restarts or error states
  - Example output:
  ```text
  NAME                                                     READY   STATUS      RESTARTS   AGE
  nodepool-validator-abc123-1-xyz789                       0/2     Completed   0          5m
  ```

- **Nodes verification (in hosted cluster):**
  - Number of nodes matches `spec.replicas` (2 nodes)
  - All nodes are in "Ready" status
  - Nodes have correct labels matching nodepool labels
  - Example output:
  ```text
  NAME                           STATUS   ROLES    AGE   VERSION
  hp-gcp-nodepool-1-node-abc12   Ready    worker   10m   v1.27.0
  hp-gcp-nodepool-1-node-def34   Ready    worker   10m   v1.27.0
  ```

- **Logs verification (optional manual check):**
  - No errors in API service logs
  - No errors in Sentinel operator logs
  - No errors in Adapter logs
  - Kubernetes Jobs completed without errors

---
