package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func main() {
	sharedmain.Main("controller",
		framework.NewController(context.Background(), &resolver{}),
	)
}

type resolver struct{}

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
	fmt.Printf("DEBUG: Resolve called with %d params\n", len(params))
	
	// Extract required parameters
	var repository, path string
	
	// For backward compatibility
	var postDevStepsTasks, postProdStepsTasks []map[string]interface{}
	
	// Dynamic parameter map to pass to template
	templateData := make(map[string]interface{})
	
	for _, param := range params {
		fmt.Printf("DEBUG: Processing param: %s (type: %s)\n", param.Name, param.Value.Type)
		
		// Set required parameters and continue processing others
		switch param.Name {
		case RepositoryParam:
			repository = param.Value.StringVal
			fmt.Printf("DEBUG: Repository: %s\n", repository)
			templateData[RepositoryParam] = repository
			continue
		case PathParam:
			path = param.Value.StringVal
			fmt.Printf("DEBUG: Path: %s\n", path)
			templateData[PathParam] = path
			continue
		case PostDevParam:
			// Handle both array and string formats for backward compatibility
			if param.Value.Type == "array" {
				fmt.Printf("DEBUG: Post-dev steps as array with %d elements\n", len(param.Value.ArrayVal))
				// Parse each array element as a task
				for i, arrayItem := range param.Value.ArrayVal {
					var task map[string]interface{}
					if err := yaml.Unmarshal([]byte(arrayItem), &task); err != nil {
						fmt.Printf("Warning: Failed to parse post-dev-steps array item %d: %v\n", i, err)
						continue
					}
					postDevStepsTasks = append(postDevStepsTasks, task)
				}
			} else if param.Value.Type == "object" {
				// Handle if it's sent as an object
				fmt.Printf("DEBUG: Post-dev steps as object (unexpected format)\n")
				taskMap := make(map[string]interface{})
				for k, v := range param.Value.ObjectVal {
					taskMap[k] = v
				}
				postDevStepsTasks = append(postDevStepsTasks, taskMap)
			} else {
				// Legacy string format
				postDevSteps := param.Value.StringVal
				fmt.Printf("DEBUG: Post-dev steps as string: %d bytes\n", len(postDevSteps))
				
				if postDevSteps != "" {
					var tasks []map[string]interface{}
					if err := yaml.Unmarshal([]byte(postDevSteps), &tasks); err != nil {
						fmt.Printf("Warning: Failed to parse post-dev-steps YAML: %v\n", err)
					} else {
						postDevStepsTasks = tasks
					}
				}
			}
		case PostProdParam:
			// Handle both array and string formats for backward compatibility
			if param.Value.Type == "array" {
				fmt.Printf("DEBUG: Post-prod steps as array with %d elements\n", len(param.Value.ArrayVal))
				// Parse each array element as a task
				for i, arrayItem := range param.Value.ArrayVal {
					var task map[string]interface{}
					if err := yaml.Unmarshal([]byte(arrayItem), &task); err != nil {
						fmt.Printf("Warning: Failed to parse post-prod-steps array item %d: %v\n", i, err)
						continue
					}
					postProdStepsTasks = append(postProdStepsTasks, task)
				}
			} else if param.Value.Type == "object" {
				// Handle if it's sent as an object 
				fmt.Printf("DEBUG: Post-prod steps as object (unexpected format)\n")
				taskMap := make(map[string]interface{})
				for k, v := range param.Value.ObjectVal {
					taskMap[k] = v
				}
				postProdStepsTasks = append(postProdStepsTasks, taskMap)
			} else {
				// Legacy string format
				postProdSteps := param.Value.StringVal
				fmt.Printf("DEBUG: Post-prod steps as string: %d bytes\n", len(postProdSteps))
				
				if postProdSteps != "" {
					var tasks []map[string]interface{}
					if err := yaml.Unmarshal([]byte(postProdSteps), &tasks); err != nil {
						fmt.Printf("Warning: Failed to parse post-prod-steps YAML: %v\n", err)
					} else {
						postProdStepsTasks = tasks
					}
				}
			}
		}
	}

	// Fetch template from Git repository
	templateContent, err := fetchTemplate(repository, path)
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
		fmt.Printf("DEBUG: Formatting %d post-dev steps\n", len(postDevStepsTasks))
		tasksBytes, err := yaml.Marshal(postDevStepsTasks)
		if err != nil {
			fmt.Printf("WARNING: Failed to marshal post-dev-steps: %v\n", err)
		} else {
			var err error
			formattedPostDevSteps, err = formatTasksYAML(string(tasksBytes))
			if err != nil {
				// Log the error but don't fail - use empty string
				fmt.Printf("WARNING: Failed to format post-dev-steps: %v\n", err)
			} else {
				fmt.Printf("DEBUG: Formatted post-dev steps: %s\n", formattedPostDevSteps)
			}
		}
	}

	// Format post-prod steps for pipeline inclusion
	formattedPostProdSteps := ""
	if len(postProdStepsTasks) > 0 {
		fmt.Printf("DEBUG: Formatting %d post-prod steps\n", len(postProdStepsTasks))
		tasksBytes, err := yaml.Marshal(postProdStepsTasks)
		if err != nil {
			fmt.Printf("WARNING: Failed to marshal post-prod-steps: %v\n", err)
		} else {
			var err error
			formattedPostProdSteps, err = formatTasksYAML(string(tasksBytes))
			if err != nil {
				// Log the error but don't fail - use empty string
				fmt.Printf("WARNING: Failed to format post-prod-steps: %v\n", err)
			} else {
				fmt.Printf("DEBUG: Formatted post-prod steps: %s\n", formattedPostProdSteps)
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
			fmt.Printf("DEBUG: Processing generic array parameter %s\n", param.Name)
			
			// Try to parse each array element as a task definition
			var tasks []map[string]interface{}
			for i, arrayItem := range param.Value.ArrayVal {
				var task map[string]interface{}
				if err := yaml.Unmarshal([]byte(arrayItem), &task); err != nil {
					fmt.Printf("Warning: Failed to parse %s array item %d as YAML: %v\n", param.Name, i, err)
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
					fmt.Printf("WARNING: Failed to marshal %s tasks: %v\n", param.Name, err)
				} else {
					formattedTasks, err := formatTasksYAML(string(tasksBytes))
					if err != nil {
						fmt.Printf("WARNING: Failed to format %s: %v\n", param.Name, err)
					} else {
						// Add formatted tasks to template data
						fmt.Printf("DEBUG: Adding generic tasks as %s\n", camelName)
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
							fmt.Printf("DEBUG: Adding task names as %s\n", namesParam)
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

	fmt.Printf("DEBUG: Creating template resource with %d bytes of data\n", len(renderedTemplate))

	// Final validation before returning
	var obj interface{}
	if err := yaml.Unmarshal([]byte(renderedTemplate), &obj); err != nil {
		fmt.Printf("DEBUG: Final YAML validation failed: %v\n", err)
	} else {
		fmt.Printf("DEBUG: Final YAML validation passed\n")
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

// fetchTemplate retrieves a template from a Git repository or Gist
func fetchTemplate(repoURL, filePath string) (string, error) {
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
	fmt.Printf("DEBUG: formatTasksYAML input:\n%s\n", yamlContent)

	if yamlContent == "" {
		fmt.Printf("DEBUG: Empty YAML content provided\n")
		return "", nil
	}

	// Parse YAML to get tasks
	var tasks []map[string]interface{}
	err := yaml.Unmarshal([]byte(yamlContent), &tasks)
	if err != nil {
		fmt.Printf("DEBUG: YAML Unmarshal error: %v\n", err)
		return "", err
	}

	// If no tasks, return empty string
	if len(tasks) == 0 {
		fmt.Printf("DEBUG: No tasks found in YAML\n")
		return "", nil
	}

	fmt.Printf("DEBUG: Found %d tasks\n", len(tasks))

	// Create a new Pipeline tasks section
	var result strings.Builder

	// Process each task
	for i, task := range tasks {
		fmt.Printf("DEBUG: Processing task %d: %v\n", i, task)

		taskBytes, err := yaml.Marshal(task)
		if err != nil {
			fmt.Printf("DEBUG: YAML Marshal error for task %d: %v\n", i, err)
			return "", err
		}

		// Convert to string and add to result
		taskStr := string(taskBytes)
		fmt.Printf("DEBUG: Raw task %d YAML:\n%s\n", i, taskStr)

		if !strings.HasPrefix(taskStr, "- ") {
			taskStr = "- " + strings.TrimPrefix(taskStr, "---\n")
			fmt.Printf("DEBUG: Fixed task %d prefix\n", i)
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
		fmt.Printf("DEBUG: Added indented task %d\n", i)
	}

	resultStr := result.String()
	fmt.Printf("DEBUG: formatTasksYAML result:\n%s\n", resultStr)
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

	fmt.Printf("DEBUG: Template content before parsing:\n%s\n", templateContent)
	fmt.Printf("DEBUG: Template data: %v\n", data)

	tmpl, err := template.New("pipeline").Funcs(funcMap).Parse(templateContent)
	if err != nil {
		fmt.Printf("DEBUG: Template parsing error: %v\n", err)
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		fmt.Printf("DEBUG: Template execution error: %v\n", err)
		return "", err
	}

	result := buf.String()
	fmt.Printf("DEBUG: Rendered template:\n%s\n", result)

	// Validate the resulting YAML
	var obj interface{}
	if err := yaml.Unmarshal([]byte(result), &obj); err != nil {
		fmt.Printf("DEBUG: Generated YAML is invalid: %v\n", err)
		// Try to identify the problematic line
		lines := strings.Split(result, "\n")
		for i, line := range lines {
			var testObj interface{}
			if err := yaml.Unmarshal([]byte(line), &testObj); err != nil {
				fmt.Printf("DEBUG: Potential YAML issue at line %d: %s\n", i+1, line)
			}
		}
	} else {
		fmt.Printf("DEBUG: Generated YAML is valid\n")
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
