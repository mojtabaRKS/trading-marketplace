//go:build integration

package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/herotech/market-dragon/internal/config"
	"github.com/herotech/market-dragon/internal/infra/database"
)

func idempotencyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := database.Migrate(cfg.DatabaseURL()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := database.Open(cfg.DSN())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DELETE FROM idempotency_keys WHERE key LIKE 'itest-%'")
	return db
}

// newCountingRouter returns a router whose POST /count handler increments calls
// and echoes the running total, guarded by the idempotency middleware.
func newCountingRouter(db *gorm.DB, calls *atomic.Int64) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	g := r.Group("")
	g.Use(Idempotency(db, slog.Default()))
	g.POST("/count", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		n := calls.Add(1)
		c.JSON(http.StatusCreated, gin.H{"count": n})
	})
	return r
}

func doPost(t *testing.T, r http.Handler, key, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/count", bytes.NewBufferString(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

// TestIdempotencyReplaysStoredResponse: same key+body twice runs the handler
// once and replays the first response.
func TestIdempotencyReplaysStoredResponse(t *testing.T) {
	db := idempotencyTestDB(t)
	var calls atomic.Int64
	r := newCountingRouter(db, &calls)

	code1, body1 := doPost(t, r, "itest-replay", `{"x":1}`)
	code2, body2 := doPost(t, r, "itest-replay", `{"x":1}`)

	if code1 != http.StatusCreated || code2 != http.StatusCreated {
		t.Fatalf("codes = %d, %d, want 201, 201", code1, code2)
	}
	if body1 != body2 {
		t.Fatalf("replayed body %q != original %q", body2, body1)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("handler ran %d times, want 1", got)
	}
}

// TestIdempotencyRejectsDifferentBody: reusing a key with a different body 409s.
func TestIdempotencyRejectsDifferentBody(t *testing.T) {
	db := idempotencyTestDB(t)
	var calls atomic.Int64
	r := newCountingRouter(db, &calls)

	if code, _ := doPost(t, r, "itest-diff", `{"x":1}`); code != http.StatusCreated {
		t.Fatalf("first code = %d, want 201", code)
	}
	if code, _ := doPost(t, r, "itest-diff", `{"x":2}`); code != http.StatusConflict {
		t.Fatalf("different-body code = %d, want 409", code)
	}
}

// TestIdempotencyConcurrentSingleEffect: many concurrent requests with the same
// key run the handler exactly once.
func TestIdempotencyConcurrentSingleEffect(t *testing.T) {
	db := idempotencyTestDB(t)
	var calls atomic.Int64
	r := newCountingRouter(db, &calls)
	srv := httptest.NewServer(r)
	defer srv.Close()

	const n = 12
	var wg sync.WaitGroup
	var created, conflict int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/count", bytes.NewBufferString(`{"x":1}`))
			req.Header.Set("Idempotency-Key", "itest-concurrent")
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("request: %v", err)
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusCreated:
				atomic.AddInt64(&created, 1)
			case http.StatusConflict:
				atomic.AddInt64(&conflict, 1)
			default:
				t.Errorf("unexpected status %d", resp.StatusCode)
			}
		}()
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("handler ran %d times under concurrency, want exactly 1", got)
	}
	if created+conflict != n {
		t.Fatalf("responses = %d created + %d conflict, want %d total", created, conflict, n)
	}
	if created < 1 {
		t.Fatalf("expected at least one 201 (the winner + any replays), got %d", created)
	}
}
