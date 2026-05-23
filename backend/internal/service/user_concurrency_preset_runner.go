package service

import (
	"context"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/robfig/cron/v3"
)

type UserConcurrencyPresetRunner struct {
	service *UserConcurrencyPresetService

	cron      *cron.Cron
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewUserConcurrencyPresetRunner(service *UserConcurrencyPresetService) *UserConcurrencyPresetRunner {
	return &UserConcurrencyPresetRunner{service: service}
}

func (r *UserConcurrencyPresetRunner) Start() {
	if r == nil || r.service == nil {
		return
	}
	r.startOnce.Do(func() {
		loc := time.Local
		if r.service.location != nil {
			loc = r.service.location
		}
		c := cron.New(cron.WithParser(scheduledTestCronParser), cron.WithLocation(loc))
		_, err := c.AddFunc("* * * * *", func() { r.runOnce() })
		if err != nil {
			logger.LegacyPrintf("service.user_concurrency_preset_runner", "[UserConcurrencyPresetRunner] not started: %v", err)
			return
		}
		r.cron = c
		r.cron.Start()
		logger.LegacyPrintf("service.user_concurrency_preset_runner", "[UserConcurrencyPresetRunner] started (tick=every minute)")
	})
}

func (r *UserConcurrencyPresetRunner) Stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		if r.cron == nil {
			return
		}
		ctx := r.cron.Stop()
		select {
		case <-ctx.Done():
		case <-time.After(3 * time.Second):
			logger.LegacyPrintf("service.user_concurrency_preset_runner", "[UserConcurrencyPresetRunner] cron stop timed out")
		}
	})
}

func (r *UserConcurrencyPresetRunner) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	scheduleTime, runDate := r.service.CurrentScheduleKey(time.Now())
	if err := r.service.RunDueSchedules(ctx, scheduleTime, runDate); err != nil {
		logger.LegacyPrintf("service.user_concurrency_preset_runner", "[UserConcurrencyPresetRunner] run due schedules failed: %v", err)
	}
}
