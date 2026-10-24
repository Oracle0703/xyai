package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type recordedAccountChange struct {
	kind      service.AccountOperatorActivityKind
	accountID int64
	before    *service.AccountActivitySnapshot
}

type fakeAccountOperatorRecorder struct {
	snapshots []int64
	changes   []recordedAccountChange
	created   [][]service.AccountActivityRef
	actors    []service.AccountOperatorActor
}

func (f *fakeAccountOperatorRecorder) Snapshot(_ context.Context, accountID int64) *service.AccountActivitySnapshot {
	f.snapshots = append(f.snapshots, accountID)
	return &service.AccountActivitySnapshot{ID: accountID}
}

func (f *fakeAccountOperatorRecorder) RecordChange(actor service.AccountOperatorActor, kind service.AccountOperatorActivityKind, accountID int64, before *service.AccountActivitySnapshot) {
	f.actors = append(f.actors, actor)
	f.changes = append(f.changes, recordedAccountChange{kind: kind, accountID: accountID, before: before})
}

func (f *fakeAccountOperatorRecorder) RecordCreated(actor service.AccountOperatorActor, refs []service.AccountActivityRef) {
	f.actors = append(f.actors, actor)
	f.created = append(f.created, refs)
}

func newAccountOperatorTrackerRouter(role string, recorder *fakeAccountOperatorRecorder) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := &AccountOperatorHandler{activity: recorder}
	admin := router.Group("/api/v1/admin")
	admin.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7})
		c.Set(string(middleware2.ContextKeyUserRole), role)
		c.Set(middleware2.ContextKeyAuthEmail, "ops@xunyou.com")
		c.Next()
	})
	admin.Use(h.ActivityTracker())
	admin.POST("/accounts", func(c *gin.Context) {
		if c.Query("fail") != "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "bad"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": 12, "platform": "openai", "type": "apikey"}})
	})
	admin.GET("/accounts", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 0}) })
	admin.POST("/accounts/exchange-code", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"code": 0}) })
	admin.POST("/accounts/:id/test", func(c *gin.Context) { c.String(http.StatusOK, "data: done\n\n") })
	admin.POST("/accounts/:id/apply-oauth-credentials", func(c *gin.Context) {
		if c.Query("fail") != "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0})
	})
	return router
}

func serveAccountOperatorRequest(router *gin.Engine, method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	return recorder
}

func TestAccountOperatorTrackerRecordsSubAdminCreate(t *testing.T) {
	recorder := &fakeAccountOperatorRecorder{}
	router := newAccountOperatorTrackerRouter(service.RoleSubAdmin, recorder)

	resp := serveAccountOperatorRequest(router, http.MethodPost, "/api/v1/admin/accounts")

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, resp.Body.String(), `"id":12`, "captured response must still reach the client")
	require.Equal(t, [][]service.AccountActivityRef{{{AccountID: 12, Action: "created"}}}, recorder.created)
	require.Equal(t, service.AccountOperatorActor{UserID: 7, Email: "ops@xunyou.com", ClientIP: "192.0.2.1", Method: http.MethodPost, Path: "/api/v1/admin/accounts"}, recorder.actors[0])
}

func TestAccountOperatorTrackerRecordsTestAndReauthChanges(t *testing.T) {
	recorder := &fakeAccountOperatorRecorder{}
	router := newAccountOperatorTrackerRouter(service.RoleSubAdmin, recorder)

	serveAccountOperatorRequest(router, http.MethodPost, "/api/v1/admin/accounts/9/test")
	serveAccountOperatorRequest(router, http.MethodPost, "/api/v1/admin/accounts/9/apply-oauth-credentials")

	require.Equal(t, []int64{9, 9}, recorder.snapshots)
	require.Len(t, recorder.changes, 2)
	require.Equal(t, service.AccountOperatorActivityTest, recorder.changes[0].kind)
	require.Equal(t, service.AccountOperatorActivityReauth, recorder.changes[1].kind)
	require.Equal(t, int64(9), recorder.changes[1].before.ID)
}

func TestAccountOperatorTrackerSkipsUntrackedRequests(t *testing.T) {
	tests := []struct {
		name   string
		role   string
		method string
		path   string
	}{
		{"full admin create", service.RoleAdmin, http.MethodPost, "/api/v1/admin/accounts"},
		{"sub admin read", service.RoleSubAdmin, http.MethodGet, "/api/v1/admin/accounts"},
		{"sub admin side-effect-free oauth exchange", service.RoleSubAdmin, http.MethodPost, "/api/v1/admin/accounts/exchange-code"},
		{"sub admin failed create", service.RoleSubAdmin, http.MethodPost, "/api/v1/admin/accounts?fail=1"},
		{"sub admin failed reauth", service.RoleSubAdmin, http.MethodPost, "/api/v1/admin/accounts/9/apply-oauth-credentials?fail=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &fakeAccountOperatorRecorder{}
			router := newAccountOperatorTrackerRouter(tt.role, recorder)
			serveAccountOperatorRequest(router, tt.method, tt.path)
			require.Empty(t, recorder.created)
			require.Empty(t, recorder.changes)
		})
	}
}

func TestAccountOperatorTrackerNilHandlerPassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var h *AccountOperatorHandler
	router.Use(h.ActivityTracker())
	router.POST("/api/v1/admin/accounts", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	resp := serveAccountOperatorRequest(router, http.MethodPost, "/api/v1/admin/accounts")
	require.Equal(t, http.StatusNoContent, resp.Code)
}

func TestAccountOperatorFormOptionsOmitsProxyCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := newStubAdminService()
	adminSvc.proxyCounts[0].Username = "proxy-user"
	adminSvc.proxyCounts[0].Password = "proxy-pass"
	router := gin.New()
	router.GET("/api/v1/admin/accounts/form-options", NewAccountOperatorHandler(adminSvc, nil).FormOptions)

	resp := serveAccountOperatorRequest(router, http.MethodGet, "/api/v1/admin/accounts/form-options")

	require.Equal(t, http.StatusOK, resp.Code)
	body := resp.Body.String()
	require.Contains(t, body, `"groups":[{"id":`)
	require.Contains(t, body, `"host":"127.0.0.1"`)
	require.NotContains(t, body, "proxy-user")
	require.NotContains(t, body, "proxy-pass")
	require.NotContains(t, body, "password")
}
