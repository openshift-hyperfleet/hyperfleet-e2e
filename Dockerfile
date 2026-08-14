# HyperFleet E2E Testing Framework
#
# Build: podman build -t quay.io/hyperfleet/hyperfleet-e2e:latest .
# Build with commit: podman build --build-arg GIT_COMMIT=$(git rev-parse HEAD) -t quay.io/hyperfleet/hyperfleet-e2e:latest .
# Run:   podman run --rm -e HYPERFLEET_API_URL=<url> quay.io/hyperfleet/hyperfleet-e2e:latest test

ARG BASE_IMAGE=registry.access.redhat.com/ubi9/go-toolset

# Build stage
FROM golang:1.26.5 AS builder

WORKDIR /build

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends make && rm -rf /var/lib/apt/lists/*

# Copy source code
COPY . .

# Build binary using make to include commit and build date
ARG GIT_COMMIT=unknown
RUN make build GIT_COMMIT=${GIT_COMMIT}

# Build ginkgo CLI (version-locked via tools/go.mod)
RUN cd tools && go build -o /build/bin/ginkgo "github.com/onsi/ginkgo/v2/ginkgo"

# Compile the E2E test binary for ginkgo CLI parallel execution
RUN CGO_ENABLED=0 go test -c -o /build/bin/e2e.test ./e2e

RUN chmod +x /build/bin/hyperfleet-e2e /build/bin/ginkgo /build/bin/e2e.test

FROM registry.ci.openshift.org/ci/hyperfleet-credential-provider:latest AS hyperfleet-credential-provider

# Runtime stage
FROM ${BASE_IMAGE}

# Install runtime dependencies and tools
USER root
RUN dnf -y install --allowerasing jq gettext curl git && dnf clean all

# Install kubectl (latest stable)
RUN curl -fsSL "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" \
    -o /usr/local/bin/kubectl && chmod +x /usr/local/bin/kubectl

# Install Helm
RUN curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Set up Helm directories (cache and config writable, plugins read-only)
# OpenShift Prow runs with random UID in group 0, requiring group-writable dirs
RUN mkdir -p /tmp/helm-home/.cache/helm \
            /tmp/helm-home/.config/helm \
            /usr/local/share/helm/plugins && \
    chown -R 0:0 /tmp/helm-home /usr/local/share/helm/plugins && \
    chmod -R g=u /tmp/helm-home /usr/local/share/helm/plugins

ENV HELM_CACHE_HOME=/tmp/helm-home/.cache/helm \
    HELM_CONFIG_HOME=/tmp/helm-home/.config/helm \
    HELM_PLUGINS=/usr/local/share/helm/plugins

# Install Helm plugins
ARG HELM_DIFF_VERSION=3.15.7
RUN helm plugin install https://github.com/databus23/helm-diff --version ${HELM_DIFF_VERSION}

ARG HELMFILE_VERSION=1.5.2
# Install helmfile
RUN curl -fsSL "https://github.com/helmfile/helmfile/releases/download/v${HELMFILE_VERSION}/helmfile_${HELMFILE_VERSION}_linux_amd64.tar.gz" | \
    tar -xz -C /usr/local/bin helmfile && \
    chmod +x /usr/local/bin/helmfile

WORKDIR /e2e

# Copy the hyperfleet-credential-provider binary
COPY --from=hyperfleet-credential-provider /app/hyperfleet-credential-provider /usr/local/bin/

# Copy binaries from builder (main binary + ginkgo CLI + test binary)
COPY --from=builder /build/bin/hyperfleet-e2e /usr/local/bin/
COPY --from=builder /build/bin/ginkgo /usr/local/bin/
COPY --from=builder /build/bin/e2e.test /usr/local/bin/

# Copy test payloads and fixtures
COPY --from=builder /build/testdata /e2e/testdata

# Copy env files
COPY --from=builder /build/env /e2e/env

# Copy cleanup scripts
COPY --from=builder /build/scripts /e2e/scripts

# Copy default config (fallback if ConfigMap is not mounted)
COPY --from=builder /build/configs /e2e/configs

ENTRYPOINT ["/usr/local/bin/hyperfleet-e2e"]
CMD ["test", "--help"]

LABEL name="hyperfleet-e2e" \
      vendor="Red Hat" \
      summary="HyperFleet E2E Testing Framework" \
      description="End to end testing for HyperFleet cluster lifecycle management"
