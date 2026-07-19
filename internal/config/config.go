// Package config loads runtime configuration from a .env file and environment
// variables via Viper. Precedence: environment variable > .env file > default.
package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all runtime configuration.
type Config struct {
	// HTTP server
	HTTPPort        string
	ShutdownTimeout time.Duration

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// Logging
	LogLevel string

	// Migrations / seeding
	AutoMigrate bool
	Seed        bool

	// Auction defaults
	AuctionWindow    time.Duration
	AuctionExtension time.Duration
}

// Load reads configuration using Viper: defaults, then an optional .env file,
// then environment variables (which take precedence).
func Load() (Config, error) {
	v := viper.New()

	setDefaults(v)

	// Optional .env file in the working directory. Absence is not an error.
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return Config{}, fmt.Errorf("read .env: %w", err)
		}
	}

	v.AutomaticEnv()

	return Config{
		HTTPPort:         v.GetString("HTTP_PORT"),
		ShutdownTimeout:  v.GetDuration("SHUTDOWN_TIMEOUT"),
		DBHost:           v.GetString("DB_HOST"),
		DBPort:           v.GetString("DB_PORT"),
		DBUser:           v.GetString("DB_USER"),
		DBPassword:       v.GetString("DB_PASSWORD"),
		DBName:           v.GetString("DB_NAME"),
		DBSSLMode:        v.GetString("DB_SSLMODE"),
		LogLevel:         v.GetString("LOG_LEVEL"),
		AutoMigrate:      v.GetBool("AUTO_MIGRATE"),
		Seed:             v.GetBool("SEED"),
		AuctionWindow:    v.GetDuration("AUCTION_WINDOW"),
		AuctionExtension: v.GetDuration("AUCTION_EXTENSION"),
	}, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("HTTP_PORT", "8080")
	v.SetDefault("SHUTDOWN_TIMEOUT", "10s")
	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", "5432")
	v.SetDefault("DB_USER", "marketd")
	v.SetDefault("DB_PASSWORD", "marketd")
	v.SetDefault("DB_NAME", "marketd")
	v.SetDefault("DB_SSLMODE", "disable")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("AUTO_MIGRATE", true)
	v.SetDefault("SEED", false)
	v.SetDefault("AUCTION_WINDOW", "24h")
	v.SetDefault("AUCTION_EXTENSION", "5m")
}

// DSN returns the PostgreSQL key/value connection string (used by GORM).
func (c Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode,
	)
}

// DatabaseURL returns the postgres:// URL form (used by golang-migrate).
func (c Config) DatabaseURL() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}
