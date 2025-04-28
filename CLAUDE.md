# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Prerequisites
- Go: `brew install go`
- ko: `brew install ko`
- kubectl: `brew install kubectl`
- kind (for local testing): `brew install kind`
- For kind clusters, set: `export KO_DOCKER_REPO=kind.local`
- Install Tekton in kind: `kubectl apply --filename https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml`

## Build Commands
- Build: `go build ./cmd/template-resolver`
- Run: `go run ./cmd/template-resolver`
- Test: `go test ./...`
- Test single package: `go test ./cmd/template-resolver`
- Test specific test: `go test ./path/to/package -run TestName`
- Lint: `golangci-lint run`
- Build with race detection: `go build -race ./cmd/template-resolver`
- Build and deploy to kind: `ko build thrivemarket.com/template-resolver/cmd/template-resolver`
- Local testing: `task run:pipelinerun`
- Update template gist: `task update`
- Run test pipeline: `task run:example`

## Test Coverage
To generate and view code coverage as an HTML file:

```bash
# Generate coverage profile
go test ./... -coverprofile=coverage.out

# Convert to HTML
go tool cover -html=coverage.out -o coverage.html

# Open in browser (macOS)
open coverage.html

# Alternative: directly open in browser without saving HTML file
go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out
```

## Code Style Guidelines
- Follow standard Go conventions
- Import ordering: standard library, third-party, internal packages
- Error handling: Always check and handle returned errors
- Naming: CamelCase for exported symbols, camelCase for unexported
- Prefer return error over panic
- Use descriptive variable names
- Keep functions small and focused
- Add comments for exported functions
- Use context.Context for request-scoped operations
- Align with the Tekton pipelines resolver pattern
- Test files should end with _test.go
- Always run the linter before committing go code

## Template Formatting Guidelines
- Be careful with YAML indentation in templates
- The Go template rendering doesn't preserve proper YAML indentation
- Tasks and their properties must be properly indented in the template
- Use direct pipeline examples for reference
- Test rendered templates with `kubectl apply --dry-run=client -f <file>` before using
- Custom steps must have proper indentation for runAfter and taskSpec properties
- For complex templates, consider using a tool like yq to validate the final YAML