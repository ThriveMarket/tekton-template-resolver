# Contributing to Template Resolver

Thank you for your interest in contributing to the Template Resolver project! This document provides guidelines and instructions for contributing.

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment for everyone.

## How to Contribute

### Reporting Bugs

If you find a bug, please create an issue using the bug report template and include:

- A clear description of the bug
- Steps to reproduce
- Expected behavior
- Actual behavior
- Environment details (Kubernetes version, Tekton version, etc.)
- Relevant logs or YAML files

### Suggesting Enhancements

For new features or improvements, create an issue using the feature request template and include:

- A clear description of the problem you want to solve
- Your proposed solution
- A use case that demonstrates the value of the feature

### Pull Requests

1. Fork the repository
2. Create a new branch from `main`
3. Make your changes
4. Add or update tests to verify your changes
5. Ensure all tests pass with `go test ./...`
6. Create a pull request against the `main` branch
7. Follow the pull request template

## Development Environment

### Prerequisites

- Go 1.19 or later
- ko (for building and deploying)
- kubectl
- kind (for local testing)

### Local Development

1. Clone the repository
2. Set up a kind cluster with Tekton installed
3. Use the development tasks available in `Taskfile.yml`

### Testing

- Write unit tests for all new code
- Ensure existing tests continue to pass
- Aim for high test coverage for critical components

## Coding Conventions

- Follow standard Go conventions
- Use meaningful variable and function names
- Add comments for public functions and complex logic
- Organize imports: standard library, third-party, internal
- Follow error handling patterns used in the codebase

## Documentation

- Update README.md with any relevant changes
- Document all new configuration options
- Include examples for new features

Thank you for contributing to Template Resolver!