# Tekton Template Resolver

[![CI/CD Pipeline](https://github.com/thrivemarket/template-resolver/actions/workflows/ci.yml/badge.svg)](https://github.com/thrivemarket/template-resolver/actions/workflows/ci.yml)

## Overview

The Template Resolver extends Tekton's capabilities by enabling teams to use a centralized pipeline template while customizing specific sections with their own tasks. The resolver fetches templates from Git repositories and dynamically renders them with user-provided parameters, providing a flexible and maintainable approach to pipeline definition.

This resolver addresses [a limitation in Tekton](https://github.com/tektoncd/pipeline/issues/8711) by implementing a custom resolution mechanism that allows pipeline templates to be parameterized with complex structures like tasks.

## How It Works

1. **Template Source**: The resolver fetches pipeline templates from Git repositories (public GitHub, GitHub Gists, or private Git repos)
2. **Dynamic Parameters**: Any parameter can contain Tekton tasks, which are automatically detected and processed
3. **Go Templating**: Templates use Go's standard templating syntax for customization
4. **Task Dependencies**: Task names are automatically extracted and made available to templates for defining dependencies

## Usage

To use the Template Resolver in your Tekton pipeline, create a ResolutionRequest with the following parameters:

### Required Parameters

- `repository`: URL of the Git repository containing the template (GitHub, GitHub Gist, or any Git repo URL)
- `path`: Path to the template file within the repository

### Dynamic Parameters

In addition to the required parameters, you can include any number of custom parameters. The resolver has the following special handling for parameters:

1. **Task Parameters**: Parameters containing Tekton tasks receive special handling:
   - Task YAML is automatically injected into the pipeline with correct indentation and structure
   - Task names are extracted and made available as `<CamelCaseParamName>Names` (useful for defining dependencies)
   - The last task name is available as `<CamelCaseParamName>Name` (convenient for creating linear sequences where subsequent tasks depend on the final custom task)

2. **Regular Parameters**: Other parameters are passed through directly to the template

> **Parameter Formats**: The resolver can detect and process tasks in both array parameters and string parameters. However, using array parameters is recommended as it provides better structure and validation.

### Example ResolutionRequest

```yaml
apiVersion: resolution.tekton.dev/v1beta1
kind: ResolutionRequest
metadata:
  name: my-app-deployment
  labels:
    resolution.tekton.dev/type: template
spec:
  params:
    # Required parameters
    - name: repository
      value: https://github.com/example/pipeline-templates
    - name: path
      value: templates/standard-deploy.yaml
      
    # Custom parameters with tasks (recommended array format)
    - name: validation-steps
      value:
        - name: run-integration-tests
          taskRef:
            name: integration-test
          params:
            - name: test-suite
              value: smoke
    
    # Custom parameter with tasks (string format also works)
    - name: security-steps
      value: |
        - name: run-security-scan
          taskSpec:
            steps:
            - name: scan
              image: security-scanner:latest
              script: |
                echo "Running security scan..."
                
    # Regular array parameter
    - name: allowed-environments
      value:
        - "dev"
        - "staging"
        - "production"
        
    # Regular string parameter
    - name: app-name
      value: "my-service"
```

### Using Parameters in Templates

When creating templates, you can access any parameter by its camel-cased name. For parameters containing tasks, you also have access to the task names:

```yaml
# Example: Using a task parameter with type checking
{{- if .ValidationSteps }}
{{- if typeIs "string" .ValidationSteps }}
{{- $steps := fromYAML .ValidationSteps }}
{{- range $i, $step := $steps }}
- {{ toYAML $step }}
{{- end }}
{{- else }}
{{- range $i, $step := .ValidationSteps }}
- {{ toYAML $step }}
{{- end }}
{{- end }}
{{- end }}

# Example: Using task names from parameters
- name: next-task
  runAfter:
  {{- if .ValidationStepsNames }}
  {{- range .ValidationStepsNames }}
  - {{ . }}
  {{- end }}
  {{- else }}
  - default-task
  {{- end }}

# Example: Using a regular array parameter
allowed-environments:
{{- range .AllowedEnvironments }}
- {{ . }}
{{- end }}

# Example: Using a regular string parameter
app-name: {{ .AppName }}
```

## Installation

### Basic Installation

Deploy the Template Resolver to your Kubernetes cluster:

```bash
ko apply -f config/
```

### Configuration Options

The Template Resolver can be configured using environment variables in the deployment:

| Variable | Description | Default |
|----------|-------------|---------|
| `DEBUG` | Enable verbose debug logging | `false` |
| `HTTP_TIMEOUT` | HTTP request timeout for template fetching | `30s` |
| `RESOLUTION_TIMEOUT` | Overall timeout for template resolution | `60s` |
| `GIT_CLONE_DEPTH` | Depth for Git clone operations | `1` |
| `GIT_DEFAULT_BRANCH` | Default branch to use for GitHub templates | `main` |
| `GIT_SSH_COMMAND` | SSH command for Git operations (for private repos) | Set in deployment |

To customize these settings, edit the environment variables in `config/deployment.yaml` before deploying.

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
- **Task** (optional): A task runner for easier development workflows (`go install github.com/go-task/task/v3/cmd/task@latest`)

For local development with Kind, set the following environment variable:

```bash
export KO_DOCKER_REPO=kind.local
```

This ensures that container images built with ko are pushed to your local Kind registry.

### Project Structure

The codebase is organized into the following components:

- **cmd/template-resolver/** - Main application code
  - **config.go** - Configuration and environment variables
  - **fetcher.go** - Template fetching logic (Git/GitHub/Gist)
  - **main.go** - Application entry point
  - **resolver.go** - Core resolver implementation
  - **server.go** - HTTP server implementation
  - **template.go** - Template rendering and YAML utilities
  - **types.go** - Resource type definitions
  - **utils.go** - Helper functions

### Using Taskfile for Development

This repository includes a Taskfile that simplifies common development operations. To see all available tasks, run:

```bash
task
```

Common tasks include:

```bash
# Build the binary
task build

# Run tests
task test

# Run tests with coverage report
task test:coverage

# Lint the code
task lint

# Build and deploy to Kubernetes
task container:deploy

# Set up a local Kind cluster with Tekton
task setup:kind

# Run an example pipeline
task run:example

# Run end-to-end tests
task e2e:test
```

### Testing with Coverage

To generate and view test coverage:

```bash
task test:coverage
```

This will create an interactive HTML report highlighting covered and uncovered code in different colors, making it easy to identify areas that need additional tests.

For more detailed information about available commands, examine the Taskfile.yml in the repository root.

## Features

- **Universal Parameter Handling**: Any parameter can contain Tekton tasks, which are automatically processed
- **Multiple Repository Types**: Support for GitHub repositories, GitHub Gists, and any Git repository URL
- **Consistent Naming Convention**: All parameters are converted to camelCase for template use
- **Task Name Extraction**: Task names are automatically extracted for use in dependencies
- **Flexible Parameter Formats**: Works with both array parameters and string parameters containing YAML
- **Type-Safe Templates**: Use `typeIs` checks in templates to handle both string and structured parameters
- **YAML Object Rendering**: Use `toYAML` function to render structured objects in templates

## Roadmap

- Template caching for improved performance
- Additional template sources (S3, OCI)
- Enhanced validation capabilities
- Support for different Git branches and tags
- Parameterized templates with default values
- Improved error reporting for template rendering issues

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create a feature branch
3. Make your changes and add tests
4. Ensure all tests pass with `task test`
5. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.