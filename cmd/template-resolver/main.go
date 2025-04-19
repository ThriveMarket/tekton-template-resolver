package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/pipeline/pkg/resolution/common"
	"github.com/tektoncd/pipeline/pkg/resolution/resolver/framework"
	"gopkg.in/yaml.v3"
	"knative.dev/pkg/injection/sharedmain"
)

// Global debug flag
var debugMode bool

// debugf prints debug messages only when debug mode is enabled
func debugf(format string, args ...interface{}) {
	if debugMode {
		log.Printf(format, args...)
	}
}

func main() {
	// Parse debug flag before sharedmain takes over flag parsing
	flag.BoolVar(&debugMode, "debug", false, "Enable debug logging")
	
	// Parse our flags first
	flag.Parse()
	
	// Reuse flag values for future flag.Parse() calls by setting arguments explicitly
	os.Args = append([]string{os.Args[0]}, flag.Args()...)
	
	if debugMode {
		log.Println("Debug mode enabled")
	}
	
	sharedmain.Main("controller",
		framework.NewController(context.Background(), NewResolver()),
	)
}

// TemplateFetcher defines the interface for fetching templates
type TemplateFetcher interface {
	FetchTemplate(repoURL, filePath string) (string, error)
}

// Default implementation for fetching templates
type gitTemplateFetcher struct{}

// resolver is the main implementation of the Tekton resolver
type resolver struct{
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
func (r *resolver) Resolve(ctx context.Context, params []pipelinev1.Param) (framework.ResolvedResource, error) {
	debugf("Resolve called with %d params", len(params))
	
	// Extract required parameters
	var repository, path string
	
	// Dynamic parameter map to pass to template
	templateData := make(map[string]interface{})
	
	// First, extract required parameters
	for _, param := range params {
		if param.Name == RepositoryParam {
			repository = param.Value.StringVal
			debugf("Repository: %s", repository)
			templateData[RepositoryParam] = repository
		} else if param.Name == PathParam {
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
		if (param.Name == RepositoryParam || param.Name == PathParam) {
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
			
			// Try to parse each array element as a task definition
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
			
			// If we found tasks, extract names and format them
			if len(tasks) > 0 {
				// Format tasks for YAML inclusion
				tasksBytes, err := yaml.Marshal(tasks)
				if err != nil {
					log.Printf("WARNING: Failed to marshal %s tasks: %v", param.Name, err)
				} else {
					formattedTasks, err := formatTasksYAML(string(tasksBytes))
					if err != nil {
						log.Printf("WARNING: Failed to format %s: %v", param.Name, err)
					} else {
						// Add formatted tasks to template data
						debugf("Adding tasks as %s", camelName)
						templateData[camelName] = formattedTasks
						
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
					}
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
						// It parsed as tasks, handle it like array tasks
						tasksBytes, err := yaml.Marshal(tasks)
						if err != nil {
							log.Printf("WARNING: Failed to marshal %s tasks: %v", param.Name, err)
							templateData[camelName] = paramVal
						} else {
							formattedTasks, err := formatTasksYAML(string(tasksBytes))
							if err != nil {
								log.Printf("WARNING: Failed to format %s: %v", param.Name, err)
								templateData[camelName] = paramVal
							} else {
								// Add formatted tasks to template data
								debugf("Adding string tasks as %s", camelName)
								templateData[camelName] = formattedTasks
								
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
							}
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

// FetchTemplate retrieves a template from a Git repository or Gist
func (g *gitTemplateFetcher) FetchTemplate(repoURL, filePath string) (string, error) {
	// Handle GitHub Gist URLs
	if strings.HasPrefix(repoURL, "https://gist.github.com/") {
		// Convert Gist URL to raw content URL
		// Example: https://gist.github.com/user/gistid -> https://gist.githubusercontent.com/user/gistid/raw/
		parts := strings.Split(repoURL, "/")
		if len(parts) < 5 {
			return "", fmt.Errorf("invalid Gist URL format: %s", repoURL)
		}

		user := parts[3]
		gistID := parts[4]

		// First try with the filename
		rawURL := fmt.Sprintf("https://gist.githubusercontent.com/%s/%s/raw/%s", user, gistID, filePath)

		// First check if we can fetch with the filename
		resp, err := http.Get(rawURL)
		if err != nil {
			return "", err
		}

		// If we got a 404, try without the filename (for single-file gists)
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close() // Close this response before making another request

			// Try without filename for single-file gists
			rawURL = fmt.Sprintf("https://gist.githubusercontent.com/%s/%s/raw/", user, gistID)
			resp, err = http.Get(rawURL)
			if err != nil {
				return "", err
			}
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("HTTP error: %s", resp.Status)
		}

		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		return string(content), nil
	}

	// Handle normal GitHub repositories
	if strings.HasPrefix(repoURL, "https://github.com/") {
		// Convert GitHub URL to raw content URL
		// Example: https://github.com/example/repo -> https://raw.githubusercontent.com/example/repo/main/
		repoURL = strings.Replace(repoURL, "https://github.com/", "https://raw.githubusercontent.com/", 1)
		if !strings.HasSuffix(repoURL, "/") {
			repoURL += "/"
		}
		repoURL += "main/" // Assuming main branch

		// Construct the full URL to the raw file
		fileURL := repoURL + filePath

		// Fetch the content
		resp, err := http.Get(fileURL)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("HTTP error: %s", resp.Status)
		}

		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		return string(content), nil
	}

	// Handle Git repositories (public or private)
	// If private, the GIT_SSH_COMMAND env var should be set in the deployment
	// to use the mounted SSH key
	tempDir, err := os.MkdirTemp("", "template-resolver-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	// Setup git command with output capturing
	cmd := exec.Command("git", "clone", "--depth=1", repoURL, tempDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Attempt to clone the repository
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git clone failed: %w, stderr: %s", err, stderr.String())
	}

	// Read the requested file from the cloned repo
	filePath = filepath.Join(tempDir, filePath)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	return string(content), nil
}

// formatTasksYAML processes the input YAML string to ensure it works correctly in a Pipeline
func formatTasksYAML(yamlContent string) (string, error) {
	debugf("formatTasksYAML input:\n%s", yamlContent)

	if yamlContent == "" {
		debugf("Empty YAML content provided\n")
		return "", nil
	}

	// Parse YAML to get tasks
	var tasks []map[string]interface{}
	err := yaml.Unmarshal([]byte(yamlContent), &tasks)
	if err != nil {
		debugf("YAML Unmarshal error: %v", err)
		return "", err
	}

	// If no tasks, return empty string
	if len(tasks) == 0 {
		debugf("No tasks found in YAML\n")
		return "", nil
	}

	debugf("Found %d tasks", len(tasks))

	// Create a new Pipeline tasks section
	var result strings.Builder

	// Process each task
	for i, task := range tasks {
		debugf("Processing task %d: %v", i, task)

		taskBytes, err := yaml.Marshal(task)
		if err != nil {
			debugf("YAML Marshal error for task %d: %v", i, err)
			return "", err
		}

		// Convert to string and add to result
		taskStr := string(taskBytes)
		debugf("Raw task %d YAML:\n%s", i, taskStr)

		if !strings.HasPrefix(taskStr, "- ") {
			taskStr = "- " + strings.TrimPrefix(taskStr, "---\n")
			debugf("Fixed task %d prefix", i)
		}

		// Properly indent each line of the task YAML
		lines := strings.Split(taskStr, "\n")
		var indentedTask strings.Builder

		// Add the first line with 4 spaces indent
		if len(lines) > 0 {
			// skipping this. I think these are pre-indented?
			indentedTask.WriteString("    " + lines[0] + "\n")

			// Indent all remaining lines with 6 spaces (4 base + 2 for YAML hierarchy)
			for _, line := range lines[1:] {
				if line != "" {
					indentedTask.WriteString("      " + line + "\n")
				}
			}
		}

		// Add the properly indented task to the result
		result.WriteString(indentedTask.String())
		debugf("Added indented task %d", i)
	}

	resultStr := result.String()
	debugf("formatTasksYAML result:\n%s", resultStr)
	return resultStr, nil
}

// renderTemplate applies Go template processing to the template content
func renderTemplate(templateContent string, data map[string]interface{}) (string, error) {
	// Create a template with custom functions
	funcMap := template.FuncMap{
		"toJson": func(v interface{}) string {
			// Skip null values
			if v == nil {
				return ""
			}

			bytes, err := json.Marshal(v)
			if err != nil {
				return fmt.Sprintf("Error: %v", err)
			}
			// Parse the JSON to create a properly indented YAML representation
			var obj interface{}
			err = json.Unmarshal(bytes, &obj)
			if err != nil {
				return fmt.Sprintf("Error: %v", err)
			}

			// Convert back to YAML representation
			yamlBytes, err := yaml.Marshal(obj)
			if err != nil {
				return fmt.Sprintf("Error: %v", err)
			}

			// Remove the first line (object marker) and trim trailing newline
			yamlStr := string(yamlBytes)
			yamlStr = strings.TrimPrefix(yamlStr, "---\n")
			return strings.TrimSpace(yamlStr)
		},
		"indent": func(spaces int, v string) string {
			padding := strings.Repeat(" ", spaces)
			lines := strings.Split(v, "\n")

			for i := range lines {
				if lines[i] != "" {
					lines[i] = padding + lines[i]
				}
			}

			return strings.Join(lines, "\n")
		},
	}

	debugf("Template content before parsing:\n%s", templateContent)
	debugf("Template data: %v", data)

	tmpl, err := template.New("pipeline").Funcs(funcMap).Parse(templateContent)
	if err != nil {
		debugf("Template parsing error: %v", err)
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		debugf("Template execution error: %v", err)
		return "", err
	}

	result := buf.String()
	debugf("Rendered template:\n%s", result)

	// Validate the resulting YAML
	var obj interface{}
	if err := yaml.Unmarshal([]byte(result), &obj); err != nil {
		debugf("Generated YAML is invalid: %v", err)
		// Try to identify the problematic line
		lines := strings.Split(result, "\n")
		for i, line := range lines {
			var testObj interface{}
			if err := yaml.Unmarshal([]byte(line), &testObj); err != nil {
				debugf("Potential YAML issue at line %d: %s", i+1, line)
			}
		}
	} else {
		debugf("Generated YAML is valid\n")
	}

	return result, nil
}

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

// Helper function to convert parameter names to camel case for Go templates
// Example: "post-dev-steps" -> "PostDevSteps"
func toCamelCase(paramName string) string {
	parts := strings.Split(paramName, "-")
	for i := range parts {
		parts[i] = strings.Title(parts[i])
	}
	return strings.Join(parts, "")
}

