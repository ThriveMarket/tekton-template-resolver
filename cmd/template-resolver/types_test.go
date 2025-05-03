package main

import (
	"testing"
	
	"github.com/stretchr/testify/assert"
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

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