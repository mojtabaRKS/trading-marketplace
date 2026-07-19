package service

import (
	"errors"
	"testing"

	"github.com/herotech/market-dragon/internal/infra/oracle"
)

func TestValidatePrice(t *testing.T) {
	cfg := OracleConfig{MaxPrice: 1_000_000, MaxDeviationRatio: 10}

	tests := []struct {
		name   string
		amount int64
		prev   int64
		want   error
	}{
		{"positive within bounds", 500, 400, nil},
		{"zero rejected", 0, 400, ErrOraclePriceInvalid},
		{"negative rejected", -5, 400, ErrOraclePriceInvalid},
		{"above ceiling rejected", 2_000_000, 400, ErrOraclePriceImplausible},
		{"huge jump up rejected", 5_000, 400, ErrOraclePriceImplausible},
		{"huge drop rejected", 30, 400, ErrOraclePriceImplausible},
		{"no prev skips deviation", 5_000, 0, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePrice(oracle.Price{ItemID: 1, Amount: tc.amount}, tc.prev, cfg)
			if !errors.Is(err, tc.want) {
				t.Fatalf("validatePrice(%d, prev=%d) = %v, want %v", tc.amount, tc.prev, err, tc.want)
			}
		})
	}
}

func TestValidatePriceDisabledGuards(t *testing.T) {
	cfg := OracleConfig{} // no ceiling, no deviation guard
	if err := validatePrice(oracle.Price{ItemID: 1, Amount: 999_999_999}, 1, cfg); err != nil {
		t.Fatalf("with guards disabled, large price should pass: %v", err)
	}
	if err := validatePrice(oracle.Price{ItemID: 1, Amount: 0}, 1, cfg); !errors.Is(err, ErrOraclePriceInvalid) {
		t.Fatalf("zero must always be rejected: %v", err)
	}
}
