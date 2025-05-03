package main

import (
	"context"
	"testing"
	
	"github.com/stretchr/testify/assert"
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