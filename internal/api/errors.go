package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/herotech/market-dragon/internal/service"
)

// ErrorResponse is the standard JSON error body returned by the API.
type ErrorResponse struct {
	Error string `json:"error" example:"item not found"`
}

// statusForError maps a domain error to an HTTP status code.
func statusForError(err error) int {
	switch {
	case errors.Is(err, service.ErrNotFound),
		errors.Is(err, service.ErrPriceUnavailable):
		return http.StatusNotFound
	case errors.Is(err, service.ErrInsufficientFunds),
		errors.Is(err, service.ErrDailyCapExceeded):
		return http.StatusPaymentRequired
	case errors.Is(err, service.ErrListingNotOpen),
		errors.Is(err, service.ErrOutOfStock),
		errors.Is(err, service.ErrActiveAuctionExists),
		errors.Is(err, service.ErrItemNotAvailable),
		errors.Is(err, service.ErrAuctionNotActive),
		errors.Is(err, service.ErrAuctionEnded):
		return http.StatusConflict
	case errors.Is(err, service.ErrInvalidAmount),
		errors.Is(err, service.ErrSelfPurchase),
		errors.Is(err, service.ErrSelfBid),
		errors.Is(err, service.ErrItemNotOwned),
		errors.Is(err, service.ErrLegendaryNeedsAuction),
		errors.Is(err, service.ErrNotLegendary),
		errors.Is(err, service.ErrBidTooLow),
		errors.Is(err, service.ErrCancelHighestBid),
		errors.Is(err, service.ErrInsufficientReserved):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// respondError writes a JSON error with the mapped status. Unexpected (500)
// errors are logged and their detail hidden from the client.
func respondError(c *gin.Context, logger *slog.Logger, err error) {
	status := statusForError(err)
	if status == http.StatusInternalServerError {
		logger.Error("request failed", slog.Any("error", err))
		c.JSON(status, ErrorResponse{Error: "internal error"})
		return
	}
	c.JSON(status, ErrorResponse{Error: err.Error()})
}
