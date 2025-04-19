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