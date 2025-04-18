#!/bin/bash
# Script to update the GitHub Gist with the latest template content
# Usage: ./scripts/update-gist.sh

set -e  # Exit on any error

# Configuration
GIST_ID="dfddf710d7884f997f0b648a07d7619c"
TEMPLATE_FILE="examples/templates/simple.yaml"
GIST_DESCRIPTION="Simple Tekton Pipeline Template with echo tasks for testing"

# Ensure we're in the project root directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
cd "$SCRIPT_DIR/.."

# Check if template exists
if [ ! -f "$TEMPLATE_FILE" ]; then
  echo "Error: Template file $TEMPLATE_FILE not found"
  exit 1
fi

# Check if gh CLI is installed
if ! command -v gh &> /dev/null; then
  echo "Error: GitHub CLI (gh) is not installed or not in PATH"
  echo "Install from: https://cli.github.com/"
  exit 1
fi

# Check if authenticated with GitHub
if ! gh auth status &> /dev/null; then
  echo "Error: Not authenticated with GitHub. Run 'gh auth login' first."
  exit 1
fi

# Update the gist
echo "Updating Gist $GIST_ID with content from $TEMPLATE_FILE..."
gh gist edit "$GIST_ID" -f simple.yaml "$TEMPLATE_FILE"

echo "Gist updated successfully!"
echo "URL: https://gist.github.com/justinabrahms/$GIST_ID"
