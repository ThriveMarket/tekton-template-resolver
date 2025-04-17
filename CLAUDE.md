# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands
- Build: `go build ./cmd/template-resolver`
- Run: `go run ./cmd/template-resolver`
- Test: `go test ./...`
- Test single package: `go test ./cmd/template-resolver`
- Test specific test: `go test ./path/to/package -run TestName`
- Lint: `golangci-lint run`
- Build with race detection: `go build -race ./cmd/template-resolver`

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