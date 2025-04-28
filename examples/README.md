# Template Resolver Examples

This directory contains example templates and usage patterns for the Template Resolver.

## Directory Structure

- `templates/` - Contains sample Go templates for pipelines
  - `simple.yaml` - A simple deployment template with echo tasks for testing

## Example Usage

- `usage-example.yaml` - Demonstrates how to reference a template from a public repository in a PipelineRun.
- `usage-example-private-repo.yaml` - Shows how to use templates from private Git repositories using SSH URLs.
- `direct-pipeline.yaml` and `direct-pipeline-run.yaml` - A working example that directly applies a Pipeline and PipelineRun without using the resolver (for testing).

## Running Modes

### Tekton Mode (Kubernetes)

This is the default mode, where the template resolver runs as a Kubernetes deployment integrated with Tekton Pipelines.

### Standalone Mode

The template-resolver can also be run in standalone mode without requiring Knative or Kubernetes:

```bash
# Start the server in standalone mode
./template-resolver -standalone -debug

# In another terminal, use curl to send a resolution request:
curl -X POST http://localhost:8080/resolve \
  -H "Content-Type: application/json" \
  -d '{
    "parameters": [
      {
        "name": "repository",
        "value": {
          "type": "string",
          "stringVal": "https://github.com/thrivemarket/template-resolver"
        }
      },
      {
        "name": "path",
        "value": {
          "type": "string",
          "stringVal": "examples/templates/simple.yaml"
        }
      }
    ]
  }'
```

The standalone mode provides these endpoints:

- `/resolve` - POST endpoint for resolving templates
- `/health` - Health check endpoint
- `/ready` - Readiness endpoint

This mode is useful for development, testing, or running the resolver in environments 
where Knative/Tekton is not available.

## Template Structure

Templates should be written using Go's template syntax. The Template Resolver provides the following variables:

- `PostDevSteps` - YAML content for steps to run after dev deployment
- `PostProdSteps` - YAML content for steps to run after prod deployment

### Sample Template Snippet

```yaml
# Standard pipeline tasks
- name: deploy-to-dev
  taskRef:
    name: deploy-task
  # ...

{{- if .PostDevSteps}}
# Start of user-defined post-dev steps
{{.PostDevSteps}}
# End of user-defined post-dev steps
{{- end}}
```

## Testing Templates

You can test a template by creating a PipelineRun that references it:

```yaml
apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  name: test-pipeline-run
spec:
  pipelineRef:
    resolver: template
    params:
      - name: repository
        value: https://github.com/your-org/your-repo
      - name: path
        value: path/to/template.yaml
      - name: post-dev-steps
        value: |
          - name: run-tests
            taskRef:
              name: integration-test
  # ...
```

## Creating Your Own Templates

1. Create a new YAML file with your pipeline definition
2. Use Go template syntax to include variable sections
3. Host the template in a Git repository
4. Reference it in your PipelineRun using the resolver