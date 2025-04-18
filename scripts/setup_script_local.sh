#!/bin/bash
set -euo pipefail

# ----------------------------------------
# Variables
# ----------------------------------------
CLUSTER_NAME="inference-router"
KIND_CONFIG="kind-config.yaml"
#VLLM_IMAGE="public.ecr.aws/q9t5s3a7/vllm-cpu-release-repo:v0.8.0"
#KGATEWAY_IMAGE="cr.kgateway.dev/kgateway-dev/envoy-wrapper:v2.0.0"
METALLB_VERSION="v0.14.9"
INFERENCE_VERSION="v0.3.0"
KGTW_VERSION="v2.0.0"
SRC_DIR="$(cd $(dirname "${BASH_SOURCE[0]}") && pwd)"

# ----------------------------------------
# Step 1: Create Kind Cluster
# ----------------------------------------
echo "🛠️  Creating Kind cluster..."
kind delete cluster --name "$CLUSTER_NAME"
kind create cluster --name "$CLUSTER_NAME" --config "$KIND_CONFIG"

echo "📦  Loading vLLM SIMULATOR image..."
tput bold
echo "Build vLLM-sim image and load to kind cluster:"
tput sgr0
echo ""
cd $SRC_DIR/../vllm-sim
make build-vllm-sim-image
kind load docker-image vllm-sim/vllm-sim:0.0.2 --name "$CLUSTER_NAME"

# ----------------------------------------
# Step 2: Install MetalLB
# ----------------------------------------
echo "🌐  Installing MetalLB..."
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/${METALLB_VERSION}/config/manifests/metallb-native.yaml
echo "⏳  Waiting for MetalLB pods to be ready..."
kubectl wait --namespace metallb-system \
  --for=condition=Ready pod \
  --selector=component=controller \
  --timeout=120s

kubectl wait --namespace metallb-system \
  --for=condition=Ready pod \
  --selector=component=speaker \
  --timeout=120s

echo "⚙️  Applying MetalLB config..."
kubectl apply -f metalb-config.yaml

# ----------------------------------------
# Step 3: vLLM
# ----------------------------------------
tput bold
echo "deploy vllm-sim model servers:"
tput sgr0
echo ""
#kubectl apply -f $SRC_DIR/manifests/vllm-sim.yaml
kubectl apply -f $SRC_DIR/vllm-sim.yaml



# ----------------------------------------
# Step 4: Deploy Inference API Components
# ----------------------------------------
# TODO - use our yamls
echo "📡  Installing Inference API..."
kubectl apply -f "https://github.com/kubernetes-sigs/gateway-api-inference-extension/releases/download/${INFERENCE_VERSION}/manifests.yaml"

#kubectl apply -f https://github.com/kubernetes-sigs/gateway-api-inference-extension/raw/main/config/manifests/inferencemodel.yaml
#kubectl apply -f https://github.com/kubernetes-sigs/gateway-api-inference-extension/raw/main/config/manifests/inferencepool-resources.yaml

kubectl apply -f $SRC_DIR/inferencemodel-local.yaml

# build and load extention image
cd $SRC_DIR/../gateway-api-inference-extension_maya
IMAGE_REGISTRY="gateway-api-inference-extension"  GIT_TAG="demo" make image-load
kind load docker-image gateway-api-inference-extension/epp:demo --name "$CLUSTER_NAME"
kubectl delete -f $SRC_DIR/inferencepool-resources-local.yaml
kubectl apply -f $SRC_DIR/inferencepool-resources-local.yaml

# ----------------------------------------
# Step 5: Install Kgateway
# ----------------------------------------
echo "🚪  Installing Kgateway..."
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml
helm upgrade -i --create-namespace --namespace kgateway-system --version "$KGTW_VERSION" kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds
helm upgrade -i --namespace kgateway-system --version "$KGTW_VERSION" kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway --set inferenceExtension.enabled=true

# ----------------------------------------
# Step 6: Apply Gateway and Routes
# ----------------------------------------
echo "📨  Applying Gateway and HTTPRoute..."
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api-inference-extension/raw/main/config/manifests/gateway/kgateway/gateway.yaml
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api-inference-extension/raw/main/config/manifests/gateway/kgateway/httproute.yaml

echo "📨  Wait Gatewayto be ready..."
# sleep 30  # Give time for pod to create
# kubectl wait --for=condition=Ready pod --selector=app.kubernetes.io/instance=inference-gateway --timeout=240s
# Wait up to 2 minutes for the Gateway to get an IP
for i in {1..24}; do
  IP=$(kubectl get gateway inference-gateway -o jsonpath='{.status.addresses[0].value}' 2>/dev/null || echo "")
  if [[ -n "$IP" ]]; then
    echo "✅  Gateway IP assigned: $IP"
    break
  fi
  echo "⏳  Still waiting for Gateway IP..."
  sleep 5
done

if [[ -z "$IP" ]]; then
  echo "❌  Timed out waiting for Gateway IP."
  exit 1
fi

# ----------------------------------------
# Step 7: Run Inference Request
# ----------------------------------------
echo "🔍  Fetching Gateway IP..."
sleep 5  # Give time for IP allocation
IP=$(kubectl get gateway/inference-gateway -o jsonpath='{.status.addresses[0].value}')
PORT=80

echo "📨  Sending test inference request to $IP:$PORT..."
curl -i "${IP}:${PORT}/v1/completions" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "Qwen/Qwen2.5-1.5B-Instruct",
    "prompt": "hi",
    "max_tokens": 10,
    "temperature": 0
  }'
  
  
curl -si -X GET "${IP}:${PORT}/v1/models"  -H 'Content-Type: application/json'

curl -i -X GET "172.18.255.1:80/v1/models"  -H 'Content-Type: application/json'
  
curl -i "172.18.255.1:80/v1/completions" -H 'Content-Type: application/json' -d '{ "model": "food-review", "prompt": "hi", "max_tokens": 10, "temperature": 0  }'

curl -i "localhost:8888/v1/completions" -H 'Content-Type: application/json' -d '{ "model": "food-review", "prompt": "hi", "max_tokens": 10, "temperature": 0  }'

curl -i "172.18.255.1:80/v1/completions" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "food-review",
    "prompt": "hi",
    "max_tokens": 10,
    "temperature": 0
  }'