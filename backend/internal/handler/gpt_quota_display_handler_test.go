package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 刷新请求体必须明确 entry_id 或 all，畸形请求不能被当成"刷新全部"去访问上游。
func TestGPTQuotaRefreshRejectsAmbiguousBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewGPTQuotaDisplayHandler(nil)
	r := gin.New()
	r.POST("/refresh", h.Refresh)

	for name, body := range map[string]string{
		"malformed json":  `{"entry_id":`,
		"string entry id": `{"entry_id":"12"}`,
		"empty object":    `{}`,
		"both targets":    `{"entry_id":1,"all":true}`,
		"non-positive id": `{"entry_id":0}`,
		"all false":       `{"all":false}`,
		"empty body":      ``,
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/refresh", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
}
