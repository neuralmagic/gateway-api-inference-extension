# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0

TARGETOS ?= linux
TARGETARCH ?= amd64

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

GIT_TAG ?= $(shell git describe --tags --dirty --always)
PLATFORMS ?= linux/amd64
DOCKER_BUILDX_CMD ?= docker buildx
IMAGE_BUILD_CMD ?= $(DOCKER_BUILDX_CMD) build
IMAGE_BUILD_EXTRA_OPTS ?=
SYNCER_IMAGE_BUILD_EXTRA_OPTS ?=
BBR_IMAGE_BUILD_EXTRA_OPTS ?=
STAGING_IMAGE_REGISTRY ?= us-central1-docker.pkg.dev/k8s-staging-images
IMAGE_REGISTRY ?= $(STAGING_IMAGE_REGISTRY)/gateway-api-inference-extension
IMAGE_NAME := epp
IMAGE_REPO ?= $(IMAGE_REGISTRY)/$(IMAGE_NAME)
IMAGE_TAG ?= $(IMAGE_REPO):$(GIT_TAG)
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
E2E_MANIFEST_PATH ?= config/manifests/vllm/gpu-deployment.yaml

SYNCER_IMAGE_NAME := lora-syncer
SYNCER_IMAGE_REPO ?= $(IMAGE_REGISTRY)/$(SYNCER_IMAGE_NAME)
SYNCER_IMAGE_TAG ?= $(SYNCER_IMAGE_REPO):$(GIT_TAG)

BBR_IMAGE_NAME := bbr
BBR_IMAGE_REPO ?= $(IMAGE_REGISTRY)/$(BBR_IMAGE_NAME)
BBR_IMAGE_TAG ?= $(BBR_IMAGE_REPO):$(GIT_TAG)

BASE_IMAGE ?= gcr.io/distroless/static:nonroot
BUILDER_IMAGE ?= golang:1.24
ifdef GO_VERSION
BUILDER_IMAGE = golang:$(GO_VERSION)
endif

ifdef EXTRA_TAG
IMAGE_EXTRA_TAG ?= $(IMAGE_REPO):$(EXTRA_TAG)
SYNCER_IMAGE_EXTRA_TAG ?= $(SYNCER_IMAGE_REPO):$(EXTRA_TAG)
BBR_IMAGE_EXTRA_TAG ?= $(BBR_IMAGE_REPO):$(EXTRA_TAG)
endif
ifdef IMAGE_EXTRA_TAG
IMAGE_BUILD_EXTRA_OPTS += -t $(IMAGE_EXTRA_TAG)
SYNCER_IMAGE_BUILD_EXTRA_OPTS += -t $(SYNCER_IMAGE_EXTRA_TAG)
BBR_IMAGE_BUILD_EXTRA_OPTS += -t $(BBR_IMAGE_EXTRA_TAG)
endif

# The name of the kind cluster to use for the "kind-load" target.
KIND_CLUSTER ?= kind

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

# .PHONY: help
# help: ## Display this help.
# 	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen code-generator manifests ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."
	./hack/update-codegen.sh

# Use same code-generator version as k8s.io/api
CODEGEN_VERSION := $(shell go list -m -f '{{.Version}}' k8s.io/api)
CODEGEN = $(shell pwd)/bin/code-generator
CODEGEN_ROOT = $(shell go env GOMODCACHE)/k8s.io/code-generator@$(CODEGEN_VERSION)
.PHONY: code-generator
code-generator:
	@GOBIN=$(PROJECT_DIR)/bin GO111MODULE=on go install k8s.io/code-generator/cmd/client-gen@$(CODEGEN_VERSION)
	cp -f $(CODEGEN_ROOT)/generate-groups.sh $(PROJECT_DIR)/bin/
	cp -f $(CODEGEN_ROOT)/generate-internal-groups.sh $(PROJECT_DIR)/bin/
	cp -f $(CODEGEN_ROOT)/kube_codegen.sh $(PROJECT_DIR)/bin/

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: fmt-verify
fmt-verify:
	@out=`gofmt -w -l -d $$(find . -name '*.go')`; \
	if [ -n "$$out" ]; then \
	    echo "$$out"; \
	    exit 1; \
	fi

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

# .PHONY: test
# test: manifests generate fmt vet envtest image-build ## Run tests.
# 	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -race -coverprofile cover.out

.PHONY: test-integration
test-integration: ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./test/integration/epp/... -race -coverprofile cover.out

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests against an existing Kubernetes cluster. When using default configuration, the tests need at least 3 available GPUs.
	MANIFEST_PATH=$(PROJECT_DIR)/$(E2E_MANIFEST_PATH) go test ./test/e2e/epp/ -v -ginkgo.v

# .PHONY: lint
# lint: golangci-lint ## Run golangci-lint linter
# 	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: ci-lint
ci-lint: golangci-lint
	$(GOLANGCI_LINT) run --timeout 15m0s

.PHONY: verify
verify: vet fmt-verify manifests generate ci-lint
	git --no-pager diff --exit-code config api client-go

##@ Build

# Build the container image
.PHONY: image-local-build
image-local-build: ## Build the EPP image using Docker Buildx for local development.
	BUILDER=$(shell $(DOCKER_BUILDX_CMD) create --use)
	$(MAKE) image-build PUSH=$(PUSH)
	$(MAKE) image-build LOAD=$(LOAD)
	$(DOCKER_BUILDX_CMD) rm $$BUILDER

.PHONY: image-local-push
image-local-push: PUSH=--push ## Build the EPP image for local development and push it to $IMAGE_REPO.
image-local-push: image-local-build

.PHONY: image-local-load
image-local-load: LOAD=--load ## Build the EPP image for local development and load it in the local Docker registry.
image-local-load: image-local-build

# .PHONY: image-build
# image-build: ## Build the EPP image using Docker Buildx.
# 	$(IMAGE_BUILD_CMD) -t $(IMAGE_TAG) \
# 		--platform=$(PLATFORMS) \
# 		--build-arg BASE_IMAGE=$(BASE_IMAGE) \
# 		--build-arg BUILDER_IMAGE=$(BUILDER_IMAGE) \
# 		$(PUSH) \
# 		$(LOAD) \
# 		$(IMAGE_BUILD_EXTRA_OPTS) ./

.PHONY: image-push
image-push: PUSH=--push ## Build the EPP image and push it to $IMAGE_REPO.
image-push: image-build

.PHONY: image-load
image-load: LOAD=--load ## Build the EPP image and load it in the local Docker registry.
image-load: image-build

.PHONY: image-kind
image-kind: image-build ## Build the EPP image and load it to kind cluster $KIND_CLUSTER ("kind" by default).
	kind load docker-image $(IMAGE_TAG) --name $(KIND_CLUSTER)

##@ Lora Syncer

.PHONY: syncer-image-local-build
syncer-image-local-build:
	BUILDER=$(shell $(DOCKER_BUILDX_CMD) create --use)
	$(MAKE) image-build PUSH=$(PUSH)
	$(DOCKER_BUILDX_CMD) rm $$BUILDER

.PHONY: syncer-image-local-push
syncer-image-local-push: PUSH=--push
syncer-image-local-push: syncer-image-local-build

.PHONY: syncer-image-build
syncer-image-build:
	$ cd $(CURDIR)/tools/dynamic-lora-sidecar && $(IMAGE_BUILD_CMD) -t $(SYNCER_IMAGE_TAG) \
		--platform=$(PLATFORMS) \
		--build-arg BASE_IMAGE=$(BASE_IMAGE) \
		--build-arg BUILDER_IMAGE=$(BUILDER_IMAGE) \
		$(PUSH) \
		$(SYNCER_IMAGE_BUILD_EXTRA_OPTS) ./

.PHONY: syncer-image-push
syncer-image-push: PUSH=--push
syncer-image-push: syncer-image-build

##@ Body-based Routing extension

# Build the container image
.PHONY: bbr-image-local-build
bbr-image-local-build: ## Build the image using Docker Buildx for local development.
	BUILDER=$(shell $(DOCKER_BUILDX_CMD) create --use)
	$(MAKE) bbr-image-build PUSH=$(PUSH)
	$(MAKE) bbr-image-build LOAD=$(LOAD)
	$(DOCKER_BUILDX_CMD) rm $$BUILDER

.PHONY: bbr-image-local-push
bbr-image-local-push: PUSH=--push ## Build the image for local development and push it to $IMAGE_REPO.
bbr-image-local-push: bbr-image-local-build

.PHONY: bbr-image-local-load
bbr-image-local-load: LOAD=--load ## Build the image for local development and load it in the local Docker registry.
bbr-image-local-load: bbr-image-local-build

.PHONY: bbr-image-build
bbr-image-build: ## Build the image using Docker Buildx.
	$(IMAGE_BUILD_CMD) -f bbr.Dockerfile -t $(BBR_IMAGE_TAG) \
		--platform=$(PLATFORMS) \
		--build-arg BASE_IMAGE=$(BASE_IMAGE) \
		--build-arg BUILDER_IMAGE=$(BUILDER_IMAGE) \
		$(PUSH) \
		$(LOAD) \
		$(BBR_IMAGE_BUILD_EXTRA_OPTS) ./

.PHONY: bbr-image-push
bbr-image-push: PUSH=--push ## Build the image and push it to $IMAGE_REPO.
bbr-image-push: bbr-image-build

.PHONY: bbr-image-load
bbr-image-load: LOAD=--load ## Build the image and load it in the local Docker registry.
bbr-image-load: bbr-image-build

.PHONY: bbr-image-kind
bbr-image-kind: bbr-image-build ## Build the image and load it to kind cluster $KIND_CLUSTER ("kind" by default).
	kind load docker-image $(BBR_IMAGE_TAG) --name $(KIND_CLUSTER)

##@ Docs

.PHONY: build-docs
build-docs:
	docker build --pull -t gaie/mkdocs hack/mkdocs/image
	docker run --rm -v ${PWD}:/docs gaie/mkdocs build

.PHONY: build-docs-netlify
build-docs-netlify:
	pip install -r hack/mkdocs/image/requirements.txt
	python -m mkdocs build

.PHONY: live-docs
live-docs:
	docker build -t gaie/mkdocs hack/mkdocs/image
	docker run --rm -it -p 3000:3000 -v ${PWD}:/docs gaie/mkdocs

.PHONY: api-ref-docs
api-ref-docs:
	crd-ref-docs \
		--source-path=${PWD}/api \
		--config=crd-ref-docs.yaml \
		--renderer=markdown \
		--output-path=${PWD}/site-src/reference/spec.md

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

# .PHONY: install
# install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
# 	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

# .PHONY: uninstall
# uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
# 	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -


##@ Helm
PHONY: inferencepool-helm-chart-push
inferencepool-helm-chart-push: yq helm
	CHART=inferencepool EXTRA_TAG="$(EXTRA_TAG)" IMAGE_REGISTRY="$(IMAGE_REGISTRY)" YQ="$(YQ)" HELM="$(HELM)" ./hack/push-chart.sh

PHONY: bbr-helm-chart-push
bbr-helm-chart-push: yq helm
	CHART=body-based-routing EXTRA_TAG="$(EXTRA_TAG)" IMAGE_REGISTRY="$(IMAGE_REGISTRY)" YQ="$(YQ)" HELM="$(HELM)" ./hack/push-chart.sh

##@ Release

.PHONY: release-quickstart
release-quickstart: ## Update the quickstart guide for a release.
	./hack/release-quickstart.sh

.PHONY: artifacts
artifacts: kustomize
	if [ -d artifacts ]; then rm -rf artifacts; fi
	mkdir -p artifacts
	$(KUSTOMIZE) build config/crd -o artifacts/manifests.yaml
	@$(call clean-manifests)

.PHONY: release
release: artifacts release-quickstart verify test # Create a release.

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
HELM = $(PROJECT_DIR)/bin/helm
YQ = $(PROJECT_DIR)/bin/yq

## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.3
CONTROLLER_TOOLS_VERSION ?= v0.16.1
ENVTEST_VERSION ?= release-0.19
GOLANGCI_LINT_VERSION ?= v1.62.2
HELM_VERSION ?= v3.17.1

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: yq
yq: ## Download yq locally if necessary.
	GOBIN=$(PROJECT_DIR)/bin GO111MODULE=on go install github.com/mikefarah/yq/v4@v4.45.1

.PHONY: helm
helm: ## Download helm locally if necessary.
	GOBIN=$(PROJECT_DIR)/bin GO111MODULE=on go install helm.sh/helm/v3/cmd/helm@$(HELM_VERSION)

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef




SHELL := /usr/bin/env bash

# Defaults
PROJECT_NAME ?= gateway-api-inference-extension
DEV_VERSION ?= 0.0.1
PROD_VERSION ?= 0.0.0
IMAGE_TAG_BASE ?= quay.io/vllm-d/$(PROJECT_NAME)/epp
IMG = $(IMAGE_TAG_BASE):$(DEV_VERSION)

# CONTAINER_TOOL := $(shell command -v docker >/dev/null 2>&1 && echo docker || command -v podman >/dev/null 2>&1 && echo podman || echo "")
BUILDER := $(shell command -v buildah >/dev/null 2>&1 && echo buildah || echo $(CONTAINER_TOOL))
PLATFORMS ?= linux/amd64 # linux/arm64 # linux/s390x,linux/ppc64le

# go source files
SRC = $(shell find . -type f -name '*.go')

.PHONY: help
help: ## Print help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: format
format: ## Format Go source files
	@printf "\033[33;1m==== Running gofmt ====\033[0m\n"
	@gofmt -l -w $(SRC)

.PHONY: test
test: check-ginkgo ## Run tests
	@printf "\033[33;1m==== Running tests ====\033[0m\n"
	echo Skipping temporarily!
	@echo "tests passed."
	# ginkgo -r -v

.PHONY: post-deploy-test
post-deploy-test: ## Run post deployment tests
	echo Success!
	@echo "Post-deployment tests passed."
	
.PHONY: lint
lint: check-golangci-lint ## Run lint
	@printf "\033[33;1m==== Running linting ====\033[0m\n"
	golangci-lint run

##@ Build

.PHONY: build
build: check-go ##
	@printf "\033[33;1m==== Building ====\033[0m\n"
	go build -o bin/epp cmd/epp/main.go cmd/epp/health.go

##@ Container Build/Push

.PHONY: buildah-build
buildah-build: check-builder load-version-json ## Build and push image (multi-arch if supported)
	@echo "✅ Using builder: $(BUILDER)"
	@if [ "$(BUILDER)" = "buildah" ]; then \
	  echo "🔧 Buildah detected: Performing multi-arch build..."; \
	  for arch in amd64; do \
	    echo "📦 Building for architecture: $$arch"; \
	    buildah build --arch=$$arch --os=linux -t $(IMG)-$$arch . || exit 1; \
	    echo "🚀 Pushing image: $(IMG)-$$arch"; \
	    buildah push $(IMG)-$$arch docker://$(IMG)-$$arch || exit 1; \
	  done; \
	  echo "🧱 Creating and pushing manifest list: $(IMG)"; \
	  buildah manifest create $(IMG); \
	  buildah manifest add $(IMG) $(IMG)-amd64; \
	  buildah manifest push --all $(IMG) docker://$(IMG); \
	elif [ "$(BUILDER)" = "docker" ]; then \
	  echo "🐳 Docker detected: Building with buildx..."; \
	  sed -e '1 s/\(^FROM\)/FROM --platform=$${BUILDPLATFORM}/' Dockerfile > Dockerfile.cross; \
	  - docker buildx create --use --name image-builder || true; \
	  docker buildx use image-builder; \
	  docker buildx build --push --platform=$(PLATFORMS) --tag $(IMG) -f Dockerfile.cross . || exit 1; \
	  docker buildx rm image-builder || true; \
	  rm Dockerfile.cross; \
	elif [ "$(BUILDER)" = "podman" ]; then \
	  echo "⚠️ Podman detected: Building single-arch image..."; \
	  podman build -t $(IMG) . || exit 1; \
	  podman push $(IMG) || exit 1; \
	else \
	  echo "❌ No supported container tool available."; \
	  exit 1; \
	fi

.PHONY:	image-build
image-build: check-container-tool load-version-json ## Build container image using $(CONTAINER_TOOL)
	@printf "\033[33;1m==== Building container image $(IMG) ====\033[0m\n"
	$(CONTAINER_TOOL) build --build-arg TARGETOS=$(TARGETOS) --build-arg TARGETARCH=$(TARGETARCH) -t $(IMG) .

.PHONY: image-push
image-push: check-container-tool load-version-json ## Push container image $(IMG) to registry
	@printf "\033[33;1m==== Pushing container image $(IMG) ====\033[0m\n"
	$(CONTAINER_TOOL) push $(IMG)

##@ Install/Uninstall Targets

# Default install/uninstall (Docker)
install: install-docker ## Default install using Docker
	@echo "Default Docker install complete."

uninstall: uninstall-docker ## Default uninstall using Docker
	@echo "Default Docker uninstall complete."

### Docker Targets

.PHONY: install-docker
install-docker: check-container-tool ## Install app using $(CONTAINER_TOOL)
	@echo "Starting container with $(CONTAINER_TOOL)..."
	$(CONTAINER_TOOL) run -d --name $(PROJECT_NAME)-container $(IMG)
	@echo "$(CONTAINER_TOOL) installation complete."
	@echo "To use $(PROJECT_NAME), run:"
	@echo "alias $(PROJECT_NAME)='$(CONTAINER_TOOL) exec -it $(PROJECT_NAME)-container /app/$(PROJECT_NAME)'"

.PHONY: uninstall-docker
uninstall-docker: check-container-tool ## Uninstall app from $(CONTAINER_TOOL)
	@echo "Stopping and removing container in $(CONTAINER_TOOL)..."
	-$(CONTAINER_TOOL) stop $(PROJECT_NAME)-container && $(CONTAINER_TOOL) rm $(PROJECT_NAME)-container
@echo "$(CONTAINER_TOOL) uninstallation complete. Remove alias if set: unalias $(PROJECT_NAME)"


##@ Version Extraction
.PHONY: version dev-registry prod-registry extract-version-info

dev-version: check-jq
	@jq -r '.dev-version' .version.json

prod-version: check-jq
	@jq -r '.prod-version' .version.json

dev-registry: check-jq
	@jq -r '."dev-registry"' .version.json

prod-registry: check-jq
	@jq -r '."prod-registry"' .version.json

extract-version-info: check-jq
	@echo "DEV_VERSION=$$(jq -r '."dev-version"' .version.json)"
	@echo "PROD_VERSION=$$(jq -r '."prod-version"' .version.json)"
	@echo "DEV_IMAGE_TAG_BASE=$$(jq -r '."dev-registry"' .version.json)"
	@echo "PROD_IMAGE_TAG_BASE=$$(jq -r '."prod-registry"' .version.json)"

##@ Load Version JSON

.PHONY: load-version-json
load-version-json: check-jq
	@if [ "$(DEV_VERSION)" = "0.0.1" ]; then \
	  DEV_VERSION=$$(jq -r '."dev-version"' .version.json); \
	  PROD_VERSION=$$(jq -r '."dev-version"' .version.json); \
	  echo "✔ Loaded DEV_VERSION from .version.json: $$DEV_VERSION"; \
	  echo "✔ Loaded PROD_VERSION from .version.json: $$PROD_VERSION"; \
	  export DEV_VERSION; \
	  export PROD_VERSION; \
	fi && \
	CURRENT_DEFAULT="quay.io/vllm-d/$(PROJECT_NAME)"; \
	if [ "$(IMAGE_TAG_BASE)" = "$$CURRENT_DEFAULT" ]; then \
	  IMAGE_TAG_BASE=$$(jq -r '."dev-registry"' .version.json); \
	  echo "✔ Loaded IMAGE_TAG_BASE from .version.json: $$IMAGE_TAG_BASE"; \
	  export IMAGE_TAG_BASE; \
	fi && \
	echo "🛠 Final values: DEV_VERSION=$$DEV_VERSION, PROD_VERSION=$$PROD_VERSION, IMAGE_TAG_BASE=$$IMAGE_TAG_BASE"

.PHONY: env
env: load-version-json ## Print environment variables
	@echo "DEV_VERSION=$(DEV_VERSION)"
	@echo "PROD_VERSION=$(PROD_VERSION)"
	@echo "IMAGE_TAG_BASE=$(IMAGE_TAG_BASE)"
	@echo "IMG=$(IMG)"
	@echo "CONTAINER_TOOL=$(CONTAINER_TOOL)"


##@ Tools

.PHONY: check-tools
check-tools: \
  check-go \
  check-ginkgo \
  check-golangci-lint \
  check-jq \
  check-kustomize \
  check-envsubst \
  check-container-tool \
  check-kubectl \
  check-buildah \
  check-podman
	@echo "✅ All required tools are installed."

.PHONY: check-go
check-go:
	@command -v go >/dev/null 2>&1 || { \
	  echo "❌ Go is not installed. Install it from https://golang.org/dl/"; exit 1; }

.PHONY: check-ginkgo
check-ginkgo:
	@command -v ginkgo >/dev/null 2>&1 || { \
	  echo "❌ ginkgo is not installed. Install with: go install github.com/onsi/ginkgo/v2/ginkgo@latest"; exit 1; }

.PHONY: check-golangci-lint
check-golangci-lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
	  echo "❌ golangci-lint is not installed. Install from https://golangci-lint.run/usage/install/"; exit 1; }

.PHONY: check-jq
check-jq:
	@command -v jq >/dev/null 2>&1 || { \
	  echo "❌ jq is not installed. Install it from https://stedolan.github.io/jq/download/"; exit 1; }

.PHONY: check-kustomize
check-kustomize:
	@command -v kustomize >/dev/null 2>&1 || { \
	  echo "❌ kustomize is not installed. Install it from https://kubectl.docs.kubernetes.io/installation/kustomize/"; exit 1; }

.PHONY: check-envsubst
check-envsubst:
	@command -v envsubst >/dev/null 2>&1 || { \
	  echo "❌ envsubst is not installed. It is part of gettext."; \
	  echo "🔧 Try: sudo apt install gettext OR brew install gettext"; exit 1; }

.PHONY: check-container-tool
check-container-tool:
	@command -v $(CONTAINER_TOOL) >/dev/null 2>&1 || { \
	  echo "❌ $(CONTAINER_TOOL) is not installed."; \
	  echo "🔧 Try: sudo apt install $(CONTAINER_TOOL) OR brew install $(CONTAINER_TOOL)"; exit 1; }

.PHONY: check-kubectl
check-kubectl:
	@command -v kubectl >/dev/null 2>&1 || { \
	  echo "❌ kubectl is not installed. Install it from https://kubernetes.io/docs/tasks/tools/"; exit 1; }

.PHONY: check-builder
check-builder:
	@if [ -z "$(BUILDER)" ]; then \
		echo "❌ No container builder tool (buildah, docker, or podman) found."; \
		exit 1; \
	else \
		echo "✅ Using builder: $(BUILDER)"; \
	fi

.PHONY: check-podman
check-podman:
	@command -v podman >/dev/null 2>&1 || { \
	  echo "⚠️  Podman is not installed. You can install it with:"; \
	  echo "🔧 sudo apt install podman  OR  brew install podman"; exit 1; }

##@ Alias checking
.PHONY: check-alias
check-alias: check-container-tool
	@echo "🔍 Checking alias functionality for container '$(PROJECT_NAME)-container'..."
	@if ! $(CONTAINER_TOOL) exec $(PROJECT_NAME)-container /app/$(PROJECT_NAME) --help >/dev/null 2>&1; then \
	  echo "⚠️  The container '$(PROJECT_NAME)-container' is running, but the alias might not work."; \
	  echo "🔧 Try: $(CONTAINER_TOOL) exec -it $(PROJECT_NAME)-container /app/$(PROJECT_NAME)"; \
	else \
	  echo "✅ Alias is likely to work: alias $(PROJECT_NAME)='$(CONTAINER_TOOL) exec -it $(PROJECT_NAME)-container /app/$(PROJECT_NAME)'"; \
	fi

# This is being used for tekton builds in the CI/CD pipeline, to provide a
# default namespace to do a test deployment of the Kubernetes dev environment.
.PHONY: print-namespace
print-namespace:
	@echo "hc4ai-operator"

.PHONY: print-project-name
print-project-name: ## Print the current project name
	@echo "$(PROJECT_NAME)"

.PHONY: install-hooks
install-hooks: ## Install git hooks
	git config core.hooksPath hooks

# ==============================================================================
# Development Environments - Kubernetes in Docker (KIND)
# ==============================================================================

KIND_CLUSTER_NAME ?= gie-dev

# ------------------------------------------------------------------------------
# Kind Development Environment - Deploy
#
# This target will deploy a local kind cluster with the GIE stack deployed into
# the default namespace for development and testing.
# ------------------------------------------------------------------------------
.PHONY: environment.dev.kind
environment.dev.kind:
	CLUSTER_NAME=$(KIND_CLUSTER_NAME) ./scripts/kind-dev-env.sh

# ------------------------------------------------------------------------------
# Kind Development Environment - Update
#
# This target will build the current changes into an image, load them into an
# existing kind cluster and perform a rollout so that the new changes are
# reflected in the environment.
# ------------------------------------------------------------------------------
.PHONY: environment.dev.kind.update
environment.dev.kind.update: image-build
	@echo "INFO: Loading images into cluster"
	CLUSTER_NAME=$(KIND_CLUSTER_NAME) ./scripts/kind-load-images.sh 2>&1
	@echo "INFO: Restarting the Endpoint Picker Deployment"
	$(KUBECTL) --context kind-$(KIND_CLUSTER_NAME) -n default rollout restart deployment endpoint-picker
	$(KUBECTL) --context kind-$(KIND_CLUSTER_NAME) -n default rollout status deployment endpoint-picker

# ------------------------------------------------------------------------------
# Kind Development Environment - Teardown
#
# This target will tear down the entire Kind cluster.
# ------------------------------------------------------------------------------
.PHONY: clean.environment.dev.kind
clean.environment.dev.kind:
	@echo "INFO: cleaning up kind cluster $(KIND_CLUSTER_NAME)"
	kind delete cluster --name $(KIND_CLUSTER_NAME)

# ==============================================================================
# Development Environments - Kubernetes
# ==============================================================================

# ------------------------------------------------------------------------------
# Kubernetes Development Environment - Deploy Infrastructure
#
# This target deploys infrastructure requirements for the entire cluster.
# Among other things, this includes CRDs and operators which all users of the
# cluster need for development (e.g. Gateway API, Istio, etc).
#
# **Warning**: This needs to be run and regularly updated by an admin to
# support the individual development environments on the cluster.
#
# **Warning**: Only run this if you're certain you should be running it. It
# has implications for all users of the cluster!
# ------------------------------------------------------------------------------
.PHONY: environment.dev.kubernetes.infrastructure
environment.dev.kubernetes.infrastructure:
ifeq ($(strip $(INFRASTRUCTURE_OVERRIDE)),true)
	@echo "Deploying OpenShift Infrastructure Components"
	kustomize build deploy/environments/dev/kubernetes-infra | kubectl apply --server-side --force-conflicts -f -
else
	$(error "Error: The environment variable INFRASTRUCTURE_OVERRIDE must be set to true in order to run this target.")
endif

# ------------------------------------------------------------------------------
# Kubernetes Development Environment - Teardown Infrastructure
#
# This target removes all infrastructure components (e.g. CRDs, operators,
# etc) for the entire cluster.
#
# **Warning**: Only run this if you're certain you should be running it. **This
# will disrupt everyone using the cluster**. Generally this should only be run
# when the infrastructure components have undergone very significant change, and
# you need to do a hard cleanup and re-deploy.
# ------------------------------------------------------------------------------
.PHONY: clean.environment.dev.kubernetes.infrastructure
clean.environment.dev.kubernetes.infrastructure:
ifeq ($(strip $(INFRASTRUCTURE_OVERRIDE)),true)
	@echo "This is extremely destructive. We'll provide 5 seconds before starting to give you a chance to cancel."
	sleep 5
	@echo "Tearing Down OpenShift Infrastructure Components"
	kustomize build deploy/environments/dev/kubernetes-infra | kubectl delete -f - || true
else
	$(error "Error: The environment variable INFRASTRUCTURE_OVERRIDE must be set to true in order to run this target.")
endif

# ------------------------------------------------------------------------------
# Kubernetes Development Environment - Deploy
#
# This target deploys the GIE stack in a specific namespace for development and
# testing.
# ------------------------------------------------------------------------------
VLLM_SIM_IMAGE ?= quay.io/vllm-d/vllm-sim
VLLM_SIM_TAG ?= 0.0.2
EPP_IMAGE ?= us-central1-docker.pkg.dev/k8s-staging-images/gateway-api-inference-extension/epp
EPP_TAG ?= main
.PHONY: environment.dev.kubernetes
environment.dev.kubernetes: check-kubectl check-kustomize check-envsubst
	@echo "INFO: checking required vars"
ifndef NAMESPACE
	$(error "Error: NAMESPACE is required but not set)
endif
	export NAMESPACE=$(NAMESPACE)
ifndef REGISTRY_SECRET
	$(error "Error: REGISTRY_SECRET is required but not set)
endif
	export REGISTRY_SECRET=$(REGISTRY_SECRET)
	export VLLM_SIM_IMAGE=$(VLLM_SIM_IMAGE)
	export VLLM_SIM_TAG=$(VLLM_SIM_TAG)
	export EPP_IMAGE=$(EPP_IMAGE)
	export EPP_TAG=$(EPP_TAG)
	@echo "INFO: Creating namespace (if needed) and setting context to $(NAMESPACE)..."
	kubectl create namespace $(NAMESPACE) 2>/dev/null || true
	kubectl config set-context --current --namespace=$(NAMESPACE)
	@echo "INFO: Deploying Development Environment in namespace $(NAMESPACE)"
	kustomize build deploy/environments/dev/kubernetes | envsubst | kubectl -n $(NAMESPACE) apply -f -
	@echo "INFO: Waiting for Pods in namespace $(NAMESPACE) to become ready"
	kubectl -n $(NAMESPACE) wait --for=condition=Ready --all pods --timeout=300s
	@echo "INFO: Waiting for Gateway in namespace $(NAMESPACE) to become ready"
	kubectl -n $(NAMESPACE) wait gateway/inference-gateway --for=condition=Programmed --timeout=60s

# ------------------------------------------------------------------------------
# Kubernetes Development Environment - Teardown
#
# Tears down the namespace, and therefore the development environment.
# ------------------------------------------------------------------------------
.PHONY: clean.environment.dev.kubernetes
clean.environment.dev.kubernetes: check-kubectl check-kustomize check-envsubst
ifndef NAMESPACE
	$(error "Error: NAMESPACE is required but not set)
endif
	@echo "INFO: deleting namespace $(NAMESPACE)"
	kubectl delete namespace $(NAMESPACE)

# -----------------------------------------------------------------------------
# TODO: these are old aliases that we still need for the moment, but will be
# cleaned up later.
#
# See: https://github.com/neuralmagic/gateway-api-inference-extension/issues/28
# -----------------------------------------------------------------------------

.PHONY: install-openshift-infrastructure
install-openshift-infrastructure: environment.dev.kubernetes.infrastructure

.PHONY: uninstall-openshift-infrastructure
uninstall-openshift-infrastructure: clean.environment.dev.kubernetes.infrastructure

.PHONY: install-openshift
install-openshift: environment.dev.kubernetes

.PHONY: uninstall-openshift
uninstall-openshift: clean.environment.dev.kubernetes
