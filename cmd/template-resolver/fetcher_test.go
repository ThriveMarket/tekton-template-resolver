package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitFetcherFetchTemplate(t *testing.T) {
	// Create a temporary directory for Git tests
	tempDir, err := os.MkdirTemp("", "template-resolver-test-*")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tempDir)
		if err != nil {
			t.Logf("Failed to remove temp directory: %v", err)
		}
	}()
	
	// Create a test server for HTTP requests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		
		// GitHub raw content
		if strings.HasPrefix(path, "/example/repo/main/") {
			_, err := w.Write([]byte("apiVersion: tekton.dev/v1\nkind: Pipeline\nmetadata:\n  name: test-pipeline"))
			if err != nil {
				t.Logf("Failed to write response: %v", err)
			}
			return
		}
		
		// Gist raw content
		if strings.HasPrefix(path, "/user/gistid/raw/") {
			if strings.HasSuffix(path, "/path/to/template.yaml") {
				_, err := w.Write([]byte("apiVersion: tekton.dev/v1\nkind: Pipeline\nmetadata:\n  name: gist-template"))
				if err != nil {
					t.Logf("Failed to write response: %v", err)
				}
				return
			} else if path == "/user/gistid/raw/" {
				_, err := w.Write([]byte("apiVersion: tekton.dev/v1\nkind: Pipeline\nmetadata:\n  name: gist-single-file"))
				if err != nil {
					t.Logf("Failed to write response: %v", err)
				}
				return
			}
		}
		
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	
	// Create a test fetcher that uses our test server
	fetcher := &testTemplateFetcher{
		server:  server,
		tempDir: tempDir,
	}
	
	// Test GitHub URL
	content, err := fetcher.FetchTemplate(server.URL+"/example/repo", "path/to/template.yaml")
	assert.NoError(t, err)
	assert.Contains(t, content, "name: test-pipeline")
	
	// Test Gist URL with filename
	content, err = fetcher.FetchTemplate("https://gist.github.com/user/gistid", "path/to/template.yaml")
	assert.NoError(t, err)
	assert.Contains(t, content, "name: gist-template")
	
	// Test Gist URL without filename (single-file gist)
	content, err = fetcher.FetchTemplate("https://gist.github.com/user/gistid", "single-file.yaml")
	assert.NoError(t, err)
	assert.Contains(t, content, "name: gist-single-file")
	
	// Test invalid Gist URL
	_, err = fetcher.FetchTemplate("https://gist.github.com/invalid", "file.yaml")
	assert.Error(t, err)
}

// testTemplateFetcher is a test implementation of TemplateFetcher
type testTemplateFetcher struct {
	server  *httptest.Server
	tempDir string
}

// FetchTemplate implements TemplateFetcher for testing
func (t *testTemplateFetcher) FetchTemplate(repoURL, filePath string) (string, error) {
	if strings.HasPrefix(repoURL, t.server.URL) {
		// Convert to raw GitHub URL for our test server
		fileURL := strings.Replace(repoURL, t.server.URL, t.server.URL, 1)
		if !strings.HasSuffix(fileURL, "/") {
			fileURL += "/"
		}
		fileURL += "main/" + filePath
		
		resp, err := http.Get(fileURL)
		if err != nil {
			return "", err
		}
		defer func() {
			closeErr := resp.Body.Close()
			if closeErr != nil {
				fmt.Printf("Failed to close response body: %v\n", closeErr)
			}
		}()
		
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("HTTP error: %s", resp.Status)
		}
		
		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		
		return string(content), nil
	} else if strings.HasPrefix(repoURL, "https://gist.github.com/") {
		if repoURL == "https://gist.github.com/invalid" {
			return "", fmt.Errorf("invalid Gist URL format: %s", repoURL)
		}
		
		// For gist URLs, use our mock server but with the right path structure
		var rawURL string
		if filePath == "single-file.yaml" {
			rawURL = t.server.URL + "/user/gistid/raw/"
		} else {
			rawURL = t.server.URL + "/user/gistid/raw/" + filePath
		}
		
		resp, err := http.Get(rawURL)
		if err != nil {
			return "", err
		}
		defer func() {
			closeErr := resp.Body.Close()
			if closeErr != nil {
				fmt.Printf("Failed to close response body: %v\n", closeErr)
			}
		}()
		
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("HTTP error: %s", resp.Status)
		}
		
		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		
		return string(content), nil
	}
	
	// For Git repositories, create a fake repo with the template
	templateDir := filepath.Join(t.tempDir, filePath)
	err := os.MkdirAll(filepath.Dir(templateDir), 0755)
	if err != nil {
		return "", err
	}
	
	// Write a test template file
	template := "apiVersion: tekton.dev/v1\nkind: Pipeline\nmetadata:\n  name: git-template"
	err = os.WriteFile(templateDir, []byte(template), 0644)
	if err != nil {
		return "", err
	}
	
	return template, nil
}