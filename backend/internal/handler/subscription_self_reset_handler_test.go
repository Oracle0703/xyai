package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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

func TestSubscriptionSelfResetHandlerListEventsRejectsBadFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/events", NewSubscriptionSelfResetHandler(nil).ListEvents)
	for _, query := range []string{"organization=unknown", "start_date=2026-09-31", "end_date=09/23/2026"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/events?"+query, nil))
		require.Equal(t, http.StatusBadRequest, w.Code, query)
	}
}

func TestParseSelfResetGroupChanges(t *testing.T) {
	changes, err := parseSelfResetGroupChanges(map[string]json.RawMessage{
		"xunyou": json.RawMessage(`{"enable":[12],"disable":[30]}`),
	})
	require.NoError(t, err)
	require.Equal(t, []int64{12}, changes[service.OrganizationXunyou].Enable)
	require.Equal(t, []int64{30}, changes[service.OrganizationXunyou].Disable)

	for _, raw := range []map[string]json.RawMessage{
		{"xunyou": json.RawMessage(`{"enable":null}`)},
		{"xunyou": json.RawMessage(`{"enable":[1],"disable":[1]}`)},
		{"unknown": json.RawMessage(`{"enable":[1]}`)},
		{},
		{"xunyou": json.RawMessage(`{}`)},
		{"xunyou": json.RawMessage(`{"enable":[],"disable":[]}`)},
		{"xunyou": json.RawMessage(`{"enable":[1]}`), "other": json.RawMessage(`{"disable":[]}`)},
		{"xunyou": json.RawMessage(`{"enable":[0]}`)},
		{"xunyou": json.RawMessage(`{"enable":[1,1]}`)},
		{"xunyou": json.RawMessage(`{"enable":{}}`)},
		{"xunyou": json.RawMessage(`{"enable":[1],"extra":[2]}`)},
		{"xunyou": json.RawMessage(`null`)},
	} {
		_, err := parseSelfResetGroupChanges(raw)
		require.Error(t, err)
		require.Equal(t, "INVALID_SELF_RESET_GROUPS", infraerrors.Reason(err))
	}
}

func TestSubscriptionSelfResetHandlerRejectsInvalidConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewSubscriptionSelfResetHandler(nil)
	r.PUT("/config", h.SaveConfig)
	r.POST("/config/close", h.CloseConfig)
	for _, body := range []string{
		`{}`, `{"expected_revision":0}`, `{"expected_revision":1.5}`, `null`,
		`{"expected_revision":1} {}`, `{"expected_revision":1,"unknown":1}`,
		strings.Repeat(" ", 64*1024) + `{"expected_revision":1}`,
		`{"expected_revision":1,"changes_by_organization":null}`,
		`{"expected_revision":1,"changes_by_organization":{}}`,
		`{"expected_revision":1,"changes_by_organization":{"xunyou":{"enable":[1],"disable":[1]}}}`,
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/config", strings.NewReader(body)))
		require.Equal(t, http.StatusBadRequest, w.Code)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/config/close", strings.NewReader(`{"expected_revision":1}`)))
	require.Equal(t, http.StatusBadRequest, w.Code)
	change, err := readSelfResetConfigChange(strings.NewReader(`{"expected_revision":42}`))
	require.NoError(t, err)
	require.Equal(t, service.SubscriptionSelfResetConfigChange{ExpectedRevision: 42}, change)
}
