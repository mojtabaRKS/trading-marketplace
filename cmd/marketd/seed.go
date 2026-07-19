package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/herotech/market-dragon/internal/infra/database"
	"github.com/herotech/market-dragon/internal/repository"
)

func seedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Load development seed data",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, logger, err := bootstrap()
			if err != nil {
				return err
			}
			db, err := database.Open(cfg.DSN())
			if err != nil {
				return fmt.Errorf("connect database: %w", err)
			}
			if err := repository.Seed(db); err != nil {
				return fmt.Errorf("seed: %w", err)
			}
			logger.Info("seed data loaded")
			return nil
		},
	}
}
