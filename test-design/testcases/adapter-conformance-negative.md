# Feature: Adapter Contract Conformance (Negative)

## Table of Contents

1. [POST to statuses endpoint returns 405](#test-title-post-to-statuses-endpoint-returns-405)
2. [PUT with missing mandatory conditions returns 400](#test-title-put-with-missing-mandatory-conditions-returns-400)
3. [Ready condition absent on reconciled cluster](#test-title-ready-condition-absent-on-reconciled-cluster)

---

## Test Title: POST to statuses endpoint returns 405

### Description

Validates that the v1.0.0 API rejects the old v0.2.0 POST method for adapter status reporting. Adapters that have not migrated to PUT will receive a 405 Method Not Allowed response instead of silently succeeding.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | v1.0.0 |
| **Created** | 2026-06-23 |
| **Updated** | 2026-06-23 |

---

### Preconditions

1. HyperFleet API is deployed with v1.0.0 adapter contract (PUT-only statuses endpoint)
2. HyperFleet Sentinel is deployed and running

---

### Test Steps

#### Step 1: Create a cluster

**Action:**
- Create a cluster via the API using a standard cluster payload

**Expected Result:**
- API returns the created cluster with an ID and generation

#### Step 2: Send a POST status report

**Action:**
- Send a POST request to `/clusters/{id}/statuses` with a valid `AdapterStatusCreateRequest` body containing all three mandatory conditions (Applied, Available, Health)

**Expected Result:**
- API returns HTTP 405 Method Not Allowed
- The POST method was removed in v1.0.0; only PUT is accepted

---

## Test Title: PUT with missing mandatory conditions returns 400

### Description

Validates that the v1.0.0 API rejects adapter status reports that omit the three mandatory conditions (Available, Applied, Health). Sends a PUT with only a `Ready` condition (which was removed in v1.0.0) and no mandatory conditions.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | v1.0.0 |
| **Created** | 2026-06-23 |
| **Updated** | 2026-06-23 |

---

### Preconditions

1. HyperFleet API is deployed with v1.0.0 adapter contract
2. HyperFleet Sentinel is deployed and running

---

### Test Steps

#### Step 1: Create a cluster

**Action:**
- Create a cluster via the API using a standard cluster payload

**Expected Result:**
- API returns the created cluster with an ID and generation

#### Step 2: Send a PUT with only a Ready condition

**Action:**
- Send a PUT request to `/clusters/{id}/statuses` with an `AdapterStatusCreateRequest` containing only `{Type: "Ready", Status: "True"}`, omitting the mandatory Available, Applied, and Health conditions

**Expected Result:**
- API returns HTTP 400 Bad Request
- The three mandatory conditions (Available, Applied, Health) must be present in every adapter status report

---

## Test Title: Ready condition absent on reconciled cluster

### Description

Validates that the v1.0.0 API does not produce a resource-level `Ready` condition on a fully reconciled cluster. Adapters that poll for `Ready=True` to determine readiness will hang forever. The correct v1.0.0 conditions are `Reconciled` and `LastKnownReconciled`.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | v1.0.0 |
| **Created** | 2026-06-23 |
| **Updated** | 2026-06-23 |

---

### Preconditions

1. HyperFleet API is deployed with v1.0.0 adapter contract
2. HyperFleet Sentinel is deployed and running
3. At least one adapter is configured and running to drive reconciliation

---

### Test Steps

#### Step 1: Create a cluster and wait for reconciliation

**Action:**
- Create a cluster via the API
- Poll until `Reconciled=True`

**Expected Result:**
- Cluster reaches `Reconciled` condition with `status: "True"`

#### Step 2: Verify Ready condition is absent

**Action:**
- Fetch the reconciled cluster and inspect `status.conditions`

**Expected Result:**
- No condition with `type: "Ready"` exists in the resource-level conditions
- `Reconciled` condition is present with `status: "True"`
- `LastKnownReconciled` condition is present with `status: "True"`

---
