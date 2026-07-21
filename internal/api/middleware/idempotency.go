package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/herotech/market-dragon/internal/model"
)

// idempotencyHeader is the request header carrying the client-chosen key.
const idempotencyHeader = "Idempotency-Key"

// responseCapture tees the handler's response into a buffer so it can be stored
// for later replay, while still writing to the real client.
type responseCapture struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseCapture) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *responseCapture) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// Idempotency makes state-changing requests safe to retry. When a request
// carries an Idempotency-Key:
//
//   - The first caller "claims" the key (a unique INSERT). It runs the handler,
//     then records the response for replay.
//   - A concurrent caller that loses the claim race sees an in-flight row and
//     gets 409 (retry later) — preventing a double effect.
//   - A later caller with a completed key replays the stored response verbatim.
//   - Reusing a key with a different request body is rejected with 409.
//
// Requests without the header are passed through unchanged.
func Idempotency(db *gorm.DB, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader(idempotencyHeader)
		if key == "" {
			c.Next()
			return
		}

		var body []byte
		if c.Request.Body != nil {
			body, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
		}
		hash := requestHash(c.Request.Method, c.Request.URL.Path, body)

		claim := model.IdempotencyKey{
			Key:            key,
			RequestHash:    hash,
			ResponseStatus: 0, // 0 == in-flight
			CreatedAt:      time.Now(),
		}
		res := db.WithContext(c.Request.Context()).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&claim)
		if res.Error != nil {
			logger.Error("idempotency claim failed", slog.Any("error", res.Error))
			c.Next() // fail open: better to process than to hard-fail
			return
		}

		if res.RowsAffected == 0 {
			replayExisting(c, db, key, hash)
			return
		}

		// We own the key: run the handler and capture its response.
		capture := &responseCapture{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = capture
		c.Next()

		status := c.Writer.Status()
		if status >= http.StatusInternalServerError {
			// Transient failure: release the claim so the client may retry.
			db.WithContext(c.Request.Context()).Delete(&model.IdempotencyKey{}, "key = ?", key)
			return
		}
		if err := db.WithContext(c.Request.Context()).
			Model(&model.IdempotencyKey{}).
			Where("key = ?", key).
			Updates(map[string]any{
				"response_status": status,
				"response_body":   capture.body.Bytes(),
			}).Error; err != nil {
			logger.Error("idempotency store failed", slog.Any("error", err))
		}
	}
}

func replayExisting(c *gin.Context, db *gorm.DB, key, hash string) {
	var existing model.IdempotencyKey
	if err := db.WithContext(c.Request.Context()).First(&existing, "key = ?", key).Error; err != nil {
		// Row vanished between claim and read (rare). Treat as conflict.
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "idempotency key conflict"})
		return
	}
	if existing.RequestHash != hash {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "idempotency key reused with a different request"})
		return
	}
	if existing.ResponseStatus == 0 {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "a request with this idempotency key is in progress"})
		return
	}
	c.Data(existing.ResponseStatus, "application/json; charset=utf-8", existing.ResponseBody)
	c.Abort()
}

func requestHash(method, path string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte(" "))
	h.Write([]byte(path))
	h.Write([]byte("\n"))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}
