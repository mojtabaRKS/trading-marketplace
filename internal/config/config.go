package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all runtime configuration, loaded from environment variables.
type Config struct {
	// HTTP server
	HTTPPort        string        `envconfig:"HTTP_PORT" default:"8080"`
	ShutdownTimeout time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"10s"`

	// Database
	DBHost     string `envconfig:"DB_HOST" default:"localhost"`
	DBPort     string `envconfig:"DB_PORT" default:"5432"`
	DBUser     string `envconfig:"DB_USER" default:"marketd"`
	DBPassword string `envconfig:"DB_PASSWORD" default:"marketd"`
	DBName     string `envconfig:"DB_NAME" default:"marketd"`
	DBSSLMode  string `envconfig:"DB_SSLMODE" default:"disable"`

	// Logging
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`

	// Auction defaults
	AuctionWindow    time.Duration `envconfig:"AUCTION_WINDOW" default:"24h"`
	AuctionExtension time.Duration `envconfig:"AUCTION_EXTENSION" default:"5m"`
}

// Load reads configuration from the environment.
func Load() (Config, error) {
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	return c, nil
}

// DSN returns the PostgreSQL connection string.
func (c Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode,
	)
}
