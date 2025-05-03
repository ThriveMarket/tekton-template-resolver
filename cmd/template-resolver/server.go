package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	pipelinev1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

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
