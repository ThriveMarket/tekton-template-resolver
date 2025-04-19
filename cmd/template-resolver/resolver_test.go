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

// TestResolverDynamicTaskParameters tests the resolver with custom task parameters
func TestResolverDynamicTaskParameters(t *testing.T) {
	// Create a mock fetcher with a template that uses dynamic parameters
	mockData := &mockFetcher{
		templates: map[string]string{
			"repo1:path1": `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: dynamic-param-pipeline
spec:
  params:
    - name: app-name
      type: string
  tasks:
    # Base task
    - name: base-task
      taskRef:
        name: some-task
    
    # Custom validation steps if provided
    {{- if .CustomValidationSteps }}
    {{ .CustomValidationSteps }}
    {{- end }}
    
    # Next task with dependencies on custom validation if provided
    - name: next-task
      runAfter:
      {{- if .CustomValidationStepsNames }}
      {{- range .CustomValidationStepsNames }}
      - {{ . }}
      {{- end }}
      {{- else }}
      - base-task
      {{- end }}
      taskRef:
        name: next-task-ref
`,
		},
	}
	
	// Create resolver with mock fetcher
	r := &resolver{
		fetcher: mockData,
	}
	
	// Test with custom validation steps parameter
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
			Name: "custom-validation-steps",
			Value: pipelinev1.ParamValue{
				Type: "array",
				ArrayVal: []string{
					`name: validation-step-1
taskRef:
  name: validator-1
params:
  - name: param1
    value: value1`,
					`name: validation-step-2
taskRef:
  name: validator-2
params:
  - name: param2
    value: value2`,
				},
			},
		},
	}
	
	// Execute the Resolve function
	result, err := r.Resolve(context.Background(), params)
	
	// Verify results
	require.NoError(t, err)
	require.NotNil(t, result)
	
	// Check that the template was rendered with our custom steps
	renderedData := string(result.Data())
	assert.Contains(t, renderedData, "name: validation-step-1")
	assert.Contains(t, renderedData, "name: validation-step-2")
	assert.Contains(t, renderedData, "- validation-step-1")
	assert.Contains(t, renderedData, "- validation-step-2")
}

// TestResolverParameterHandling tests the resolver with various parameter types
func TestResolverParameterHandling(t *testing.T) {
	// Create a mock fetcher with a template that uses various parameter types
	mockData := &mockFetcher{
		templates: map[string]string{
			"repo1:path1": `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: param-handling-pipeline
spec:
  params:
    - name: app-name
      type: string
  tasks:
    # Base task
    - name: base-task
      taskRef:
        name: some-task
    
    # Custom steps via array parameter
    {{- if .CustomSteps }}
    {{ .CustomSteps }}
    {{- end }}
    
    # Second task with dependencies on custom steps
    - name: second-task
      runAfter:
      {{- if .CustomStepsNames }}
      {{- range .CustomStepsNames }}
      - {{ . }}
      {{- end }}
      {{- else }}
      - base-task
      {{- end }}
      taskRef:
        name: second-task-ref
        
    # Post-dev steps via string parameter (legacy format)
    {{- if .PostDevSteps }}
    {{ .PostDevSteps }}
    {{- end }}
`,
		},
	}
	
	// Create resolver with mock fetcher
	r := &resolver{
		fetcher: mockData,
	}
	
	// Test with both array and string parameters containing tasks
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
		// Array parameter with tasks
		{
			Name: "custom-steps",
			Value: pipelinev1.ParamValue{
				Type: "array",
				ArrayVal: []string{
					`name: custom-validation
taskRef:
  name: validator
params:
  - name: target
    value: custom`,
				},
			},
		},
		// String parameter with tasks (legacy format)
		{
			Name: "post-dev-steps",
			Value: pipelinev1.ParamValue{
				Type: "string",
				StringVal: `- name: dev-validation
  taskRef:
    name: validator
  params:
    - name: target
      value: dev`,
			},
		},
		// Regular string parameter
		{
			Name: "simple-param",
			Value: pipelinev1.ParamValue{
				Type:      "string",
				StringVal: "simple-value",
			},
		},
		// Regular array parameter
		{
			Name: "environments",
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
	
	// Check that the template was rendered with both task types
	renderedData := string(result.Data())
	assert.Contains(t, renderedData, "name: custom-validation")
	assert.Contains(t, renderedData, "name: dev-validation")
	assert.Contains(t, renderedData, "- custom-validation")
}

// TestResolverMultipleTaskParameters tests the resolver with multiple custom task parameters
func TestResolverMultipleTaskParameters(t *testing.T) {
	// Create a mock fetcher with a template that uses multiple dynamic parameters
	mockData := &mockFetcher{
		templates: map[string]string{
			"repo1:path1": `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: multi-param-pipeline
spec:
  params:
    - name: app-name
      type: string
  tasks:
    # Base task
    - name: base-task
      taskRef:
        name: some-task
    
    # Security audit if provided
    {{- if .SecurityAuditSteps }}
    {{ .SecurityAuditSteps }}
    {{- end }}
    
    # Compliance checks if provided
    {{- if .ComplianceCheckSteps }}
    {{ .ComplianceCheckSteps }}
    {{- end }}
    
    # Final task with dependencies on all previous tasks
    - name: final-task
      runAfter:
      - base-task
      {{- if .SecurityAuditStepsNames }}
      {{- range .SecurityAuditStepsNames }}
      - {{ . }}
      {{- end }}
      {{- end }}
      {{- if .ComplianceCheckStepsNames }}
      {{- range .ComplianceCheckStepsNames }}
      - {{ . }}
      {{- end }}
      {{- end }}
      taskRef:
        name: final-task-ref
`,
		},
	}
	
	// Create resolver with mock fetcher
	r := &resolver{
		fetcher: mockData,
	}
	
	// Test with multiple custom task parameters
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
			Name: "security-audit-steps",
			Value: pipelinev1.ParamValue{
				Type: "array",
				ArrayVal: []string{
					`name: security-scan
taskRef:
  name: security-scanner
params:
  - name: scan-type
    value: vulnerability`,
				},
			},
		},
		{
			Name: "compliance-check-steps",
			Value: pipelinev1.ParamValue{
				Type: "array",
				ArrayVal: []string{
					`name: compliance-check
taskRef:
  name: compliance-tool
params:
  - name: policy
    value: pci-dss`,
				},
			},
		},
	}
	
	// Execute the Resolve function
	result, err := r.Resolve(context.Background(), params)
	
	// Verify results
	require.NoError(t, err)
	require.NotNil(t, result)
	
	// Check that the template was rendered with both custom step types
	renderedData := string(result.Data())
	assert.Contains(t, renderedData, "name: security-scan")
	assert.Contains(t, renderedData, "name: compliance-check")
	assert.Contains(t, renderedData, "- security-scan")
	assert.Contains(t, renderedData, "- compliance-check")
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