package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type userConcurrencyPresetRepoStub struct {
	presets     map[int64]*UserConcurrencyPreset
	runs        []*UserConcurrencyPresetRun
	due         []*UserConcurrencyPreset
	nextID      int64
	createRunID int64
	markedRuns  []time.Time
	createErr   error
	updateErr   error
	applyErr    error
}

func newUserConcurrencyPresetRepoStub() *userConcurrencyPresetRepoStub {
	return &userConcurrencyPresetRepoStub{
		presets:     make(map[int64]*UserConcurrencyPreset),
		nextID:      1,
		createRunID: 1,
	}
}

func (s *userConcurrencyPresetRepoStub) Create(_ context.Context, preset *UserConcurrencyPreset) (*UserConcurrencyPreset, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	clone := *preset
	clone.ID = s.nextID
	clone.UserIDs = append([]int64(nil), preset.UserIDs...)
	s.nextID++
	s.presets[clone.ID] = &clone
	return &clone, nil
}

func (s *userConcurrencyPresetRepoStub) GetByID(_ context.Context, id int64) (*UserConcurrencyPreset, error) {
	preset := s.presets[id]
	if preset == nil {
		return nil, errors.New("preset not found")
	}
	clone := *preset
	clone.UserIDs = append([]int64(nil), preset.UserIDs...)
	return &clone, nil
}

func (s *userConcurrencyPresetRepoStub) List(context.Context) ([]*UserConcurrencyPreset, error) {
	out := make([]*UserConcurrencyPreset, 0, len(s.presets))
	for _, preset := range s.presets {
		clone := *preset
		clone.UserIDs = append([]int64(nil), preset.UserIDs...)
		out = append(out, &clone)
	}
	return out, nil
}

func (s *userConcurrencyPresetRepoStub) ListDue(context.Context, string, time.Time) ([]*UserConcurrencyPreset, error) {
	return s.due, nil
}

func (s *userConcurrencyPresetRepoStub) Update(_ context.Context, preset *UserConcurrencyPreset) (*UserConcurrencyPreset, error) {
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	clone := *preset
	clone.UserIDs = append([]int64(nil), preset.UserIDs...)
	s.presets[clone.ID] = &clone
	return &clone, nil
}

func (s *userConcurrencyPresetRepoStub) Delete(_ context.Context, id int64) error {
	delete(s.presets, id)
	return nil
}

func (s *userConcurrencyPresetRepoStub) MarkScheduledRun(_ context.Context, _ int64, runDate time.Time) error {
	s.markedRuns = append(s.markedRuns, runDate)
	return nil
}

func (s *userConcurrencyPresetRepoStub) CreateRun(_ context.Context, run *UserConcurrencyPresetRun) (*UserConcurrencyPresetRun, error) {
	clone := *run
	clone.ID = s.createRunID
	clone.UserIDs = append([]int64(nil), run.UserIDs...)
	s.createRunID++
	s.runs = append(s.runs, &clone)
	return &clone, nil
}

func (s *userConcurrencyPresetRepoStub) ListRuns(context.Context, int64, int) ([]*UserConcurrencyPresetRun, error) {
	return s.runs, nil
}

type userConcurrencyPresetUserRepoStub struct {
	users             map[int64]*User
	batchSetUserIDs   []int64
	batchSetValue     int
	batchSetCallCount int
	batchSetErr       error
}

func newUserConcurrencyPresetUserRepoStub(users ...*User) *userConcurrencyPresetUserRepoStub {
	out := &userConcurrencyPresetUserRepoStub{users: make(map[int64]*User)}
	for _, user := range users {
		clone := *user
		out.users[user.ID] = &clone
	}
	return out
}

func (s *userConcurrencyPresetUserRepoStub) Create(context.Context, *User) error {
	panic("unexpected Create")
}
func (s *userConcurrencyPresetUserRepoStub) CreateWithEmailAliasGuard(context.Context, *User) error {
	panic("unexpected CreateWithEmailAliasGuard")
}
func (s *userConcurrencyPresetUserRepoStub) GetByID(_ context.Context, id int64) (*User, error) {
	user := s.users[id]
	if user == nil {
		return nil, ErrUserNotFound
	}
	clone := *user
	return &clone, nil
}
func (s *userConcurrencyPresetUserRepoStub) GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error) {
	return s.GetByID(ctx, id)
}
func (s *userConcurrencyPresetUserRepoStub) GetByEmail(context.Context, string) (*User, error) {
	panic("unexpected GetByEmail")
}
func (s *userConcurrencyPresetUserRepoStub) GetFirstAdmin(context.Context) (*User, error) {
	panic("unexpected GetFirstAdmin")
}
func (s *userConcurrencyPresetUserRepoStub) Update(context.Context, *User) error {
	panic("unexpected Update")
}
func (s *userConcurrencyPresetUserRepoStub) Delete(context.Context, int64) error {
	panic("unexpected Delete")
}
func (s *userConcurrencyPresetUserRepoStub) GetUserAvatar(context.Context, int64) (*UserAvatar, error) {
	panic("unexpected GetUserAvatar")
}
func (s *userConcurrencyPresetUserRepoStub) UpsertUserAvatar(context.Context, int64, UpsertUserAvatarInput) (*UserAvatar, error) {
	panic("unexpected UpsertUserAvatar")
}
func (s *userConcurrencyPresetUserRepoStub) DeleteUserAvatar(context.Context, int64) error {
	panic("unexpected DeleteUserAvatar")
}
func (s *userConcurrencyPresetUserRepoStub) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected List")
}
func (s *userConcurrencyPresetUserRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters")
}
func (s *userConcurrencyPresetUserRepoStub) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserIDs")
}
func (s *userConcurrencyPresetUserRepoStub) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserID")
}
func (s *userConcurrencyPresetUserRepoStub) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	panic("unexpected UpdateUserLastActiveAt")
}
func (s *userConcurrencyPresetUserRepoStub) UpdateBalance(context.Context, int64, float64) error {
	panic("unexpected UpdateBalance")
}
func (s *userConcurrencyPresetUserRepoStub) DeductBalance(context.Context, int64, float64) error {
	panic("unexpected DeductBalance")
}
func (s *userConcurrencyPresetUserRepoStub) UpdateConcurrency(context.Context, int64, int) error {
	panic("unexpected UpdateConcurrency")
}
func (s *userConcurrencyPresetUserRepoStub) BatchSetConcurrency(_ context.Context, userIDs []int64, value int) (int, error) {
	s.batchSetCallCount++
	s.batchSetUserIDs = append([]int64(nil), userIDs...)
	s.batchSetValue = value
	if s.batchSetErr != nil {
		return 0, s.batchSetErr
	}
	for _, id := range userIDs {
		if user := s.users[id]; user != nil {
			user.Concurrency = value
		}
	}
	return len(userIDs), nil
}
func (s *userConcurrencyPresetUserRepoStub) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	panic("unexpected BatchAddConcurrency")
}
func (s *userConcurrencyPresetUserRepoStub) BatchUpdateLimits(context.Context, []int64, *int, *int) (int, error) {
	panic("unexpected BatchUpdateLimits")
}
func (s *userConcurrencyPresetUserRepoStub) ExistsByEmail(context.Context, string) (bool, error) {
	panic("unexpected ExistsByEmail")
}
func (s *userConcurrencyPresetUserRepoStub) ExistsByEmailAlias(context.Context, string) (bool, error) {
	panic("unexpected ExistsByEmailAlias")
}
func (s *userConcurrencyPresetUserRepoStub) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	panic("unexpected RemoveGroupFromAllowedGroups")
}
func (s *userConcurrencyPresetUserRepoStub) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected AddGroupToAllowedGroups")
}
func (s *userConcurrencyPresetUserRepoStub) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected RemoveGroupFromUserAllowedGroups")
}
func (s *userConcurrencyPresetUserRepoStub) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	panic("unexpected ListUserAuthIdentities")
}
func (s *userConcurrencyPresetUserRepoStub) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected UnbindUserAuthProvider")
}
func (s *userConcurrencyPresetUserRepoStub) UpdateTotpSecret(context.Context, int64, *string) error {
	panic("unexpected UpdateTotpSecret")
}
func (s *userConcurrencyPresetUserRepoStub) EnableTotp(context.Context, int64) error {
	panic("unexpected EnableTotp")
}
func (s *userConcurrencyPresetUserRepoStub) DisableTotp(context.Context, int64) error {
	panic("unexpected DisableTotp")
}

type userConcurrencyPresetInvalidatorStub struct {
	userIDs []int64
}

func (s *userConcurrencyPresetInvalidatorStub) InvalidateAuthCacheByKey(context.Context, string) {}
func (s *userConcurrencyPresetInvalidatorStub) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}
func (s *userConcurrencyPresetInvalidatorStub) InvalidateAuthCacheByGroupID(context.Context, int64) {}

func TestUserConcurrencyPresetServiceCreateValidation(t *testing.T) {
	repo := newUserConcurrencyPresetRepoStub()
	userRepo := newUserConcurrencyPresetUserRepoStub(&User{ID: 11, Role: RoleUser})
	svc := NewUserConcurrencyPresetService(repo, userRepo, nil, nil)

	_, err := svc.CreatePreset(context.Background(), &UserConcurrencyPreset{Name: "", TargetConcurrency: 12, UserIDs: []int64{11}})
	require.ErrorContains(t, err, "name is required")

	_, err = svc.CreatePreset(context.Background(), &UserConcurrencyPreset{Name: "bad", TargetConcurrency: 0, UserIDs: []int64{11}})
	require.ErrorContains(t, err, "target_concurrency must be >= 1")

	_, err = svc.CreatePreset(context.Background(), &UserConcurrencyPreset{Name: "bad", TargetConcurrency: 12})
	require.ErrorContains(t, err, "user_ids is required")
}

func TestUserConcurrencyPresetServiceRejectsAdminTargets(t *testing.T) {
	repo := newUserConcurrencyPresetRepoStub()
	userRepo := newUserConcurrencyPresetUserRepoStub(&User{ID: 1, Role: RoleAdmin})
	svc := NewUserConcurrencyPresetService(repo, userRepo, nil, nil)

	_, err := svc.CreatePreset(context.Background(), &UserConcurrencyPreset{Name: "admins", TargetConcurrency: 12, UserIDs: []int64{1}})

	require.ErrorContains(t, err, "admin users cannot be preset targets")
}

func TestUserConcurrencyPresetServiceApplyPresetSetsConcurrencyAndInvalidatesAuthCache(t *testing.T) {
	repo := newUserConcurrencyPresetRepoStub()
	repo.presets[7] = &UserConcurrencyPreset{ID: 7, Name: "day", TargetConcurrency: 12, UserIDs: []int64{11, 22}}
	userRepo := newUserConcurrencyPresetUserRepoStub(
		&User{ID: 11, Role: RoleUser, Concurrency: 3},
		&User{ID: 22, Role: RoleUser, Concurrency: 4},
	)
	invalidator := &userConcurrencyPresetInvalidatorStub{}
	svc := NewUserConcurrencyPresetService(repo, userRepo, invalidator, nil)

	run, err := svc.ApplyPreset(context.Background(), 7, UserConcurrencyPresetTriggerManual)

	require.NoError(t, err)
	require.Equal(t, UserConcurrencyPresetRunSuccess, run.Status)
	require.Equal(t, 2, run.AffectedCount)
	require.Equal(t, []int64{11, 22}, userRepo.batchSetUserIDs)
	require.Equal(t, 12, userRepo.batchSetValue)
	require.Equal(t, []int64{11, 22}, invalidator.userIDs)
	require.Len(t, repo.runs, 1)
	require.Equal(t, UserConcurrencyPresetRunSuccess, repo.runs[0].Status)
}

func TestUserConcurrencyPresetServiceApplyPresetLogsFailure(t *testing.T) {
	repo := newUserConcurrencyPresetRepoStub()
	repo.presets[7] = &UserConcurrencyPreset{ID: 7, Name: "day", TargetConcurrency: 12, UserIDs: []int64{11}}
	userRepo := newUserConcurrencyPresetUserRepoStub(&User{ID: 11, Role: RoleUser, Concurrency: 3})
	userRepo.batchSetErr = errors.New("db down")
	svc := NewUserConcurrencyPresetService(repo, userRepo, nil, nil)

	run, err := svc.ApplyPreset(context.Background(), 7, UserConcurrencyPresetTriggerManual)

	require.ErrorContains(t, err, "db down")
	require.NotNil(t, run)
	require.Equal(t, UserConcurrencyPresetRunFailed, run.Status)
	require.Equal(t, "db down", run.ErrorMessage)
	require.Len(t, repo.runs, 1)
}

func TestUserConcurrencyPresetServiceRunDueSchedulesMarksOnlySuccessfulRuns(t *testing.T) {
	repo := newUserConcurrencyPresetRepoStub()
	runDate := time.Date(2026, 5, 23, 9, 0, 0, 0, time.Local)
	repo.due = []*UserConcurrencyPreset{{ID: 7, Name: "day", TargetConcurrency: 12, UserIDs: []int64{11}}}
	repo.presets[7] = repo.due[0]
	userRepo := newUserConcurrencyPresetUserRepoStub(&User{ID: 11, Role: RoleUser, Concurrency: 3})
	svc := NewUserConcurrencyPresetService(repo, userRepo, nil, nil)

	err := svc.RunDueSchedules(context.Background(), "09:00", runDate)

	require.NoError(t, err)
	require.Len(t, repo.markedRuns, 1)
	require.Equal(t, runDate, repo.markedRuns[0])
}
