package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type requestInterceptHandlerRepoStub struct {
	values map[string]string
}

func (r *requestInterceptHandlerRepoStub) Get(ctx context.Context, key string) (*service.Setting, error) {
	value, err := r.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &service.Setting{Key: key, Value: value, UpdatedAt: time.Now()}, nil
}

func (r *requestInterceptHandlerRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if r.values == nil {
		r.values = map[string]string{}
	}
	value, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (r *requestInterceptHandlerRepoStub) Set(ctx context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *requestInterceptHandlerRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := map[string]string{}
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *requestInterceptHandlerRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *requestInterceptHandlerRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	return r.values, nil
}

func (r *requestInterceptHandlerRepoStub) Delete(ctx context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestRequestInterceptHandlerSaveListAndTest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewRequestInterceptHandler(service.NewRequestInterceptRulesService(&requestInterceptHandlerRepoStub{}))
	router := gin.New()
	router.GET("/rules", handler.List)
	router.PUT("/rules", handler.SaveAll)
	router.POST("/test", handler.Test)

	saveBody := `{"rules":[{"id":"greeting","name":"问候","enabled":true,"priority":1,"match_mode":"exact","keywords":["hi"],"reply":"你好，我是迅游AI，有什么可以帮助你？","scopes":["all"],"normalize":{"trim_space":true,"case_insensitive":true,"full_width_to_half":true,"collapse_space":true,"remove_punctuation":true}}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/rules", bytes.NewBufferString(saveBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "greeting")

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/rules", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "问候")

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(`{"text":" HI ","endpoint":"/v1/responses"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"matched":true`)
	require.Contains(t, w.Body.String(), "迅游AI")
}

func TestRequestInterceptHandlerUpsertAndDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewRequestInterceptHandler(service.NewRequestInterceptRulesService(&requestInterceptHandlerRepoStub{}))
	router := gin.New()
	router.PUT("/rules/:id", handler.Upsert)
	router.DELETE("/rules/:id", handler.Delete)
	router.GET("/rules", handler.List)

	payload := map[string]any{
		"name":       "政策",
		"enabled":    true,
		"priority":   1,
		"match_mode": "contains",
		"keywords":   []string{"示例敏感词"},
		"reply":      "你的问题超出法律规定，请问一些其他的。",
		"scopes":     []string{"all"},
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/rules/policy", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "policy")

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/rules/policy", nil))
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/rules", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"rules":[]`)
}
