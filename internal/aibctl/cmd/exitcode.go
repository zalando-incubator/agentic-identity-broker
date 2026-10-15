package aibctlcmd

import (
	"errors"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/auth"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/client"
)

const (
	ExitOK         = 0
	ExitGeneric    = 1
	ExitUsage      = 2
	ExitAuth       = 3
	ExitNotFound   = 4
	ExitConflict   = 5
	ExitValidation = 6
)

// UsageError covers invalid arguments, flags, input documents, and configuration.
type UsageError struct {
	Err error
	Cmd *cobra.Command
}

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// PreconditionError reports a state the command refuses to change, such as
// creating credentials that already exist.
type PreconditionError struct{ Err error }

func (e *PreconditionError) Error() string { return e.Err.Error() }
func (e *PreconditionError) Unwrap() error { return e.Err }

// ExitCode maps an error to the documented process exit codes.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		return ExitUsage
	}
	var preconditionErr *PreconditionError
	if errors.As(err, &preconditionErr) {
		return ExitConflict
	}
	var authErr *auth.Error
	if errors.As(err, &authErr) {
		return ExitAuth
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return ExitAuth
		case http.StatusNotFound:
			return ExitNotFound
		case http.StatusConflict, http.StatusPreconditionFailed, http.StatusPreconditionRequired:
			return ExitConflict
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			return ExitValidation
		}
	}
	return ExitGeneric
}
