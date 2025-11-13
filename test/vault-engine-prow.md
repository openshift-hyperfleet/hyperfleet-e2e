# Vault Engine on Prow - Documentation

**Metadata**
- **Date:** 2025-11-13
- **Authors:** Ying Zhang
- **Related Jira(s):** [HYPERFLEET-140](https://issues.redhat.com/browse/HYPERFLEET-140)

## Overview
This document provides guidance on how to apply for, manage, and use Vault secrets in the OpenShift CI (Prow) environment.

## How to Apply

### Prerequisites
- Users must have logged in to the DPTP Vault system at least once before they can be added as members
- Access the Vault system at: https://selfservice.vault.ci.openshift.org

### Applying Vault Access for New Team

Follow the official documentation: https://docs.ci.openshift.org/docs/how-tos/adding-a-new-secret-to-ci/

#### Step 1: Create a Secret Collection for Your Team

1. Navigate to the [Secret Collection UI](https://selfservice.vault.ci.openshift.org/secretcollection?ui=true)
2. Create a new collection for your team (e.g., "hyperfleet")
3. Add team members to the collection
   - **Note**: Users must have logged in to the DPTP Vault system at least once before they are listed as potential members

#### Step 2: Create a New Secret in Vault

1. **Log in to Vault**
   - Navigate to https://vault.ci.openshift.org
   - Use your OIDC authentication (leave the Role field blank/Default)

2. **Navigate to Your Secret Collection**
   - After logging in, click on **kv**
   - You should see your secret collection listed
   - Click on the link for your secret collection

3. **Create the Secret**
   - Click **Create secret +**
   - Enter the new path in the "Path for this secret" box
     - Format: `selfservice/<your-secret-collection>/<newpath>`
     - Example: `selfservice/hyperfleet/hyperfleet-e2e`
   - The message "The secret path may not end in /" should disappear when the path is valid

4. **Add Secret Data and Metadata**

   Add your secret data as key-value pairs, along with the following special `secretsync` key-value pairs to ensure the secret is propagated to the build clusters:

   **Required secretsync Fields:**

   - **secretsync/target-namespace**: The namespace of your secret in the build clusters
     - Common values: `"ci"` or `"test-credentials"`
     - Multiple namespaces can be targeted using a comma-separated list
     - **Note**: If you are adding secrets for multi-stage test, it should be `"test-credentials"` ns. Only if you were adding a [cluster profile](https://docs.ci.openshift.org/docs/how-tos/adding-a-cluster-profile/#providing-credentials) secret it would be `"ci"`.

   - **secretsync/target-name**: The name of your secret in the build clusters
     - Example: `"cluster-secrets-hyperfleet-clm"`

   **Your Secret Data:**
   ```
   <token_name>: <token_value>
   ```

5. **Save the Secret**
   - Click **Save** to create the secret
   - The secret will be automatically synchronized to the target namespace(s)

**Example Configuration:**
```yaml
Path: selfservice/hyperfleet/hyperfleet-clm

Key-Value Pairs:
  secretsync/target-namespace: "test-credentials"
  secretsync/target-name: "cluster-secrets-hyperfleet"
  api-token: "your-token-value-here"
  access-key: "your-access-key-here"
```

## How to Use

Update the release repository configuration to include your new secret configuration.

1. Add the new alias in the ci-tools repo. Example [PR](https://github.com/openshift/ci-tools/pull/4822)
2. Add the above slice for new team secret.The new added cluster profile will be used for the test jobs on PROW. Example [PR](https://github.com/openshift/release/pull/71341)
3. After the two PRs are merged,you can use the new cluster profile alias in test job
```yaml
- as: hyperfleet-api-e2e-critical-high-presubmit
  steps:
    cluster_profile: hyperfleet-clm
    env:
      cluster_name: hyperfleet-e2e-tests
      ...
    test:
    - ref: hyperfleet-api-e2e-tests
    workflow: hyperfleet-api-gcp-hcp
```


## How to Manage Secrets

### Updating Secrets

1. Log in to the [Vault Self-Service UI](https://selfservice.vault.ci.openshift.org)
2. Navigate to your team's secret collection
3. Locate the secret you want to update
4. Modify the secret values as needed
5. Save changes - the secret will be automatically synchronized to the target namespaces

### Managing Team Access

1. Access the [Secret Collection UI](https://selfservice.vault.ci.openshift.org/secretcollection?ui=true)
2. Select your team's collection
3. Add or remove team members as needed
4. Ensure new members have logged in to Vault at least once before adding them

## Support

For assistance or questions:
- Review the official documentation linked above
- Contact your team's Vault administrator
- Reach out to the DPTP (Developer Productivity Test Platform) team in slack channel [#forum-ocp-testplatform](https://redhat.enterprise.slack.com/archives/CBN38N3MW)
