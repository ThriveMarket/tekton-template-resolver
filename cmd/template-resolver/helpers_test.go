package main

import (
	"testing"
	
	"github.com/stretchr/testify/assert"
)

// TestToCamelCase tests the toCamelCase function
func TestToCamelCase(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "post-dev-steps",
			expected: "PostDevSteps",
		},
		{
			input:    "single",
			expected: "Single",
		},
		{
			input:    "multiple-word-parameter",
			expected: "MultipleWordParameter",
		},
		{
			input:    "security-audit-steps",
			expected: "SecurityAuditSteps",
		},
		{
			input:    "with-numbers-123",
			expected: "WithNumbers123",
		},
		{
			input:    "already-Capitalized-Words",
			expected: "AlreadyCapitalizedWords",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := toCamelCase(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestMapLegacyNames tests the mapLegacyNames function
func TestMapLegacyNames(t *testing.T) {
	// Test with empty map
	t.Run("EmptyMap", func(t *testing.T) {
		data := make(map[string]interface{})
		mapLegacyNames(data)
		
		// Check that defaults were added
		assert.Equal(t, "", data["PostDevSteps"])
		assert.Equal(t, "", data["PostProdSteps"])
		assert.Equal(t, []string{}, data["DevTaskNames"])
		assert.Equal(t, []string{}, data["ProdTaskNames"])
		assert.Equal(t, "default-dev-validation", data["DevTaskName"])
		assert.Equal(t, "default-prod-validation", data["ProdTaskName"])
	})
	
	// Test with existing DevTaskNames
	t.Run("WithExistingNames", func(t *testing.T) {
		data := map[string]interface{}{
			"DevTaskNames":  []string{"task1", "task2"},
			"ProdTaskNames": []string{"task3"},
		}
		mapLegacyNames(data)
		
		// Check that we set DevTaskName to the last task
		assert.Equal(t, "task2", data["DevTaskName"])
		assert.Equal(t, "task3", data["ProdTaskName"])
	})
	
	// Test with existing values
	t.Run("WithExistingValues", func(t *testing.T) {
		data := map[string]interface{}{
			"PostDevSteps":  "existing-dev",
			"PostProdSteps": "existing-prod",
			"DevTaskName":   "existing-dev-name",
			"ProdTaskName":  "existing-prod-name",
			"DevTaskNames":  []string{"task1"},
			"ProdTaskNames": []string{"task2"},
		}
		mapLegacyNames(data)
		
		// Check that existing values were preserved
		assert.Equal(t, "existing-dev", data["PostDevSteps"])
		assert.Equal(t, "existing-prod", data["PostProdSteps"])
		assert.Equal(t, "existing-dev-name", data["DevTaskName"])
		assert.Equal(t, "existing-prod-name", data["ProdTaskName"])
	})
}