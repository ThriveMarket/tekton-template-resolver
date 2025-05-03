package main

import (
	"os"
	"testing"
	"time"
	
	"github.com/stretchr/testify/assert"
)

func TestConfigEnvHelpers(t *testing.T) {
	// Test getEnvWithDefault
	os.Setenv("TEST_ENV_VAR", "test-value")
	assert.Equal(t, "test-value", getEnvWithDefault("TEST_ENV_VAR", "default-value"))
	assert.Equal(t, "default-value", getEnvWithDefault("NONEXISTENT_ENV_VAR", "default-value"))
	
	// Test getEnvWithDefaultInt
	os.Setenv("TEST_INT_VAR", "123")
	assert.Equal(t, 123, getEnvWithDefaultInt("TEST_INT_VAR", 456))
	assert.Equal(t, 456, getEnvWithDefaultInt("NONEXISTENT_INT_VAR", 456))
	
	// Test with invalid int
	os.Setenv("TEST_INVALID_INT", "not-an-int")
	assert.Equal(t, 789, getEnvWithDefaultInt("TEST_INVALID_INT", 789))
	
	// Test getEnvWithDefaultDuration
	os.Setenv("TEST_DURATION_VAR", "10s")
	assert.Equal(t, 10*time.Second, getEnvWithDefaultDuration("TEST_DURATION_VAR", 20*time.Second))
	assert.Equal(t, 20*time.Second, getEnvWithDefaultDuration("NONEXISTENT_DURATION_VAR", 20*time.Second))
	
	// Test with invalid duration
	os.Setenv("TEST_INVALID_DURATION", "not-a-duration")
	assert.Equal(t, 30*time.Second, getEnvWithDefaultDuration("TEST_INVALID_DURATION", 30*time.Second))
	
	// Clean up
	os.Unsetenv("TEST_ENV_VAR")
	os.Unsetenv("TEST_INT_VAR")
	os.Unsetenv("TEST_INVALID_INT")
	os.Unsetenv("TEST_DURATION_VAR")
	os.Unsetenv("TEST_INVALID_DURATION")
}

func TestDebugf(t *testing.T) {
	// There's not much we can test here without mocking log.Printf
	// or capturing stdout, but we can at least ensure it doesn't panic
	
	// Test with debug mode off
	debugMode = false
	debugf("This should not be printed")
	
	// Test with debug mode on
	debugMode = true
	debugf("This should be printed")
	
	// Reset
	debugMode = false
}