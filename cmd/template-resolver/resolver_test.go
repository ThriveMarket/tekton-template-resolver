package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

// mockFetcher is an implementation of TemplateFetcher for testing
type mockFetcher struct {
	templates map[string]string
}

// FetchTemplate implements the TemplateFetcher interface for testing
func (m *mockFetcher) FetchTemplate(repo, path string) (string, error) {
	key := repo + ":" + path
	if template, ok := m.templates[key]; ok {
		return template, nil
	}
	return "apiVersion: tekton.dev/v1\nkind: Pipeline\nmetadata:\n  name: default-pipeline\nspec:\n  params:\n  - name: param1\n    type: string\n", nil
}

// TestResolverBasicParams tests the resolver with basic parameters
func TestResolverBasicParams(t *testing.T) {
	// Create a mock fetcher
	mockData := &mockFetcher{
		templates: map[string]string{
			"repo1:path1": `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: test-pipeline
spec:
  params:
    - name: app-name
      type: string
  tasks:
    - name: task1
      taskRef:
        name: some-task
`,
		},
	}

	// Create resolver with mock fetcher
	r := &resolver{
		fetcher: mockData,
	}

	// Test with basic parameters
	params := []pipelinev1.Param{
		{
			Name: "repository",
			Value: pipelinev1.ParamValue{
				Type:      "string",
				StringVal: "repo1",
			},
		},
		{
			Name: "path",
			Value: pipelinev1.ParamValue{
				Type:      "string",
				StringVal: "path1",
			},
		},
		{
			Name: "simple-param",
			Value: pipelinev1.ParamValue{
				Type:      "string",
				StringVal: "value1",
			},
		},
	}

	// Execute the Resolve function
	result, err := r.Resolve(context.Background(), params)

	// Verify results
	require.NoError(t, err)
	require.NotNil(t, result)

	// Check that the template was rendered
	renderedData := string(result.Data())
	assert.Contains(t, renderedData, "name: test-pipeline")
}

// TestResolverArrayParameter tests the resolver with a regular array parameter (not tasks)
func TestResolverArrayParameter(t *testing.T) {
	// Create a mock fetcher with a template that uses a regular array parameter
	mockData := &mockFetcher{
		templates: map[string]string{
			"repo1:path1": `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: array-param-pipeline
spec:
  params:
    - name: app-name
      type: string
  tasks:
    - name: task1
      taskRef:
        name: some-task
      params:
        - name: environments
          value: |
            {{- range .AllowedEnvironments }}
            - {{ . }}
            {{- end }}
`,
		},
	}

	// Create resolver with mock fetcher
	r := &resolver{
		fetcher: mockData,
	}

	// Test with a regular array parameter
	params := []pipelinev1.Param{
		{
			Name: "repository",
			Value: pipelinev1.ParamValue{
				Type:      "string",
				StringVal: "repo1",
			},
		},
		{
			Name: "path",
			Value: pipelinev1.ParamValue{
				Type:      "string",
				StringVal: "path1",
			},
		},
		{
			Name: "allowed-environments",
			Value: pipelinev1.ParamValue{
				Type:     "array",
				ArrayVal: []string{"dev", "staging", "production"},
			},
		},
	}

	// Execute the Resolve function
	result, err := r.Resolve(context.Background(), params)

	// Verify results
	require.NoError(t, err)
	require.NotNil(t, result)

	// Check that the template was rendered with the array values
	renderedData := string(result.Data())
	assert.Contains(t, renderedData, "- dev")
	assert.Contains(t, renderedData, "- staging")
	assert.Contains(t, renderedData, "- production")
}
