# Setup Guide

This guide covers setting up a HyperFleet environment for running E2E tests locally.

## Table of Contents

- [Deployment Options](#deployment-options)
  - [Option 1: Kind (Local)](#option-1-kind-local)
  - [Option 2: GCP](#option-2-gcp)
- [Authentication mode](#authentication-mode)
- [Configure Test Settings](#configure-test-settings)
- [Troubleshooting](#troubleshooting)

## Deployment Options

Clone the infrastructure repository:

```bash
git clone https://github.com/openshift-hyperfleet/hyperfleet-infra/
cd hyperfleet-infra/terraform
```

Choose one of the following deployment options based on your needs:

- **Kind (local):** Fast setup, no cloud dependencies, uses port-forwarding
- **GCP:** Cloud environment, requires GCP access, slower setup, uses a LoadBalancer service

Both options deploy the desire delivery stack next to the adapters: Redis (the desire store), the applier, and the remote adapter `cl-desire`. The `remote-transport` suites need it ([test cases](../test-design/testcases/adapter-with-remote-transport.md)). See [Desire delivery](https://github.com/openshift-hyperfleet/hyperfleet-infra/blob/main/README.md#desire-delivery) in the hyperfleet-infra README.

The tests reach the API through the gateway, `svc/hyperfleet-gateway`, which listens on plain HTTP on port 8000. `svc/hyperfleet-api` is ClusterIP-only and serves TLS to the gateway, so do not point the tests at it.

## Authentication mode

The E2E runner defaults to `AUTH_MODE=EDGE+API`. Set `AUTH_MODE` to the same
value used to deploy HyperFleet. Supported values are `NONE`, `API`, `EDGE`, and
`EDGE+API`.

For authenticated modes, E2E acquires a projected token for
`hyperfleet-e2e-runner` using Kubernetes TokenRequest. The e2e Helmfile
environment creates this ServiceAccount before tests start. The runner needs
permission to create TokenRequests; the multi-issuer test also creates a
temporary alternate ServiceAccount.
The token namespace defaults to `NAMESPACE`, and the audit identity is derived
automatically.

| Mode | Machine token sent by E2E | Machine token sent by test adapters |
| ---- | ------------------------- | ---------------------------------- |
| `NONE` | No token | Chart default; auth is disabled |
| `API` | `Bearer <token>` | Chart default `Bearer` |
| `EDGE` | `ServiceAccount <token>` | `ServiceAccount <token>` |
| `EDGE+API` | `ServiceAccount <token>` | `ServiceAccount <token>` |

The schemes select different Authorino authentication methods. `ServiceAccount`
triggers Kubernetes TokenReview; `Bearer` triggers human OIDC JWT validation.
This prevents a machine token from entering the human-token path. See the
[infra authentication explanation](https://github.com/openshift-hyperfleet/hyperfleet-infra/blob/main/README.md#L306-L308).

### Option 1: Kind (Local)

**1. Deploy HyperFleet to Kind cluster:**

```bash
# option 1: Export namespace and helmfile env
export NAMESPACE=<your-dev-namespace> ; export HELMFILE_ENV=e2e-kind ; make local-up-kind
# option 2: Set in command line
NAMESPACE=<your-dev-namespace> HELMFILE_ENV=e2e-kind make local-up-kind
```

**2. Port-forward the gateway:**

```bash
export API_LOCAL_PORT=8000
kubectl port-forward -n ${NAMESPACE} svc/hyperfleet-gateway ${API_LOCAL_PORT}:8000
```

**3. Configure environment variables:**

```bash
export HYPERFLEET_API_URL=http://localhost:${API_LOCAL_PORT}
export NAMESPACE=<your-dev-namespace>
```

**4. Verify deployment:**

```bash
# Check Helm releases
helm list -n ${NAMESPACE}

# Verify all pods are running
kubectl get pods -n ${NAMESPACE}

# Test API connectivity
curl -f -X GET ${HYPERFLEET_API_URL}/api/hyperfleet/v1/clusters/
```

### Option 2: GCP

**1. Deploy HyperFleet to GCP cluster:**

> Note: Make sure your terraform files are up to date. See [hyperfleet-infra/CONTRIBUTING.md](https://github.com/openshift-hyperfleet/hyperfleet-infra/blob/main/CONTRIBUTING.md#development-setup) for details.
- terraform/envs/gke/dev.tfbackend
- terraform/envs/gke/dev.tfvars
> See [hyperfleet-infra/README.md](https://github.com/openshift-hyperfleet/hyperfleet-infra/blob/main/README.md) for infrastructure deployment details.

```bash
# option 1: Export namespace and helmfile env
export NAMESPACE=<your-dev-namespace> ; export HELMFILE_ENV=e2e-gcp ; make local-up-gcp
# option 2: Set in command line
NAMESPACE=<your-dev-namespace> HELMFILE_ENV=e2e-gcp make local-up-gcp
```

**2. Wait for the gateway's external IP:**

In `e2e-gcp`, `svc/hyperfleet-gateway` is a LoadBalancer.

```bash
# Wait for the external IP to be assigned (may take 1-2 minutes)
kubectl get svc hyperfleet-gateway -n ${NAMESPACE} -w
```

With `AUTH_MODE=EDGE` or `EDGE+API`, Authorino denies requests whose host it does not know. For a dynamically assigned LoadBalancer IP, re-apply the gateway with `AUTHORINO_HOSTS` set to that IP before running tests. CI setup performs this second apply automatically.

**3. Configure environment variables:**

```bash
export API_EXTERNAL_IP=$(kubectl get svc hyperfleet-gateway -n ${NAMESPACE} -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
export HYPERFLEET_API_URL=http://${API_EXTERNAL_IP}:8000
export NAMESPACE=<your-dev-namespace>
```

**4. Verify deployment:**

```bash
# Check Helm releases
helm list -n ${NAMESPACE}

# Verify all pods are running
kubectl get pods -n ${NAMESPACE}

# Test API connectivity
curl -f -X GET ${HYPERFLEET_API_URL}/api/hyperfleet/v1/clusters/
```

## Configure Test Settings

Every live run starts with a read-only drift check: `hyperfleet-e2e` runs
`helm get values hyperfleet-api --all --namespace "$NAMESPACE"` and compares the
deployed `config.entities` with its descriptor table. Set `NAMESPACE` to the
namespace that holds the API release and make sure `helm` and `KUBECONFIG` can
read it. A mismatch fails the suite before any spec runs; see
[debugging.md](debugging.md#startup-entity-drift-check). `make list-tests` and
`--dry-run` skip the check.

### Override Image Settings (Optional)

If your deployment uses custom image settings, update `env/env.local` in this repo to match your infrastructure deployment settings:

- **Kind deployments:** Match settings from [`env.kind`](https://github.com/openshift-hyperfleet/hyperfleet-infra/blob/main/env.kind#L18-L21)
- **GCP deployments:** Match settings from [`env.gcp`](https://github.com/openshift-hyperfleet/hyperfleet-infra/blob/main/env.gcp#L18-L21)

**Update `env/env.local`:**

```bash
# env/env.local
export IMAGE_REGISTRY=<registry>
export <COMPONENT>_IMAGE_REPO=<repo>
export <COMPONENT>_IMAGE_TAG=<tag>
```

**Source the configuration:**

```bash
source env/env.local
```

This configuration is required for running tier2 tests.

## Troubleshooting

### Infrastructure Setup Issues

For additional help with infrastructure deployment and configuration, see:

- [hyperfleet-infra README](https://github.com/openshift-hyperfleet/hyperfleet-infra/blob/main/README.md) - Main infrastructure documentation

For test-specific troubleshooting (timeouts, API errors, namespace mismatches), see the [Runbook Troubleshooting](runbook.md#troubleshooting) section.

---

**Next Steps:** Once your environment is set up, see the [Runbook](runbook.md) for running tests and troubleshooting.
