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
	
	// Legacy parameters - kept for backward compatibility
	// We process all parameters dynamically now, these are just for documentation
	PostDevParam    = "post-dev-steps"
	PostProdParam   = "post-prod-steps"
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
	
	// For backward compatibility
	var postDevStepsTasks, postProdStepsTasks []map[string]interface{}
	
	// Dynamic parameter map to pass to template
	templateData := make(map[string]interface{})
	
	for _, param := range params {
		debugf("Processing param: %s (type: %s)", param.Name, param.Value.Type)
		
		// Set required parameters and continue processing others
		switch param.Name {
		case RepositoryParam:
			repository = param.Value.StringVal
			debugf("Repository: %s", repository)
			templateData[RepositoryParam] = repository
			continue
		case PathParam:
			path = param.Value.StringVal
			debugf("Path: %s", path)
			templateData[PathParam] = path
			continue
		case PostDevParam:
			// Handle both array and string formats for backward compatibility
			if param.Value.Type == "array" {
				debugf("Post-dev steps as array with %d elements", len(param.Value.ArrayVal))
				// Parse each array element as a task
				for i, arrayItem := range param.Value.ArrayVal {
					var task map[string]interface{}
					if err := yaml.Unmarshal([]byte(arrayItem), &task); err != nil {
						log.Printf("WARNING: Failed to parse post-dev-steps array item %d: %v\n", i, err)
						continue
					}
					postDevStepsTasks = append(postDevStepsTasks, task)
				}
			} else if param.Value.Type == "object" {
				// Handle if it's sent as an object
				debugf("Post-dev steps as object (unexpected format)\n")
				taskMap := make(map[string]interface{})
				for k, v := range param.Value.ObjectVal {
					taskMap[k] = v
				}
				postDevStepsTasks = append(postDevStepsTasks, taskMap)
			} else {
				// Legacy string format
				postDevSteps := param.Value.StringVal
				debugf("Post-dev steps as string: %d bytes", len(postDevSteps))
				
				if postDevSteps != "" {
					var tasks []map[string]interface{}
					if err := yaml.Unmarshal([]byte(postDevSteps), &tasks); err != nil {
						log.Printf("WARNING: Failed to parse post-dev-steps YAML: %v\n", err)
					} else {
						postDevStepsTasks = tasks
					}
				}
			}
		case PostProdParam:
			// Handle both array and string formats for backward compatibility
			if param.Value.Type == "array" {
				debugf("Post-prod steps as array with %d elements", len(param.Value.ArrayVal))
				// Parse each array element as a task
				for i, arrayItem := range param.Value.ArrayVal {
					var task map[string]interface{}
					if err := yaml.Unmarshal([]byte(arrayItem), &task); err != nil {
						log.Printf("WARNING: Failed to parse post-prod-steps array item %d: %v\n", i, err)
						continue
					}
					postProdStepsTasks = append(postProdStepsTasks, task)
				}
			} else if param.Value.Type == "object" {
				// Handle if it's sent as an object 
				debugf("Post-prod steps as object (unexpected format)\n")
				taskMap := make(map[string]interface{})
				for k, v := range param.Value.ObjectVal {
					taskMap[k] = v
				}
				postProdStepsTasks = append(postProdStepsTasks, taskMap)
			} else {
				// Legacy string format
				postProdSteps := param.Value.StringVal
				debugf("Post-prod steps as string: %d bytes", len(postProdSteps))
				
				if postProdSteps != "" {
					var tasks []map[string]interface{}
					if err := yaml.Unmarshal([]byte(postProdSteps), &tasks); err != nil {
						log.Printf("WARNING: Failed to parse post-prod-steps YAML: %v\n", err)
					} else {
						postProdStepsTasks = tasks
					}
				}
			}
		}
	}

	// Fetch template from Git repository
	templateContent, err := r.fetcher.FetchTemplate(repository, path)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch template: %w", err)
	}

	// Extract task names from post-deploy steps
	devTaskName := "default-dev-validation"   // Default when no custom steps
	prodTaskName := "default-prod-validation" // Default when no custom steps
	var devTaskNames []string
	var prodTaskNames []string

	// Extract all dev task names from the tasks
	for _, task := range postDevStepsTasks {
		if name, ok := task["name"].(string); ok {
			devTaskNames = append(devTaskNames, name)
		}
	}

	// Set the last task name for backward compatibility
	if len(devTaskNames) > 0 {
		devTaskName = devTaskNames[len(devTaskNames)-1]
	}

	// Extract all prod task names from the tasks
	for _, task := range postProdStepsTasks {
		if name, ok := task["name"].(string); ok {
			prodTaskNames = append(prodTaskNames, name)
		}
	}

	// Set the last task name for backward compatibility
	if len(prodTaskNames) > 0 {
		prodTaskName = prodTaskNames[len(prodTaskNames)-1]
	}

	// Format post-dev steps for pipeline inclusion
	formattedPostDevSteps := ""
	if len(postDevStepsTasks) > 0 {
		debugf("Formatting %d post-dev steps", len(postDevStepsTasks))
		tasksBytes, err := yaml.Marshal(postDevStepsTasks)
		if err != nil {
			log.Printf("WARNING: Failed to marshal post-dev-steps: %v\n", err)
		} else {
			var err error
			formattedPostDevSteps, err = formatTasksYAML(string(tasksBytes))
			if err != nil {
				// Log the error but don't fail - use empty string
				log.Printf("WARNING: Failed to format post-dev-steps: %v\n", err)
			} else {
				debugf("Formatted post-dev steps: %s", formattedPostDevSteps)
			}
		}
	}

	// Format post-prod steps for pipeline inclusion
	formattedPostProdSteps := ""
	if len(postProdStepsTasks) > 0 {
		debugf("Formatting %d post-prod steps", len(postProdStepsTasks))
		tasksBytes, err := yaml.Marshal(postProdStepsTasks)
		if err != nil {
			log.Printf("WARNING: Failed to marshal post-prod-steps: %v\n", err)
		} else {
			var err error
			formattedPostProdSteps, err = formatTasksYAML(string(tasksBytes))
			if err != nil {
				// Log the error but don't fail - use empty string
				log.Printf("WARNING: Failed to format post-prod-steps: %v\n", err)
			} else {
				debugf("Formatted post-prod steps: %s", formattedPostProdSteps)
			}
		}
	}

	// Add legacy parameters to the template data
	camelDevParam := toCamelCase(PostDevParam)
	camelProdParam := toCamelCase(PostProdParam)
	
	templateData[camelDevParam] = formattedPostDevSteps
	templateData[camelProdParam] = formattedPostProdSteps
	templateData["DevTaskName"] = devTaskName
	templateData["ProdTaskName"] = prodTaskName
	templateData["DevTaskNames"] = devTaskNames
	templateData["ProdTaskNames"] = prodTaskNames
	
	// Process all other parameters not explicitly handled
	for _, param := range params {
		// Skip parameters we've already processed
		if param.Name == RepositoryParam || param.Name == PathParam || 
		   param.Name == PostDevParam || param.Name == PostProdParam {
			continue
		}
		
		// Convert parameter name to camel case for template
		camelName := toCamelCase(param.Name)
		
		// Skip if we've already set this parameter name
		if _, exists := templateData[camelName]; exists {
			continue
		}
		
		// Process based on parameter type
		switch param.Value.Type {
		case pipelinev1.ParamTypeArray:
			debugf("Processing generic array parameter %s", param.Name)
			
			// Try to parse each array element as a task definition
			var tasks []map[string]interface{}
			for i, arrayItem := range param.Value.ArrayVal {
				var task map[string]interface{}
				if err := yaml.Unmarshal([]byte(arrayItem), &task); err != nil {
					log.Printf("WARNING: Failed to parse %s array item %d as YAML: %v\n", param.Name, i, err)
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
					log.Printf("WARNING: Failed to marshal %s tasks: %v\n", param.Name, err)
				} else {
					formattedTasks, err := formatTasksYAML(string(tasksBytes))
					if err != nil {
						log.Printf("WARNING: Failed to format %s: %v\n", param.Name, err)
					} else {
						// Add formatted tasks to template data
						debugf("Adding generic tasks as %s", camelName)
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
			templateData[camelName] = param.Value.StringVal
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

// Handle backward compatibility for legacy parameter names
func mapLegacyNames(templateData map[string]interface{}) {
	// Ensure PostDevSteps and related fields are available
	if _, exists := templateData["PostDevSteps"]; !exists {
		templateData["PostDevSteps"] = ""
	}
	if _, exists := templateData["PostProdSteps"]; !exists {
		templateData["PostProdSteps"] = ""
	}
	
	// Default task names if not set
	if _, exists := templateData["DevTaskNames"]; !exists {
		templateData["DevTaskNames"] = []string{}
	}
	if _, exists := templateData["ProdTaskNames"]; !exists {
		templateData["ProdTaskNames"] = []string{}
	}
	
	// Backward compatibility for single task name
	if _, exists := templateData["DevTaskName"]; !exists {
		devNames, ok := templateData["DevTaskNames"].([]string)
		if ok && len(devNames) > 0 {
			templateData["DevTaskName"] = devNames[len(devNames)-1]
		} else {
			templateData["DevTaskName"] = "default-dev-validation"
		}
	}
	if _, exists := templateData["ProdTaskName"]; !exists {
		prodNames, ok := templateData["ProdTaskNames"].([]string)
		if ok && len(prodNames) > 0 {
			templateData["ProdTaskName"] = prodNames[len(prodNames)-1]
		} else {
			templateData["ProdTaskName"] = "default-prod-validation"
		}
	}
}
