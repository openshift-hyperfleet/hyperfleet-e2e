# Feature: Adapter Framework - Remote Transport through the Applier

## Table of Contents

1. [Adapter creates a cluster's resources through the applier and reports their status](#test-title-adapter-creates-a-clusters-resources-through-the-applier-and-reports-their-status)
2. [Cluster delete waits until the applier confirms the remote objects are gone](#test-title-cluster-delete-waits-until-the-applier-confirms-the-remote-objects-are-gone)
3. [Adapter reports undelivered desires while the applier is unavailable](#test-title-adapter-reports-undelivered-desires-while-the-applier-is-unavailable)
4. [Delete then reapply waits for the delete to be confirmed](#test-title-delete-then-reapply-waits-for-the-delete-to-be-confirmed)

---

## Overview

The delivery chain under test: HyperFleet API → Sentinel → adapter on the `remote` transport → desire store (Redis) → hyperfleet-applier → Kubernetes. The adapter writes an ApplyDesire and a ReadDesire per resource. The applier applies the resource and copies the live object into the ReadDesire's status, and the adapter builds its conditions from that copy. On delete, the adapter replaces the ApplyDesire with a DeleteDesire, and the applier reports the object deleted only once it is gone.

The adapter is `cl-desire` (config key `desire.adapter`). Its task config lives in hyperfleet-infra, [`helmfile/configs/base/adapters/remote/`](https://github.com/openshift-hyperfleet/hyperfleet-infra/tree/main/helmfile/configs/base/adapters/remote), and started from the adapter repo's example `charts/examples/remote-two-resources`. Its desires go to the store partition named after the run namespace. For a cluster `<id>` it manages:

| Object | Desire identity (group / resource / namespace / name) |
|--------|-------------------------------------------------------|
| Namespace `<id>-remote` | `""` / `namespaces` / `""` / `<id>-remote` |
| ConfigMap `cluster-config` | `""` / `configmaps` / `<id>-remote` / `cluster-config` |

`cl-desire` is also one of `adapters.cluster`, so the tier suites check it like any other required adapter: cluster creation checks its conditions, and the delete specs check that it reports `Finalized=True` and that `<id>-remote` is removed. The remote-transport suite fails in `BeforeAll` when `adapters.cluster` does not list it. This document covers what only the remote path has: the objects the applier writes, a delete that waits on them, and an applier outage.

The suites check only what the API and the cluster show; they do not read the desire store. To inspect the store while debugging, see [Debugging](#debugging).

Test 4 does not run through `cl-desire`: it deploys its own remote adapter, `cl-desire-reapply` (config in `testdata/adapter-configs/cl-desire-reapply/`), which has the delete expression also driven by a cluster label, `e2e.hyperfleet.io/delete-configmap`. That is the only way to delete a remote object and want it back while the cluster lives: `cl-desire` deletes only on `is_deleting`, and the API never clears `deleted_time`. The adapter's conditions are binary (no `Unknown`): the reasons carry the distinction — `ResourceNotFound` (confirmed absent), `GenerationMismatch` (visible but not at the requested generation), `MirrorNotSynced` (presence unconfirmed). `Available` reports the delivered state against the want: `True` when the objects are present at the current generation, and also `True` with reason `DeletionDelivered` when a label-requested deletion is confirmed — so a label-driven delete that stays deleted converges (`Reconciled=True` with the object gone) instead of hanging unreconciled. The spec always flips the gate off before the delete confirms, so it only ever wants the object back. The spec adds the adapter to the required adapters for its duration (so the cluster stays unreconciled, and the Sentinel's cadence drives the adapter's retry after the delete confirms), and creates an additional NetworkPolicy, `redis-ingress-cl-desire-reapply`. Both changes are restored in `DeferCleanup`; the spec is `Serial` because of them.

Coverage moved here from the retired transport suite:

| Retired scenario | Replacement |
|------------------|-------------|
| Happy path: resources land on the target, status is read back | Test 1 |
| Same generation causes no new write | Test 1, Step 5 |
| Target nobody delivers to (unregistered consumer) | Test 3 |
| Transport endpoint unavailable, then recovery | Not covered yet. [HYPERFLEET-1771](https://redhat.atlassian.net/browse/HYPERFLEET-1771) adds "desire store unavailable, then recovery" once the Postgres-backed store and its API land |

Discovery misconfiguration, nested discovery and an unreachable status API were dropped: they are single-component adapter behaviour, covered by adapter tests, and `nested_discoveries` does not exist in v2 task configs.

---

## Environment Setup

Bring up the stack with the [Setup Guide](../../docs/setup.md); both options deploy Redis, the applier and `cl-desire`. For the manual steps below, also set:

```bash
export API_URL=${HYPERFLEET_API_URL}
export ADAPTER_NAME=cl-desire
```

---

## Test Title: Adapter creates a cluster's resources through the applier and reports their status

### Description

This test validates the create path end to end. Creating a cluster makes `cl-desire` hand its two objects to the applier through the store. The applier creates them and reports the live objects back, and the adapter builds its conditions from them, which lets the cluster reach `Reconciled=True`. `cl-desire` has no Kubernetes access of its own, so the objects on the cluster prove the applier created them. The task config reports `Available=True` only when the mirrored Namespace is `Active` and both generation annotations match, so `Available=True` proves the status was read back through the applier. The test then checks that the applier, which re-applies every desire on each pass, does not rewrite an object whose desire has not changed.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | MVP |
| **Created** | 2026-10-02 |
| **Updated** | 2026-10-13 |

---

### Preconditions

1. `adapters.cluster` lists `desire.adapter` (`cl-desire`)

---

### Test Steps

#### Step 1: Create a cluster
**Action:**
```bash
CLUSTER_ID=$(curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json | jq -r '.id')
echo "CLUSTER_ID=${CLUSTER_ID}"
```

**Expected Result:**
- The API returns a cluster ID and `generation: 1`

#### Step 2: Wait for the cluster to become Reconciled
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID} \
  | jq '.status.conditions[] | select(.type == "Reconciled")'
```

**Expected Result:**
- `Reconciled` is `True` within `timeouts.cluster.reconciled`

#### Step 3: Verify the adapter status
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/statuses \
  | jq --arg a "${ADAPTER_NAME}" '.items[] | select(.adapter == $a)
      | {observed_generation, conditions: [.conditions[] | {type, status, reason}]}'
```

**Expected Result:**
- `observed_generation` equals the cluster generation (`1`)
- `Applied`, `Available` and `Health` are `True` (reasons `ResourcesObserved`, `LiveResourcesReady`, `Healthy` in the task config)

#### Step 4: Verify both objects exist on the cluster
**Action:**
```bash
kubectl get namespace ${CLUSTER_ID}-remote -o jsonpath='{.metadata.labels}{"\n"}{.metadata.annotations}{"\n"}'
kubectl get configmap cluster-config -n ${CLUSTER_ID}-remote -o jsonpath='{.metadata.annotations}{"\n"}{.data}{"\n"}'
```

**Expected Result:**
- The Namespace has label `hyperfleet.io/cluster-id: ${CLUSTER_ID}`
- Both objects have annotation `hyperfleet.io/generation: "1"`
- The ConfigMap has `cluster_id: ${CLUSTER_ID}`

#### Step 5: Verify the applier does not rewrite an unchanged object
**Action:**
```bash
kubectl get configmap cluster-config -n ${CLUSTER_ID}-remote -o jsonpath='{.metadata.resourceVersion}{"\n"}'
# repeat for 3 polling intervals (30s at the default 10s)
```

**Expected Result:**
- `resourceVersion` stays the same. The applier server-side applies the desire on every pass (every 5s); an unchanged apply is a no-op and must not write the object.

---

## Test Title: Cluster delete waits until the applier confirms the remote objects are gone

### Description

This test validates delete past finalizers. A finalizer on the remote ConfigMap holds its deletion open. While it is held, the applier keeps reporting the ConfigMap as pending, the task does not delete the parent Namespace, `cl-desire` does not report `Finalized=True`, and the API keeps the cluster. Once the finalizer is released, the applier confirms the delete, the adapter finalizes, and the API hard-deletes the cluster inside the status update that completes finalization. So after the 404, the Namespace check runs once, with no retry: a Namespace still there means the adapter finalized early, a product finding. Do not turn that check into polling.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | MVP |
| **Created** | 2026-10-02 |
| **Updated** | 2026-10-13 |

---

### Preconditions

1. The create test passed for `${CLUSTER_ID}`

---

### Test Steps

#### Step 1: Hold the remote ConfigMap with a finalizer
**Action:**
```bash
kubectl patch configmap cluster-config -n ${CLUSTER_ID}-remote --type=strategic \
  -p '{"metadata":{"finalizers":["e2e.hyperfleet.io/hold"]}}'
```

**Expected Result:**
- The ConfigMap carries `e2e.hyperfleet.io/hold`. The applier's server-side apply does not own `metadata.finalizers`, so it leaves the finalizer alone.
- Cleanup removes the finalizer even when the test fails, so the Namespace cannot stay in deletion.

#### Step 2: Delete the cluster
**Action:**
```bash
curl -s -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID} | jq '{deleted_time, generation}'
```

**Expected Result:**
- The API accepts the request and sets `deleted_time`

#### Step 3: Wait for the applier to delete the ConfigMap
**Action:**
```bash
kubectl get configmap cluster-config -n ${CLUSTER_ID}-remote -o jsonpath='{.metadata.deletionTimestamp}{"\n"}'
```

**Expected Result:**
- `deletionTimestamp` is set within `timeouts.adapter.processing`: the applier issued the delete

#### Step 4: Verify the delete is held for 3 polling intervals
**Action:**
```bash
curl -s -o /dev/null -w '%{http_code}\n' ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}
kubectl get namespace ${CLUSTER_ID}-remote -o jsonpath='{.metadata.deletionTimestamp}{"\n"}'
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/statuses \
  | jq --arg a "${ADAPTER_NAME}" '.items[] | select(.adapter == $a) | .conditions[] | select(.type == "Finalized")'
```

**Expected Result:**
- The cluster GET returns `200`
- The Namespace exists and has no `deletionTimestamp`
- `cl-desire` does not report `Finalized=True`
- Do not check the `Finalized` reason or the observed generation here: the adapter's reports during the hold carry `Available=Unknown`, which the API discards, so the API still shows the pre-delete status

#### Step 5: Release the finalizer and wait for the hard-delete
**Action:**
```bash
kubectl patch configmap cluster-config -n ${CLUSTER_ID}-remote --type=strategic \
  -p '{"metadata":{"$deleteFromPrimitiveList/finalizers":["e2e.hyperfleet.io/hold"]}}'
curl -s -o /dev/null -w '%{http_code}\n' ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}
```

**Expected Result:**
- The cluster returns `404` within `timeouts.cluster.deleted`

#### Step 6: Check once that the remote Namespace is gone
**Action:**
```bash
kubectl get namespace ${CLUSTER_ID}-remote
```

**Expected Result:**
- NotFound. A Namespace still in `Terminating` fails the test.

---

## Test Title: Adapter reports undelivered desires while the applier is unavailable

### Description

This test validates the remote path when nothing serves the target. With the applier scaled to 0, the adapter still writes its desires and runs successfully, but its read desires stay unsynced, so it cannot report its objects applied (`Applied=Unknown`, reason `MirrorNotSynced` in the task config). The cluster must not become Reconciled, and nothing may appear on the target. Once the applier is restored, the cluster must converge on its own, driven by the Sentinel's events while the cluster is not Reconciled.

The spec is `Serial` and `Disruptive`: scaling down the shared applier blocks remote delivery for every other spec.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | MVP |
| **Created** | 2026-10-13 |
| **Updated** | 2026-10-13 |

---

### Preconditions

1. The applier Deployment (`app.kubernetes.io/instance=hyperfleet-applier`) runs in the run namespace with 1 replica, the chart default

---

### Test Steps

#### Step 1: Scale the applier to 0 and wait for its pod to go
**Action:**
```bash
kubectl scale deploy -n ${NAMESPACE} -l app.kubernetes.io/instance=hyperfleet-applier --replicas=0
kubectl get pods -n ${NAMESPACE} -l app.kubernetes.io/instance=hyperfleet-applier
```

**Expected Result:**
- No applier pod remains, not even a terminating one, which could still apply the new cluster's desires
- Cleanup restores the applier to 1 replica even when the test fails

#### Step 2: Create a cluster
**Action:**
- Create the cluster as in Test 1, Step 1, **after** the scale-down.

**Expected Result:**
- The API returns a cluster ID
- The cluster must be created after the outage starts: while `Available` is `Unknown`, the API keeps only an adapter's first report, so a cluster created earlier would keep showing its healthy pre-outage status
- The cluster's cleanup restores the applier before it deletes the cluster; without the applier the delete is never confirmed and the cluster would never be hard-deleted

#### Step 3: Verify the adapter runs but does not report its objects applied
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/statuses \
  | jq --arg a "${ADAPTER_NAME}" '.items[] | select(.adapter == $a) | [.conditions[] | {type, status, reason}]'
```

**Expected Result:**
- `Health` is `True`: the adapter ran, only delivery is missing
- `Applied` is not `True`

#### Step 4: Verify the cluster stays unreconciled for 3 polling intervals
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID} \
  | jq '.status.conditions[] | select(.type == "Reconciled")'
kubectl get namespace ${CLUSTER_ID}-remote
```

**Expected Result:**
- `Reconciled` stays `False`
- The Namespace does not exist

#### Step 5: Restore the applier and verify the cluster converges
**Action:**
```bash
kubectl scale deploy -n ${NAMESPACE} -l app.kubernetes.io/instance=hyperfleet-applier --replicas=1
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID} \
  | jq '.status.conditions[] | select(.type == "Reconciled")'
kubectl get namespace ${CLUSTER_ID}-remote
```

**Expected Result:**
- `Reconciled` becomes `True` within `timeouts.cluster.reconciled`
- The Namespace exists

---

## Test Title: Delete then reapply waits for the delete to be confirmed

### Description

This test validates the delete-then-reapply path against the real applier. A remote object is deleted through a delete expression the test controls (a cluster label, not `is_deleting`), the deletion is held open by a finalizer, and the object is wanted back before the delete is confirmed. The store must refuse the new `ApplyDesire` while the `DeleteDesire` is unconfirmed, so the object is never applied onto the terminating object: it keeps its UID and its pre-delete content, and the adapter reports `Available=False` at the new generation — a wrongly accepted reapply would instead land on the terminating object and report `Available=True`. Once the finalizer is released, the applier confirms the delete, the next event's apply replaces the `DeleteDesire`, and the object is recreated with a new UID and the new content.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | MVP |
| **Created** | 2026-10-22 |
| **Updated** | 2026-10-22 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. A dedicated cl-desire-reapply adapter is available for deployment via Helm

---

### Test Steps

#### Step 1: Deploy the adapter and make it required

**Action:**
- Create a network policy for the `cl-desire-reapply` admitting traffic from this adapter to the redis instance (desire store): the spec does this via `AdmitDesireStoreClient`
- Deploy `cl-desire-reapply` via Helm, release `adapter-clusters-cl-desire-reapply`: the spec does this through `helper.DeployAdapter`.
- Add `cl-desire-reapply` to the Cluster entity's `required_adapters` and roll the API: the spec does this through `helper.UpgradeAPIRequiredAdapters`

**Expected Result:**
- The adapter runs and reaches the desire store
- The API's `Reconciled` aggregation now includes `cl-desire-reapply`
- All changes are undone by the spec's cleanup even when it fails

#### Step 2: Create a cluster and wait for it to be Reconciled

**Action:**
- Create a cluster and record its ID:
```bash
jq --arg n "hp-cluster-manual-$RANDOM" '.name = $n' testdata/payloads/clusters/cluster-request.json > cluster.json
CLUSTER_ID=$(curl -s -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @cluster.json | jq -r '.id')
```
- Poll until the cluster is `Reconciled=True`:
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID} \
  | jq '.status.conditions[] | select(.type == "Reconciled")'
```
- Check the ConfigMap and record its UID:
```bash
kubectl get configmap reapply-config -n ${CLUSTER_ID}-reapply -o jsonpath='{.metadata.uid}{"\n"}{.metadata.annotations}{"\n"}{.data}{"\n"}'
```

**Expected Result:**
- The cluster reaches `Reconciled=True` within `timeouts.cluster.reconciled`
- `cl-desire-reapply` reports `Applied`, `Available` and `Health` `True` at generation 1
- The ConfigMap exists with generation annotation `1` and `cluster_id` set; record its UID

#### Step 3: Delete the ConfigMap through the gate, held by a finalizer

**Action:**
- Hold the ConfigMap with a finalizer:
```bash
kubectl patch configmap reapply-config -n ${CLUSTER_ID}-reapply --type=strategic \
  -p '{"metadata":{"finalizers":["e2e.hyperfleet.io/hold"]}}'
```
- Turn the delete gate on, carrying the cluster's other labels (the API replaces the label set wholesale on PATCH):
```bash
curl -s -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID} \
  -H "Content-Type: application/json" \
  -d '{"labels":{"environment":"test","shard":"1","team":"platform","e2e.hyperfleet.io/delete-configmap":"true"}}'
```
- Poll until the ConfigMap has a `deletionTimestamp`:
```bash
kubectl get configmap reapply-config -n ${CLUSTER_ID}-reapply -o jsonpath='{.metadata.deletionTimestamp}{"\n"}'
```

**Expected Result:**
- The label patch replaces the whole label set, so the cluster's other labels are carried in the patch
- The patch bumps the generation to 2
- The ConfigMap gets a `deletionTimestamp` within `timeouts.adapter.processing`: the applier issued the delete, the finalizer holds it
- `cl-desire-reapply` reports `Applied=True` with `Available=False`, reason `GenerationMismatch`: the object is still visible in the mirror, but its generation annotation no longer advances while it is being deleted

#### Step 4: Want the object back while the delete is held

**Action:**
- Turn the delete gate off again (same patch, `"false"`):
```bash
curl -s -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID} \
  -H "Content-Type: application/json" \
  -d '{"labels":{"environment":"test","shard":"1","team":"platform","e2e.hyperfleet.io/delete-configmap":"false"}}'
```
- Poll the adapter status until `cl-desire-reapply` reports at generation 3 with `Available=False` — the accepted report is the proof that the adapter processed the gate-off event and the store refused the reapply:
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/statuses \
  | jq -r '.items[] | select(.adapter=="cl-desire-reapply") | .conditions[]'
```
- Re-check for `timeouts.adapter.processing/2` (the stuck-state convention): the ConfigMap must keep its UID, `deletionTimestamp` and generation annotation `1`:
```bash
kubectl get configmap reapply-config -n ${CLUSTER_ID}-reapply -o jsonpath='{.metadata.uid}{"\n"}{.metadata.annotations}{"\n"}'
```

**Expected Result:**
- The patch bumps the generation to 3
- The ConfigMap keeps its UID, its `deletionTimestamp`, generation annotation `1` and its data: the store refused the new apply, nothing was applied onto the terminating object. A generation annotation of `3` with the same UID means a reapply landed on the terminating object — the regression this test exists to catch
- `cl-desire-reapply`'s stored row reaches generation 3 with `Applied=False` and `Available=False`, both reason `MirrorNotSynced` — the executor marks the rejected resource unsynced: the reports are accepted (the status is `False`, not `Unknown`), so the row advancing is the direct proof that the adapter processed the gate-off event and the store refused the reapply — the distinction the tri-state discard rule used to hide. The row flipping to `Available=True` during the hold means a reapply landed on the terminating object
- The cluster stays `Reconciled=False`, which also keeps the Sentinel re-triggering the adapter

#### Step 5: Release the finalizer and verify the recreate

**Action:**
- Release the finalizer:
```bash
kubectl patch configmap reapply-config -n ${CLUSTER_ID}-reapply --type=strategic \
  -p '{"metadata":{"$deleteFromPrimitiveList/finalizers":["e2e.hyperfleet.io/hold"]}}'
```
- Poll the adapter status until `cl-desire-reapply` reports `Applied`, `Available` and `Health` `True` at generation 3:
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/${CLUSTER_ID}/statuses \
  | jq -r '.items[] | select(.adapter=="cl-desire-reapply") | .conditions[]'
```
- Verify the ConfigMap was recreated with a new UID and the new content:
```bash
kubectl get configmap reapply-config -n ${CLUSTER_ID}-reapply -o yaml
```

**Expected Result:**
- The delete confirms once the object is gone; the next event's apply replaces the `DeleteDesire` atomically
- `cl-desire-reapply` reports `Applied`, `Available` and `Health` `True` at generation 3 within `timeouts.cluster.reconciled` (the accepted report proves the recreate: `Available=True` requires the ConfigMap present with generation annotation `3`)
- The ConfigMap exists again with a **new UID**, no `deletionTimestamp`, generation annotation `3`
- The Namespace `<id>-reapply` kept its UID throughout: its delete expression stays gated on `is_deleting`, the label gate did not leak into it
- The cluster reaches `Reconciled=True`

---

## Falsifiability

Each change below must turn the suite red. Never change an expected value to make a red run green.

| Change | Fails in |
|--------|----------|
| Scale the applier Deployment to 0 replicas | Test 1: `Reconciled` times out |
| Remove `cl-desire` from `adapters.cluster` | Remote-transport suite `BeforeAll` |
| Make the `Finalized` status expression `is_deleting && adapter.?executionStatus.orValue("") == "success"` in the `cl-desire` task-config ConfigMap and restart the adapter | Test 2: the cluster is hard-deleted while the ConfigMap is held |
| Point the `remote-primary` transport's `target_cluster` at another literal | Test 1: the applier never sees the desires, so `Reconciled` times out |
| Make the task config report `Applied=True` whenever the adapter ran | Test 3: `Applied` is `True` with no applier |

---

## Debugging

When a test fails, the desire store shows where the chain stopped. Each record is an `apply`, `read` or `delete` desire with the applier's `Successful` condition:

```bash
REDIS_POD=$(kubectl get pod -n ${NAMESPACE} -l app.kubernetes.io/instance=redis -o jsonpath='{.items[0].metadata.name}')

for key in $(kubectl exec -n ${NAMESPACE} ${REDIS_POD} -- redis-cli --scan --pattern "desire/{${NAMESPACE}/*${CLUSTER_ID}*"); do
  echo "${key}"
  kubectl exec -n ${NAMESPACE} ${REDIS_POD} -- redis-cli GET "${key}" | jq '{status: .status.conditions, read: .readStatus.conditions}'
done
```

The key layout is internal to the applier and may change; this is for people, not for assertions.
