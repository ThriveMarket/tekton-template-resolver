package main

import (
	"bytes"
	"context"
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
	EnvironmentParam = "environment"
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
	// Extract parameters
	var repository, path, postDevSteps, postProdSteps, environment string
	for _, param := range params {
		switch param.Name {
		case RepositoryParam:
			repository = param.Value.StringVal
		case PathParam:
			path = param.Value.StringVal
		case PostDevParam:
			postDevSteps = param.Value.StringVal
		case PostProdParam:
			postProdSteps = param.Value.StringVal
		case EnvironmentParam:
			environment = param.Value.StringVal
		}
	}

	// Default environment to dev if not specified
	if environment == "" {
		environment = "dev"
	}

	// Fetch template from Git repository
	templateContent, err := fetchTemplate(repository, path)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch template: %w", err)
	}

	// Define template data
	templateData := map[string]interface{}{
		"PostDevSteps":  postDevSteps,
		"PostProdSteps": postProdSteps,
		"Environment":   environment,
	}

	// Render the template
	renderedTemplate, err := renderTemplate(templateContent, templateData)
	if err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
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

// renderTemplate applies Go template processing to the template content
func renderTemplate(templateContent string, data map[string]interface{}) (string, error) {
	tmpl, err := template.New("pipeline").Parse(templateContent)
	if err != nil {
		return "", err
	}
	
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	
	return buf.String(), nil
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