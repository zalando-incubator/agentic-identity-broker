// Package main is the entry point for aibctl, the broker admin CLI.
package main

import (
	"os"

	aibctlcmd "github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/cmd"
)

func main() {
	os.Exit(aibctlcmd.Execute(aibctlcmd.Deps{Stdout: os.Stdout, Stderr: os.Stderr, Stdin: os.Stdin}, os.Args[1:]))
}
