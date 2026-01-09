# NodePool E2E Test Cases

**Status**: Draft

## Overview

This document defines detailed E2E test cases for NodePool lifecycle management, focusing on:
- Happy path workflow coverage from API request to final status reporting
- Failure scenario handling and error reporting
- Complete CLM workflow validation: API request → API service processing → Sentinel message posting → Adapter resource handling → Adapter status reporting

### API Endpoints Under Test

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools` | POST | Create a new nodepool |
| `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools` | GET | List all nodepools |
| `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}` | GET | Get nodepool details |
| `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}/statuses` | GET | Get adapter statuses |

---

## Test Case: E2E-004

### Full Nodepool Creation Flow on GCP

| Field | Value |
|-------|-------|
| **Test Case ID** | E2E-004 |
| **Title** | Full Nodepool Creation Flow on GCP |
| **Priority** | P0 (MVP-Critical) |
| **Type** | Happy Path |
| **Scope** | MVP |

#### Objective

Validate end-to-end nodepool creation from API request to Ready state for an existing cluster on GCP, covering the complete CLM workflow: API request → API service processing → Sentinel message posting → Adapter resource handling → Adapter status reporting.

#### Prerequisites

| Prerequisite | Description |
|--------------|-------------|
| PRE-001 | Cluster created via E2E-001 and in Ready state |
| PRE-002 | Valid authentication credentials available |
| PRE-003 | GCP provider configured and accessible |
| PRE-004 | Test environment properly configured |

#### Test Data

```json
{
  "name": "gpu-nodepool-e2e-004",
  "machineType": "n1-standard-8",
  "replicas": 2,
  "labels": {
    "workload": "gpu",
    "tier": "compute",
    "environment": "test"
  }
}
```

**Note**: The detailed request body spec is still evolving and subject to change.

#### Test Steps

##### Step 1: Submit Nodepool Creation Request

**Action**: Send POST request to create nodepool

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "name": "gpu-nodepool-e2e-004",
  "machineType": "n1-standard-8",
  "replicas": 2,
  "labels": {
    "workload": "gpu",
    "tier": "compute",
    "environment": "test"
  }
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 1.1 | HTTP Status Code | 201 Created |
| 1.2 | Response contains nodepool ID | Non-empty string UUID |
| 1.3 | Response status.phase | "Not Ready" |
| 1.4 | Response status.adapters | Empty array `[]` |
| 1.5 | Response status.lastUpdated | Timestamp set |
| 1.6 | Response generation | 1 |
| 1.7 | Response metadata.name | "gpu-nodepool-e2e-004" |
| 1.8 | Response metadata.labels | Contains submitted labels |

**Workflow Verification (API Service Processing)**:
- API service receives and validates request
- API service persists nodepool resource to database
- API service returns response with initial status

---

##### Step 2: Verify Nodepool Appears in List

**Action**: Send GET request to list nodepools

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 2.1 | HTTP Status Code | 200 OK |
| 2.2 | Response contains created nodepool | Nodepool ID matches created |
| 2.3 | Nodepool details match request | Name, machineType, replicas match |

**Label Filtering Test**:
```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools?labels=workload%3Dgpu
```

| # | Check | Expected Result |
|---|-------|-----------------|
| 2.4 | Label filter works | Returns nodepool with matching label |
| 2.5 | Non-matching filter returns empty | `?labels=workload%3Dcpu` returns empty |

---

##### Step 3: Monitor Nodepool Status (Sentinel Message Posting)

**Action**: Poll nodepool status via GET request

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}
```

**Workflow Verification (Sentinel Processing)**:
- Sentinel operator detects new nodepool resource
- Sentinel polls API for nodepool status
- Sentinel publishes events for nodepool to message broker
- Events contain nodepool spec and current status

**Validation Points (During Processing)**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 3.1 | HTTP Status Code | 200 OK |
| 3.2 | status.phase | "Not Ready" (until adapters complete) |
| 3.3 | status.adapters | Gradually populated as adapters report |
| 3.4 | status.lastUpdated | Updated as status changes |

**Polling Strategy**:
- Poll interval: 10 seconds
- Max wait time: 15 minutes
- Exit condition: status.phase = "Ready" OR timeout

---

##### Step 4: Monitor Adapter Statuses (Adapter Resource Handling)

**Action**: Poll adapter statuses via GET request

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}/statuses
```

**Workflow Verification (Adapter Processing)**:
- Adapters consume events from message broker
- Adapters evaluate preconditions
- Adapters create Kubernetes Jobs for nodepool operations
- Adapters report status back to API

**Expected Adapter Status Transitions**:

| Adapter | Phase | Available | Applied | Health |
|---------|-------|-----------|---------|--------|
| Validation Adapter | Initial | False (JobRunning) | True (JobLaunched) | True (NoErrors) |
| Validation Adapter | Complete | True (JobSucceeded) | True (JobLaunched) | True (NoErrors) |
| Nodepool Adapter | Initial | False (JobRunning) | True (JobLaunched) | True (NoErrors) |
| Nodepool Adapter | Complete | True (JobSucceeded) | True (JobLaunched) | True (NoErrors) |

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 4.1 | HTTP Status Code | 200 OK |
| 4.2 | Response structure | ONE NodepoolStatus object |
| 4.3 | adapterStatuses array | Contains all adapter statuses |
| 4.4 | Each adapter has conditions | Available, Applied, Health present |
| 4.5 | Validation Adapter transitions | False → True for Available |
| 4.6 | Nodepool Adapter transitions | False → True for Available |

**Sample Expected Response (During Processing)**:
```json
{
  "nodepoolId": "{nodepool_id}",
  "adapterStatuses": [
    {
      "name": "validation-adapter",
      "conditions": [
        {
          "type": "Available",
          "status": "False",
          "reason": "JobRunning",
          "message": "Validation job is executing",
          "lastTransitionTime": "2025-01-09T10:00:00Z"
        },
        {
          "type": "Applied",
          "status": "True",
          "reason": "JobLaunched",
          "message": "Kubernetes Job created successfully"
        },
        {
          "type": "Health",
          "status": "True",
          "reason": "NoErrors",
          "message": "Adapter executing normally"
        }
      ],
      "data": {}
    }
  ]
}
```

---

##### Step 5: Verify Final State (Status Reporting Complete)

**Action**: Verify nodepool reaches Ready state

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 5.1 | status.phase | "Ready" |
| 5.2 | status.adapters | All adapters listed |
| 5.3 | Each adapter.available | "True" |
| 5.4 | Each adapter.observedGeneration | 1 (matches nodepool.generation) |
| 5.5 | status.lastUpdated | Recent timestamp |

**Verify Final Adapter Statuses**:
```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}/statuses
```

| # | Check | Expected Result |
|---|-------|-----------------|
| 5.6 | All adapters Available | True |
| 5.7 | All adapters Applied | True |
| 5.8 | All adapters Health | True |
| 5.9 | No error conditions | No Failed/Error reasons |

---

##### Step 6: Verify Nodes are Running in Cluster

**Action**: Verify actual nodes are created and joined to cluster

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 6.1 | Node count | 2 nodes (matches replicas) |
| 6.2 | Node status | All nodes Ready |
| 6.3 | Node labels | Contain nodepool labels |
| 6.4 | Node machine type | Matches n1-standard-8 |

#### Expected Duration

| Phase | Duration |
|-------|----------|
| API Response | < 2 seconds |
| Sentinel Event Publishing | < 30 seconds |
| Adapter Processing | 5-10 minutes |
| Node Provisioning | 5-10 minutes |
| **Total** | **10-15 minutes** |

#### Success Criteria

| Criteria | Description |
|----------|-------------|
| SC-001 | Nodepool transitions to Ready state |
| SC-002 | All adapters complete successfully with Available=True |
| SC-003 | Nodes are created and healthy in the cluster |
| SC-004 | No errors in logs (API, Sentinel, Adapters, Jobs) |
| SC-005 | Kubernetes Jobs complete successfully |
| SC-006 | Complete workflow chain validated |

#### Cleanup

1. Delete the created nodepool
2. Wait for nodepool deletion to complete
3. Verify no orphaned nodes remain

---

## Test Case: E2E-FAIL-002

### Nodepool API Request Body Validation Failures

| Field | Value |
|-------|-------|
| **Test Case ID** | E2E-FAIL-002 |
| **Title** | Nodepool API Request Body Validation Failures |
| **Priority** | P1 |
| **Type** | Failure Scenario |
| **Scope** | MVP |

#### Objective

Validate API properly validates nodepool creation request body and rejects invalid requests with clear error messages. Ensure validation happens at API layer before any adapter processing begins.

#### Prerequisites

| Prerequisite | Description |
|--------------|-------------|
| PRE-001 | Cluster created via E2E-001 and in Ready state |
| PRE-002 | Valid authentication credentials available |

#### Test Scenarios

---

##### Scenario 2.1: Missing Required Field - Name

**Action**: Submit nodepool creation without required `name` field

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "machineType": "n1-standard-8",
  "replicas": 2
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 2.1.1 | HTTP Status Code | 400 Bad Request |
| 2.1.2 | Error response contains field name | "name" mentioned |
| 2.1.3 | Error message is descriptive | "name is required" or similar |
| 2.1.4 | No resource created in database | GET nodepools doesn't show new entry |
| 2.1.5 | No adapter processing triggered | No events in message broker |

**Expected Error Response**:
```json
{
  "kind": "Error",
  "id": "400",
  "href": "/api/hyperfleet/v1/errors/400",
  "code": "HYPERFLEET-400",
  "reason": "Validation failed: 'name' is a required field"
}
```

---

##### Scenario 2.2: Invalid Field Value - Negative Replicas

**Action**: Submit nodepool creation with negative replicas value

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "name": "test-nodepool-invalid",
  "machineType": "n1-standard-8",
  "replicas": -1
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 2.2.1 | HTTP Status Code | 400 Bad Request |
| 2.2.2 | Error response contains field name | "replicas" mentioned |
| 2.2.3 | Error message indicates constraint | "must be positive" or "must be >= 0" |
| 2.2.4 | No resource created in database | GET nodepools doesn't show new entry |

**Expected Error Response**:
```json
{
  "kind": "Error",
  "id": "400",
  "href": "/api/hyperfleet/v1/errors/400",
  "code": "HYPERFLEET-400",
  "reason": "Validation failed: 'replicas' must be a positive integer"
}
```

---

##### Scenario 2.3: Invalid Field Value - Empty Machine Type

**Action**: Submit nodepool creation with empty machineType

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "name": "test-nodepool-invalid",
  "machineType": "",
  "replicas": 2
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 2.3.1 | HTTP Status Code | 400 Bad Request |
| 2.3.2 | Error response contains field name | "machineType" mentioned |
| 2.3.3 | Error message indicates constraint | "cannot be empty" or "is required" |
| 2.3.4 | No resource created in database | GET nodepools doesn't show new entry |

---

##### Scenario 2.4: Invalid Field Type - Wrong Data Type

**Action**: Submit nodepool creation with wrong data type for replicas

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "name": "test-nodepool-invalid",
  "machineType": "n1-standard-8",
  "replicas": "two"
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 2.4.1 | HTTP Status Code | 400 Bad Request |
| 2.4.2 | Error indicates type mismatch | "integer expected" or similar |
| 2.4.3 | No resource created in database | GET nodepools doesn't show new entry |

---

##### Scenario 2.5: Invalid JSON Syntax

**Action**: Submit nodepool creation with malformed JSON

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "name": "test-nodepool-invalid",
  "machineType": "n1-standard-8"
  "replicas": 2
}
```
(Note: Missing comma after machineType)

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 2.5.1 | HTTP Status Code | 400 Bad Request |
| 2.5.2 | Error indicates JSON parse error | "invalid JSON" or "parse error" |
| 2.5.3 | No resource created in database | GET nodepools doesn't show new entry |

---

##### Scenario 2.6: Unsupported Field

**Action**: Submit nodepool creation with unsupported field

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "name": "test-nodepool-invalid",
  "machineType": "n1-standard-8",
  "replicas": 2,
  "unknownField": "someValue"
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 2.6.1 | HTTP Status Code | 400 Bad Request (or 201 if ignoring unknown fields) |
| 2.6.2 | If rejected: Error mentions unknown field | "unknownField is not recognized" |
| 2.6.3 | If accepted: Unknown field is ignored | Response doesn't contain unknownField |

---

##### Scenario 2.7: Duplicate Nodepool Name

**Action**: Submit nodepool creation with already existing name

**Prerequisites**: Create a nodepool with name "existing-nodepool" first

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "name": "existing-nodepool",
  "machineType": "n1-standard-8",
  "replicas": 2
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 2.7.1 | HTTP Status Code | 409 Conflict |
| 2.7.2 | Error indicates duplicate | "already exists" or "duplicate name" |
| 2.7.3 | Original nodepool unchanged | GET original nodepool shows no changes |

---

##### Scenario 2.8: Non-Existent Cluster ID

**Action**: Submit nodepool creation to non-existent cluster

```
POST /api/hyperfleet/v1/clusters/non-existent-cluster-id/nodepools
Content-Type: application/json

{
  "name": "test-nodepool",
  "machineType": "n1-standard-8",
  "replicas": 2
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 2.8.1 | HTTP Status Code | 404 Not Found |
| 2.8.2 | Error indicates cluster not found | "cluster not found" or similar |
| 2.8.3 | No resource created | No nodepool created in any cluster |

**Expected Error Response**:
```json
{
  "kind": "Error",
  "id": "404",
  "href": "/api/hyperfleet/v1/errors/404",
  "code": "HYPERFLEET-404",
  "reason": "Cluster 'non-existent-cluster-id' not found"
}
```

---

##### Scenario 2.9: Invalid Cluster ID Format

**Action**: Submit nodepool creation with malformed cluster ID

```
POST /api/hyperfleet/v1/clusters/invalid!@#$%cluster/nodepools
Content-Type: application/json

{
  "name": "test-nodepool",
  "machineType": "n1-standard-8",
  "replicas": 2
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 2.9.1 | HTTP Status Code | 400 Bad Request or 404 Not Found |
| 2.9.2 | Error indicates invalid format | "invalid cluster ID format" |

---

#### Success Criteria

| Criteria | Description |
|----------|-------------|
| SC-001 | API returns HTTP 400 for validation errors |
| SC-002 | API returns HTTP 404 for non-existent cluster |
| SC-003 | API returns HTTP 409 for duplicate resources |
| SC-004 | Error messages are clear and indicate which field failed |
| SC-005 | Error messages indicate expected format/values |
| SC-006 | No resources created in database when validation fails |
| SC-007 | API doesn't crash or return 500 errors for validation failures |
| SC-008 | Validation happens before any adapter processing begins |
| SC-009 | No events published to message broker for failed validations |

---

## Test Case: E2E-FAIL-004

### Adapter Failed (Unexpected Error) - Nodepool Operations

| Field | Value |
|-------|-------|
| **Test Case ID** | E2E-FAIL-004 |
| **Title** | Adapter Failed (Unexpected Error) - Nodepool Operations |
| **Priority** | P1 |
| **Type** | Failure Scenario |
| **Scope** | MVP |

#### Objective

Validate system handles adapter unexpected errors with proper status reporting for nodepool operations, distinguishing between Job creation failures (Applied: False) and Job execution failures (Applied: True, Health: False).

#### Prerequisites

| Prerequisite | Description |
|--------------|-------------|
| PRE-001 | Cluster created via E2E-001 and in Ready state |
| PRE-002 | Valid authentication credentials available |
| PRE-003 | Ability to configure adapter with invalid settings |

---

### Scenario A: Job Creation Failure (Applied: False)

#### Objective

Validate system properly reports when an adapter cannot create a Kubernetes Job due to infrastructure/configuration issues.

#### Test Setup

Configure the nodepool adapter with invalid YAML that references unknown Kubernetes custom resource:
- Use invalid adapter-business YAML file that references a CR (e.g., Crossplane CR) that the Kubernetes cluster does not recognize
- OR reference non-existent CRD in Job specification

#### Test Steps

##### Step A.1: Create Nodepool with Misconfigured Adapter

**Action**: Create nodepool that will trigger misconfigured adapter

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "name": "nodepool-fail-004-a",
  "machineType": "n1-standard-8",
  "replicas": 2
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| A.1.1 | HTTP Status Code | 201 Created |
| A.1.2 | Nodepool ID generated | Non-empty string |
| A.1.3 | status.phase | "Not Ready" |

---

##### Step A.2: Monitor Adapter Status

**Action**: Poll adapter statuses

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}/statuses
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| A.2.1 | HTTP Status Code | 200 OK |
| A.2.2 | Affected adapter reported | Adapter present in adapterStatuses |
| A.2.3 | Available condition | False |
| A.2.4 | Available.reason | "ResourceCreationFailed" or similar |
| A.2.5 | Available.message | Describes Job creation failure |
| A.2.6 | Applied condition | False |
| A.2.7 | Applied.reason | "JobNotCreated" or similar |
| A.2.8 | Applied.message | "Failed to create Job: unknown custom resource" |
| A.2.9 | Health condition | False |
| A.2.10 | Health.reason | "UnexpectedError" |
| A.2.11 | Health.message | "Kubernetes API rejected Job creation" |

**Expected Adapter Status Response**:
```json
{
  "nodepoolId": "{nodepool_id}",
  "adapterStatuses": [
    {
      "name": "nodepool-adapter",
      "conditions": [
        {
          "type": "Available",
          "status": "False",
          "reason": "ResourceCreationFailed",
          "message": "Failed to create Job: unknown custom resource 'crossplane.io/v1beta1' not found",
          "lastTransitionTime": "2025-01-09T10:00:00Z"
        },
        {
          "type": "Applied",
          "status": "False",
          "reason": "JobNotCreated",
          "message": "Kubernetes Job was not created due to configuration error"
        },
        {
          "type": "Health",
          "status": "False",
          "reason": "UnexpectedError",
          "message": "Kubernetes API rejected Job creation: GroupVersionKind not found"
        }
      ],
      "data": {
        "error": "resource crossplane.io/v1beta1 not found in cluster",
        "errorType": "ResourceNotFound"
      }
    }
  ]
}
```

---

##### Step A.3: Verify Nodepool Status

**Action**: Check nodepool status

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| A.3.1 | status.phase | "Not Ready" |
| A.3.2 | status.adapters | Shows affected adapter with available: "False" |
| A.3.3 | Nodepool remains in error state | Does not transition to Ready |

---

### Scenario B: Job Execution Failure (Applied: True, Health: False)

#### Objective

Validate system properly reports when an adapter creates a Job successfully but the Job fails during execution.

#### Test Setup

Configure adapter with incorrect parameter that causes Job to fail during execution:
- Provide invalid credentials
- Pass incorrect API endpoint
- Pass malformed parameter that causes Job container to exit with non-zero code

#### Test Steps

##### Step B.1: Create Nodepool with Configuration Causing Job Failure

**Action**: Create nodepool that will trigger Job execution failure

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "name": "nodepool-fail-004-b",
  "machineType": "invalid-machine-type-xyz",
  "replicas": 2
}
```

**Note**: The invalid machine type or other misconfiguration will cause the Job to fail during execution.

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| B.1.1 | HTTP Status Code | 201 Created |
| B.1.2 | Nodepool ID generated | Non-empty string |
| B.1.3 | status.phase | "Not Ready" |

---

##### Step B.2: Monitor Job Creation (Success)

**Action**: Poll adapter statuses immediately after creation

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}/statuses
```

**Validation Points (Initial)**:
| # | Check | Expected Result |
|---|-------|-----------------|
| B.2.1 | Applied condition | True |
| B.2.2 | Applied.reason | "JobCreated" or "JobLaunched" |
| B.2.3 | Health condition | True (initially, Job is running) |
| B.2.4 | Available condition | False (JobRunning) |

---

##### Step B.3: Monitor Job Execution Failure

**Action**: Continue polling adapter statuses until Job completes

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}/statuses
```

**Validation Points (After Job Failure)**:
| # | Check | Expected Result |
|---|-------|-----------------|
| B.3.1 | Available condition | False |
| B.3.2 | Available.reason | "JobFailed" |
| B.3.3 | Available.message | "Job completed with errors" |
| B.3.4 | Applied condition | True |
| B.3.5 | Applied.reason | "JobCreated" |
| B.3.6 | Applied.message | "Kubernetes Job created successfully" |
| B.3.7 | Health condition | False |
| B.3.8 | Health.reason | "JobExecutionFailed" |
| B.3.9 | Health.message | Contains exit code information |
| B.3.10 | data field | Contains detailed error information |

**Expected Adapter Status Response**:
```json
{
  "nodepoolId": "{nodepool_id}",
  "adapterStatuses": [
    {
      "name": "nodepool-adapter",
      "conditions": [
        {
          "type": "Available",
          "status": "False",
          "reason": "JobFailed",
          "message": "Job completed with errors",
          "lastTransitionTime": "2025-01-09T10:05:00Z"
        },
        {
          "type": "Applied",
          "status": "True",
          "reason": "JobCreated",
          "message": "Kubernetes Job created successfully"
        },
        {
          "type": "Health",
          "status": "False",
          "reason": "JobExecutionFailed",
          "message": "Job container exited with code 1: Invalid machine type 'invalid-machine-type-xyz'"
        }
      ],
      "data": {
        "exitCode": 1,
        "errorType": "ExecutionError",
        "errorMessage": "Invalid machine type 'invalid-machine-type-xyz' not available in region us-east1",
        "jobName": "nodepool-adapter-job-xyz123",
        "containerLogs": "Error: Machine type 'invalid-machine-type-xyz' not found..."
      }
    }
  ]
}
```

---

##### Step B.4: Verify Nodepool Status

**Action**: Check nodepool status

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| B.4.1 | status.phase | "Not Ready" |
| B.4.2 | status.adapters | Shows affected adapter with available: "False" |
| B.4.3 | Nodepool remains in error state | Does not transition to Ready |

---

#### Success Criteria

| Criteria | Description |
|----------|-------------|
| **Scenario A (Job Creation Failure)** | |
| SC-A.1 | Applied: False indicates Job was NOT created |
| SC-A.2 | Health: False indicates infrastructure/configuration error |
| SC-A.3 | Clear error showing which resource/CRD is missing |
| SC-A.4 | Detailed error information in data field |
| **Scenario B (Job Execution Failure)** | |
| SC-B.1 | Applied: True indicates Job was created successfully |
| SC-B.2 | Health: False indicates Job execution failed |
| SC-B.3 | Available: False indicates work did not complete successfully |
| SC-B.4 | Exit code and container logs captured in error details |
| **General** | |
| SC-001 | Clear distinction between creation failures vs. execution failures |
| SC-002 | Detailed error information in data field with error type and context |
| SC-003 | Nodepool status.phase remains "Not Ready" |
| SC-004 | No system crash or unhandled exceptions |

#### Cleanup

1. Delete created nodepools
2. Restore adapter configuration to valid state
3. Verify system returns to normal operation

---

## Test Case: E2E-FAIL-006

### Database Connection Failure - Nodepool Operations

| Field | Value |
|-------|-------|
| **Test Case ID** | E2E-FAIL-006 |
| **Title** | Database Connection Failure - Nodepool Operations |
| **Priority** | P1 |
| **Type** | Failure Scenario |
| **Scope** | MVP |

#### Objective

Validate API handles database connection failures gracefully for nodepool operations, ensuring proper error responses, no data corruption, and automatic recovery when connection is restored.

#### Prerequisites

| Prerequisite | Description |
|--------------|-------------|
| PRE-001 | Cluster created and in Ready state |
| PRE-002 | Existing nodepool created for GET/PATCH/DELETE tests |
| PRE-003 | Ability to simulate database connection failure (stop PostgreSQL) |
| PRE-004 | Ability to restore database connection |

---

#### Test Steps

##### Step 1: Establish Baseline - Normal Operations

**Action**: Verify normal operations before simulating failure

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 1.1 | HTTP Status Code | 200 OK |
| 1.2 | Response contains nodepools | Valid response body |
| 1.3 | Baseline data captured | Record existing nodepool count/state |

---

##### Step 2: Simulate Database Connection Failure

**Action**: Stop PostgreSQL service or simulate connection failure

**Methods**:
- Stop PostgreSQL container/pod
- Block network access to PostgreSQL
- Invalidate database credentials (if applicable)

---

##### Step 3: Test Nodepool Operations During Outage

##### Step 3.1: Test GET Nodepools (List)

**Action**: Attempt to list nodepools during outage

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 3.1.1 | HTTP Status Code | 503 Service Unavailable |
| 3.1.2 | Error response present | Contains error details |
| 3.1.3 | Error message appropriate | "Database unavailable" or similar |
| 3.1.4 | API doesn't crash | Service remains running |

**Expected Error Response**:
```json
{
  "kind": "Error",
  "id": "503",
  "href": "/api/hyperfleet/v1/errors/503",
  "code": "HYPERFLEET-503",
  "reason": "Service temporarily unavailable: database connection failed"
}
```

---

##### Step 3.2: Test GET Single Nodepool

**Action**: Attempt to get single nodepool during outage

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 3.2.1 | HTTP Status Code | 503 Service Unavailable |
| 3.2.2 | Error response present | Contains error details |
| 3.2.3 | API doesn't crash | Service remains running |

---

##### Step 3.3: Test POST Nodepool (Create)

**Action**: Attempt to create nodepool during outage

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "name": "nodepool-during-outage",
  "machineType": "n1-standard-8",
  "replicas": 2
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 3.3.1 | HTTP Status Code | 503 Service Unavailable |
| 3.3.2 | Error response present | Contains error details |
| 3.3.3 | No partial data written | No orphaned records |
| 3.3.4 | API doesn't crash | Service remains running |

---

##### Step 3.4: Test GET Nodepool Statuses

**Action**: Attempt to get nodepool statuses during outage

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}/statuses
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 3.4.1 | HTTP Status Code | 503 Service Unavailable |
| 3.4.2 | Error response present | Contains error details |
| 3.4.3 | API doesn't crash | Service remains running |

---

##### Step 3.5: Multiple Rapid Requests

**Action**: Send multiple requests rapidly during outage

```bash
# Send 10 rapid requests
for i in {1..10}; do
  curl -X GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools &
done
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 3.5.1 | All requests return 503 | Consistent error handling |
| 3.5.2 | API remains responsive | No timeouts or hangs |
| 3.5.3 | No memory leaks | Service memory stable |
| 3.5.4 | No connection pool exhaustion | Subsequent requests work |

---

##### Step 4: Restore Database Connection

**Action**: Restore PostgreSQL service

**Methods**:
- Start PostgreSQL container/pod
- Restore network access
- Wait for connection pool to reconnect

**Wait Time**: 30-60 seconds for connection recovery

---

##### Step 5: Verify Operations Resume

##### Step 5.1: Test GET Nodepools (List) After Recovery

**Action**: List nodepools after recovery

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 5.1.1 | HTTP Status Code | 200 OK |
| 5.1.2 | Response valid | Contains nodepool list |
| 5.1.3 | Data unchanged | Same nodepools as baseline |

---

##### Step 5.2: Test POST Nodepool After Recovery

**Action**: Create nodepool after recovery

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "name": "nodepool-after-recovery",
  "machineType": "n1-standard-4",
  "replicas": 1
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 5.2.1 | HTTP Status Code | 201 Created |
| 5.2.2 | Nodepool created | ID returned |
| 5.2.3 | Nodepool visible in list | GET returns new nodepool |
| 5.2.4 | No duplicate entries | Only one instance created |

---

##### Step 5.3: Verify No Data Corruption

**Action**: Verify data integrity after recovery

```
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 5.3.1 | Pre-existing nodepools intact | All baseline nodepools present |
| 5.3.2 | Nodepool data correct | Fields match original values |
| 5.3.3 | No orphaned records | No "nodepool-during-outage" entry |
| 5.3.4 | Status data intact | status.phase, status.adapters unchanged |

---

##### Step 5.4: Verify End-to-End Workflow Functions

**Action**: Create nodepool and verify full workflow completes

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
Content-Type: application/json

{
  "name": "nodepool-workflow-test",
  "machineType": "n1-standard-4",
  "replicas": 1
}
```

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 5.4.1 | Nodepool created | 201 Created |
| 5.4.2 | Sentinel processes event | Events published |
| 5.4.3 | Adapters receive events | Adapter statuses appear |
| 5.4.4 | Nodepool reaches Ready | status.phase = "Ready" |

---

#### Additional Scenarios

##### Scenario 6.1: Intermittent Connection Failures

**Action**: Simulate intermittent database connectivity

**Steps**:
1. Create nodepool
2. During adapter processing, briefly interrupt database
3. Restore connection
4. Verify nodepool eventually reaches Ready state

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 6.1.1 | System recovers automatically | No manual intervention needed |
| 6.1.2 | Nodepool reaches Ready | Eventually transitions to Ready |
| 6.1.3 | No data inconsistency | Status reflects actual state |

---

##### Scenario 6.2: Connection Failure During Transaction

**Action**: Simulate connection failure during write operation

**Steps**:
1. Begin nodepool creation
2. Interrupt database during write
3. Verify transaction rollback
4. Restore connection
5. Retry creation

**Validation Points**:
| # | Check | Expected Result |
|---|-------|-----------------|
| 6.2.1 | No partial data | Transaction rolled back |
| 6.2.2 | Retry succeeds | Creation works after recovery |
| 6.2.3 | Single entry created | No duplicates |

---

#### Success Criteria

| Criteria | Description |
|----------|-------------|
| SC-001 | API returns 503 errors during database outage |
| SC-002 | API doesn't crash during outage |
| SC-003 | Operations resume normally after recovery |
| SC-004 | No data corruption after recovery |
| SC-005 | No partial writes or orphaned records |
| SC-006 | Error messages are appropriate and informative |
| SC-007 | System handles rapid requests during outage |
| SC-008 | End-to-end workflow functions after recovery |
| SC-009 | Connection pool recovers automatically |

#### Cleanup

1. Delete nodepools created during test
2. Verify database is in healthy state
3. Verify API service is functioning normally

---

## Appendix

### A. Test Environment Requirements

| Requirement | Description |
|-------------|-------------|
| GCP Project | Configured with appropriate permissions |
| Kubernetes Cluster | Running and accessible |
| PostgreSQL | Database accessible with test credentials |
| API Service | Deployed and running |
| Sentinel | Deployed and running |
| Adapters | Deployed with test configurations |

### B. Test Data Management

- All test nodepools should use unique names with test prefixes
- Cleanup should be performed after each test case
- Baseline data should be recorded before failure scenarios

### C. Monitoring and Logging

During test execution, monitor:
- API service logs
- Sentinel operator logs
- Adapter logs
- Kubernetes Job logs
- Database connection logs

### D. Related Documents

- [HyperFleet API E2E Scenarios](../hyperfleet-e2e-scenario/hyperfleet-api-e2e-scenario.md)
- [HyperFleet API CUJ](../hyperfleet-critical-user-journey/hyperfleet-api-cuj.md)
- [HyperFleet Adapter CUJ](../hyperfleet-critical-user-journey/hyperfleet-adapter-cuj.md)

