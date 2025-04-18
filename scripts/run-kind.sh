#!/bin/bash

# This shell script deploys a kind cluster with an Istio-based Gateway API
# implementation fully configured. It deploys the vllm simulator, which it
# exposes with a Gateway and HTTPRoute. The Gateway is configured with the
# a filter for the ext_proc endpoint picker.

set -eo pipefail

# ------------------------------------------------------------------------------
# Variables
# ------------------------------------------------------------------------------

# TODO: get image names, paths, versions, etc. from the .version.json file
# See: https://github.com/neuralmagic/gateway-api-inference-extension/issues/28

# Set a default CLUSTER_NAME if not provided
: "${CLUSTER_NAME:=inference-gateway}"

# Set the default IMAGE_REGISTRY if not provided
: "${IMAGE_REGISTRY:=quay.io/vllm-d}"

# Set a default VLLM_SIMULATOR_VERSION if not provided
: "${VLLM_SIMULATOR_VERSION:=0.0.2}"

# Set a default ENDPOINT_PICKER_VERSION if not provided
: "${ENDPOINT_PICKER_VERSION:=0.0.1}"

# Set a default ENDPOINT_PICKER_IMAGE if not provided
: "${ENDPOINT_PICKER_IMAGE:=gateway-api-inference-extension/epp}"

# ------------------------------------------------------------------------------
# Setup & Requirement Checks
# ------------------------------------------------------------------------------

# Check for a supported container runtime if an explicit one was not set
if [ -z "${CONTAINER_RUNTIME}" ]; then
  if command -v docker &> /dev/null; then
    CONTAINER_RUNTIME="docker"
  elif command -v podman &> /dev/null; then
    CONTAINER_RUNTIME="podman"
  else
    echo "Neither docker nor podman could be found in PATH" >&2
    exit 1
  fi
fi

set -u

# Check for required programs
for cmd in kind kubectl ${CONTAINER_RUNTIME}; do
    if ! command -v "$cmd" &> /dev/null; then
        echo "Error: $cmd is not installed or not in the PATH."
        exit 1
    fi
done

# @TODO Make sure the EPP and vllm-sim images are present or built
# EPP: `make image-load` in the GIE repo
# vllm-sim: ``
# note: you may need to retag the built images to match the expected path and
# versions listed above
# See: https://github.com/neuralmagic/gateway-api-inference-extension/issues/28

# ------------------------------------------------------------------------------
# Cluster Deployment
# ------------------------------------------------------------------------------

# Check if the cluster already exists
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster '${CLUSTER_NAME}' already exists, re-using"
else
    kind create cluster --name "${CLUSTER_NAME}"
fi

# Set the kubectl context to the kind cluster
KUBE_CONTEXT="kind-${CLUSTER_NAME}"

set -x

# Hotfix for https://github.com/kubernetes-sigs/kind/issues/3880
CONTAINER_NAME="${CLUSTER_NAME}-control-plane"
${CONTAINER_RUNTIME} exec -it ${CONTAINER_NAME} /bin/bash -c "sysctl net.ipv4.conf.all.arp_ignore=0"

# Wait for all pods to be ready
kubectl --context ${KUBE_CONTEXT} -n kube-system wait --for=condition=Ready --all pods --timeout=300s
kubectl --context ${KUBE_CONTEXT} -n local-path-storage wait --for=condition=Ready --all pods --timeout=300s

# Load the vllm simulator image into the cluster
if [ "${CONTAINER_RUNTIME}" == "podman" ]; then
	podman tag localhost/vllm-d/vllm-sim:${VLLM_SIMULATOR_VERSION} ${IMAGE_REGISTRY}/vllm-sim:${VLLM_SIMULATOR_VERSION}
	podman save ${IMAGE_REGISTRY}/vllm-sim:${VLLM_SIMULATOR_VERSION} -o /dev/stdout | kind --name ${CLUSTER_NAME} load image-archive /dev/stdin
else
	kind --name ${CLUSTER_NAME} load docker-image ${IMAGE_REGISTRY}/vllm-sim:${VLLM_SIMULATOR_VERSION}
fi

# Load the ext_proc endpoint-picker image into the cluster
if [ "${CONTAINER_RUNTIME}" == "podman" ]; then
	podman tag localhost/${ENDPOINT_PICKER_IMAGE}:${ENDPOINT_PICKER_VERSION} ${IMAGE_REGISTRY}/${ENDPOINT_PICKER_IMAGE}:${ENDPOINT_PICKER_VERSION}
	podman save ${IMAGE_REGISTRY}/${ENDPOINT_PICKER_IMAGE}:${ENDPOINT_PICKER_VERSION} -o /dev/stdout | kind --name ${CLUSTER_NAME} load image-archive /dev/stdin
else
	kind --name ${CLUSTER_NAME} load docker-image ${IMAGE_REGISTRY}/${ENDPOINT_PICKER_IMAGE}:${ENDPOINT_PICKER_VERSION}
fi

# ------------------------------------------------------------------------------
# CRD Deployment (Gateway API + GIE)
# ------------------------------------------------------------------------------

kubectl kustomize deploy/components/crds |
	kubectl --context ${KUBE_CONTEXT} apply --server-side --force-conflicts -f -

# ------------------------------------------------------------------------------
# Sail Operator Deployment
# ------------------------------------------------------------------------------

# Deploy the Sail Operator
kubectl kustomize --enable-helm deploy/components/sail-operator |
	kubectl --context ${KUBE_CONTEXT} apply --server-side --force-conflicts -f -

# Wait for the Sail Operator to be ready
kubectl --context ${KUBE_CONTEXT} -n sail-operator wait deployment/sail-operator --for=condition=Available --timeout=60s

# ------------------------------------------------------------------------------
# Development Environment
# ------------------------------------------------------------------------------

# Deploy the environment to the "default" namespace
kubectl kustomize deploy/environments/kind | sed 's/REPLACE_NAMESPACE/default/gI' \
	| kubectl --context ${KUBE_CONTEXT} apply -f -

# Wait for all pods to be ready
kubectl --context ${KUBE_CONTEXT} wait --for=condition=Ready --all pods --timeout=300s

# Wait for the gateway to be ready
kubectl --context ${KUBE_CONTEXT} wait gateway/inference-gateway --for=condition=Programmed --timeout=60s

cat <<EOF
-----------------------------------------
Deployment completed!

* Kind Cluster Name: ${CLUSTER_NAME}
* Kubectl Context: ${KUBE_CONTEXT}

Status:

* The vllm simulator is running
* The Gateway is exposing the simulator
* The Endpoint Picker is loaded into the Gateway via ext_proc

You can watch the Endpoint Picker logs with:

  $ kubectl --context ${KUBE_CONTEXT} logs -f deployments/endpoint-picker

You can use a port-forward to access the Gateway:

  $ kubectl --context ${KUBE_CONTEXT} port-forward service/inference-gateway-istio 8080:80

With that running in the background, you can make requests:

  $ curl -v -w '\n' -X POST -H 'Content-Type: application/json' -d '{"model":"model1","messages":[{"role":"user","content":"help"}]}' http://localhost:8080

-----------------------------------------
EOF
