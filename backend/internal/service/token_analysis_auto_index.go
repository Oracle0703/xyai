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
	s.autoIndexWG.Add(1)
	go func() {
		defer s.autoIndexWG.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		s.autoIndexOnce()
		for {
			select {
			case <-ticker.C:
				s.autoIndexOnce()
			case <-s.autoIndexStop:
				return
			}
		}
	}()
}

// StopAutoIndex 停止自动索引循环并等待退出(幂等, 未启动时安全)。
func (s *TokenAnalysisService) StopAutoIndex() {
	if s == nil {
		return
	}
	s.autoIndexStopOnce.Do(func() {
		close(s.autoIndexStop)
	})
	s.autoIndexWG.Wait()
}

func (s *TokenAnalysisService) autoIndexOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 起点取昨天: 跨天瞬间昨日文件尾部可能还有未索引行, offset 续读使重扫近零成本。
	now := time.Now()
	req := TokenAnalysisIndexRequest{
		StartDate: now.AddDate(0, 0, -1).Format("2006-01-02"),
		EndDate:   now.Format("2006-01-02"),
	}
	result, err := s.IndexRange(ctx, req)
	if err != nil {
		// 与手动触发撞车(已在运行)属预期, 静默跳过本轮。
		if infraerrors.Code(err) == http.StatusConflict {
			return
		}
		log.Printf("[TokenAnalysisAutoIndex] index range failed: %v", err)
		return
	}
	if result != nil && (result.IndexedRows > 0 || result.FailedRows > 0) {
		log.Printf("[TokenAnalysisAutoIndex] indexed=%d skipped=%d failed=%d files=%d", result.IndexedRows, result.SkippedRows, result.FailedRows, result.Files)
	}
}
