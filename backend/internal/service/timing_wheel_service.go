package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/zeromicro/go-zero/core/collection"
)

var newTimingWheel = collection.NewTimingWheel

// TimingWheelService wraps go-zero's TimingWheel for task scheduling
type TimingWheelService struct {
	tw             *collection.TimingWheel
	recurringMu    sync.Mutex
	recurringTasks map[string]*recurringTask
	stopped        bool
	stopOnce       sync.Once
}

type recurringTask struct {
	cancelled bool
}

// NewTimingWheelService creates a new TimingWheelService instance
func NewTimingWheelService() (*TimingWheelService, error) {
	// 1 second tick, 3600 slots = supports up to 1 hour delay
	// execute function: runs func() type tasks
	tw, err := newTimingWheel(1*time.Second, 3600, func(key, value any) {
		if fn, ok := value.(func()); ok {
			fn()
		}
	})
	if err != nil {
		return nil, fmt.Errorf("创建 timing wheel 失败: %w", err)
	}
	return &TimingWheelService{tw: tw, recurringTasks: make(map[string]*recurringTask)}, nil
}

// Start starts the timing wheel
func (s *TimingWheelService) Start() {
	logger.LegacyPrintf("service.timing_wheel", "%s", "[TimingWheel] Started (auto-start by go-zero)")
}

// Stop stops the timing wheel
func (s *TimingWheelService) Stop() {
	s.stopOnce.Do(func() {
		s.recurringMu.Lock()
		s.stopped = true
		for name, task := range s.recurringTasks {
			task.cancelled = true
			delete(s.recurringTasks, name)
		}
		s.recurringMu.Unlock()
		s.tw.Stop()
		logger.LegacyPrintf("service.timing_wheel", "%s", "[TimingWheel] Stopped")
	})
}

// Schedule schedules a one-time task
func (s *TimingWheelService) Schedule(name string, delay time.Duration, fn func()) {
	s.recurringMu.Lock()
	defer s.recurringMu.Unlock()
	if s.stopped {
		logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] SetTimer failed for %q: timing wheel is stopped", name)
		return
	}
	s.cancelRecurringLocked(name)
	if err := s.tw.SetTimer(name, fn, delay); err != nil {
		logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] SetTimer failed for %q: %v", name, err)
	}
}

// ScheduleRecurring schedules a recurring task
func (s *TimingWheelService) ScheduleRecurring(name string, interval time.Duration, fn func()) {
	task := &recurringTask{}
	var schedule func()
	schedule = func() {
		fn()
		s.recurringMu.Lock()
		defer s.recurringMu.Unlock()
		if s.stopped || task.cancelled || s.recurringTasks[name] != task {
			return
		}
		if err := s.tw.SetTimer(name, schedule, interval); err != nil {
			logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] recurring SetTimer failed for %q: %v", name, err)
		}
	}
	s.recurringMu.Lock()
	defer s.recurringMu.Unlock()
	if s.stopped {
		logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] initial SetTimer failed for %q: timing wheel is stopped", name)
		return
	}
	s.cancelRecurringLocked(name)
	s.recurringTasks[name] = task
	if err := s.tw.SetTimer(name, schedule, interval); err != nil {
		delete(s.recurringTasks, name)
		logger.LegacyPrintf("service.timing_wheel", "[TimingWheel] initial SetTimer failed for %q: %v", name, err)
	}
}

// Cancel cancels a scheduled task
func (s *TimingWheelService) Cancel(name string) {
	s.recurringMu.Lock()
	defer s.recurringMu.Unlock()
	s.cancelRecurringLocked(name)
}

func (s *TimingWheelService) cancelRecurringLocked(name string) {
	if task := s.recurringTasks[name]; task != nil {
		task.cancelled = true
		delete(s.recurringTasks, name)
	}
	_ = s.tw.RemoveTimer(name)
}
