//go:build unit

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func getSettingsJSON(t *testing.T, h *SettingHandler) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	h.GetSettings(c)
	require.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return envelope.Data
}

func TestOpenAIRequestTimezoneRewriteGlobalSettingDefaultsOff(t *testing.T) {
	h, _ := newStepUpSwitchTestHandler(t, map[string]string{})
	require.Equal(t, false, getSettingsJSON(t, h)[service.SettingKeyEnableOpenAIRequestTimezoneRewrite])
}

func TestOpenAIRequestTimezoneRewriteGlobalSettingKeptWhenOmitted(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyEnableOpenAIRequestTimezoneRewrite: "true",
	})

	rec := doUpdateSettings(t, h, map[string]any{"site_name": "Example Gateway"}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "true", repo.values[service.SettingKeyEnableOpenAIRequestTimezoneRewrite],
		"a payload without the switch must not turn the rewrite off")
	require.Equal(t, true, getSettingsJSON(t, h)[service.SettingKeyEnableOpenAIRequestTimezoneRewrite])

	rec = doUpdateSettings(t, h, map[string]any{service.SettingKeyEnableOpenAIRequestTimezoneRewrite: false}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyEnableOpenAIRequestTimezoneRewrite])

	rec = doUpdateSettings(t, h, map[string]any{service.SettingKeyEnableOpenAIRequestTimezoneRewrite: "yes"}, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "false", repo.values[service.SettingKeyEnableOpenAIRequestTimezoneRewrite])
}
