package main

import (
	"log"
	"os"
	"strconv"
	"time"
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
	debugMode         bool
	httpTimeout       time.Duration
	resolutionTimeout time.Duration
	gitCloneDepth     int
	gitDefaultBranch  string
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
