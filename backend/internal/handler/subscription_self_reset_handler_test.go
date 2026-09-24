package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionSelfResetHandlerFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	old := service.DefaultIdempotencyCoordinator()
	service.SetDefaultIdempotencyCoordinator(nil)
	t.Cleanup(func() { service.SetDefaultIdempotencyCoordinator(old) })
	h := NewSubscriptionSelfResetHandler(nil)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7}) })
	r.POST("/:id/reset", h.Reset)
	for _, tc := range []struct {
		path, body, key string
		code            int
	}{
		{"/1/reset", `{"quota_date":"2026-09-23"}`, "", 400},
		{"/1/reset", `{"quota_date":"2026-09-23"}`, "test-key", 503},
		{"/0/reset", `{"quota_date":"2026-09-23"}`, "test-key", 400},
		{"/1/reset", `{"quota_date":"2026-09-23","user_id":9}`, "test-key", 400},
		{"/1/reset", `{"quota_date":"2026-09-31"}`, "test-key", 400},
		{"/1/reset", `null`, "test-key", 400},
		{"/1/reset", `{"quota_date":"2026-09-23"} {}`, "test-key", 400},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Idempotency-Key", tc.key)
		r.ServeHTTP(w, req)
		require.Equal(t, tc.code, w.Code, tc.body)
	}
}
