package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/herotech/market-dragon/internal/api"
	"github.com/herotech/market-dragon/internal/infra/database"
	"github.com/herotech/market-dragon/internal/repository"
	"github.com/herotech/market-dragon/internal/service"
)

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the HTTP API server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context())
		},
	}
}

func runServe(ctx context.Context) error {
	cfg, logger, err := bootstrap()
	if err != nil {
		return err
	}

	if cfg.AutoMigrate {
		if err := database.Migrate(cfg.DatabaseURL()); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		logger.Info("migrations applied")
	}

	db, err := database.Open(cfg.DSN())
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}

	repos := repository.New(db)
	_ = repos // reserved for future repository-backed services

	if cfg.Seed {
		if err := repository.Seed(db); err != nil {
			return fmt.Errorf("seed: %w", err)
		}
		logger.Info("seed data loaded")
	}

	wallets := service.NewWalletService(db)
	listings := service.NewListingService(db, wallets)
	auctions := service.NewAuctionService(db, wallets, cfg.AuctionWindow, cfg.AuctionExtension)

	router := api.NewRouter(api.Deps{
		Logger:   logger,
		Listings: listings,
		Auctions: auctions,
	})
	srv := api.NewServer(":"+cfg.HTTPPort, router, logger)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		return err
	case <-sigCtx.Done():
	}

	logger.Info("shutdown signal received, draining connections")
	if err := srv.Shutdown(cfg.ShutdownTimeout); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("server stopped cleanly")
	return nil
}
