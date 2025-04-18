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
	// Extract parameters
	var repository, path, postDevSteps, postProdSteps string
	for _, param := range params {
		fmt.Printf("DEBUG: Processing param: %s\n", param.Name)
		switch param.Name {
		case RepositoryParam:
			repository = param.Value.StringVal
			fmt.Printf("DEBUG: Repository: %s\n", repository)
		case PathParam:
			path = param.Value.StringVal
			fmt.Printf("DEBUG: Path: %s\n", path)
		case PostDevParam:
			postDevSteps = param.Value.StringVal
			fmt.Printf("DEBUG: Post-dev steps length: %d bytes\n", len(postDevSteps))
		case PostProdParam:
			postProdSteps = param.Value.StringVal
			fmt.Printf("DEBUG: Post-prod steps length: %d bytes\n", len(postProdSteps))
		}
	}

	// Fetch template from Git repository
	templateContent, err := fetchTemplate(repository, path)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch template: %w", err)
	}

	// Extract task names from post-deploy steps by parsing the YAML
	devTaskName := "default-dev-validation"   // Default when no custom steps
	prodTaskName := "default-prod-validation" // Default when no custom steps
	var devTaskNames []string
	var prodTaskNames []string

	// Parse the post-dev-steps YAML to extract all task names
	if postDevSteps != "" {
		var tasks []map[string]interface{}
		err := yaml.Unmarshal([]byte(postDevSteps), &tasks)
		if err != nil {
			// Log the error but don't fail - use default
			fmt.Printf("Warning: Failed to parse post-dev-steps YAML: %v\n", err)
		} else {
			// Extract all task names
			for _, task := range tasks {
				if name, ok := task["name"].(string); ok {
					devTaskNames = append(devTaskNames, name)
				}
			}

			// Set the last task name for backward compatibility
			if len(devTaskNames) > 0 {
				devTaskName = devTaskNames[len(devTaskNames)-1]
			}
		}
	}

	// Parse the post-prod-steps YAML to extract all task names
	if postProdSteps != "" {
		var tasks []map[string]interface{}
		err := yaml.Unmarshal([]byte(postProdSteps), &tasks)
		if err != nil {
			// Log the error but don't fail - use default
			fmt.Printf("Warning: Failed to parse post-prod-steps YAML: %v\n", err)
		} else {
			// Extract all task names
			for _, task := range tasks {
				if name, ok := task["name"].(string); ok {
					prodTaskNames = append(prodTaskNames, name)
				}
			}

			// Set the last task name for backward compatibility
			if len(prodTaskNames) > 0 {
				prodTaskName = prodTaskNames[len(prodTaskNames)-1]
			}
		}
	}

	// Format post-dev steps for pipeline inclusion
	formattedPostDevSteps := ""
	if postDevSteps != "" {
		fmt.Printf("DEBUG: Formatting post-dev steps: %s\n", postDevSteps)
		var err error
		formattedPostDevSteps, err = formatTasksYAML(postDevSteps)
		if err != nil {
			// Log the error but don't fail - use empty string
			fmt.Printf("WARNING: Failed to format post-dev-steps: %v\n", err)
		} else {
			fmt.Printf("DEBUG: Formatted post-dev steps: %s\n", formattedPostDevSteps)
		}
	}

	// Format post-prod steps for pipeline inclusion
	formattedPostProdSteps := ""
	if postProdSteps != "" {
		fmt.Printf("DEBUG: Formatting post-prod steps: %s\n", postProdSteps)
		var err error
		formattedPostProdSteps, err = formatTasksYAML(postProdSteps)
		if err != nil {
			// Log the error but don't fail - use empty string
			fmt.Printf("WARNING: Failed to format post-prod-steps: %v\n", err)
		} else {
			fmt.Printf("DEBUG: Formatted post-prod steps: %s\n", formattedPostProdSteps)
		}
	}

	// Define template data with properly formatted steps
	templateData := map[string]interface{}{
		"PostDevSteps":  formattedPostDevSteps,
		"PostProdSteps": formattedPostProdSteps,
		"DevTaskName":   devTaskName,
		"ProdTaskName":  prodTaskName,
		"DevTaskNames":  devTaskNames,
		"ProdTaskNames": prodTaskNames,
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
