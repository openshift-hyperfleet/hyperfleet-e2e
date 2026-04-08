# Feature: Clusters Resource Type Lifecycle Management

## Table of Contents

1. [Clusters Resource Type - Full Lifecycle Workflow Validation](#test-title-clusters-resource-type---full-lifecycle-workflow-validation)
2. [Clusters Resource Type - K8s Resources Check Aligned with Preinstalled Clusters Related Adapters Specified](#test-title-clusters-resource-type---k8s-resources-check-aligned-with-preinstalled-clusters-related-adapters-specified)
3. [Clusters Resource Type - Adapter Dependency Relationships Workflow Validation](#test-title-clusters-resource-type---adapter-dependency-relationships-workflow-validation)
4. [Cluster can reflect adapter failure in top-level status](#test-title-cluster-can-reflect-adapter-failure-in-top-level-status)
5. [Cluster can reach correct status after adapter crash and recovery](#test-title-cluster-can-reach-correct-status-after-adapter-crash-and-recovery)
6. [DELETE hierarchical: subresource cleanup before resource hard-delete](#test-title-delete-hierarchical-subresource-cleanup-before-resource-hard-delete)
7. [409 Conflict returned for mutations on tombstoned resources](#test-title-409-conflict-returned-for-mutations-on-tombstoned-resources)
8. [Stale Applied=False before tombstone does not trigger premature hard-delete](#test-title-stale-appliedfalse-before-tombstone-does-not-trigger-premature-hard-delete)
9. [Adapter unhealthy blocks hard-delete via Finalized=False](#test-title-adapter-unhealthy-blocks-hard-delete-via-finalizedfalse)
10. [Concurrent deletion events handled idempotently](#test-title-concurrent-deletion-events-handled-idempotently)
11. [Resource already gone is treated as deletion success](#test-title-resource-already-gone-is-treated-as-deletion-success)
12. [Multi-generation resource cleanup via label selectors](#test-title-multi-generation-resource-cleanup-via-label-selectors)
13. [Independent subresource deletion](#test-title-independent-subresource-deletion)
14. [DELETE while creation is still in progress](#test-title-delete-while-creation-is-still-in-progress)
15. [DELETE while update reconciliation is still in progress](#test-title-delete-while-update-reconciliation-is-still-in-progress)
16. [Recreate cluster with same name after hard-delete](#test-title-recreate-cluster-with-same-name-after-hard-delete)
17. [Multiple consecutive updates](#test-title-multiple-consecutive-updates)
18. [Concurrent updates on same cluster](#test-title-concurrent-updates-on-same-cluster)

---

## Test Title: Clusters Resource Type - Full Lifecycle Workflow Validation

### Description

This test validates the complete cluster lifecycle workflow end-to-end: create, update, and delete. It verifies that when a cluster resource is created via the HyperFleet API, the system correctly processes the resource through its lifecycle, required adapters execute successfully, and the cluster reaches `Reconciled=True`. Then it updates the cluster via PATCH, verifies adapters re-reconcile with the new generation, and finally deletes the cluster, verifying adapters clean up K8s resources, report `Finalized=True`, and the cluster is hard-deleted from the database.

---

| **Field** | **Value**     |
|-----------|---------------|
| **Pos/Neg** | Positive      |
| **Priority** | Tier0         |
| **Status** | Partially Automated |
| **Automation** | Partially Automated |
| **Version** | MVP           |
| **Created** | 2026-01-29    |
| **Updated** | 2026-04-08    |


---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully 

---

### Test Steps

#### Step 1: Submit an API request to create a Cluster resource

**Action:**
- Submit a POST request to create a Cluster resource:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Response includes the created cluster ID and initial metadata
- Initial cluster conditions have `status: False` for both condition `{"type": "Ready"}` and `{"type": "Available"}`

#### Step 2: Verify initial status of cluster
**Action:**
- Poll cluster status for initial response
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `Ready` condition `status: False`
- Cluster `Available` condition `status: False`

#### Step 3: Verify required adapter execution results

**Action:**
- Retrieve adapter statuses information:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- Response returns HTTP 200 (OK) status code
- All required adapters from config are present in the response:
  - `clusters-namespace`
  - `clusters-job`
  - `clusters-deployment`
- Each required adapter has all required condition types: `Applied`, `Available`, `Health`
- Each condition has `status: "True"` indicating successful execution
- **Adapter condition metadata validation** (for each condition in adapter.conditions):
  - `reason`: Non-empty string providing human-readable summary of the condition state
  - `message`: Non-empty string with detailed human-readable description
  - `last_transition_time`: Valid RFC3339 timestamp of the last status change
- **Adapter status metadata validation** (for each required adapter):
  - `created_time`: Valid RFC3339 timestamp when the adapter status was first created
  - `last_report_time`: Valid RFC3339 timestamp when the adapter last reported its status
  - `observed_generation`: Non-nil integer value equal to 1 for new creation requests

**Note:** Required adapters are configurable via:
- Config file: `configs/config.yaml` under `adapters.cluster`
- Environment variable: `HYPERFLEET_ADAPTERS_CLUSTER` (comma-separated list)

#### Step 4: Verify final cluster state after creation

**Action:**
- Wait for cluster Reconciled condition to transition to True
- Retrieve final cluster status information:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `Reconciled` condition transitions from `status: False` to `status: True`
- Final cluster conditions have `status: True` for both condition `{"type": "Reconciled"}` and `{"type": "Available"}`
- Validate that the observedGeneration for the Reconciled and Available conditions is 1 for a new creation request
- This confirms the cluster has reached the desired end state

#### Step 5: Update the cluster via PATCH

**Action:**
- Submit a PATCH request to update the cluster:
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"labels": {"updated-by": "e2e-test"}}'
```
- Wait for `Reconciled=True` again

**Expected Result:**
- PATCH returns successful response
- `generation` increments to `2`
- All adapters re-reconcile and report `observed_generation=2`
- Cluster reaches `Reconciled=True` again

#### Step 6: Delete the cluster and verify pending deletion state

**Action:**
- Submit a DELETE request:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```
- Immediately GET the cluster to verify pending deletion state:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- DELETE returns successful response (200 or 202)
- GET still returns the cluster (not yet hard-deleted)
- `deleted_at` field is set to a valid RFC3339 timestamp
- `generation` has incremented (from 2 to 3)
- `Reconciled` condition transitions to `status: "False"` (pending adapter cleanup)

#### Step 7: Verify adapters report Finalized=True after cleanup

**Action:**
- Poll adapter statuses until all adapters complete cleanup:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- All required adapters report the `Finalized` condition with `status: "True"`
- `Applied` condition transitions to `status: "False"` (resources no longer exist)
- `Available` condition transitions to `status: "False"` (resources no longer exist)
- `Health` remains `status: "True"` (adapter itself is healthy)
- `observed_generation` equals `3` (matching the post-delete generation)

#### Step 8: Verify cluster is hard-deleted from database

**Action:**
- Wait for the cluster record to be hard-deleted, then attempt to GET:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- GET returns HTTP 404 (Not Found)
- The cluster record has been permanently removed from the database
- Adapter status records for this cluster are also removed

#### Step 9: Cleanup (AfterEach)

**Action:**
- If the test fails before hard-delete completes, clean up remaining resources:
```bash
kubectl delete namespace {cluster_id} --ignore-not-found
```

**Expected Result:**
- Any remaining resources are cleaned up

---

## Test Title: Clusters Resource Type - K8s Resources Check Aligned with Preinstalled Clusters Related Adapters Specified

### Description

This test verifies that Kubernetes resources are successfully created with correct templated values for all required cluster adapters. The test dynamically reads the list of required adapters from config, waits for each adapter to complete execution, then validates that corresponding Kubernetes resources (Namespace, Job, Deployment) exist with properly rendered metadata (labels, annotations) matching the cluster request payload. This ensures adapter Kubernetes resource management and templating work correctly across all configured adapters.

---

| **Field** | **Value**     |
|-----------|---------------|
| **Pos/Neg** | Positive      |
| **Priority** | Tier0         |
| **Status** | Automated     |
| **Automation** | Automated     |
| **Version** | MVP           |
| **Created** | 2026-01-29    |
| **Updated** | 2026-04-08    |


---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Submit an API request to create a Cluster resource

**Action:**
- Submit a POST request to create a Cluster resource:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Response includes the created cluster ID and initial metadata
- Initial cluster conditions have `status: False` for both condition `{"type": "Ready"}` and `{"type": "Available"}`

#### Step 2: Wait for all required adapters to complete

**Action:**
- Poll adapter statuses until all required adapters complete execution:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- All required adapters from config (cl-namespace, cl-job, cl-deployment) are present
- Each adapter has all three conditions (`Applied`, `Available`, `Health`) with `status: True`

**Note:** Required adapters are configurable via `configs/config.yaml` under `adapters.cluster`

#### Step 3: Verify Kubernetes resources for each adapter with correct metadata

**Action:**
- For each required adapter, retrieve and validate corresponding Kubernetes resources:

**For cl-namespace adapter:**
```bash
kubectl get namespace {cluster_id} -o yaml
```

**Expected Result:**
- Namespace exists with name matching the cluster ID
- Namespace status phase is `Active`
- Required annotations:
  - `hyperfleet.io/generation`: Equals "1" for new creation request

**For cl-job adapter:**
```bash
kubectl get job -n {cluster_id} -l hyperfleet.io/cluster-id={cluster_id},hyperfleet.io/resource-type=job -o yaml
```

**Expected Result:**
- Job exists in the cluster namespace, identified by the label selector
- Job has completed successfully (status.succeeded > 0 or status.conditions contains type=Complete with status=True)
- Required annotations:
  - `hyperfleet.io/generation`: Equals "1" for new creation request

**For cl-deployment adapter:**
```bash
kubectl get deployment -n {cluster_id} -l hyperfleet.io/cluster-id={cluster_id},hyperfleet.io/resource-type=deployment -o yaml
```

**Expected Result:**
- Deployment exists in the cluster namespace, identified by the label selector
- Deployment is available (status.availableReplicas > 0 and status.conditions contains type=Available with status=True)
- Required annotations:
  - `hyperfleet.io/generation`: Equals "1" for new creation request

#### Step 4: Delete the cluster and verify K8s resources are cleaned up

**Action:**
- Submit a DELETE request:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```
- Wait for adapters to complete cleanup and cluster to be hard-deleted
- Verify K8s resources have been removed:
```bash
kubectl get namespace {cluster_id}
kubectl get job -n {cluster_id} -l hyperfleet.io/cluster-id={cluster_id}
kubectl get deployment -n {cluster_id} -l hyperfleet.io/cluster-id={cluster_id}
```

**Expected Result:**
- Namespace no longer exists (or is in Terminating state)
- Job no longer exists
- Deployment no longer exists
- Cluster is hard-deleted (GET returns 404)

#### Step 5: Cleanup (AfterEach)

**Action:**
- If the test fails before hard-delete completes, clean up remaining resources:
```bash
kubectl delete namespace {cluster_id} --ignore-not-found
```

---

## Test Title: Clusters Resource Type - Adapter Dependency Relationships Workflow Validation

### Description

This test validates that CLM correctly handles adapter dependency relationships when processing a clusters resource request. Specifically, it verifies the dependency relationship where the cl-deployment adapter depends on the cl-job adapter completion. The test continuously polls and validates throughout the workflow period to ensure: (1) cl-deployment's Applied condition remains False until cl-job's Available condition reaches True, enforcing the dependency precondition; (2) during cl-job execution, cl-deployment's Available condition stays Unknown (never False), confirming the adapter waits correctly without attempting execution; (3) successful completion with cl-deployment's Available eventually transitioning to True. This validation demonstrates that the workflow engine properly enforces adapter dependencies and ensures dependent adapters wait for prerequisites before executing.

---

| **Field** | **Value**     |
|-----------|---------------|
| **Pos/Neg** | Positive      |
| **Priority** | Tier0         |
| **Status** | Automated     |
| **Automation** | Automated     |
| **Version** | MVP           |
| **Created** | 2026-01-29    |
| **Updated** | 2026-02-11    |


---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully 
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Submit an API request to create a Cluster resource
**Action:**
- Submit a POST request to create a Cluster resource:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- API returns successful response

#### Step 2: Verify cl-deployment initial state and dependency waiting behavior

**Action:**
- Poll adapter statuses to capture cl-deployment's initial waiting state:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
At the initial state (when cl-deployment first appears in statuses):
- Response returns HTTP 200 (OK) status code
- The `cl-deployment` adapter is present with initial waiting state:
  - `Applied` condition has `status: "False"` (deployment hasn't been applied yet, waiting for cl-job dependency)
  - `Available` condition has `status: "Unknown"` (deployment hasn't been applied yet)
  - `Health` condition has `status: "True"` (adapter itself is healthy, just waiting)

#### Step 3: Verify dependency relationship and condition transitions throughout entire workflow

**Action:**
- Continuously poll adapter statuses from the initial state until cl-deployment completes:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
Throughout the entire period (from initial state until cl-deployment completes), validate the following on each poll:

**Validation 1 - Dependency enforcement (during cl-job execution):**
- While `cl-job` adapter's `Available` condition has NOT reached `status: "True"`:
  - The `cl-deployment` adapter's `Applied` condition must remain `status: "False"`
  - The `cl-deployment` adapter's `Available` condition must remain `status: "Unknown"` (never `status: "False"`)
  - This validates that cl-deployment waits for cl-job to complete without attempting to apply resources

**Validation 2 - Success condition:**
- Once `cl-job` adapter's `Available` reaches `status: "True"`, cl-deployment can proceed with execution
- Once `cl-deployment` completes execution, its `Available` condition eventually becomes `status: "True"`
- This confirms the complete dependency workflow succeeded

**Note:** After cl-job completes, cl-deployment's `Available` condition may temporarily be `False` (e.g., `MinimumReplicasUnavailable` during deployment startup) before becoming `True`, which is expected behavior and not validated.

#### Step 4: Cleanup resources

**Action:**
- Delete the namespace created for this cluster:
```bash
kubectl delete namespace {cluster_id}
```

**Expected Result:**
- Namespace and all associated resources are deleted successfully

**Note:** This is a workaround cleanup method. Once CLM supports DELETE operations for "clusters" resource type, this step should be replaced with:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

---

## Test Title: Cluster can reflect adapter failure in top-level status

### Description

This test validates that the end-to-end workflow correctly handles adapter failure scenarios. When an adapter's precondition configuration contains an invalid API endpoint URL, the adapter framework should detect the failure and report error status. The cluster's top-level conditions (`Ready`, `Available`) should remain `False`, accurately reflecting that the cluster has not reached a healthy state. This is a common configuration error scenario when external teams implement their own adapters.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Automated |
| **Automation** | Automated |
| **Version** | MVP |
| **Created** | 2026-02-11 |
| **Updated** | 2026-03-19 |


---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully

---

### Test Steps

#### Step 1: Deploy dedicated precondition-error-adapter with invalid precondition URL
**Action:**
- Deploy a precondition-error-adapter via Helm with AdapterConfig containing a precondition that references an invalid API endpoint URL, separate from the normal adapters used in other tests. For example:
```yaml
preconditions:
  - name: "clusterStatus"
    apiCall:
      method: "GET"
      url: "http://invalid-service:8080/api/nonexistent"
    capture:
      - name: "clusterName"
        field: "name"
```

**Expected Result:**
- precondition-error-adapter is deployed and running successfully

#### Step 2: Submit an API request to create a Cluster resource

**Action:**
- Submit a POST request to create a Cluster resource:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- API returns successful response with cluster ID

#### Step 3: Verify adapter failure is reported via status API

**Action:**
- Poll adapter statuses until the precondition-error-adapter reports its status:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- The precondition-error-adapter is present in the statuses response
- The adapter reports `Applied` condition with `status: "False"`
- The adapter reports `Available` condition with `status: "False"`
- The adapter reports `Health` condition with `status: "False"`, with reason and message indicating precondition failure details

#### Step 4: Verify cluster top-level status reflects adapter failure

**Action:**
- Retrieve cluster status:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `Ready` condition remains `status: "False"`
- Cluster `Available` condition remains `status: "False"`
- Cluster does not transition to Ready state while any adapter reports failure

#### Step 5: Cleanup Resources (AfterEach)

**Action:**
- Delete the namespace created for this cluster:
```bash
kubectl delete namespace {cluster_id}
```
- Uninstall the precondition-error-adapter Helm release
- Clean up the Pub/Sub subscription created by the adapter (if using Google Pub/Sub broker):
```bash
gcloud pubsub subscriptions delete {subscription_id} --project={project_id}
```

**Expected Result:**
- Namespace and all associated resources are deleted successfully
- precondition-error-adapter deployment is removed
- Pub/Sub subscription is deleted (if applicable)

**Note:** This is a workaround cleanup method. Once CLM supports DELETE operations for "clusters" resource type, the namespace deletion should be replaced with:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

---

## Test Title: Cluster can reach correct status after adapter crash and recovery

### Description

This test validates the system's self-healing capability. When an adapter crashes during cluster processing, the system should ensure that the cluster's status is eventually reported correctly after the adapter recovers. This confirms that no cluster is left in an inconsistent state due to adapter failures.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier2 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | MVP |
| **Created** | 2026-02-11 |
| **Updated** | 2026-03-27 |


---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Deploy dedicated crash-adapter and then simulate crash

**Action:**
- Deploy a dedicated crash-adapter via Helm (`${ADAPTER_DEPLOYMENT_NAME}`), separate from the normal adapters used in other tests
- Scale down the crash-adapter deployment to simulate a crash:
```bash
kubectl scale deployment ${ADAPTER_DEPLOYMENT_NAME} -n ${NAMESPACE} --replicas=0
```
- Wait briefly to ensure the adapter is fully stopped before proceeding to Step 2

**Expected Result:**
- crash-adapter becomes unavailable

#### Step 2: Submit an API request to create a Cluster resource

**Action:**
- Submit a POST request to create a Cluster resource:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- API returns successful response with cluster ID

#### Step 3: Verify crash-adapter has not reported status

**Action:**
- Poll adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```
- Retrieve cluster status:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Statuses response does not contain an entry for `crash-adapter` (it is unavailable)
- Other required adapters have reported their statuses
- Cluster `Ready` condition remains `status: "False"`

#### Step 4: Restore crash-adapter and verify cluster reaches correct status

**Action:**
- Scale up the crash-adapter deployment back to 1 replica:
```bash
kubectl scale deployment ${ADAPTER_DEPLOYMENT_NAME} -n ${NAMESPACE} --replicas=1
```
- Poll adapter statuses until the crash-adapter reports:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```
- Retrieve cluster status:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- crash-adapter status entry is now present in the statuses response
- crash-adapter reports all three condition types with `status: "True"`: `Applied`, `Available`, `Health`
- `observed_generation` is set to `1`
- Cluster `Ready` condition transitions to `status: "True"`
- Cluster `Available` condition transitions to `status: "True"`
- This confirms no cluster is left in an inconsistent state due to adapter failures

#### Step 5: Cleanup Resources (AfterEach)

**Action:**
- Delete the namespace created for this cluster:
```bash
kubectl delete namespace {cluster_id}
```
- Uninstall the crash-adapter Helm release
- Clean up the Pub/Sub subscription created by the adapter (if using Google Pub/Sub broker):
```bash
gcloud pubsub subscriptions delete {subscription_id} --project={project_id}
```

**Expected Result:**
- Namespace and all associated resources are deleted successfully
- crash-adapter deployment is removed
- Pub/Sub subscription is deleted (if applicable)

**Note:** This is a workaround cleanup method. Once CLM supports DELETE operations for "clusters" resource type, the namespace deletion should be replaced with:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

---
## Test Title: DELETE hierarchical: subresource cleanup before resource hard-delete

### Description

This test validates the hierarchical deletion workflow. When a cluster with a nodepool (subresource) is deleted, the API marks both for deletion simultaneously, but hard-deletes hierarchically: subresource records are removed first (when subresource `Reconciled=True`), then the resource record is removed only after all subresource records are gone and the resource itself reaches `Reconciled=True`.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Preconditions

1. Environment is prepared with all required platform resources
2. HyperFleet API, Sentinel (for both clusters and nodepools), and adapters are deployed
3. Adapters support `lifecycle.delete` in their task configs

---

### Test Steps

#### Step 1: Create a cluster and nodepool, wait for both to reach Reconciled=True

**Action:**
- Create a cluster via POST, wait for `Reconciled=True`
- Create a nodepool under this cluster via POST, wait for nodepool `Reconciled=True`

**Expected Result:**
- Both cluster and nodepool reach `Reconciled=True`

#### Step 2: Delete the cluster

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Both cluster and nodepool have `deleted_at` set (API cascades deletion to subresources)
- Both cluster and nodepool `generation` incremented

#### Step 3: Verify nodepool is hard-deleted before cluster

**Action:**
- Poll nodepool and cluster status until deletion completes:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Nodepool adapters report `Finalized=True` -> nodepool `Reconciled=True`
- Nodepool record is hard-deleted (GET returns 404) **before** cluster record is hard-deleted
- After nodepool record is gone AND cluster adapters report `Finalized=True` -> cluster `Reconciled=True`
- Cluster record is hard-deleted (GET returns 404)

#### Step 4: Cleanup (AfterEach)

**Action:**
- Clean up any remaining K8s resources if test fails mid-way

---

## Test Title: 409 Conflict returned for mutations on tombstoned resources

### Description

This test validates the API behavior during the deletion workflow. When a cluster is marked for deletion (`deleted_at` set), the API must reject all mutations (PATCH) with HTTP 409 Conflict to prevent new generation events from triggering resource creation while cleanup is in progress. Read operations (GET, LIST) and idempotent DELETE must still be allowed.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Preconditions

1. Environment is prepared with all required platform resources
2. HyperFleet API and adapters are deployed

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled=True

**Action:**
- Create a cluster, wait for `Reconciled=True`

#### Step 2: Delete the cluster

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- `deleted_at` is set on the cluster

#### Step 3: Verify PATCH is rejected with 409

**Action:**
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"labels": {"should": "fail"}}'
```

**Expected Result:**
- API returns HTTP 409 (Conflict)
- Response body indicates the resource is pending deletion

#### Step 4: Verify GET and LIST are still allowed

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters
```

**Expected Result:**
- GET returns HTTP 200 with the cluster data (including `deleted_at`)
- LIST returns HTTP 200 and includes the cluster in results

#### Step 5: Verify creating nodepool on tombstoned cluster is rejected

**Action:**
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```

**Expected Result:**
- API returns HTTP 409 (Conflict)
- Subresource creation is rejected on a resource pending deletion

#### Step 6: Verify DELETE is idempotent

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Second DELETE returns successful response (idempotent -- does not error)

#### Step 7: Cleanup (AfterEach)

**Action:**
- Wait for hard-delete or clean up remaining K8s resources

---

## Test Title: Stale Applied=False before tombstone does not trigger premature hard-delete

### Description

This test validates DDR edge case #2 (Critical). When an adapter has `Applied=False` from before deletion started (e.g., precondition not met), the deletion workflow must not prematurely hard-delete the resource. The workflow gates hard-delete on all adapters reporting `Finalized=True` (via aggregate `Reconciled`), not on individual `Applied` status. This is verified end-to-end: create a cluster with a failing adapter, delete it, and confirm the resource remains in pending deletion state.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Preconditions

1. Environment is prepared with all required platform resources
2. A dedicated test adapter is deployed with a precondition that will fail (e.g., invalid API endpoint), causing it to report `Applied=False` persistently

---

### Test Steps

#### Step 1: Create cluster with a failing adapter

**Action:**
- Deploy a test adapter with a failing precondition
- Add it to required adapters
- Create a cluster

**Expected Result:**
- Cluster does NOT reach `Reconciled=True` (failing adapter blocks it)
- Failing adapter reports `Applied=False`, `Health=False`

#### Step 2: Delete the cluster while adapter has stale Applied=False

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

#### Step 3: Verify cluster is NOT prematurely hard-deleted

**Action:**
- Wait for a reasonable period and poll the cluster:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster is still accessible (NOT hard-deleted)
- `deleted_at` is set
- `Reconciled` remains `False` -- the workflow waits for `Finalized=True` from all adapters, not `Applied=False`
- The failing adapter has NOT reported `Finalized=True` (it cannot confirm cleanup while unhealthy)

#### Step 4: Cleanup (AfterEach)

**Action:**
- Restore/fix the adapter, or force cleanup of remaining resources

---

## Test Title: Concurrent deletion events handled idempotently

### Description

This test validates DDR edge case #6. Multiple DELETE calls for the same cluster must be handled idempotently across the entire workflow. The API should not produce errors, K8s delete operations are safe to call multiple times, and the system should reach the same final state regardless of how many DELETE requests are sent.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Test Steps

#### Step 1: Create cluster and wait for Reconciled=True

#### Step 2: Send multiple DELETE requests concurrently

**Action:**
- Send 3-5 DELETE requests simultaneously:
```bash
for i in $(seq 1 5); do
  curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} &
done
wait
```

**Expected Result:**
- All DELETE requests return successful responses (no 500 errors)
- `deleted_at` is set exactly once
- `generation` increments exactly once (not once per DELETE call)

#### Step 3: Verify deletion completes normally

**Expected Result:**
- Adapters clean up resources normally
- Cluster is eventually hard-deleted (GET returns 404)
- No duplicate cleanup attempts or errors

---

## Test Title: Resource already gone is treated as deletion success

### Description

This test validates DDR edge case #1. When K8s resources have already been deleted externally (e.g., manually or by another process) before the deletion workflow runs, the workflow should still complete successfully. Adapters treat `NotFound` from resource discovery as success and report `Finalized=True`, allowing the cluster to be hard-deleted normally.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Test Steps

#### Step 1: Create cluster and wait for Reconciled=True

#### Step 2: Manually delete K8s resources before API deletion

**Action:**
- Delete the K8s resources that adapters would normally clean up:
```bash
kubectl delete namespace {cluster_id}
```

#### Step 3: Delete the cluster via API

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

#### Step 4: Verify deletion workflow completes despite resources already gone

**Expected Result:**
- Adapters discover resources are already gone (NotFound)
- Adapters report `Finalized=True` (treat NotFound as successfully deleted)
- Cluster reaches deletion `Reconciled=True` and is hard-deleted normally (GET returns 404)

---

## Test Title: Adapter unhealthy blocks hard-delete via Finalized=False

### Description

This test validates DDR edge case #11 (Critical). When an adapter is unhealthy during the deletion workflow, it must report `Finalized=False` because it cannot reliably confirm cleanup. This blocks the workflow from completing hard-delete, preventing data loss. Once the adapter recovers, it processes the deletion and the workflow completes normally. This is verified end-to-end: scale down an adapter, delete the cluster, confirm it stays in pending deletion, then restore the adapter and confirm hard-delete completes.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Test Steps

#### Step 1: Create cluster and wait for Reconciled=True

#### Step 2: Make an adapter unhealthy, then delete the cluster

**Action:**
- Scale down one adapter to simulate unhealthy state:
```bash
kubectl scale deployment ${ADAPTER_DEPLOYMENT} -n ${NAMESPACE} --replicas=0
```
- Delete the cluster:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

#### Step 3: Verify cluster is NOT hard-deleted while adapter is unhealthy

**Action:**
- Poll cluster and adapter statuses over a reasonable period

**Expected Result:**
- The unhealthy adapter does NOT report `Finalized=True`
- Cluster `Reconciled` remains `False`
- Cluster is NOT hard-deleted (GET still returns 200 with `deleted_at` set)
- Other healthy adapters may report `Finalized=True`, but hard-delete is blocked until ALL adapters confirm

#### Step 4: Restore the adapter and verify deletion workflow completes

**Action:**
- Scale the adapter back up:
```bash
kubectl scale deployment ${ADAPTER_DEPLOYMENT} -n ${NAMESPACE} --replicas=1
```

**Expected Result:**
- Adapter recovers, processes the deletion, reports `Finalized=True`
- Cluster reaches `Reconciled=True` and is hard-deleted (GET returns 404)

---

## Test Title: Multi-generation resource cleanup via label selectors

### Description

This test validates DDR edge case #10. When a cluster has been updated multiple times (multiple generations), the deletion workflow must clean up all resource instances across all generations, not just the latest. This is verified end-to-end: create a cluster, update it multiple times to produce generation-specific K8s resources, delete it, and confirm all resources across all generations are cleaned up.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Test Steps

#### Step 1: Create cluster, wait for Reconciled=True

#### Step 2: Update the cluster to create multi-generation resources

**Action:**
- PATCH the cluster multiple times to increment generation
- Wait for adapters to reconcile each update (some adapters create generation-specific resources, e.g., Jobs with generation in the name)

**Expected Result:**
- Multiple generation-specific resources exist (e.g., `validation-{cluster_id}-gen1`, `validation-{cluster_id}-gen2`)

#### Step 3: Delete the cluster

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

#### Step 4: Verify ALL generation resources are cleaned up

**Expected Result:**
- All generation-specific K8s resources are deleted (not just the latest generation)
- Adapters report `Finalized=True` only after all resources across all generations are confirmed deleted
- Cluster is hard-deleted (GET returns 404)

---

## Test Title: Independent subresource deletion

### Description

This test validates DDR edge case #7. A single subresource (nodepool) can be deleted independently without deleting the parent resource (cluster). The deletion workflow marks only the subresource for deletion, subresource-level adapters handle cleanup, and the parent cluster remains unaffected throughout.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Test Steps

#### Step 1: Create cluster with nodepool, wait for both to reach Reconciled=True

#### Step 2: Delete only the nodepool (not the cluster)

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Nodepool has `deleted_at` set
- Cluster does NOT have `deleted_at` set (unaffected by subresource deletion)

#### Step 3: Verify nodepool is hard-deleted while cluster remains

**Expected Result:**
- Nodepool adapters clean up K8s resources, report `Finalized=True`
- Nodepool `Reconciled=True` -> nodepool record hard-deleted (GET returns 404)
- Cluster remains accessible and in `Reconciled=True` state (unaffected)
- Cluster K8s resources remain intact

---

## Test Title: DELETE while creation is still in progress

### Description

This test validates DDR edge case #3. When a user creates a cluster and immediately deletes it before adapters finish processing the creation (cluster has not yet reached `Reconciled=True`), the deletion workflow should still complete successfully. The adapter receives the deletion event, sees `deleted_at`, and switches to cleanup mode regardless of whether creation was fully completed.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Test Steps

#### Step 1: Create a cluster (do NOT wait for Reconciled=True)

**Action:**
- Submit a POST request to create a Cluster resource:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster is created with a valid ID
- `Reconciled` is `False` (adapters are still processing)

#### Step 2: Immediately delete the cluster

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- DELETE returns successful response
- `deleted_at` is set

#### Step 3: Verify deletion workflow completes

**Action:**
- Poll cluster status until hard-delete:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Adapters detect `deleted_at`, switch to deletion mode, clean up any resources that were partially created
- Adapters report `Finalized=True`
- Cluster reaches deletion `Reconciled=True` and is hard-deleted (GET returns 404)
- No orphaned K8s resources remain

#### Step 4: Cleanup (AfterEach)

**Action:**
- Clean up any remaining K8s resources if test fails:
```bash
kubectl delete namespace {cluster_id} --ignore-not-found
```

---

## Test Title: DELETE while update reconciliation is still in progress

### Description

This test validates the interaction between update and delete workflows. When a user updates a cluster via PATCH and immediately deletes it before adapters finish reconciling the update, the deletion workflow should take priority. The adapter receives the next event, sees `deleted_at`, and switches to cleanup mode instead of continuing the update reconciliation.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled=True

**Action:**
- Create a cluster, wait for `Reconciled=True` at `generation=1`

#### Step 2: Update the cluster (do NOT wait for re-reconciliation)

**Action:**
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"labels": {"updated-by": "e2e-test"}}'
```

**Expected Result:**
- `generation` increments to `2`
- `Reconciled` transitions to `False` (adapters are re-reconciling)

#### Step 3: Immediately delete the cluster before update reconciliation completes

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- DELETE returns successful response
- `deleted_at` is set
- `generation` increments to `3`

#### Step 4: Verify deletion workflow completes

**Action:**
- Poll cluster status until hard-delete:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Adapters detect `deleted_at` on next reconciliation, switch to deletion mode
- Adapters clean up K8s resources and report `Finalized=True`
- Cluster reaches deletion `Reconciled=True` and is hard-deleted (GET returns 404)
- No orphaned K8s resources remain

---

## Test Title: Recreate cluster with same name after hard-delete

### Description

This test validates that after a cluster is fully deleted (hard-deleted from database), a new cluster can be created with the same name without conflicts. This is a common user scenario: delete a cluster, then recreate it with the same configuration. The system must ensure no state from the previous cluster interferes with the new creation.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled=True

**Action:**
- Create a cluster with a specific name, wait for `Reconciled=True`
- Record the cluster name

#### Step 2: Delete the cluster and wait for hard-delete

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```
- Wait until GET returns 404 (hard-delete complete)

**Expected Result:**
- Cluster is fully hard-deleted from database
- K8s resources are cleaned up

#### Step 3: Recreate a cluster with the same name

**Action:**
- Submit a POST request with the same cluster name:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{"name": "{same_cluster_name}", ...}'
```

**Expected Result:**
- POST returns successful response with a new cluster ID (different from the deleted one)
- No conflict with the previously deleted cluster
- The new cluster proceeds through the normal workflow and reaches `Reconciled=True`
- K8s resources are created fresh (no leftover state from the previous cluster)

#### Step 4: Cleanup (AfterEach)

**Action:**
- Delete the recreated cluster or clean up K8s resources

---

## Test Title: Multiple consecutive updates

### Description

This test validates that the system correctly handles multiple consecutive PATCH operations. Each PATCH increments the generation, and adapters must reconcile each generation change. After all updates, the final `observed_generation` must match the latest `generation`, confirming no generation was skipped or lost.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled=True

**Expected Result:**
- `generation=1`, all adapters `observed_generation=1`

#### Step 2: Apply 3 consecutive PATCH updates

**Action:**
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"labels": {"update": "1"}}'

# Wait for Reconciled=True

curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"labels": {"update": "2"}}'

# Wait for Reconciled=True

curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"labels": {"update": "3"}}'
```

**Expected Result:**
- After each PATCH, `generation` increments (2, 3, 4)
- After each PATCH, `Reconciled` transitions to `False` then back to `True`

#### Step 3: Verify final state

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- Cluster `generation=4`
- All adapters `observed_generation=4`
- Cluster `Reconciled=True`
- K8s resources reflect the latest update (labels include `{"update": "3"}`)

#### Step 4: Cleanup (AfterEach)

**Action:**
- Delete cluster via API or clean up K8s resources

---

## Test Title: Concurrent updates on same cluster

### Description

This test validates that the system handles concurrent PATCH requests to the same cluster without errors or inconsistent state. Multiple simultaneous updates should each succeed or return a reasonable conflict error, and the final state should be consistent with the latest generation.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-08 |
| **Updated** | 2026-04-08 |

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled=True

#### Step 2: Send multiple PATCH requests concurrently

**Action:**
```bash
for i in $(seq 1 5); do
  curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
    -H "Content-Type: application/json" \
    -d "{\"labels\": {\"concurrent-update\": \"$i\"}}" &
done
wait
```

**Expected Result:**
- All PATCH requests return successful responses (no 500 errors)
- `generation` reflects the total number of accepted updates
- No data corruption or inconsistent state

#### Step 3: Wait for final reconciliation and verify consistent state

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- Cluster reaches `Reconciled=True`
- All adapters `observed_generation` matches cluster `generation`
- Final cluster state is consistent (labels reflect one of the concurrent updates)

#### Step 4: Cleanup (AfterEach)

**Action:**
- Delete cluster via API or clean up K8s resources

---
