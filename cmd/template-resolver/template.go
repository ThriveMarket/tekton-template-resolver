package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

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
		"typeIs": func(typeName string, val interface{}) bool {
			return strings.Contains(fmt.Sprintf("%T", val), typeName)
		},
		"toString": func(val interface{}) string {
			// Convert any value to a string
			switch v := val.(type) {
			case string:
				return v
			case []byte:
				return string(v)
			case error:
				return v.Error()
			case fmt.Stringer:
				return v.String()
			default:
				if val == nil {
					return ""
				}

				// Try to marshal to JSON
				if bytes, err := json.Marshal(val); err == nil {
					return string(bytes)
				}

				// Fallback to %v formatting
				return fmt.Sprintf("%v", val)
			}
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

			// Remove the document separator
			yamlStr = strings.TrimPrefix(yamlStr, "---\n")

			// Remove the leading dash for items in a list (will be added by the template)
			yamlStr = strings.TrimPrefix(yamlStr, "- ")

			// Process each line to normalize indentation
			lines := strings.Split(yamlStr, "\n")

			// Find the minimum indentation level (ignore empty lines)
			minIndent := -1
			for _, line := range lines {
				if len(strings.TrimSpace(line)) == 0 {
					continue // Skip empty lines
				}

				// Count leading spaces
				indent := len(line) - len(strings.TrimLeft(line, " "))
				if minIndent == -1 || indent < minIndent {
					minIndent = indent
				}
			}

			// Remove the minimum indentation from each line
			if minIndent > 0 {
				for i, line := range lines {
					if len(line) >= minIndent {
						lines[i] = line[minIndent:]
					}
				}
			}

			// Reassemble the YAML string and trim any trailing whitespace
			yamlStr = strings.Join(lines, "\n")
			yamlStr = strings.TrimSpace(yamlStr)

			debugf("toYAML function result after indentation fix: %s", yamlStr)
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
