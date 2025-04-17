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
      value: |
        - name: run-integration-tests
          taskRef:
            name: integration-test
          params:
            - name: test-suite
              value: smoke
    - name: post-prod-steps
      value: |
        - name: verify-deployment
          taskRef:
            name: deployment-verification
          params:
            - name: timeout
              value: "300"
```

## Installation

Deploy the Template Resolver to your Kubernetes cluster:

```bash
ko apply -f config/
```

## Development

Build and test locally:

```bash
# Build the resolver
go build ./cmd/template-resolver

# Run tests
go test ./...

# Build and deploy to local kind cluster
ko build thrivemarket.com/template-resolver/cmd/template-resolver
# Update deployment.yaml with the resulting image URL
```

## Roadmap

- Support for authenticated Git repositories
- Template caching for improved performance
- Additional template sources (S3, OCI)
- Enhanced validation capabilities

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.