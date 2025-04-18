#!/bin/bash

# ------------------------------------------------------------------------------
# This shell script loads images into a kind cluster that are needed for a
# development environment including the vllm simulator and the GIE itself.
# ------------------------------------------------------------------------------

set -eo pipefail

# ------------------------------------------------------------------------------
# Variables
# ------------------------------------------------------------------------------

# Set a default CLUSTER_NAME if not provided
: "${CLUSTER_NAME:=gie-dev}"

# Set the default IMAGE_REGISTRY if not provided
: "${IMAGE_REGISTRY:=quay.io/vllm-d}"

# Set a default VLLM_SIMULATOR_IMAGE if not provided
: "${VLLM_SIMULATOR_IMAGE:=vllm-sim}"

# Set a default VLLM_SIMULATOR_TAG if not provided
: "${VLLM_SIMULATOR_TAG:=0.0.2}"

# Set a default ENDPOINT_PICKER_IMAGE if not provided
: "${ENDPOINT_PICKER_IMAGE:=gateway-api-inference-extension/epp}"

# Set a default ENDPOINT_PICKER_TAG if not provided
: "${ENDPOINT_PICKER_TAG:=0.0.1}"

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
for cmd in kind ${CONTAINER_RUNTIME}; do
    if ! command -v "$cmd" &> /dev/null; then
        echo "Error: $cmd is not installed or not in the PATH."
        exit 1
    fi
done

# ------------------------------------------------------------------------------
# Load Container Images
# ------------------------------------------------------------------------------

# Load the vllm simulator image into the cluster
if [ "${CONTAINER_RUNTIME}" == "podman" ]; then
	podman save ${IMAGE_REGISTRY}/${VLLM_SIMULATOR_IMAGE}:${VLLM_SIMULATOR_TAG} -o /dev/stdout | kind --name ${CLUSTER_NAME} load image-archive /dev/stdin
else
	kind --name ${CLUSTER_NAME} load docker-image ${IMAGE_REGISTRY}/${VLLM_SIMULATOR_IMAGE}:${VLLM_SIMULATOR_TAG}
fi

# Load the ext_proc endpoint-picker image into the cluster
if [ "${CONTAINER_RUNTIME}" == "podman" ]; then
	podman save ${IMAGE_REGISTRY}/${ENDPOINT_PICKER_IMAGE}:${ENDPOINT_PICKER_TAG} -o /dev/stdout | kind --name ${CLUSTER_NAME} load image-archive /dev/stdin
else
	kind --name ${CLUSTER_NAME} load docker-image ${IMAGE_REGISTRY}/${ENDPOINT_PICKER_IMAGE}:${ENDPOINT_PICKER_TAG}
fi
