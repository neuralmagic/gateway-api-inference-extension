#!/bin/bash

# This shell script deploys a kind cluster with an Istio-based Gateway API
# implementation fully configured. It deploys the vllm simulator, which it
# exposes with a Gateway -> HTTPRoute -> InferencePool. The Gateway is
# configured with the a filter for the ext_proc endpoint picker.

set -eo pipefail

# ------------------------------------------------------------------------------
# Variables
# ------------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# TODO: get image names, paths, versions, etc. from the .version.json file
# See: https://github.com/neuralmagic/gateway-api-inference-extension/issues/28

# Set a default CLUSTER_NAME if not provided
: "${CLUSTER_NAME:=gie-dev}"

# Set the namespace to deploy the Gateway stack to
: "${PROJECT_NAMESPACE:=default}"

# Set the host port to map to the Gateway's inbound port (30080)
: "${GATEWAY_HOST_PORT:=30080}"

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
    kind create cluster --name "${CLUSTER_NAME}" --config - << EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080
    hostPort: ${GATEWAY_HOST_PORT}
    protocol: TCP
EOF
fi

# Set the kubectl context to the kind cluster
KUBE_CONTEXT="kind-${CLUSTER_NAME}"

set -x

# Load the required container images
"${SCRIPT_DIR}/kind-load-images.sh"

# Hotfix for https://github.com/kubernetes-sigs/kind/issues/3880
CONTAINER_NAME="${CLUSTER_NAME}-control-plane"
${CONTAINER_RUNTIME} exec -it ${CONTAINER_NAME} /bin/bash -c "sysctl net.ipv4.conf.all.arp_ignore=0"

# Wait for all pods to be ready
kubectl --context ${KUBE_CONTEXT} -n kube-system wait --for=condition=Ready --all pods --timeout=300s
kubectl --context ${KUBE_CONTEXT} -n local-path-storage wait --for=condition=Ready --all pods --timeout=300s

# ------------------------------------------------------------------------------
# CRD Deployment (Gateway API + GIE)
# ------------------------------------------------------------------------------

kubectl kustomize deploy/components/crds |
	kubectl --context ${KUBE_CONTEXT} apply --server-side --force-conflicts -f -

# ------------------------------------------------------------------------------
# Istio Control Plane Deployment
# ------------------------------------------------------------------------------

kubectl kustomize deploy/components/istio-control-plane | kubectl --context ${KUBE_CONTEXT} apply -f -

# ------------------------------------------------------------------------------
# Development Environment
# ------------------------------------------------------------------------------

# Deploy the environment to the "default" namespace
kubectl kustomize deploy/environments/dev/kind | sed "s/REPLACE_NAMESPACE/${PROJECT_NAMESPACE}/gI" \
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

* The vllm simulator is running and exposed via InferencePool
* The Gateway is exposing the InferencePool via HTTPRoute
* The Endpoint Picker is loaded into the Gateway via ext_proc

You can watch the Endpoint Picker logs with:

  $ kubectl --context ${KUBE_CONTEXT} logs -f deployments/endpoint-picker

With that running in the background, you can make requests:

  $ curl -s -w '\n' http://localhost:${GATEWAY_HOST_PORT}/v1/completions -H 'Content-Type: application/json' -d '{"model":"food-review","prompt":"hi","max_tokens":10,"temperature":0}' | jq

-----------------------------------------
EOF
