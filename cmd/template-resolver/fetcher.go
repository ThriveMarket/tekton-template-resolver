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
)

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
