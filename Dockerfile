# Build Stage: using Go 1.24.1 image
FROM quay.io/projectquay/golang:1.24 AS builder
ARG TARGETOS
ARG TARGETARCH

# Install build tools
RUN dnf install -y gcc-c++ libstdc++ libstdc++-devel && dnf clean all

WORKDIR /workspace

## NeuralMagic internal repos pull config
ARG GIT_NM_USER
ARG NM_TOKEN
### use git token
RUN echo -e "machine github.com\n\tlogin ${GIT_NM_USER}\n\tpassword ${NM_TOKEN}" >> ~/.netrc
ENV GOPRIVATE=github.com/neuralmagic
ENV GIT_TERMINAL_PROMPT=1

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download
RUN rm -rf ~/.netrc # remove git token

# Copy the go source
COPY cmd ./cmd
COPY pkg ./pkg
COPY internal ./internal
COPY api ./api

# HuggingFace tokenizer bindings
RUN mkdir -p lib
RUN curl -L https://github.com/daulet/tokenizers/releases/download/v1.20.2/libtokenizers.${TARGETOS}-${TARGETARCH}.tar.gz | tar -xz -C lib
RUN ranlib lib/*.a

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make image-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
ENV CGO_ENABLED=1
ENV GOOS=${TARGETOS:-linux}
ENV GOARCH=${TARGETARCH}
RUN go build -o bin/epp -ldflags="-extldflags '-L$(pwd)/lib'" cmd/epp/main.go cmd/epp/health.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.access.redhat.com/ubi9/ubi:latest
WORKDIR /
COPY --from=builder /workspace/bin/epp /app/epp
USER 65532:65532

ENTRYPOINT ["/app/epp"]
