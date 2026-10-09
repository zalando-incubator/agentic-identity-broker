package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/migrationguard"
)

const migrateBinary = "/usr/local/bin/migrate"

func main() {
	args := os.Args[1:]
	// A sole informational flag cannot execute SQL; keep the image's default help usable.
	if len(args) != 1 || !informationalFlag(args[0]) {
		dir, databaseURL, err := migrationArgs(args)
		if err != nil {
			fmt.Fprintln(os.Stderr, "migration guard:", err)
			os.Exit(1)
		}
		if err := migrationguard.Validate(dir, databaseURL); err != nil {
			// Validator I/O errors can include user-supplied paths; never log credentials.
			fmt.Fprintln(os.Stderr, "migration guard: migration validation failed")
			os.Exit(1)
		}
	}

	if err := syscall.Exec(migrateBinary, append([]string{migrateBinary}, args...), os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "migration guard: failed to start migrate")
		os.Exit(1)
	}
}

func informationalFlag(arg string) bool {
	switch arg {
	case "-help", "--help", "-h", "-version", "--version":
		return true
	default:
		return false
	}
}

func migrationArgs(args []string) (dir, databaseURL string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-source" || arg == "--source" || strings.HasPrefix(arg, "-source=") || strings.HasPrefix(arg, "--source=") {
			return "", "", fmt.Errorf("-source bypasses the guarded migration directory")
		}

		var flag, value string
		var separateValue bool
		switch {
		case arg == "-path" || arg == "--path":
			flag, separateValue = "-path", true
		case arg == "-database" || arg == "--database":
			flag, separateValue = "-database", true
		case strings.HasPrefix(arg, "-path="):
			flag, value = "-path", strings.TrimPrefix(arg, "-path=")
		case strings.HasPrefix(arg, "--path="):
			flag, value = "-path", strings.TrimPrefix(arg, "--path=")
		case strings.HasPrefix(arg, "-database="):
			flag, value = "-database", strings.TrimPrefix(arg, "-database=")
		case strings.HasPrefix(arg, "--database="):
			flag, value = "-database", strings.TrimPrefix(arg, "--database=")
		default:
			continue
		}

		if separateValue {
			if i+1 >= len(args) || args[i+1] == "" || strings.HasPrefix(args[i+1], "-") {
				return "", "", fmt.Errorf("missing %s value", flag)
			}
			i++
			value = args[i]
		}
		if value == "" {
			return "", "", fmt.Errorf("missing %s value", flag)
		}
		switch flag {
		case "-path":
			if dir != "" {
				return "", "", fmt.Errorf("duplicate -path flag")
			}
			dir = value
		case "-database":
			if databaseURL != "" {
				return "", "", fmt.Errorf("duplicate -database flag")
			}
			databaseURL = value
		}
	}
	if dir == "" || databaseURL == "" {
		return "", "", fmt.Errorf("-path and -database are required")
	}
	if dir != "/app/migrations" {
		return "", "", fmt.Errorf("migration path must be /app/migrations")
	}
	return dir, databaseURL, nil
}
