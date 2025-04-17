package main

import (
	"context"
	"strings"
	"testing"

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderTemplate(templateContent, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("renderTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("renderTemplate() result doesn't contain %q\nGot:\n%s", want, result)
				}
			}
		})
	}
}