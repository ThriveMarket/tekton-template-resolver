# Tekton Template Resolver

## Overview

The Template Resolver extends Tekton's capabilities by enabling teams to use a centralized pipeline while customizing post-deployment steps for their specific applications. We maintain a standardized CI/CD workflow with consistent Slack notifications and service updates, while empowering teams with the flexibility to define their own validation and post-deployment processes.

This resolver addresses [a limitation in Tekton](https://github.com/tektoncd/pipeline/issues/8711) by implementing a custom resolution mechanism that fetches templates from Git repositories and renders them with user-provided parameters.

## How It Works

1. **Template Source**: The resolver fetches pipeline templates from public Git repositories
2. **Go Templating**: Templates use Go's standard templating syntax for customization
3. **Post-Deploy Steps**: Teams define custom steps for dev and prod deployment validation
4. **Order Dependencies**: Parallel execution is managed through step dependencies

## Usage

To use the Template Resolver in your Tekton pipeline, create a ResolutionRequest with the following parameters:

> **Note**: The resolver supports arbitrary parameter names and will make all parameters available to the template. For parameters containing Tekton tasks, task names are automatically extracted and made available as `ParameterNamesNames` (e.g., `SecurityAuditStepsNames` for a parameter named `security-audit-steps`).

> **Parameter Types**: Array parameters containing YAML tasks should use Tekton's array parameter type for structured data validation. This provides better error reporting and eliminates parsing issues with malformed YAML strings.

```yaml
apiVersion: resolution.tekton.dev/v1beta1
kind: ResolutionRequest
metadata:
  name: my-app-deployment
  labels:
    resolution.tekton.dev/type: template
spec:
  params:
    - name: repository
      value: https://github.com/example/pipeline-templates
    - name: path
      value: templates/standard-deploy.yaml
    - name: post-dev-steps
      value:
        - name: run-integration-tests
          taskRef:
            name: integration-test
          params:
            - name: test-suite
              value: smoke
    - name: post-prod-steps
      value:
        - name: verify-deployment
          taskRef:
            name: deployment-verification
          params:
            - name: timeout
              value: "300"
    # Custom parameter with tasks
    - name: security-audit-steps
      value:
        - name: run-security-scan
          taskSpec:
            steps:
            - name: scan
              image: security-scanner:latest
              script: |
                echo "Running security scan..."
    # Regular array parameter (not tasks)
    - name: allowed-environments
      value:
        - "dev"
        - "staging"
        - "production"
```

### Using Custom Parameters in Templates

When creating templates for use with this resolver, you can access any parameter by its camel-cased name:

```yaml
# Example: Using a custom "security-audit-steps" parameter
{{- if .SecurityAuditSteps }}
# Include the security audit tasks
{{.SecurityAuditSteps}}
{{- end }}

# Example: Using task names from custom parameters
- name: next-task
  runAfter:
  {{- if .SecurityAuditStepsNames }}
  {{- range .SecurityAuditStepsNames }}
  - {{.}}
  {{- end }}
  {{- else }}
  - default-task
  {{- end }}

# Example: Using a regular array parameter
allowed-environments:
{{- range .AllowedEnvironments }}
- {{.}}
{{- end }}
```

## Installation

### Basic Installation

Deploy the Template Resolver to your Kubernetes cluster:

```bash
ko apply -f config/
```

### Private Git Repository Access

To use templates from private Git repositories, you need to create an SSH deploy key:

1. Generate an SSH key pair for repository access:
   ```bash
   ssh-keygen -t ed25519 -f deploy_key -N ""
   ```

2. Add the public key (`deploy_key.pub`) as a deploy key in your Git repository settings with read-only access.

3. Create a Kubernetes secret with the private key:
   ```bash
   kubectl create secret generic git-ssh-key \
     --namespace tekton-pipelines-resolvers \
     --from-file=ssh-privatekey=deploy_key
   ```

4. The deployment automatically mounts this secret and configures Git to use it when cloning repositories.

## Development

### Prerequisites

Before you begin, ensure you have the following tools installed:

- **Go**: Required to build the resolver (`brew install go`)
- **ko**: Used to build and deploy container images (`brew install ko`)
- **kubectl**: Required for interacting with Kubernetes clusters (`brew install kubectl`)
- **Kind** (optional but recommended): For local testing with a Kubernetes cluster (`brew install kind`)

For local development with Kind, set the following environment variable:

```bash
export KO_DOCKER_REPO=kind.local
```

This ensures that container images built with ko are pushed to your local Kind registry.

### Installing Tekton in your Kind cluster

After creating your Kind cluster, you need to install Tekton Pipelines:

```bash
kubectl apply --filename https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml
```

Wait for Tekton to be ready:

```bash
kubectl wait --for=condition=ready pod -l app=tekton-pipelines-controller -n tekton-pipelines
```

### Build and Test Manually

```bash
# Build the resolver
go build ./cmd/template-resolver

# Run tests
go test ./...

# Build and deploy to local kind cluster
ko build thrivemarket.com/template-resolver/cmd/template-resolver
# Update deployment.yaml with the resulting image URL
kubectl apply -f config/deployment.yaml
```

### Testing with Coverage

To generate and view test coverage:

```bash
# Generate coverage profile
go test ./... -coverprofile=coverage.out

# Convert to HTML
go tool cover -html=coverage.out -o coverage.html

# Open in browser (macOS)
open coverage.html
```

This will create an interactive HTML report highlighting covered and uncovered code in different colors, making it easy to identify areas that need additional tests.

### Helper Scripts

The repository includes helper scripts to simplify common operations:

```bash
# Test the resolver locally (builds, deploys, and runs a test request)
./scripts/test-locally.sh

# Update the GitHub Gist with the latest template
./scripts/update-gist.sh

# Execute and monitor a test pipeline
./scripts/run-pipeline.sh
```

The `run-pipeline.sh` script simplifies pipeline execution by:
- Cleaning up any previous pipeline runs
- Applying the pipeline definition
- Monitoring the pipeline execution status
- Displaying detailed information in case of failures

## Roadmap

- Template caching for improved performance
- Additional template sources (S3, OCI)
- Enhanced validation capabilities
- Support for different Git branches
- Parameterized templates with default values
- Support for arbitrary template keys/context (beyond just post-deploy steps)

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.