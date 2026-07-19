package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/herotech/market-dragon/internal/config"
	"github.com/herotech/market-dragon/internal/infra/logging"
)

var rootCmd = &cobra.Command{
	Use:           "marketd",
	Short:         "Market Dragon — secure trading & auction marketplace",
	SilenceUsage:  true,
	SilenceErrors: false,
}

// Execute runs the root command and exits non-zero on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(serveCmd(), migrateCmd(), seedCmd())
}

// bootstrap loads config and builds the application logger.
func bootstrap() (config.Config, *slog.Logger, error) {
	cfg, err := config.Load()
	if err != nil {
		return config.Config{}, nil, err
	}
	logger := logging.New(cfg.LogLevel)
	slog.SetDefault(logger)
	return cfg, logger, nil
}
