package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/pipeline/pkg/resolution/common"
	"github.com/tektoncd/pipeline/pkg/resolution/resolver/framework"
	"gopkg.in/yaml.v3"
)

// resolver is the main implementation of the Tekton resolver
type resolver struct {
	fetcher TemplateFetcher
}

// NewResolver creates a new resolver with the default template fetcher
func NewResolver() *resolver {
	return &resolver{
		fetcher: &gitTemplateFetcher{},
	}
}

// Initialize sets up any dependencies needed by the resolver. None atm.
func (r *resolver) Initialize(context.Context) error {
	return nil
}

// GetName returns a string name to refer to this resolver by.
func (r *resolver) GetName(context.Context) string {
	return "Template"
}

// GetSelector returns a map of labels to match requests to this resolver.
func (r *resolver) GetSelector(context.Context) map[string]string {
	return map[string]string{
		common.LabelKeyResolverType: "template",
	}
}

// Parameters required for template resolution
const (
	RepositoryParam = "repository"
	PathParam       = "path"
)

// Validate ensures that the resolution params from a request are as expected.
func (r *resolver) ValidateParams(ctx context.Context, params []pipelinev1.Param) error {
	// Create a map for easier lookup
	paramMap := make(map[string]bool)
	for _, param := range params {
		paramMap[param.Name] = true
	}

	// Check for required parameters
	if !paramMap[RepositoryParam] {
		return fmt.Errorf("missing required parameter: %s", RepositoryParam)
	}
	if !paramMap[PathParam] {
		return fmt.Errorf("missing required parameter: %s", PathParam)
	}

	// Post-dev and post-prod steps are optional
	return nil
}

// Resolve fetches the template from Git, applies parameters, and returns the rendered template.
// For YAML array parameters that look like Tekton tasks:
// - The structured objects are stored directly in templateData[camelName] for iteration
// - The task names are stored in templateData[camelName+"Names"] for runAfter references
// - The original string is also stored as templateData[camelName+"Raw"] for direct fromYAML usage
func (r *resolver) Resolve(ctx context.Context, params []pipelinev1.Param) (framework.ResolvedResource, error) {
	debugf("Resolve called with %d params", len(params))

	// Extract required parameters
	var repository, path string

	// Dynamic parameter map to pass to template
	templateData := make(map[string]interface{})

	// First, extract required parameters
	for _, param := range params {
		switch param.Name {
		case RepositoryParam:
			repository = param.Value.StringVal
			debugf("Repository: %s", repository)
			templateData[RepositoryParam] = repository
		case PathParam:
			path = param.Value.StringVal
			debugf("Path: %s", path)
			templateData[PathParam] = path
		}
	}

	// Fetch template from Git repository
	templateContent, err := r.fetcher.FetchTemplate(repository, path)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch template: %w", err)
	}

	// Process all parameters including the required ones we already set
	for _, param := range params {
		debugf("Processing param: %s (type: %s)", param.Name, param.Value.Type)

		// Convert parameter name to camel case for template
		camelName := toCamelCase(param.Name)

		// Skip parameters we've already set (repository and path)
		// and skip if we've already processed this parameter name
		if param.Name == RepositoryParam || param.Name == PathParam {
			continue
		}

		// Also skip if we've already set this parameter name through another parameter
		if _, exists := templateData[camelName]; exists {
			continue
		}

		// Process based on parameter type
		switch param.Value.Type {
		case pipelinev1.ParamTypeArray:
			debugf("Processing array parameter %s", param.Name)

			// Try to parse structured YAML arrays
			if strings.Contains(param.Name, "steps") || strings.Contains(param.Name, "tasks") {
				// First try to parse the array directly as JSON
				// This is needed for complex YAML structures
				allItemsJSON := "["
				for i, val := range param.Value.ArrayVal {
					if i > 0 {
						allItemsJSON += ","
					}
					allItemsJSON += val
				}
				allItemsJSON += "]"

				debugf("Trying to parse array as JSON: %s", allItemsJSON)

				var taskObjects []map[string]interface{}
				if err := json.Unmarshal([]byte(allItemsJSON), &taskObjects); err == nil {
					debugf("Successfully parsed JSON array with %d objects", len(taskObjects))

					// Create a YAML string for the template to use with fromYAML
					yamlBytes, err := yaml.Marshal(taskObjects)
					if err == nil {
						yamlString := string(yamlBytes)
						debugf("Adding YAML string as %s", camelName)
						templateData[camelName] = yamlString
					} else {
						debugf("Failed to convert objects to YAML: %v, using original JSON", err)
						templateData[camelName] = allItemsJSON
					}

					// Store the structured objects with a different key
					structuredKey := camelName + "Objects"
					debugf("Adding structured task objects as %s", structuredKey)
					templateData[structuredKey] = taskObjects

					// Extract task names (for runAfter references)
					var taskNames []string
					for _, task := range taskObjects {
						if name, ok := task["name"].(string); ok {
							taskNames = append(taskNames, name)
						}
					}

					// Add names for reference in templates
					if len(taskNames) > 0 {
						namesParam := camelName + "Names"
						debugf("Adding task names as %s: %v", namesParam, taskNames)
						templateData[namesParam] = taskNames

						// Add last task name for convenience
						lastNameParam := camelName + "Name"
						lastTaskName := taskNames[len(taskNames)-1]
						debugf("Adding last task name as %s: %s", lastNameParam, lastTaskName)
						templateData[lastNameParam] = lastTaskName
					}

					// Skip the rest of the processing
					continue
				}

				debugf("Failed to parse structured JSON array: %v", err)
			}

			// Fall back to standard array processing
			var tasks []map[string]interface{}
			for i, arrayItem := range param.Value.ArrayVal {
				var task map[string]interface{}
				if err := yaml.Unmarshal([]byte(arrayItem), &task); err != nil {
					log.Printf("WARNING: Failed to parse %s array item %d as YAML: %v", param.Name, i, err)
					continue
				}

				// Check if this looks like a task (has a "name" field)
				if _, hasName := task["name"]; hasName {
					tasks = append(tasks, task)
				}
			}

			// If we found tasks, store them as a YAML string and extract names
			if len(tasks) > 0 {
				// Create a YAML string for the template to use with fromYAML
				yamlBytes, err := yaml.Marshal(tasks)
				if err == nil {
					yamlString := string(yamlBytes)
					debugf("Adding YAML string as %s", camelName)
					templateData[camelName] = yamlString
				} else {
					debugf("Failed to convert tasks to YAML: %v", err)
					templateData[camelName] = ""
				}

				// Store the task objects with a different key
				structuredKey := camelName + "Objects"
				debugf("Adding structured task objects as %s", structuredKey)
				templateData[structuredKey] = tasks

				// Extract task names
				var taskNames []string
				for _, task := range tasks {
					if name, ok := task["name"].(string); ok {
						taskNames = append(taskNames, name)
					}
				}

				// Add task names to template data
				if len(taskNames) > 0 {
					namesParam := camelName + "Names"
					debugf("Adding task names as %s", namesParam)
					templateData[namesParam] = taskNames

					// Add last task name for convenience
					lastNameParam := camelName + "Name"
					lastTaskName := taskNames[len(taskNames)-1]
					debugf("Adding last task name as %s: %s", lastNameParam, lastTaskName)
					templateData[lastNameParam] = lastTaskName
				}
			} else {
				// Just a regular array parameter
				templateData[camelName] = param.Value.ArrayVal
			}

		case pipelinev1.ParamTypeObject:
			// Pass through object parameters
			templateData[camelName] = param.Value.ObjectVal

		default: // String or other type
			// Try to parse string as YAML tasks if it looks like YAML
			if param.Value.Type == pipelinev1.ParamTypeString && strings.Contains(param.Value.StringVal, "name:") {
				paramVal := param.Value.StringVal
				if paramVal != "" {
					var tasks []map[string]interface{}
					if err := yaml.Unmarshal([]byte(paramVal), &tasks); err != nil {
						// Not valid YAML tasks, treat as a regular string
						templateData[camelName] = paramVal
					} else if len(tasks) > 0 {
						// It parsed as tasks, store as YAML string for templates
						// Create a YAML string for the template to use with fromYAML
						yamlBytes, err := yaml.Marshal(tasks)
						if err == nil {
							yamlString := string(yamlBytes)
							debugf("Adding YAML string as %s", camelName)
							templateData[camelName] = yamlString
						} else {
							debugf("Failed to convert tasks to YAML: %v", err)
							templateData[camelName] = paramVal
						}

						// Store the task objects with a different key
						structuredKey := camelName + "Objects"
						debugf("Adding structured task objects as %s", structuredKey)
						templateData[structuredKey] = tasks

						// Extract task names
						var taskNames []string
						for _, task := range tasks {
							if name, ok := task["name"].(string); ok {
								taskNames = append(taskNames, name)
							}
						}

						// Add task names to template data
						if len(taskNames) > 0 {
							namesParam := camelName + "Names"
							debugf("Adding task names as %s", namesParam)
							templateData[namesParam] = taskNames

							// Add last task name for convenience
							lastNameParam := camelName + "Name"
							lastTaskName := taskNames[len(taskNames)-1]
							debugf("Adding last task name as %s: %s", lastNameParam, lastTaskName)
							templateData[lastNameParam] = lastTaskName
						}
					} else {
						// Empty tasks array, use empty string
						templateData[camelName] = ""
					}
				} else {
					templateData[camelName] = paramVal
				}
			} else {
				// Regular string parameter
				templateData[camelName] = param.Value.StringVal
			}
		}
	}

	// Render the template
	renderedTemplate, err := renderTemplate(templateContent, templateData)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	debugf("Creating template resource with %d bytes of data", len(renderedTemplate))

	// Final validation before returning
	var obj interface{}
	if err := yaml.Unmarshal([]byte(renderedTemplate), &obj); err != nil {
		debugf("Final YAML validation failed: %v", err)
	} else {
		debugf("Final YAML validation passed\n")
	}

	return &templateResource{
		data: []byte(renderedTemplate),
		source: &pipelinev1.RefSource{
			URI: repository,
			Digest: map[string]string{
				"sha1": "unknown", // In a real implementation, we should calculate this
			},
			EntryPoint: path,
		},
	}, nil
}
