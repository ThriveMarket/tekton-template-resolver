#!/bin/bash
# Script to build and test the template resolver locally
# Usage: ./scripts/test-locally.sh

set -e  # Exit on any error

# Ensure we're in the project root directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
cd "$SCRIPT_DIR/.."

echo "Building the template resolver..."
IMAGE_URL=$(ko build thrivemarket.com/template-resolver/cmd/template-resolver)
echo "Built image: $IMAGE_URL"

echo "Updating deployment.yaml with the new image..."
# OS-compatible sed command
if [[ "$OSTYPE" == "darwin"* ]]; then
  # macOS
  sed -i '' "s|image:.*|image: $IMAGE_URL|g" config/deployment.yaml
else
  # Linux
  sed -i "s|image:.*|image: $IMAGE_URL|g" config/deployment.yaml
fi

echo "Redeploying the resolver..."
kubectl delete deployment template-resolver -n tekton-pipelines-resolvers || true
kubectl apply -f config/deployment.yaml

echo "Waiting for resolver to start..."
sleep 5

echo "Applying test request..."
kubectl delete resolutionrequest test-request || true
kubectl apply -f test-request.yaml

echo "Waiting for resolution..."
sleep 2

echo "Resolution result:"
kubectl get resolutionrequest test-request -o yaml

echo "Script completed successfully!"