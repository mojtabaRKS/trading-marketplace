package main

import (
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/herotech/market-dragon/internal/infra/database"
)

func migrateCmd() *cobra.Command {
	var down int
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply (up) or roll back (--down N) database migrations",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, logger, err := bootstrap()
			if err != nil {
				return err
			}
			if down > 0 {
				if err := database.MigrateDown(cfg.DatabaseURL(), down); err != nil {
					return err
				}
				logger.Info("migrations rolled back", slog.Int("steps", down))
				return nil
			}
			if err := database.Migrate(cfg.DatabaseURL()); err != nil {
				return err
			}
			logger.Info("migrations applied")
			return nil
		},
	}
	cmd.Flags().IntVar(&down, "down", 0, "roll back N migration steps instead of applying all up")
	return cmd
}
