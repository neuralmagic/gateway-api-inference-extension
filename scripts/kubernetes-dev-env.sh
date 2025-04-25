#!/bin/bash

# This shell script deploys a Kubernetes or OpenShift cluster with an
# KGateway-based Gateway API implementation fully configured. It deploys the
# vllm simulator, which it exposes with a Gateway -> HTTPRoute -> InferencePool.
# The Gateway is configured with the a filter for the ext_proc endpoint picker.

set -eux

# ------------------------------------------------------------------------------
# Variables
# ------------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export CLEAN="${CLEAN:-false}"

# Validate required inputs
if [[ -z "${NAMESPACE:-}" ]]; then
  echo "ERROR: NAMESPACE environment variable is not set."
  exit 1
fi
if [[ -z "${VLLM_MODE:-}" ]]; then
  echo "ERROR: VLLM_MODE is not set. Please export one of: vllm-sim, vllm, vllm-p2p"
  exit 1
fi

# GIE Configuration node
export POOL_NAME="${POOL_NAME:-vllm-llama3-8b-instruct}"
export REDIS_SVC_NAME="${REDIS_SVC_NAME:-lookup-server-service}"
export REDIS_HOST="${REDIS_HOST:-${REDIS_SVC_NAME}.${NAMESPACE}.svc.cluster.local}" #TODO- remove Redis to kustomize
export REDIS_PORT="${REDIS_PORT:-8100}"
export HF_TOKEN="${HF_TOKEN:-}"

# vLLM Specific Configuration node
case "${VLLM_MODE}" in
  vllm-sim)
    export VLLM_SIM_IMAGE="${VLLM_SIM_IMAGE:-quay.io/vllm-d/vllm-sim}"
    export VLLM_SIM_TAG="${VLLM_SIM_TAG:-0.0.2}"
    export EPP_IMAGE="${EPP_IMAGE:-us-central1-docker.pkg.dev/k8s-staging-images/gateway-api-inference-extension/epp}"
    export EPP_TAG="${EPP_TAG:-main}"
    export VLLM_DEPLOYMENT_NAME="${VLLM_DEPLOYMENT_NAME:-vllm-sim}"
    ;;
  vllm | vllm-p2p)
    # Shared across both full model modes - // TODO - make more env variables similar
    # TODO: Consider unifying more environment variables for consistency and reuse
    export HF_SECRET_NAME="${HF_SECRET_NAME:-hf-token}"
    export HF_TOKEN=$(echo -n "${HF_TOKEN:-}" | base64 | tr -d '\n')
    export VOLUME_MOUNT_PATH="${VOLUME_MOUNT_PATH:-/data}"
    export VLLM_REPLICA_COUNT="${VLLM_REPLICA_COUNT:-3}"


    if [[ "$VLLM_MODE" == "vllm" ]]; then
      export VLLM_IMAGE="${VLLM_IMAGE:-quay.io/vllm-d/vllm-d-dev}"
      export VLLM_TAG="${VLLM_TAG:-0.0.2}"
      export VLLM_DEPLOYMENT_NAME="${VLLM_DEPLOYMENT_NAME:-vllm-llama3-8b-instruct}"
      export EPP_IMAGE="${EPP_IMAGE:-quay.io/vllm-d/gateway-api-inference-extension-dev}"
      export EPP_TAG="${EPP_TAG:-0.0.4}"
      export MODEL_NAME="${MODEL_NAME:-meta-llama/Llama-3.1-8B-Instruct}"
      export MODEL_LABEL="${MODEL_LABEL:-llama3-8b}"
      export HF_SECRET_KEY="${HF_SECRET_KEY:-token}"
      export HF_TOKEN="${HF_TOKEN:-}"
      export VLLM_REPLICA_COUNT="${VLLM_REPLICA_COUNT:-2}"
      export MAX_MODEL_LEN="${MAX_MODEL_LEN:-8192}"
      export PVC_NAME="${PVC_NAME:-vllm-storage-claim}"

    elif [[ "$VLLM_MODE" == "vllm-p2p" ]]; then
      export VLLM_IMAGE="${VLLM_IMAGE:-lmcache/vllm-openai}"
      export VLLM_TAG="${VLLM_TAG:-2025-03-10}"
      export EPP_IMAGE="${EPP_IMAGE:- quay.io/vmaroon/gateway-api-inference-extension/epp}"
      export EPP_TAG="${EPP_TAG:-kv-aware}"
      export MODEL_NAME="${MODEL_NAME:-mistralai/Mistral-7B-Instruct-v0.2}"
      export MODEL_LABEL="${MODEL_LABEL:-mistral7b}"
      export HF_SECRET_KEY="${HF_SECRET_KEY:-${HF_SECRET_NAME}_${MODEL_LABEL}}"
      export VLLM_DEPLOYMENT_NAME="${VLLM_DEPLOYMENT_NAME:-vllm-${MODEL_LABEL}}"
      export MAX_MODEL_LEN="${MAX_MODEL_LEN:-32768}"
      export PVC_NAME="${PVC_NAME:-vllm-p2p-storage-claim}"
      export PVC_ACCESS_MODE="${PVC_ACCESS_MODE:-ReadWriteOnce}"
      export PVC_SIZE="${PVC_SIZE:-10Gi}"
      export PVC_STORAGE_CLASS="${PVC_STORAGE_CLASS:-standard}"
      export REDIS_IMAGE="${REDIS_IMAGE:-redis}"
      export REDIS_TAG="${REDIS_TAG:-7.2.3}"
      export REDIS_REPLICA_COUNT="${REDIS_REPLICA_COUNT:-1}"
      export POD_IP="POD_IP"
      export REDIS_TARGET_PORT="${REDIS_TARGET_PORT:-6379}"
      export REDIS_SERVICE_TYPE="${REDIS_SERVICE_TYPE:-ClusterIP}"
    fi
    ;;
  *)
    echo "ERROR: Unsupported VLLM_MODE: ${VLLM_MODE}. Must be one of: vllm-sim, vllm, vllm-p2p"
    exit 1
    ;;
esac

# ------------------------------------------------------------------------------
# Deployment
# ------------------------------------------------------------------------------

kubectl create namespace ${NAMESPACE} 2>/dev/null || true

# Hack to deal with KGateways broken OpenShift support
export PROXY_UID=$(kubectl get namespace ${NAMESPACE} -o json | jq -e -r '.metadata.annotations["openshift.io/sa.scc.uid-range"]' | perl -F'/' -lane 'print $F[0]+1');

set -o pipefail

if [[ "$CLEAN" == "true" ]]; then
  echo "INFO: ${CLEAN^^}ING environment in namespace ${NAMESPACE} for mode ${VLLM_MODE}"
  kustomize build deploy/environments/dev/kubernetes-kgateway | envsubst | kubectl -n "${NAMESPACE}" delete --ignore-not-found=true -f -
  kustomize build deploy/environments/dev/kubernetes-vllm/${VLLM_MODE} | envsubst | kubectl -n "${NAMESPACE}" delete --ignore-not-found=true -f -
else
  echo "INFO: Deploying vLLM Environment in namespace ${NAMESPACE}"
  oc adm policy add-scc-to-user anyuid -z default -n ${NAMESPACE}  # TODO - Change to security context
  kustomize build deploy/environments/dev/kubernetes-vllm/${VLLM_MODE} | envsubst | kubectl -n "${NAMESPACE}" apply -f -

  echo "INFO: Deploying Gateway Environment in namespace ${NAMESPACE}"
  kustomize build deploy/environments/dev/kubernetes-kgateway | envsubst | kubectl -n "${NAMESPACE}" apply -f -

  echo "INFO: Waiting for resources in namespace ${NAMESPACE} to become ready"
  kubectl -n "${NAMESPACE}" wait deployment/endpoint-picker --for=condition=Available --timeout=60s
  kubectl -n "${NAMESPACE}" wait gateway/inference-gateway --for=condition=Programmed --timeout=60s
  kubectl -n "${NAMESPACE}" wait deployment/inference-gateway --for=condition=Available --timeout=60s
  # Mode-specific wait
  case "${VLLM_MODE}" in
    vllm-sim)
      kubectl -n "${NAMESPACE}" wait deployment/vllm-sim --for=condition=Available --timeout=60s
      ;;
    vllm)
      kubectl -n "${NAMESPACE}" wait deployment/vllm-llama3-8b-instruct --for=condition=Available --timeout=180s
      ;;
    vllm-p2p)
      kubectl -n "${NAMESPACE}" wait deployment/vllm-mistral7b --for=condition=Available --timeout=180s
      kubectl -n "${NAMESPACE}" wait deployment/${REDIS_SVC_NAME} --for=condition=Available --timeout=60s
      ;;
  esac
fi


