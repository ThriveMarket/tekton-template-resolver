#!/bin/bash
# Script to run the test pipeline, cleaning up any previous run
# Usage: ./scripts/run-pipeline.sh

set -e  # Exit on any error

# Ensure we're in the project root directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
cd "$SCRIPT_DIR/.."

# Configuration
PIPELINE_RUN_NAME="simple-pipeline-run"
PIPELINE_FILE="examples/execute-pipeline.yaml"

# Make the script executable
chmod +x "$0"

# Check if kubectl is installed
if ! command -v kubectl &> /dev/null; then
  echo "Error: kubectl is not installed or not in PATH"
  exit 1
fi

# Check if the pipeline file exists
if [ ! -f "$PIPELINE_FILE" ]; then
  echo "Error: Pipeline file $PIPELINE_FILE not found"
  exit 1
fi

# Delete the previous pipeline run if it exists
echo "Cleaning up previous pipeline run..."
kubectl delete pipelinerun "$PIPELINE_RUN_NAME" --ignore-not-found

# Wait a moment to ensure the deletion is complete
sleep 2

# Apply the new pipeline run
echo "Creating new pipeline run from $PIPELINE_FILE..."
kubectl apply -f "$PIPELINE_FILE"

# Check the initial state
echo "Initial pipeline run state:"
kubectl get pipelinerun "$PIPELINE_RUN_NAME" -o custom-columns=NAME:.metadata.name,SUCCEEDED:.status.conditions[0].status,REASON:.status.conditions[0].reason,STARTTIME:.status.startTime,COMPLETIONTIME:.status.completionTime

# Wait for the pipeline to complete or for a certain amount of time
TIMEOUT=60  # seconds
ELAPSED=0
INTERVAL=5  # seconds

echo "Waiting for pipeline run to complete (timeout: ${TIMEOUT}s)..."
while [ $ELAPSED -lt $TIMEOUT ]; do
  # Check if the pipeline has completed
  SUCCEEDED=$(kubectl get pipelinerun "$PIPELINE_RUN_NAME" -o jsonpath='{.status.conditions[0].status}' 2>/dev/null || echo "Unknown")
  REASON=$(kubectl get pipelinerun "$PIPELINE_RUN_NAME" -o jsonpath='{.status.conditions[0].reason}' 2>/dev/null || echo "Unknown")
  
  if [ "$SUCCEEDED" = "True" ]; then
    echo "Pipeline run completed successfully!"
    break
  elif [ "$SUCCEEDED" = "False" ]; then
    echo "Pipeline run failed with reason: $REASON"
    break
  fi
  
  echo "Still running... (${ELAPSED}s)"
  sleep $INTERVAL
  ELAPSED=$((ELAPSED + INTERVAL))
done

# Show the final state
echo "Final pipeline run state:"
kubectl get pipelinerun "$PIPELINE_RUN_NAME" -o custom-columns=NAME:.metadata.name,SUCCEEDED:.status.conditions[0].status,REASON:.status.conditions[0].reason,STARTTIME:.status.startTime,COMPLETIONTIME:.status.completionTime

# If there's a failure, show more details
if [ "$SUCCEEDED" = "False" ]; then
  echo -e "\nPipeline run details:"
  kubectl get pipelinerun "$PIPELINE_RUN_NAME" -o yaml | grep -A 10 "message:"
fi

echo "Done."