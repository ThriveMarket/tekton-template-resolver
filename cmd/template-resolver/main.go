package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strconv"

	"github.com/tektoncd/pipeline/pkg/resolution/resolver/framework"
	"knative.dev/pkg/injection/sharedmain"
)

func main() {
	// Check for standalone mode before any flag parsing
	// This allows us to handle flags differently in each mode
	isStandalone := false
	standalonePort := 8080

	// Pre-scan args for standalone flag without using the flag package
	for i, arg := range os.Args {
		if arg == "-standalone" || arg == "--standalone" {
			isStandalone = true
		} else if (arg == "-port" || arg == "--port") && i+1 < len(os.Args) {
			if port, err := strconv.Atoi(os.Args[i+1]); err == nil {
				standalonePort = port
			}
		} else if arg == "-debug" || arg == "--debug" {
			debugMode = true
		}
	}

	// Check environment variable for debug mode
	if debugEnv := getEnvWithDefault(EnvDebug, ""); debugEnv == "true" || debugEnv == "1" {
		debugMode = true
	}

	// Load configuration from environment variables
	httpTimeout = getEnvWithDefaultDuration(EnvHTTPTimeout, DefaultHTTPTimeout)
	resolutionTimeout = getEnvWithDefaultDuration(EnvResolutionTimeout, DefaultResolutionTimeout)
	gitCloneDepth = getEnvWithDefaultInt(EnvGitCloneDepth, DefaultGitCloneDepth)
	gitDefaultBranch = getEnvWithDefault(EnvGitBranch, DefaultGitBranch)

	if debugMode {
		log.Println("Debug mode enabled")
		log.Printf("Configuration: HTTP Timeout=%v, Resolution Timeout=%v, Git Clone Depth=%d, Git Default Branch=%s",
			httpTimeout, resolutionTimeout, gitCloneDepth, gitDefaultBranch)
	}

	// Create a new resolver instance
	resolver := NewResolver()

	// Initialize the resolver
	if err := resolver.Initialize(context.Background()); err != nil {
		log.Fatalf("Failed to initialize resolver: %v", err)
	}

	// Choose between standalone mode and Knative mode
	if isStandalone {
		// In standalone mode, explicitly parse our own flags
		fs := flag.NewFlagSet("standalone", flag.ExitOnError)
		fs.BoolVar(&debugMode, "debug", debugMode, "Enable debug logging")
		_ = fs.Int("port", standalonePort, "Port to listen on in standalone mode")
		_ = fs.Bool("standalone", true, "Run in standalone mode without Knative")
		if err := fs.Parse(os.Args[1:]); err != nil {
			log.Fatalf("Error parsing flags: %v", err)
		}

		runStandalone(resolver, standalonePort)
	} else {
		// In Knative mode, let Knative handle all flag parsing
		// Don't register our own flags, let Knative control them
		sharedmain.Main("controller",
			framework.NewController(context.Background(), resolver),
		)
	}
}
