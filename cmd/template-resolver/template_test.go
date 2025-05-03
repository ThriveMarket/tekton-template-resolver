package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
    {{- if .TextWithLeadingWhitespace}}
    # Text with trimLeading function
    {{trimLeading .TextWithLeadingWhitespace}}
    {{- end}}
`

	tests := []struct {
		name         string
		data         map[string]interface{}
		wantContains []string
		wantErr      bool
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
			wantErr:      true,
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
		{
			name: "with trimLeading function",
			data: map[string]interface{}{
				"TextWithLeadingWhitespace": "    name: test-trim-leading",
			},
			wantContains: []string{
				"# Text with trimLeading function",
				"name: test-trim-leading",
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

// In some cases, we want the yaml to be at a certain indentation
// level, but if it's in a list.. we need it to trim the preceeding
// whitespace so that it will align correctly.
func TestPreindentationTemplate(t *testing.T) {
	templateContent := `
foo:
  - bar
  {{- if .PostDevSteps }}
  {{- $steps := fromYAML .PostDevSteps }}
  {{- range $i, $step := $steps }}
  - {{ toYAML $step | indent 4 | trimLeading }}
  {{- end }}
  {{- end}}
  - baz
`

	data := map[string]interface{}{
		"PostDevSteps": `- name: test
  taskRef:
    name: test-task`,
	}

	result, err := renderTemplate(templateContent, data)

	if err != nil {
		t.Errorf("renderTemplate() error = %v", err)
		return
	}

	if !strings.Contains(result, "  - name:") {
		t.Errorf("renderTemplate() result didn't strip whitespace before name.\nGot:\n%s", result)
	}

}
