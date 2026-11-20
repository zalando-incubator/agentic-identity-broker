// Package logging holds deliberate violations of the coding-standards/LOGGING.md lint rules.
package logging

import (
	"context"
	"fmt"
	"log"
	"log/slog"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

var global = slog.New(slog.DiscardHandler)

func Plumbing(ctx context.Context, logger *slog.Logger) {
	logger.InfoContext(ctx, "grant revoked", "event", "consent.grant.revoked")
	logger.Info("grant revoked")                     // want sloglint `InfoContext should be used`
	slog.InfoContext(ctx, "grant revoked")           // want sloglint `default logger should not be used`
	slog.Default().InfoContext(ctx, "grant revoked") // want forbidigo `LOGGING\.md section 1`
	global.InfoContext(ctx, "grant revoked")         // want sloglint `global logger should not be used`
	fmt.Println("grant revoked")                     // want forbidigo `LOGGING\.md section 1`
	log.Printf("grant %s revoked", "g1")             // want forbidigo `LOGGING\.md section 1`
}

func Shape(ctx context.Context, logger *slog.Logger, agentID string, err error) {
	logger.InfoContext(ctx, fmt.Sprintf("grant for %s revoked", agentID)) // want sloglint `message should be a string literal or a constant`
	logger.InfoContext(ctx, "Grant revoked")                              // want sloglint `message should be lowercased`
	logger.InfoContext(ctx, "grant revoked", "agentID", agentID)          // want sloglint `keys should be written in snake_case`
	logger.WarnContext(ctx, "grant revoke failed", "err", err)            // want sloglint `"err" key is forbidden`
	logger.WarnContext(ctx, "grant revoke failed", "cause", err)          // want sloglint `"cause" key is forbidden`
	logger.InfoContext(ctx, "grant revoked", "action", "grant_revoked")   // want sloglint `"action" key is forbidden`
	logger.WarnContext(ctx, "grant revoke failed", "error", err)
}

func Principals(ctx context.Context, logger *slog.Logger, p id.Principal, s model.Secret) error {
	logger.InfoContext(ctx, "grant revoked", "principal", p)
	logger.InfoContext(ctx, "grant revoked", slog.String("principal", "someone")) // want gocritic `slog\.String under a reserved principal key`
	logger.InfoContext(ctx, "grant revoked", slog.String("owner", p.String()))    // want gocritic `principal logged as a string`
	logger.InfoContext(ctx, "client registered", "client_secret", s)              // want telemetrylint `credential: a Secret value must not be`
	return fmt.Errorf("grant for %s not found", p)                                // want telemetrylint `principal-sink: a principal must not be in an error string`
}
