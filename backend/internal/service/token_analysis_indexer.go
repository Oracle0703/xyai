package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const tokenAnalysisDuplicateWindow = 30 * time.Minute

func (s *TokenAnalysisService) indexRange(ctx context.Context, req TokenAnalysisIndexRequest) (*TokenAnalysisIndexResult, error) {
	start, end, err := tokenAnalysisIndexDates(req)
	if err != nil {
		return nil, err
	}
	result := &TokenAnalysisIndexResult{}
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file := filepath.Join(tokenAnalysisArchiveDir(s.cfg), day.Format("2006-01-02")+".jsonl")
		fileResult, err := s.indexArchiveFile(ctx, file)
		if err != nil {
			return nil, err
		}
		result.IndexedRows += fileResult.IndexedRows
		result.SkippedRows += fileResult.SkippedRows
		result.FailedRows += fileResult.FailedRows
		if fileResult.Files > 0 {
			result.Files += fileResult.Files
		}
	}
	return result, nil
}

func (s *TokenAnalysisService) indexArchiveFile(ctx context.Context, file string) (*TokenAnalysisIndexResult, error) {
	result := &TokenAnalysisIndexResult{}
	f, err := os.Open(file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return result, nil
		}
		return nil, fmt.Errorf("open request archive %s: %w", file, err)
	}
	defer f.Close()
	result.Files = 1

	startedAt := time.Now().UTC()
	_ = s.repo.UpdateIndexState(ctx, TokenAnalysisIndexState{SourceFile: file, StartedAt: &startedAt, UpdatedAt: startedAt})

	reader := bufio.NewReader(f)
	var offset int64
	var lastArchiveID string
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			offset += int64(len(line))
			indexed, skipped, failed, archiveID, err := s.indexArchiveLine(ctx, file, offset, line)
			if archiveID != "" {
				lastArchiveID = archiveID
			}
			result.IndexedRows += indexed
			result.SkippedRows += skipped
			result.FailedRows += failed
			if err != nil {
				result.FailedRows++
				_ = s.repo.UpdateIndexState(ctx, TokenAnalysisIndexState{
					SourceFile:    file,
					LastOffset:    offset,
					LastArchiveID: lastArchiveID,
					FailedRows:    1,
					LastError:     err.Error(),
					UpdatedAt:     time.Now().UTC(),
				})
			} else if indexed > 0 || failed > 0 {
				_ = s.repo.UpdateIndexState(ctx, TokenAnalysisIndexState{
					SourceFile:    file,
					LastOffset:    offset,
					LastArchiveID: lastArchiveID,
					ProcessedRows: indexed,
					FailedRows:    failed,
					UpdatedAt:     time.Now().UTC(),
				})
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return nil, fmt.Errorf("read request archive %s: %w", file, readErr)
		}
	}
	finishedAt := time.Now().UTC()
	_ = s.repo.UpdateIndexState(ctx, TokenAnalysisIndexState{
		SourceFile:    file,
		LastOffset:    offset,
		LastArchiveID: lastArchiveID,
		FinishedAt:    &finishedAt,
		UpdatedAt:     finishedAt,
	})
	return result, nil
}

func (s *TokenAnalysisService) indexArchiveLine(ctx context.Context, file string, offset int64, line []byte) (indexed int64, skipped int64, failed int64, archiveID string, err error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return 0, 1, 0, "", nil
	}
	var event tokenAnalysisArchiveEvent
	if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
		return 0, 0, 1, "", nil
	}
	archiveID = event.ArchiveID
	if event.Event != "request" {
		return 0, 1, 0, archiveID, nil
	}
	eventTime, err := time.Parse(time.RFC3339Nano, event.Timestamp)
	if err != nil {
		return 0, 0, 0, archiveID, fmt.Errorf("parse archive timestamp %s: %w", event.Timestamp, err)
	}
	bodySummary, err := SummarizeTokenAnalysisRequest(event.Endpoint, []byte(event.Body), tokenAnalysisMaxPreviewChars(s.cfg))
	if err != nil {
		return 0, 0, 0, archiveID, err
	}
	if bodySummary.Model == "" {
		bodySummary.Model = event.Model
	}

	match, err := s.repo.FindNearestUsageLog(ctx, eventTime, event.UserID, event.APIKeyID, firstNonEmptyTokenAnalysisString(bodySummary.Model, event.Model), tokenAnalysisUsageMatchWindow(s.cfg))
	if err != nil {
		return 0, 0, 0, archiveID, err
	}
	usage := TokenAnalysisUsageSignals{}
	var usageLogID *int64
	var confidence int16
	if match != nil {
		usage = match.UsageSignals()
		usageLogID = &match.UsageLogID
		confidence = match.MatchConfidence
	}
	sameBodyCount, err := s.repo.CountSameBodyRecent(ctx, event.BodySHA256, event.UserID, event.APIKeyID, eventTime, tokenAnalysisDuplicateWindow)
	if err != nil {
		return 0, 0, 0, archiveID, err
	}
	score, reasons := ScoreTokenAnalysisRisk(bodySummary, usage, TokenAnalysisDuplicateSignals{SameBodyRecentCount: sameBodyCount})
	summary := &TokenAnalysisRequestSummary{
		ArchiveID:            event.ArchiveID,
		UsageLogID:           usageLogID,
		MatchConfidence:      confidence,
		EventTime:            eventTime,
		UserID:               event.UserID,
		APIKeyID:             event.APIKeyID,
		AccountID:            event.AccountID,
		GroupID:              event.GroupID,
		Model:                firstNonEmptyTokenAnalysisString(bodySummary.Model, event.Model),
		Endpoint:             firstNonEmptyTokenAnalysisString(event.Endpoint, event.Path),
		Method:               event.Method,
		RequestBodySize:      event.BodySize,
		RequestBodyTruncated: event.BodyTruncated,
		BodySHA256:           event.BodySHA256,
		MessageCount:         bodySummary.MessageCount,
		SystemChars:          bodySummary.SystemChars,
		UserChars:            bodySummary.UserChars,
		LastUserPreview:      bodySummary.LastUserPreview,
		ToolsCount:           bodySummary.ToolsCount,
		ImageCount:           bodySummary.ImageCount,
		SummaryJSON:          bodySummary.SummaryJSON,
		RiskScore:            score,
		RiskReasons:          reasons,
		IndexedAt:            time.Now().UTC(),
		SourceFile:           filepath.Base(file),
		SourceOffset:         &offset,
	}
	if err := s.repo.UpsertRequestSummary(ctx, summary); err != nil {
		return 0, 0, 0, archiveID, err
	}
	return 1, 0, 0, archiveID, nil
}

func tokenAnalysisIndexDates(req TokenAnalysisIndexRequest) (time.Time, time.Time, error) {
	loc := time.Local
	if strings.TrimSpace(req.Timezone) != "" {
		loaded, err := time.LoadLocation(req.Timezone)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid timezone: %w", err)
		}
		loc = loaded
	}
	startRaw := strings.TrimSpace(req.StartDate)
	endRaw := strings.TrimSpace(req.EndDate)
	if startRaw == "" {
		startRaw = time.Now().In(loc).Format("2006-01-02")
	}
	if endRaw == "" {
		endRaw = startRaw
	}
	start, err := time.ParseInLocation("2006-01-02", startRaw, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start_date: %w", err)
	}
	end, err := time.ParseInLocation("2006-01-02", endRaw, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end_date: %w", err)
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("end_date must be on or after start_date")
	}
	return start, end, nil
}

func tokenAnalysisArchiveDir(cfg *config.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.Gateway.RequestArchive.Dir) != "" {
		return strings.TrimSpace(cfg.Gateway.RequestArchive.Dir)
	}
	return "data/request-archive"
}

func tokenAnalysisMaxPreviewChars(cfg *config.Config) int {
	if cfg != nil && cfg.TokenAnalysis.MaxPreviewChars > 0 {
		return cfg.TokenAnalysis.MaxPreviewChars
	}
	return 300
}

func tokenAnalysisUsageMatchWindow(cfg *config.Config) time.Duration {
	seconds := 10
	if cfg != nil && cfg.TokenAnalysis.UsageMatchWindowSeconds > 0 {
		seconds = cfg.TokenAnalysis.UsageMatchWindowSeconds
	}
	return time.Duration(seconds) * time.Second
}

func firstNonEmptyTokenAnalysisString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
