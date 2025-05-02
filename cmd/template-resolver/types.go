package main

import (
	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

// TemplateFetcher defines the interface for fetching templates
type TemplateFetcher interface {
	FetchTemplate(repoURL, filePath string) (string, error)
}

// Default implementation for fetching templates
type gitTemplateFetcher struct{}

// templateResource wraps the rendered template data
type templateResource struct {
	data   []byte
	source *pipelinev1.RefSource
}

// Data returns the bytes of our rendered template
func (r *templateResource) Data() []byte {
	return r.data
}

// Annotations returns any metadata needed alongside the data
func (r *templateResource) Annotations() map[string]string {
	return nil
}

// RefSource returns source reference information about the template
func (r *templateResource) RefSource() *pipelinev1.RefSource {
	return r.source
}
