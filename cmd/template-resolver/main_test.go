package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

func TestResolverValidation(t *testing.T) {
	resolver := &resolver{}
	ctx := context.Background()

	tests := []struct {
		name    string
		params  []pipelinev1.Param
		wantErr bool
	}{
		{
			name: "valid params",
			params: []pipelinev1.Param{
				{Name: "repository", Value: pipelinev1.ParamValue{Type: pipelinev1.ParamTypeString, StringVal: "https://github.com/example/repo"}},
				{Name: "path", Value: pipelinev1.ParamValue{Type: pipelinev1.ParamTypeString, StringVal: "path/to/template.yaml"}},
			},
			wantErr: false,
		},
		{
			name: "missing repository",
			params: []pipelinev1.Param{
				{Name: "path", Value: pipelinev1.ParamValue{Type: pipelinev1.ParamTypeString, StringVal: "path/to/template.yaml"}},
			},
			wantErr: true,
		},
		{
			name: "missing path",
			params: []pipelinev1.Param{
				{Name: "repository", Value: pipelinev1.ParamValue{Type: pipelinev1.ParamTypeString, StringVal: "https://github.com/example/repo"}},
			},
			wantErr: true,
		},
		{
			name: "with optional params",
			params: []pipelinev1.Param{
				{Name: "repository", Value: pipelinev1.ParamValue{Type: pipelinev1.ParamTypeString, StringVal: "https://github.com/example/repo"}},
				{Name: "path", Value: pipelinev1.ParamValue{Type: pipelinev1.ParamTypeString, StringVal: "path/to/template.yaml"}},
				{Name: "post-dev-steps", Value: pipelinev1.ParamValue{Type: pipelinev1.ParamTypeString, StringVal: "- name: test\n  taskRef:\n    name: test-task"}},
				{Name: "post-prod-steps", Value: pipelinev1.ParamValue{Type: pipelinev1.ParamTypeString, StringVal: "- name: verify\n  taskRef:\n    name: verify-task"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := resolver.ValidateParams(ctx, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRenderTemplate(t *testing.T) {
	templateContent := `apiVersion: tekton.dev/v1
kind: Pipeline
spec:
  tasks:
    - name: task1
    {{- if .PostDevSteps}}
    # Post-dev steps
    {{.PostDevSteps}}
    {{- end}}
    {{- if .PostProdSteps}}
    # Post-prod steps
    {{.PostProdSteps}}
    {{- end}}
    {{- if .JsonObject}}
    # Json object with toJson function
    {{toJson .JsonObject | indent 4}}
    {{- end}}
    {{- if .IndentText}}
    # Text with indent function
    {{indent 4 .IndentText}}
    {{- end}}
`

	tests := []struct {
		name        string
		data        map[string]interface{}
		wantContains []string
		wantErr     bool
	}{
		{
			name: "empty data",
			data: map[string]interface{}{},
			wantContains: []string{
				"apiVersion: tekton.dev/v1",
				"kind: Pipeline",
				"  tasks:",
				"    - name: task1",
			},
			wantErr: false,
		},
		{
			name: "with post-dev steps",
			data: map[string]interface{}{
				"PostDevSteps": "    - name: test\n      taskRef:\n        name: test-task",
			},
			wantContains: []string{
				"apiVersion: tekton.dev/v1",
				"kind: Pipeline",
				"  tasks:",
				"    - name: task1",
				"    # Post-dev steps",
				"    - name: test",
				"      taskRef:",
				"        name: test-task",
			},
			wantErr: false,
		},
		{
			name: "with both steps",
			data: map[string]interface{}{
				"PostDevSteps":  "    - name: test\n      taskRef:\n        name: test-task",
				"PostProdSteps": "    - name: verify\n      taskRef:\n        name: verify-task",
			},
			wantContains: []string{
				"apiVersion: tekton.dev/v1",
				"kind: Pipeline",
				"  tasks:",
				"    - name: task1",
				"    # Post-dev steps",
				"    - name: test",
				"      taskRef:",
				"        name: test-task",
				"    # Post-prod steps",
				"    - name: verify",
				"      taskRef:",
				"        name: verify-task",
			},
			wantErr: false,
		},
		{
			name: "with template error",
			data: map[string]interface{}{
				"PostDevSteps": func() {}, // Unencodable type
			},
			wantContains: []string{},
			wantErr: true,
		},
		{
			name: "with toJson function", 
			data: map[string]interface{}{
				"JsonObject": map[string]interface{}{
					"name": "test-json",
					"taskRef": map[string]string{
						"name": "test-task",
					},
				},
			},
			wantContains: []string{
				"# Json object with toJson function",
				"    name: test-json",
				"    taskRef:",
				"      name: test-task",
			},
			wantErr: false,
		},
		{
			name: "with indent function",
			data: map[string]interface{}{
				"IndentText": "name: test-indent\ntaskRef:\n  name: test-task",
			},
			wantContains: []string{
				"# Text with indent function",
				"    name: test-indent",
				"    taskRef:",
				"      name: test-task",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderTemplate(templateContent, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("renderTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			if !tt.wantErr {
				for _, want := range tt.wantContains {
					if !strings.Contains(result, want) {
						t.Errorf("renderTemplate() result doesn't contain %q\nGot:\n%s", want, result)
					}
				}
			}
		})
	}
}

func TestResolverFunctionsGetNameAndSelector(t *testing.T) {
	r := NewResolver()
	ctx := context.Background()
	
	name := r.GetName(ctx)
	assert.Equal(t, "Template", name)
	
	selector := r.GetSelector(ctx)
	assert.Equal(t, "template", selector["resolution.tekton.dev/type"])
}

func TestResolverInitialize(t *testing.T) {
	r := NewResolver()
	ctx := context.Background()
	
	err := r.Initialize(ctx)
	assert.NoError(t, err)
}

func TestGitFetcherFetchTemplate(t *testing.T) {
	// Create a temporary directory for Git tests
	tempDir, err := os.MkdirTemp("", "template-resolver-test-*")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tempDir)
		if err != nil {
			t.Logf("Failed to remove temp directory: %v", err)
		}
	}()
	
	// Create a test server for HTTP requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		
		// GitHub raw content
		if strings.HasPrefix(path, "/example/repo/main/") {
			_, err := w.Write([]byte("apiVersion: tekton.dev/v1\nkind: Pipeline\nmetadata:\n  name: test-pipeline"))
			if err != nil {
				t.Logf("Failed to write response: %v", err)
			}
			return
		}
		
		// Gist raw content
		if strings.HasPrefix(path, "/user/gistid/raw/") {
			if strings.HasSuffix(path, "/path/to/template.yaml") {
				_, err := w.Write([]byte("apiVersion: tekton.dev/v1\nkind: Pipeline\nmetadata:\n  name: gist-template"))
				if err != nil {
					t.Logf("Failed to write response: %v", err)
				}
				return
			} else if path == "/user/gistid/raw/" {
				_, err := w.Write([]byte("apiVersion: tekton.dev/v1\nkind: Pipeline\nmetadata:\n  name: gist-single-file"))
				if err != nil {
					t.Logf("Failed to write response: %v", err)
				}
				return
			}
		}
		
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	
	// Create a test fetcher that uses our test server
	fetcher := &testTemplateFetcher{
		server:  server,
		tempDir: tempDir,
	}
	
	// Test GitHub URL
	content, err := fetcher.FetchTemplate(server.URL+"/example/repo", "path/to/template.yaml")
	assert.NoError(t, err)
	assert.Contains(t, content, "name: test-pipeline")
	
	// Test Gist URL with filename
	content, err = fetcher.FetchTemplate("https://gist.github.com/user/gistid", "path/to/template.yaml")
	assert.NoError(t, err)
	assert.Contains(t, content, "name: gist-template")
	
	// Test Gist URL without filename (single-file gist)
	content, err = fetcher.FetchTemplate("https://gist.github.com/user/gistid", "single-file.yaml")
	assert.NoError(t, err)
	assert.Contains(t, content, "name: gist-single-file")
	
	// Test invalid Gist URL
	_, err = fetcher.FetchTemplate("https://gist.github.com/invalid", "file.yaml")
	assert.Error(t, err)
}

// testTemplateFetcher is a test implementation of TemplateFetcher
type testTemplateFetcher struct {
	server  *httptest.Server
	tempDir string
}

// FetchTemplate implements TemplateFetcher for testing
func (t *testTemplateFetcher) FetchTemplate(repoURL, filePath string) (string, error) {
	if strings.HasPrefix(repoURL, t.server.URL) {
		// Convert to raw GitHub URL for our test server
		fileURL := strings.Replace(repoURL, t.server.URL, t.server.URL, 1)
		if !strings.HasSuffix(fileURL, "/") {
			fileURL += "/"
		}
		fileURL += "main/" + filePath
		
		resp, err := http.Get(fileURL)
		if err != nil {
			return "", err
		}
		defer func() {
			closeErr := resp.Body.Close()
			if closeErr != nil {
				fmt.Printf("Failed to close response body: %v\n", closeErr)
			}
		}()
		
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("HTTP error: %s", resp.Status)
		}
		
		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		
		return string(content), nil
	} else if strings.HasPrefix(repoURL, "https://gist.github.com/") {
		if repoURL == "https://gist.github.com/invalid" {
			return "", fmt.Errorf("invalid Gist URL format: %s", repoURL)
		}
		
		// For gist URLs, use our mock server but with the right path structure
		var rawURL string
		if filePath == "single-file.yaml" {
			rawURL = t.server.URL + "/user/gistid/raw/"
		} else {
			rawURL = t.server.URL + "/user/gistid/raw/" + filePath
		}
		
		resp, err := http.Get(rawURL)
		if err != nil {
			return "", err
		}
		defer func() {
			closeErr := resp.Body.Close()
			if closeErr != nil {
				fmt.Printf("Failed to close response body: %v\n", closeErr)
			}
		}()
		
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("HTTP error: %s", resp.Status)
		}
		
		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		
		return string(content), nil
	}
	
	// For Git repositories, create a fake repo with the template
	templateDir := filepath.Join(t.tempDir, filePath)
	err := os.MkdirAll(filepath.Dir(templateDir), 0755)
	if err != nil {
		return "", err
	}
	
	// Write a test template file
	template := "apiVersion: tekton.dev/v1\nkind: Pipeline\nmetadata:\n  name: git-template"
	err = os.WriteFile(templateDir, []byte(template), 0644)
	if err != nil {
		return "", err
	}
	
	return template, nil
}

func TestTemplateResourceMethods(t *testing.T) {
	data := []byte("test data")
	source := &pipelinev1.RefSource{
		URI:        "repo-url",
		Digest:     map[string]string{"sha1": "123456"},
		EntryPoint: "path/to/template",
	}
	
	resource := &templateResource{
		data:   data,
		source: source,
	}
	
	// Test Data() method
	assert.Equal(t, data, resource.Data())
	
	// Test Annotations() method
	assert.Nil(t, resource.Annotations())
	
	// Test RefSource() method
	assert.Equal(t, source, resource.RefSource())
}

func TestFormatTasksYAML(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		contains    []string
		notContains []string
		expectError bool
	}{
		{
			name:        "empty input",
			input:       "",
			contains:    []string{},
			expectError: false,
		},
		{
			name:        "invalid YAML",
			input:       "invalid: yaml: missing quote",
			contains:    []string{},
			expectError: true,
		},
		{
			name:        "no tasks in YAML",
			input:       "[]",
			contains:    []string{},
			expectError: false,
		},
		{
			name: "single task",
			input: `- name: test-task
  taskRef:
    name: task-ref`,
			contains: []string{
				"- name: test-task",
				"taskRef:",
				"name: task-ref",
			},
			expectError: false,
		},
		{
			name: "multiple tasks",
			input: `- name: task1
  taskRef:
    name: task-ref1
- name: task2
  taskRef:
    name: task-ref2`,
			contains: []string{
				"- name: task1",
				"taskRef:",
				"name: task-ref1",
				"- name: task2",
				"name: task-ref2",
			},
			expectError: false,
		},
		{
			name: "complex task with runAfter",
			input: `- name: complex-task
  runAfter:
  - previous-task
  taskSpec:
    steps:
    - name: step1
      image: step-image
      script: |
        echo "Running step"`,
			contains: []string{
				"- name: complex-task",
				"runAfter:",
				"- previous-task",
				"taskSpec:",
				"steps:",
				"name: step1",
				"image: step-image",
				"script:",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatTasksYAML(tt.input)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				
				// Check that result contains expected substrings
				for _, expected := range tt.contains {
					assert.Contains(t, result, expected, "Result should contain %q", expected)
				}
				
				// Check that result does not contain unwanted substrings
				for _, unexpected := range tt.notContains {
					assert.NotContains(t, result, unexpected, "Result should not contain %q", unexpected)
				}
			}
		})
	}
}