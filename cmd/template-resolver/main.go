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
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/pipeline/pkg/resolution/common"
	"github.com/tektoncd/pipeline/pkg/resolution/resolver/framework"
	"gopkg.in/yaml.v3"
	"knative.dev/pkg/injection/sharedmain"
)

// Configuration constants with defaults
const (
	// Environment variable names
	EnvDebug             = "DEBUG"
	EnvHTTPTimeout       = "HTTP_TIMEOUT"
	EnvResolutionTimeout = "RESOLUTION_TIMEOUT"
	EnvGitCloneDepth     = "GIT_CLONE_DEPTH"
	EnvGitBranch         = "GIT_DEFAULT_BRANCH"
	
	// Default values
	DefaultHTTPTimeout       = 30 * time.Second
	DefaultResolutionTimeout = 60 * time.Second
	DefaultGitCloneDepth     = 1
	DefaultGitBranch         = "main"
)

// Global config flags
var (
	debugMode bool
	httpTimeout time.Duration
	resolutionTimeout time.Duration
	gitCloneDepth int
	gitDefaultBranch string
)

// debugf prints debug messages only when debug mode is enabled
func debugf(format string, args ...interface{}) {
	if debugMode {
		log.Printf(format, args...)
	}
}

// getEnvWithDefault gets an environment variable value or returns the default if not set
func getEnvWithDefault(key string, defaultValue string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultValue
}

// getEnvWithDefaultInt gets an environment variable as int or returns the default if not set
func getEnvWithDefaultInt(key string, defaultValue int) int {
	if val, ok := os.LookupEnv(key); ok {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
		log.Printf("WARNING: Invalid value for %s, using default: %d", key, defaultValue)
	}
	return defaultValue
}

// getEnvWithDefaultDuration gets an environment variable as duration or returns default
func getEnvWithDefaultDuration(key string, defaultValue time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		if duration, err := time.ParseDuration(val); err == nil {
			return duration
		}
		log.Printf("WARNING: Invalid value for %s, using default: %v", key, defaultValue)
	}
	return defaultValue
}

func main() {
	// Check for standalone mode before any flag parsing
	// This allows us to handle flags differently in each mode
	isStandalone := false
	standalonePort := 8080
	
	// Pre-scan args for standalone flag without using the flag package
	for i, arg := range os.Args {
		if arg == "-standalone" || arg == "--standalone" {
			isStandalone = true
		} else if (arg == "-port" || arg == "--port") && i+1 < len(os.Args) {
			if port, err := strconv.Atoi(os.Args[i+1]); err == nil {
				standalonePort = port
			}
		} else if arg == "-debug" || arg == "--debug" {
			debugMode = true
		}
	}
	
	// Check environment variable for debug mode
	if debugEnv := getEnvWithDefault(EnvDebug, ""); debugEnv == "true" || debugEnv == "1" {
		debugMode = true
	}
	
	// Load configuration from environment variables
	httpTimeout = getEnvWithDefaultDuration(EnvHTTPTimeout, DefaultHTTPTimeout)
	resolutionTimeout = getEnvWithDefaultDuration(EnvResolutionTimeout, DefaultResolutionTimeout)
	gitCloneDepth = getEnvWithDefaultInt(EnvGitCloneDepth, DefaultGitCloneDepth)
	gitDefaultBranch = getEnvWithDefault(EnvGitBranch, DefaultGitBranch)
	
	if debugMode {
		log.Println("Debug mode enabled")
		log.Printf("Configuration: HTTP Timeout=%v, Resolution Timeout=%v, Git Clone Depth=%d, Git Default Branch=%s",
			httpTimeout, resolutionTimeout, gitCloneDepth, gitDefaultBranch)
	}
	
	// Create a new resolver instance
	resolver := NewResolver()
	
	// Initialize the resolver
	if err := resolver.Initialize(context.Background()); err != nil {
		log.Fatalf("Failed to initialize resolver: %v", err)
	}
	
	// Choose between standalone mode and Knative mode
	if isStandalone {
		// In standalone mode, explicitly parse our own flags
		fs := flag.NewFlagSet("standalone", flag.ExitOnError)
		fs.BoolVar(&debugMode, "debug", debugMode, "Enable debug logging")
		_ = fs.Int("port", standalonePort, "Port to listen on in standalone mode")
		_ = fs.Bool("standalone", true, "Run in standalone mode without Knative")
		if err := fs.Parse(os.Args[1:]); err != nil {
			log.Fatalf("Error parsing flags: %v", err)
		}
		
		runStandalone(resolver, standalonePort)
	} else {
		// In Knative mode, let Knative handle all flag parsing
		// Don't register our own flags, let Knative control them
		sharedmain.Main("controller",
			framework.NewController(context.Background(), resolver),
		)
	}
}

// runStandalone starts a simple HTTP server that can process template resolution requests
// without requiring the Knative/Tekton infrastructure
func runStandalone(resolver *resolver, port int) {
	log.Printf("Starting standalone server on port %d", port)
	
	http.HandleFunc("/resolve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to read request body: %v", err), http.StatusBadRequest)
			return
		}
		
		var request struct {
			Parameters []pipelinev1.Param `json:"parameters"`
		}
		
		if err := json.Unmarshal(body, &request); err != nil {
			http.Error(w, fmt.Sprintf("Failed to parse request: %v", err), http.StatusBadRequest)
			return
		}
		
		// Validate parameters
		if err := resolver.ValidateParams(r.Context(), request.Parameters); err != nil {
			http.Error(w, fmt.Sprintf("Invalid parameters: %v", err), http.StatusBadRequest)
			return
		}
		
		// Resolve the template
		result, err := resolver.Resolve(r.Context(), request.Parameters)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to resolve template: %v", err), http.StatusInternalServerError)
			return
		}
		
		// Return the resolved template
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(result.Data()); err != nil {
			log.Printf("Error writing response: %v", err)
		}
	})
	
	// Add a health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprintln(w, "OK"); err != nil {
			log.Printf("Error writing health response: %v", err)
		}
	})
	
	// Add a readiness endpoint
	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprintln(w, "Ready"); err != nil {
			log.Printf("Error writing readiness response: %v", err)
		}
	})
	
	// Start the server
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
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

		// Create an HTTP client with timeout
		client := &http.Client{
			Timeout: httpTimeout,
		}

		// First try with the filename
		rawURL := fmt.Sprintf("https://gist.githubusercontent.com/%s/%s/raw/%s", user, gistID, filePath)
		debugf("Fetching Gist from URL: %s", rawURL)

		// First check if we can fetch with the filename
		resp, err := client.Get(rawURL)
		if err != nil {
			return "", fmt.Errorf("failed to fetch gist: %w", err)
		}

		// If we got a 404, try without the filename (for single-file gists)
		if resp.StatusCode == http.StatusNotFound {
			if err := resp.Body.Close(); err != nil { // Close this response before making another request
				return "", fmt.Errorf("failed to close response body: %w", err)
			}

			// Try without filename for single-file gists
			rawURL = fmt.Sprintf("https://gist.githubusercontent.com/%s/%s/raw/", user, gistID)
			debugf("File not found with name, trying single-file Gist URL: %s", rawURL)
			resp, err = client.Get(rawURL)
			if err != nil {
				return "", fmt.Errorf("failed to fetch single-file gist: %w", err)
			}
		}

		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
				err = fmt.Errorf("failed to close response body: %w", closeErr)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("HTTP error fetching Gist: %s", resp.Status)
		}

		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read Gist content: %w", err)
		}

		debugf("Successfully fetched Gist content (%d bytes)", len(content))
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
		repoURL += gitDefaultBranch + "/" // Use configured default branch

		// Construct the full URL to the raw file
		fileURL := repoURL + filePath
		debugf("Fetching GitHub file from URL: %s", fileURL)

		// Create an HTTP client with timeout
		client := &http.Client{
			Timeout: httpTimeout,
		}

		// Fetch the content
		resp, err := client.Get(fileURL)
		if err != nil {
			return "", fmt.Errorf("failed to fetch GitHub file: %w", err)
		}
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
				err = fmt.Errorf("failed to close response body: %w", closeErr)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("HTTP error fetching GitHub file: %s", resp.Status)
		}

		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read GitHub file content: %w", err)
		}

		debugf("Successfully fetched GitHub file content (%d bytes)", len(content))
		return string(content), nil
	}

	// Handle Git repositories (public or private)
	// If private, the GIT_SSH_COMMAND env var should be set in the deployment
	// to use the mounted SSH key
	tempDir, err := os.MkdirTemp("", "template-resolver-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil && err == nil {
			err = fmt.Errorf("failed to clean up temp directory: %w", removeErr)
		}
	}()

	// Setup git command with output capturing and configurable depth
	cloneCmd := fmt.Sprintf("--depth=%d", gitCloneDepth)
	debugf("Cloning Git repository %s with %s", repoURL, cloneCmd)
	cmd := exec.Command("git", "clone", cloneCmd, repoURL, tempDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Create a context with timeout for the git command
	ctx, cancel := context.WithTimeout(context.Background(), resolutionTimeout)
	defer cancel()
	cmd = exec.CommandContext(ctx, "git", "clone", cloneCmd, repoURL, tempDir)
	cmd.Stderr = &stderr

	// Attempt to clone the repository
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("git clone timed out after %v", resolutionTimeout)
		}
		return "", fmt.Errorf("git clone failed: %w, stderr: %s", err, stderr.String())
	}

	// Read the requested file from the cloned repo
	filePath = filepath.Join(tempDir, filePath)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	debugf("Successfully read file from Git repository (%d bytes)", len(content))
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
		"fromYAML": func(yamlStr string) interface{} {
			// Handle empty strings
			if strings.TrimSpace(yamlStr) == "" {
				return nil
			}
			
			// Parse the YAML string into a structured object
			var result interface{}
			err := yaml.Unmarshal([]byte(yamlStr), &result)
			if err != nil {
				debugf("Error parsing YAML with fromYAML function: %v", err)
				// Return a map with error information
				return map[string]string{
					"error": fmt.Sprintf("Error parsing YAML: %v", err),
				}
			}
			
			debugf("Successfully parsed YAML with fromYAML function: %v", result)
			return result
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
		"last": func(obj map[string]interface{}, key string) bool {
			// Determine if this is the last key in a map (for comma handling in JSON)
			if obj == nil {
				return false
			}
			
			// Get all keys from the map
			keys := make([]string, 0, len(obj))
			for k := range obj {
				keys = append(keys, k)
			}
			
			// Sort keys to ensure consistent order
			sort.Strings(keys)
			
			// Check if the given key is the last one
			return keys[len(keys)-1] == key
		},
		"toYAML": func(obj interface{}) string {
			// Convert an object back to a YAML string for template inclusion
			if obj == nil {
				return ""
			}
			
			// Marshal the object to YAML
			yamlBytes, err := yaml.Marshal(obj)
			if err != nil {
				debugf("Error converting object to YAML with toYAML function: %v", err)
				return fmt.Sprintf("Error: %v", err)
			}
			
			// Convert to string and clean up
			yamlStr := string(yamlBytes)
			
			// Remove the document separator and ensure proper indentation
			yamlStr = strings.TrimPrefix(yamlStr, "---\n")
			
			// Remove the leading dash for items in a list (will be added by the template)
			if strings.HasPrefix(yamlStr, "- ") {
				yamlStr = yamlStr[2:]
			}
			
			// Trim trailing newline
			yamlStr = strings.TrimSpace(yamlStr)
			
			debugf("toYAML function result: %s", yamlStr)
			return yamlStr
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
		// Using manual capitalization instead of deprecated strings.Title
		if len(parts[i]) > 0 {
			r := []rune(parts[i])
			r[0] = unicode.ToUpper(r[0])
			parts[i] = string(r)
		}
	}
	return strings.Join(parts, "")
}

