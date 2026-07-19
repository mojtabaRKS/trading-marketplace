package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"gorm.io/gorm"

	"github.com/herotech/market-dragon/internal/api"
	"github.com/herotech/market-dragon/internal/config"
	"github.com/herotech/market-dragon/internal/infra/database"
	"github.com/herotech/market-dragon/internal/infra/oracle"
	"github.com/herotech/market-dragon/internal/repository"
	"github.com/herotech/market-dragon/internal/service"
	"github.com/herotech/market-dragon/internal/worker"
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
	oracles := buildOracleService(cfg, db, logger)

	router := api.NewRouter(api.Deps{
		Logger:   logger,
		Listings: listings,
		Auctions: auctions,
		Oracle:   oracles,
	})
	srv := api.NewServer(":"+cfg.HTTPPort, router, logger)

	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	settler := worker.NewSettlementWorker(auctions, cfg.SettleInterval, logger)
	go settler.Run(sigCtx)

	if err := oracles.LoadLastKnownGood(sigCtx); err != nil {
		logger.Warn("could not warm oracle cache", "error", err)
	}
	poller := worker.NewOraclePoller(oracles, cfg.OraclePollInterval, logger)
	go poller.Run(sigCtx)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

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

// buildOracleService wires a mock upstream feed behind the resilient client
// (timeout + retries + circuit breaker) and the validating OracleService. The
// mock stands in for the real Oracle Price Service defined by oracle.Source.
func buildOracleService(cfg config.Config, db *gorm.DB, logger *slog.Logger) *service.OracleService {
	source := oracle.NewMockSource(
		oracle.Price{ItemID: 1, Amount: 50},
		oracle.Price{ItemID: 2, Amount: 1_200},
		oracle.Price{ItemID: 3, Amount: 250_000},
		oracle.Price{ItemID: 4, Amount: 300_000},
	)
	breaker := oracle.NewCircuitBreaker(cfg.OracleBreakerTrip, cfg.OracleBreakerCooldown)
	client := oracle.NewResilientClient(source, oracle.ClientConfig{
		Timeout:    cfg.OracleTimeout,
		MaxRetries: cfg.OracleMaxRetries,
		Backoff:    cfg.OracleBackoff,
	}, breaker)
	return service.NewOracleService(client, db, service.OracleConfig{
		MaxPrice:          cfg.OracleMaxPrice,
		MaxDeviationRatio: cfg.OracleMaxDeviation,
	}, logger)
}
