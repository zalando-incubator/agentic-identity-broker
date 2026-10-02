package main

import (
	"context"
	"errors"
	"fmt"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
	"github.com/spf13/cobra"
)

func newRefreshSessionsCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "refresh-sessions",
		Short: "Offline local refresh-session maintenance",
		Args:  cobra.NoArgs,
	}
	command.AddCommand(&cobra.Command{
		Use:   "invalidate-restored",
		Short: "Invalidate local refresh authority after a restore",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) (runErr error) {
			loader := config.NewLoader()
			loader.SetCommand(cmd)
			cfg, err := loader.GetConfig(cmd.Context())
			if err != nil {
				return fmt.Errorf("load restore configuration: %w", err)
			}
			logger := initializeLogger(cfg.Log)
			store, err := storageadapter.NewAdapter(&cfg.Storage)
			if err != nil {
				return fmt.Errorf("open restore storage: %w", err)
			}
			defer func() {
				if err := store.Close(context.Background()); err != nil {
					runErr = errors.Join(runErr, fmt.Errorf("close restore storage: %w", err))
				}
			}()
			maintenance, err := app.NewBuilder().WithConfig(cfg).WithStorage(store).WithLogger(logger).BuildRefreshSessionMaintenance()
			if err != nil {
				return fmt.Errorf("build restore maintenance: %w", err)
			}
			if err := maintenance.InvalidateRestored(cmd.Context()); err != nil {
				return fmt.Errorf("restore invalidation failed: %w", err)
			}
			return nil
		},
	})
	return command
}
