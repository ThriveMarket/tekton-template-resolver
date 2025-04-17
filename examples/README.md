# Template Resolver Examples

This directory contains example templates and usage patterns for the Template Resolver.

## Directory Structure

- `templates/` - Contains sample Go templates for pipelines
  - `standard-deploy.yaml` - A standard deployment template with customizable post-deploy steps

## Example Usage

- `usage-example.yaml` - Demonstrates how to reference a template from a public repository in a PipelineRun.
- `usage-example-private-repo.yaml` - Shows how to use templates from private Git repositories using SSH URLs.

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