package main

import (
	"strings"
	"unicode"
)

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
