package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

var userConcurrencyPresetScheduleTimePattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

type UserConcurrencyPresetService struct {
	repo                 UserConcurrencyPresetRepository
	userRepo             UserRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
	location             *time.Location
}

func NewUserConcurrencyPresetService(
	repo UserConcurrencyPresetRepository,
	userRepo UserRepository,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	cfg *config.Config,
) *UserConcurrencyPresetService {
	location := time.Local
	if cfg != nil && strings.TrimSpace(cfg.Timezone) != "" {
		if parsed, err := time.LoadLocation(cfg.Timezone); err == nil && parsed != nil {
			location = parsed
		}
	}
	return &UserConcurrencyPresetService{
		repo:                 repo,
		userRepo:             userRepo,
		authCacheInvalidator: authCacheInvalidator,
		location:             location,
	}
}

func (s *UserConcurrencyPresetService) ListPresets(ctx context.Context) ([]*UserConcurrencyPreset, error) {
	return s.repo.List(ctx)
}

func (s *UserConcurrencyPresetService) CreatePreset(ctx context.Context, preset *UserConcurrencyPreset) (*UserConcurrencyPreset, error) {
	if err := s.validatePreset(ctx, preset); err != nil {
		return nil, err
	}
	preset.Name = strings.TrimSpace(preset.Name)
	preset.Description = strings.TrimSpace(preset.Description)
	preset.UserIDs = normalizeInt64IDs(preset.UserIDs)
	if !preset.ScheduleEnabled {
		preset.ScheduleTime = ""
	}
	return s.repo.Create(ctx, preset)
}

func (s *UserConcurrencyPresetService) UpdatePreset(ctx context.Context, id int64, preset *UserConcurrencyPreset) (*UserConcurrencyPreset, error) {
	if id <= 0 {
		return nil, errors.New("invalid preset id")
	}
	if err := s.validatePreset(ctx, preset); err != nil {
		return nil, err
	}
	preset.ID = id
	preset.Name = strings.TrimSpace(preset.Name)
	preset.Description = strings.TrimSpace(preset.Description)
	preset.UserIDs = normalizeInt64IDs(preset.UserIDs)
	if !preset.ScheduleEnabled {
		preset.ScheduleTime = ""
	}
	return s.repo.Update(ctx, preset)
}

func (s *UserConcurrencyPresetService) DeletePreset(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid preset id")
	}
	return s.repo.Delete(ctx, id)
}

func (s *UserConcurrencyPresetService) ListRuns(ctx context.Context, presetID int64, limit int) ([]*UserConcurrencyPresetRun, error) {
	if presetID <= 0 {
		return nil, errors.New("invalid preset id")
	}
	return s.repo.ListRuns(ctx, presetID, limit)
}

func (s *UserConcurrencyPresetService) RunDueSchedules(ctx context.Context, scheduleTime string, runDate time.Time) error {
	presets, err := s.repo.ListDue(ctx, scheduleTime, runDate)
	if err != nil {
		return err
	}
	for _, preset := range presets {
		run, applyErr := s.applyPreset(ctx, preset, UserConcurrencyPresetTriggerScheduled)
		if applyErr != nil {
			continue
		}
		if run != nil && run.Status == UserConcurrencyPresetRunSuccess {
			if err := s.repo.MarkScheduledRun(ctx, preset.ID, runDate); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *UserConcurrencyPresetService) ApplyPreset(ctx context.Context, id int64, trigger string) (*UserConcurrencyPresetRun, error) {
	preset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.applyPreset(ctx, preset, trigger)
}

func (s *UserConcurrencyPresetService) CurrentScheduleKey(now time.Time) (string, time.Time) {
	if s.location != nil {
		now = now.In(s.location)
	}
	runDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return now.Format("15:04"), runDate
}

func (s *UserConcurrencyPresetService) applyPreset(ctx context.Context, preset *UserConcurrencyPreset, trigger string) (*UserConcurrencyPresetRun, error) {
	started := time.Now()
	if trigger != UserConcurrencyPresetTriggerManual && trigger != UserConcurrencyPresetTriggerScheduled {
		trigger = UserConcurrencyPresetTriggerManual
	}
	run := &UserConcurrencyPresetRun{
		PresetID:          preset.ID,
		Trigger:           trigger,
		TargetConcurrency: preset.TargetConcurrency,
		UserIDs:           append([]int64(nil), preset.UserIDs...),
		StartedAt:         started,
		FinishedAt:        started,
	}

	validUserIDs, validateErr := s.validTargetUserIDs(ctx, preset.UserIDs)
	if validateErr != nil {
		run.Status = UserConcurrencyPresetRunFailed
		run.ErrorMessage = validateErr.Error()
		run.FinishedAt = time.Now()
		created, _ := s.repo.CreateRun(ctx, run)
		if created != nil {
			return created, validateErr
		}
		return run, validateErr
	}

	affected, err := s.userRepo.BatchSetConcurrency(ctx, validUserIDs, preset.TargetConcurrency)
	run.FinishedAt = time.Now()
	run.AffectedCount = affected
	if err != nil {
		run.Status = UserConcurrencyPresetRunFailed
		run.ErrorMessage = err.Error()
		created, _ := s.repo.CreateRun(ctx, run)
		if created != nil {
			return created, err
		}
		return run, err
	}

	run.Status = UserConcurrencyPresetRunSuccess
	created, err := s.repo.CreateRun(ctx, run)
	if s.authCacheInvalidator != nil {
		for _, uid := range validUserIDs {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, uid)
		}
	}
	if created != nil {
		return created, err
	}
	return run, err
}

func (s *UserConcurrencyPresetService) validatePreset(ctx context.Context, preset *UserConcurrencyPreset) error {
	if preset == nil {
		return errors.New("preset is required")
	}
	if strings.TrimSpace(preset.Name) == "" {
		return errors.New("name is required")
	}
	if preset.TargetConcurrency < 1 {
		return errors.New("target_concurrency must be >= 1")
	}
	if len(normalizeInt64IDs(preset.UserIDs)) == 0 {
		return errors.New("user_ids is required")
	}
	if preset.ScheduleEnabled && !userConcurrencyPresetScheduleTimePattern.MatchString(strings.TrimSpace(preset.ScheduleTime)) {
		return errors.New("schedule_time must use HH:mm")
	}
	_, err := s.validTargetUserIDs(ctx, preset.UserIDs)
	return err
}

func (s *UserConcurrencyPresetService) validTargetUserIDs(ctx context.Context, userIDs []int64) ([]int64, error) {
	ids := normalizeInt64IDs(userIDs)
	if len(ids) == 0 {
		return nil, errors.New("user_ids is required")
	}
	valid := make([]int64, 0, len(ids))
	for _, id := range ids {
		user, err := s.userRepo.GetByID(ctx, id)
		if err != nil {
			continue
		}
		if user.Role == RoleAdmin {
			return nil, fmt.Errorf("admin users cannot be preset targets: user_id=%d", id)
		}
		valid = append(valid, id)
	}
	if len(valid) == 0 {
		return nil, errors.New("no valid target users")
	}
	return valid, nil
}
