package service

import (
	"context"
	"log"
	"net/http"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// StartAutoIndex 启动后台自动索引循环: 每 token_analysis.auto_index_interval_seconds
// 秒对 [昨天, 今天] 做一次增量索引(offset 续读, 跨天补拽昨日文件尾部, 幂等)。
// interval<=0 或 index_enabled=false 时为空操作; 页面手动触发仍可用于补历史日期。
func (s *TokenAnalysisService) StartAutoIndex() {
	if s == nil || s.cfg == nil || !s.cfg.TokenAnalysis.IndexEnabled {
		return
	}
	interval := time.Duration(s.cfg.TokenAnalysis.AutoIndexIntervalSeconds) * time.Second
	s.startAutoIndexWithInterval(interval)
}

func (s *TokenAnalysisService) startAutoIndexWithInterval(interval time.Duration) {
	if s == nil || s.repo == nil || interval <= 0 {
		return
	}
	s.autoIndexStartOnce.Do(func() {
		if !s.beginBackgroundTask() {
			return
		}
		// lifecycleCtx 在 Stop 时取消, 让正在执行的索引轮次尽快经 repo 调用报错退出,
		// 避免大文件回扫拖住优雅停机(codex 审查中等3); 手动异步索引同源。
		go func() {
			defer s.endBackgroundTask()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			s.autoIndexOnce(s.lifecycleCtx)
			for {
				select {
				case <-ticker.C:
					s.autoIndexOnce(s.lifecycleCtx)
				case <-s.autoIndexStop:
					return
				}
			}
		}()
	})
}

// StopAutoIndex 停止自动索引循环与进行中的手动异步索引并等待退出
// (幂等, 未启动时安全); 先取消再等待, 停机不会被长索引拖住。
func (s *TokenAnalysisService) StopAutoIndex() {
	if s == nil {
		return
	}
	s.autoIndexStopOnce.Do(func() {
		s.lifecycleMu.Lock()
		s.lifecycleStopped = true
		if s.lifecycleCancel != nil {
			s.lifecycleCancel()
		}
		close(s.autoIndexStop)
		s.lifecycleMu.Unlock()
	})
	s.backgroundWG.Wait()
}

func (s *TokenAnalysisService) autoIndexOnce(baseCtx context.Context) {
	ctx, cancel := context.WithTimeout(baseCtx, 10*time.Minute)
	defer cancel()

	// 起点取昨天: 跨天瞬间昨日文件尾部可能还有未索引行, offset 续读使重扫近零成本。
	now := time.Now()
	req := TokenAnalysisIndexRequest{
		StartDate: now.AddDate(0, 0, -1).Format("2006-01-02"),
		EndDate:   now.Format("2006-01-02"),
	}
	result, err := s.IndexRange(ctx, req)
	if err != nil {
		// 与手动触发撞车(已在运行)属预期; 停机取消的报错也无需告警。
		if infraerrors.Code(err) == http.StatusConflict || ctx.Err() != nil {
			return
		}
		log.Printf("[TokenAnalysisAutoIndex] index range failed: %v", err)
		return
	}
	if result != nil && (result.IndexedRows > 0 || result.FailedRows > 0) {
		log.Printf("[TokenAnalysisAutoIndex] indexed=%d skipped=%d failed=%d files=%d", result.IndexedRows, result.SkippedRows, result.FailedRows, result.Files)
	}
}
