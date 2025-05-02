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
    {{- $validationSteps := fromYAML .CustomValidationSteps }}
    {{- range $i, $step := $validationSteps }}
    - name: {{ $step.name }}
      taskRef:
        name: {{ index $step.taskRef "name" }}
      {{- if $step.params }}
      params:
      {{- range $step.params }}
        - name: {{ .name }}
          value: {{ .value }}
      {{- end }}
      {{- end }}
    {{- end }}
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
    {{- $customSteps := fromYAML .CustomSteps }}
    {{- range $i, $step := $customSteps }}
    - name: {{ $step.name }}
      taskRef:
        name: {{ index $step.taskRef "name" }}
      {{- if $step.params }}
      params:
      {{- range $step.params }}
        - name: {{ .name }}
          value: {{ .value }}
      {{- end }}
      {{- end }}
    {{- end}}
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
    {{- $postDevSteps := fromYAML .PostDevSteps }}
    {{- range $i, $step := $postDevSteps }}
    - name: {{ $step.name }}
      taskRef:
        name: {{ index $step.taskRef "name" }}
      {{- if $step.params }}
      params:
      {{- range $step.params }}
        - name: {{ .name }}
          value: {{ .value }}
      {{- end }}
      {{- end }}
    {{- end }}
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
    {{- $securitySteps := fromYAML .SecurityAuditSteps }}
    {{- range $i, $step := $securitySteps }}
    - name: {{ $step.name }}
      taskRef:
        name: {{ index $step.taskRef "name" }}
      {{- if $step.params }}
      params:
      {{- range $step.params }}
        - name: {{ .name }}
          value: {{ .value }}
      {{- end }}
      {{- end }}
    {{- end }}
    {{- end }}
    
    # Compliance checks if provided
    {{- if .ComplianceCheckSteps }}
    {{- $complianceSteps := fromYAML .ComplianceCheckSteps }}
    {{- range $i, $step := $complianceSteps }}
    - name: {{ $step.name }}
      taskRef:
        name: {{ index $step.taskRef "name" }}
      {{- if $step.params }}
      params:
      {{- range $step.params }}
        - name: {{ .name }}
          value: {{ .value }}
      {{- end }}
      {{- end }}
    {{- end }}
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

// TestResolverArbitraryObjectParameters tests the resolver with arbitrary structured object parameters
func TestResolverArbitraryObjectParameters(t *testing.T) {
	// Create a mock fetcher with a template that uses structured objects with fromYAML
	mockData := &mockFetcher{
		templates: map[string]string{
			"repo1:path1": `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: arbitrary-object-pipeline
spec:
  params:
    - name: app-name
      type: string
  tasks:
    # Base task
    - name: base-task
      taskRef:
        name: some-task
      
    # Use of arbitrary object in top-level YAML with fromYAML
    {{- $deployConfig := fromYAML .DeploymentConfig }}
    {{- if $deployConfig }}
    - name: deploy-task
      taskRef:
        name: deployer
      params:
        - name: namespace
          value: {{ $deployConfig.namespace }}
        - name: replicas
          value: "{{ $deployConfig.replicas }}"
        {{- if $deployConfig.resources }}
        - name: cpu-limit
          value: "{{ $deployConfig.resources.limits.cpu }}"
        - name: memory-limit
          value: "{{ $deployConfig.resources.limits.memory }}"
        {{- end }}
    {{- end }}
      
    # Security scan config as structured object with fromYAML
    {{- $securityConfig := fromYAML .SecurityConfig }}
    {{- if $securityConfig }}
    - name: security-scan
      taskRef:
        name: security-scanner
      params:
        {{- range $key, $value := $securityConfig }}
        - name: {{ $key }}
          value: "{{ $value }}"
        {{- end }}
    {{- end }}
      
    # Using complex nested objects with fromYAML
    {{- $serviceMesh := fromYAML .ServiceMesh }}
    {{- if $serviceMesh }}
    - name: mesh-config
      taskRef:
        name: service-mesh-configurator
      params:
        - name: config
          value: |
            # Mesh Configuration
            {{- if $serviceMesh.istio }}
            istio:
              enabled: {{ $serviceMesh.istio.enabled }}
              {{- if $serviceMesh.istio.gateway }}
              gateway:
                name: {{ $serviceMesh.istio.gateway.name }}
                namespace: {{ $serviceMesh.istio.gateway.namespace }}
              {{- end }}
              {{- if $serviceMesh.istio.virtualServices }}
              virtualServices:
              {{- range $serviceMesh.istio.virtualServices }}
                - name: {{ .name }}
                  hosts:
                  {{- range .hosts }}
                    - {{ . }}
                  {{- end }}
              {{- end }}
              {{- end }}
            {{- end }}
    {{- end }}
`,
		},
	}
	
	// Create resolver with mock fetcher
	r := &resolver{
		fetcher: mockData,
	}
	
	// Test with complex structured object parameters
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
		// Deployment config object parameter
		{
			Name: "deployment-config",
			Value: pipelinev1.ParamValue{
				Type: "string",
				StringVal: `namespace: production
replicas: 3
resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi`,
			},
		},
		// Security config object parameter
		{
			Name: "security-config",
			Value: pipelinev1.ParamValue{
				Type: "string",
				StringVal: `scanType: vulnerability
severity: high
enableRemediation: true
notifyEmail: security@example.com`,
			},
		},
		// Complex nested service mesh object parameter
		{
			Name: "service-mesh",
			Value: pipelinev1.ParamValue{
				Type: "string",
				StringVal: `istio:
  enabled: true
  gateway:
    name: main-gateway
    namespace: istio-system
  virtualServices:
    - name: api-service
      hosts:
        - api.example.com
        - api-internal.example.com
    - name: web-service
      hosts:
        - www.example.com`,
			},
		},
	}
	
	// Execute the Resolve function
	result, err := r.Resolve(context.Background(), params)
	
	// Verify results
	require.NoError(t, err)
	require.NotNil(t, result)
	
	// Check that the template was rendered with the structured objects
	renderedData := string(result.Data())
	
	// Check deployment config values were correctly rendered
	assert.Contains(t, renderedData, "value: production")
	assert.Contains(t, renderedData, "value: \"3\"")
	assert.Contains(t, renderedData, "value: \"500m\"")
	assert.Contains(t, renderedData, "value: \"512Mi\"")
	
	// Check security config was rendered with range over map
	assert.Contains(t, renderedData, "name: scanType")
	assert.Contains(t, renderedData, "value: \"vulnerability\"")
	assert.Contains(t, renderedData, "name: severity")
	assert.Contains(t, renderedData, "value: \"high\"")
	assert.Contains(t, renderedData, "name: enableRemediation")
	assert.Contains(t, renderedData, "value: \"true\"")
	
	// Check complex nested object was rendered with proper nesting
	assert.Contains(t, renderedData, "enabled: true")
	assert.Contains(t, renderedData, "name: main-gateway")
	assert.Contains(t, renderedData, "namespace: istio-system")
	assert.Contains(t, renderedData, "- name: api-service")
	assert.Contains(t, renderedData, "- api.example.com")
	assert.Contains(t, renderedData, "- www.example.com")
}

// TestResolverYAMLListProcessing tests parsing and iterating over a list of YAML objects
func TestResolverYAMLListProcessing(t *testing.T) {
	// Create a mock fetcher with a template that processes a list of YAML objects
	mockData := &mockFetcher{
		templates: map[string]string{
			"repo1:path1": `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: yaml-list-processing-pipeline
spec:
  params:
    - name: app-name
      type: string
  tasks:
    # Base task
    - name: base-task
      taskRef:
        name: some-task
      
    # Process a list of environment configurations using fromYAML
    {{- $envConfigs := fromYAML .EnvironmentConfigs }}
    {{- if $envConfigs }}
    {{- range $i, $env := $envConfigs }}
    # Environment configuration for {{ $env.name }}
    - name: deploy-to-{{ $env.name }}
      taskRef:
        name: env-deployer
      params:
        - name: environment
          value: {{ $env.name }}
        - name: cluster
          value: {{ $env.cluster }}
        - name: namespace
          value: {{ $env.namespace }}
        {{- if $env.resources }}
        - name: cpu-limit
          value: "{{ $env.resources.limits.cpu }}"
        - name: memory-limit
          value: "{{ $env.resources.limits.memory }}"
        {{- end }}
        {{- if $env.replicas }}
        - name: replicas
          value: "{{ $env.replicas }}"
        {{- end }}
        {{- if $env.features }}
        - name: features
          value: |
            {{- range $feature, $enabled := $env.features }}
            {{ $feature }}: {{ $enabled }}
            {{- end }}
        {{- end }}
    {{- end }}
    {{- end }}
    
    # Process a list of service configurations using fromYAML
    {{- $serviceConfigs := fromYAML .ServiceConfigs }}
    {{- if $serviceConfigs }}
    # Service configurations
    - name: configure-services
      taskRef:
        name: service-configurator
      params:
        - name: services-json
          value: |
            [
            {{- range $i, $svc := $serviceConfigs }}
            {{- if $i }},{{- end }}
            {
              "name": "{{ $svc.name }}",
              "port": {{ $svc.port }},
              "targetPort": {{ $svc.targetPort }},
              "type": "{{ $svc.type }}",
              "annotations": {
              {{- range $key, $value := $svc.annotations }}
                "{{ $key }}": "{{ $value }}"{{- if not (last $svc.annotations $key) }},{{- end }}
              {{- end }}
              }
            }
            {{- end }}
            ]
    {{- end }}
`,
		},
	}
	
	// Create resolver with mock fetcher
	r := &resolver{
		fetcher: mockData,
	}
	
	// Test with lists of YAML objects
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
		// Environment configs as a YAML list
		{
			Name: "environment-configs",
			Value: pipelinev1.ParamValue{
				Type: "string",
				StringVal: `- name: development
  cluster: dev-cluster
  namespace: app-dev
  replicas: 1
  resources:
    limits:
      cpu: 250m
      memory: 256Mi
  features:
    logging: true
    monitoring: false
    tracing: false
- name: production
  cluster: prod-cluster
  namespace: app-prod
  replicas: 3
  resources:
    limits:
      cpu: 1000m
      memory: 1Gi
  features:
    logging: true
    monitoring: true
    tracing: true`,
			},
		},
		// Service configs as a YAML list
		{
			Name: "service-configs",
			Value: pipelinev1.ParamValue{
				Type: "string",
				StringVal: `- name: web-frontend
  port: 80
  targetPort: 8080
  type: ClusterIP
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8080"
- name: api-backend
  port: 443
  targetPort: 8443
  type: ClusterIP
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8443"
    service.beta.kubernetes.io/aws-load-balancer-backend-protocol: "https"`,
			},
		},
	}
	
	// Execute the Resolve function
	result, err := r.Resolve(context.Background(), params)
	
	// Verify results
	require.NoError(t, err)
	require.NotNil(t, result)
	
	// Check that the template was rendered with all the objects from the lists
	renderedData := string(result.Data())
	
	// Verify environment config rendering
	assert.Contains(t, renderedData, "name: deploy-to-development")
	assert.Contains(t, renderedData, "value: development")
	assert.Contains(t, renderedData, "value: dev-cluster")
	assert.Contains(t, renderedData, "value: app-dev")
	assert.Contains(t, renderedData, "value: \"1\"")
	assert.Contains(t, renderedData, "value: \"250m\"")
	
	assert.Contains(t, renderedData, "name: deploy-to-production")
	assert.Contains(t, renderedData, "value: production")
	assert.Contains(t, renderedData, "value: prod-cluster")
	assert.Contains(t, renderedData, "value: app-prod")
	assert.Contains(t, renderedData, "value: \"3\"")
	assert.Contains(t, renderedData, "value: \"1000m\"")
	
	// Verify features map iteration
	assert.Contains(t, renderedData, "logging: true")
	assert.Contains(t, renderedData, "monitoring: true")
	assert.Contains(t, renderedData, "tracing: true")
	
	// Verify service config rendering in JSON format
	assert.Contains(t, renderedData, `"name": "web-frontend"`)
	assert.Contains(t, renderedData, `"port": 80`)
	assert.Contains(t, renderedData, `"targetPort": 8080`)
	assert.Contains(t, renderedData, `"type": "ClusterIP"`)
	assert.Contains(t, renderedData, `"prometheus.io/scrape": "true"`)
	
	assert.Contains(t, renderedData, `"name": "api-backend"`)
	assert.Contains(t, renderedData, `"port": 443`)
	assert.Contains(t, renderedData, `"targetPort": 8443`)
	assert.Contains(t, renderedData, `"service.beta.kubernetes.io/aws-load-balancer-backend-protocol": "https"`)
}

// TestDirectYAMLObjectRendering tests the ability to directly render YAML objects
// without having to manually enumerate all properties
func TestDirectYAMLObjectRendering(t *testing.T) {
	// Create a mock fetcher with a template that directly renders YAML objects
	mockData := &mockFetcher{
		templates: map[string]string{
			"repo1:path1": `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: direct-yaml-rendering-pipeline
spec:
  params:
    - name: app-name
      type: string
  tasks:
    # Base task
    - name: base-task
      taskRef:
        name: some-task
      
    # Directly render YAML objects without enumerating properties
    {{- $validationSteps := fromYAML .ValidationSteps }}
    {{- range $i, $step := $validationSteps }}
    - {{ toYAML $step }}
    {{- end }}
    
    # Render a list as a single YAML block
    - name: infrastructure
      taskRef:
        name: infrastructure-manager
      params:
        - name: resources
          value: |
            {{- $resources := fromYAML .ResourceConfig }}
            {{- range $i, $res := $resources }}
            - {{ toYAML $res }}
            {{- end }}
`,
		},
	}
	
	// Create resolver with mock fetcher
	r := &resolver{
		fetcher: mockData,
	}
	
	// Test with complex object parameters
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
		// Validation steps with complex nested structure
		{
			Name: "validation-steps",
			Value: pipelinev1.ParamValue{
				Type: "string",
				StringVal: `- name: security-validation
  taskRef:
    name: security-validator
  runAfter:
    - previous-step
  params:
    - name: scan-level
      value: deep
    - name: timeout
      value: 30m
  workspaces:
    - name: source
      workspace: shared-workspace
- name: compliance-validation
  taskRef:
    name: compliance-validator
  params:
    - name: standards
      value:
        - pci-dss
        - hipaa
        - gdpr
  results:
    - name: compliant
      description: Whether the deployment is compliant
    - name: report
      description: Compliance report URL`,
			},
		},
		// Resource configurations with nested structures
		{
			Name: "resource-config",
			Value: pipelinev1.ParamValue{
				Type: "string",
				StringVal: `- type: compute
  name: app-server
  size: large
  replicas: 3
  storage:
    size: 100Gi
    type: ssd
  network:
    ingress:
      enabled: true
      port: 443
- type: database
  name: app-db
  engine: postgres
  version: "13.4"
  storage:
    size: 500Gi
    type: ssd
    backup:
      enabled: true
      retention: 7d
  credentials:
    secretRef: db-creds`,
			},
		},
	}
	
	// Execute the Resolve function
	result, err := r.Resolve(context.Background(), params)
	
	// Verify results
	require.NoError(t, err)
	require.NotNil(t, result)
	
	// Check that the YAML was correctly rendered without property enumeration
	renderedData := string(result.Data())
	
	// Check for direct rendering of validation steps
	assert.Contains(t, renderedData, "name: security-validation")
	assert.Contains(t, renderedData, "runAfter:")
	assert.Contains(t, renderedData, "- previous-step")
	assert.Contains(t, renderedData, "name: compliance-validation")
	assert.Contains(t, renderedData, "- pci-dss")
	assert.Contains(t, renderedData, "- hipaa")
	assert.Contains(t, renderedData, "description: Whether the deployment is compliant")
	
	// Check for rendering of resource configurations
	assert.Contains(t, renderedData, "type: compute")
	assert.Contains(t, renderedData, "name: app-server")
	assert.Contains(t, renderedData, "size: 100Gi")
	assert.Contains(t, renderedData, "type: database")
	assert.Contains(t, renderedData, "engine: postgres")
	assert.Contains(t, renderedData, "retention: 7d")
	assert.Contains(t, renderedData, "secretRef: db-creds")
}
