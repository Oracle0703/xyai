package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserConcurrencyPresetHandlerCreateAndApply(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newHandlerPresetRepoStub()
	userRepo := newHandlerPresetUserRepoStub(&service.User{ID: 11, Role: service.RoleUser})
	svc := service.NewUserConcurrencyPresetService(repo, userRepo, nil, nil)
	h := NewUserConcurrencyPresetHandler(svc)
	r := gin.New()
	r.POST("/presets", h.Create)
	r.POST("/presets/:id/apply", h.Apply)
	r.GET("/presets/:id/runs", h.ListRuns)

	body := bytes.NewBufferString(`{"name":"day","target_concurrency":12,"user_ids":[11],"schedule_enabled":true,"schedule_time":"09:00"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/presets", body)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"name":"day"`)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/presets/1/apply", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"status":"success"`)
	require.Equal(t, 12, userRepo.batchSetValue)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/presets/1/runs?limit=10", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"trigger":"manual"`)
}

func TestUserConcurrencyPresetHandlerCreateValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newHandlerPresetRepoStub()
	userRepo := newHandlerPresetUserRepoStub(&service.User{ID: 11, Role: service.RoleUser})
	h := NewUserConcurrencyPresetHandler(service.NewUserConcurrencyPresetService(repo, userRepo, nil, nil))
	r := gin.New()
	r.POST("/presets", h.Create)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/presets", bytes.NewBufferString(`{"name":"","target_concurrency":12,"user_ids":[11]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "name is required")
}

type handlerPresetRepoStub struct {
	presets map[int64]*service.UserConcurrencyPreset
	runs    []*service.UserConcurrencyPresetRun
	nextID  int64
}

func newHandlerPresetRepoStub() *handlerPresetRepoStub {
	return &handlerPresetRepoStub{presets: make(map[int64]*service.UserConcurrencyPreset), nextID: 1}
}

func (s *handlerPresetRepoStub) Create(_ context.Context, preset *service.UserConcurrencyPreset) (*service.UserConcurrencyPreset, error) {
	clone := *preset
	clone.ID = s.nextID
	clone.CreatedAt = time.Now()
	clone.UpdatedAt = clone.CreatedAt
	s.nextID++
	s.presets[clone.ID] = &clone
	return &clone, nil
}

func (s *handlerPresetRepoStub) GetByID(_ context.Context, id int64) (*service.UserConcurrencyPreset, error) {
	return s.presets[id], nil
}

func (s *handlerPresetRepoStub) List(context.Context) ([]*service.UserConcurrencyPreset, error) {
	out := make([]*service.UserConcurrencyPreset, 0, len(s.presets))
	for _, preset := range s.presets {
		out = append(out, preset)
	}
	return out, nil
}

func (s *handlerPresetRepoStub) ListDue(context.Context, string, time.Time) ([]*service.UserConcurrencyPreset, error) {
	return nil, nil
}

func (s *handlerPresetRepoStub) Update(_ context.Context, preset *service.UserConcurrencyPreset) (*service.UserConcurrencyPreset, error) {
	s.presets[preset.ID] = preset
	return preset, nil
}

func (s *handlerPresetRepoStub) Delete(_ context.Context, id int64) error {
	delete(s.presets, id)
	return nil
}

func (s *handlerPresetRepoStub) MarkScheduledRun(context.Context, int64, time.Time) error { return nil }

func (s *handlerPresetRepoStub) CreateRun(_ context.Context, run *service.UserConcurrencyPresetRun) (*service.UserConcurrencyPresetRun, error) {
	clone := *run
	clone.ID = int64(len(s.runs) + 1)
	clone.CreatedAt = time.Now()
	s.runs = append(s.runs, &clone)
	return &clone, nil
}

func (s *handlerPresetRepoStub) ListRuns(context.Context, int64, int) ([]*service.UserConcurrencyPresetRun, error) {
	return s.runs, nil
}

type handlerPresetUserRepoStub struct {
	*userConcurrencyPresetUserRepoAdapter
	users         map[int64]*service.User
	batchSetValue int
}

func newHandlerPresetUserRepoStub(users ...*service.User) *handlerPresetUserRepoStub {
	out := &handlerPresetUserRepoStub{users: make(map[int64]*service.User)}
	for _, user := range users {
		clone := *user
		out.users[user.ID] = &clone
	}
	return out
}

func (s *handlerPresetUserRepoStub) GetByID(_ context.Context, id int64) (*service.User, error) {
	return s.users[id], nil
}

func (s *handlerPresetUserRepoStub) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.User, error) {
	return s.GetByID(ctx, id)
}

func (s *handlerPresetUserRepoStub) BatchSetConcurrency(_ context.Context, userIDs []int64, value int) (int, error) {
	s.batchSetValue = value
	return len(userIDs), nil
}

type userConcurrencyPresetUserRepoAdapter struct{}

func (s *userConcurrencyPresetUserRepoAdapter) Create(context.Context, *service.User) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) GetByEmail(context.Context, string) (*service.User, error) {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) GetFirstAdmin(context.Context) (*service.User, error) {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) Update(context.Context, *service.User) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) Delete(context.Context, int64) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) GetUserAvatar(context.Context, int64) (*service.UserAvatar, error) {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) UpsertUserAvatar(context.Context, int64, service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) DeleteUserAvatar(context.Context, int64) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) List(context.Context, pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	panic("unexpected")
}

func (s *userConcurrencyPresetUserRepoAdapter) ListWithFilters(context.Context, pagination.PaginationParams, service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) UpdateBalance(context.Context, int64, float64) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) DeductBalance(context.Context, int64, float64) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) UpdateConcurrency(context.Context, int64, int) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) BatchUpdateLimits(context.Context, []int64, *int, *int) (int, error) {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) ExistsByEmail(context.Context, string) (bool, error) {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) ListUserAuthIdentities(context.Context, int64) ([]service.UserAuthIdentityRecord, error) {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) UpdateTotpSecret(context.Context, int64, *string) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) EnableTotp(context.Context, int64) error {
	panic("unexpected")
}
func (s *userConcurrencyPresetUserRepoAdapter) DisableTotp(context.Context, int64) error {
	panic("unexpected")
}

func TestUserConcurrencyPresetHandlerJSONShape(t *testing.T) {
	run := service.UserConcurrencyPresetRun{ID: 1, Trigger: service.UserConcurrencyPresetTriggerManual}
	payload, err := json.Marshal(run)
	require.NoError(t, err)
	require.Contains(t, string(payload), `"trigger":"manual"`)
}
