package service

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
	// 项目归因: 加载已知仓库根(跨天累积), 索引过程中边学习边用于
	// Copilot 附件路径前缀匹配, 结束时把新学到的根批量落库。
	roots, err := newTokenAnalysisProjectRoots(ctx, s.repo)
	if err != nil {
		return nil, err
	}
	result := &TokenAnalysisIndexResult{}
	archiveDir := s.archiveDir(ctx)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file := filepath.Join(archiveDir, day.Format("2006-01-02")+".jsonl")
		fileResult, err := s.indexArchiveFile(ctx, file, roots)
		if err != nil {
			if flushErr := roots.flush(ctx, s.repo); flushErr != nil {
				return nil, errors.Join(err, flushErr)
			}
			return nil, err
		}
		result.IndexedRows += fileResult.IndexedRows
		result.SkippedRows += fileResult.SkippedRows
		result.FailedRows += fileResult.FailedRows
		if fileResult.Files > 0 {
			result.Files += fileResult.Files
		}
	}
	if err := roots.flush(ctx, s.repo); err != nil {
		return nil, err
	}
	return result, nil
}

// tokenAnalysisProjectRoots 维护索引期间的已知仓库根缓存:
// learned 暂存本次新学到的根, lookup 为全量(持久化 + 新学)小写归一集合。
type tokenAnalysisProjectRoots struct {
	lookup  map[string]string
	sorted  []string
	learned map[string]string
	dirty   bool
}

func newTokenAnalysisProjectRoots(ctx context.Context, repo TokenAnalysisRepository) (*tokenAnalysisProjectRoots, error) {
	persisted, err := repo.LoadProjectRoots(ctx)
	if err != nil {
		return nil, err
	}
	r := &tokenAnalysisProjectRoots{
		lookup:  make(map[string]string, len(persisted)),
		learned: make(map[string]string),
	}
	for root, project := range persisted {
		r.lookup[root] = project
	}
	r.rebuildSorted()
	return r, nil
}

func (r *tokenAnalysisProjectRoots) learn(workdir, project string) {
	if workdir == "" || project == "" {
		return
	}
	key := strings.ToLower(workdir)
	if _, ok := r.lookup[key]; ok {
		return
	}
	r.lookup[key] = project
	r.learned[key] = project
	r.dirty = true
}

func (r *tokenAnalysisProjectRoots) match(hints []string) (string, string) {
	if len(hints) == 0 {
		return "", ""
	}
	if r.dirty {
		r.rebuildSorted()
	}
	if len(r.sorted) == 0 {
		return "", ""
	}
	root := MatchAttributionKnownRoot(hints, r.sorted)
	if root == "" {
		return "", ""
	}
	return root, r.lookup[root]
}

func (r *tokenAnalysisProjectRoots) rebuildSorted() {
	r.sorted = r.sorted[:0]
	for root := range r.lookup {
		r.sorted = append(r.sorted, root)
	}
	sort.Slice(r.sorted, func(i, j int) bool { return len(r.sorted[i]) > len(r.sorted[j]) })
	r.dirty = false
}

func (r *tokenAnalysisProjectRoots) flush(ctx context.Context, repo TokenAnalysisRepository) error {
	if len(r.learned) == 0 {
		return nil
	}
	if err := repo.UpsertProjectRoots(ctx, r.learned); err != nil {
		return err
	}
	r.learned = make(map[string]string)
	return nil
}

func (s *TokenAnalysisService) indexArchiveFile(ctx context.Context, file string, roots *tokenAnalysisProjectRoots) (*TokenAnalysisIndexResult, error) {
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

	state, err := s.repo.GetIndexState(ctx, file)
	if err != nil {
		return nil, err
	}
	var offset int64
	var lastArchiveID string
	if state != nil && state.LastOffset > 0 {
		if _, err := f.Seek(state.LastOffset, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek request archive %s: %w", file, err)
		}
		offset = state.LastOffset
		lastArchiveID = state.LastArchiveID
	}

	startedAt := time.Now().UTC()
	_ = s.repo.UpdateIndexState(ctx, TokenAnalysisIndexState{
		SourceFile:    file,
		LastOffset:    offset,
		LastArchiveID: lastArchiveID,
		StartedAt:     &startedAt,
		UpdatedAt:     startedAt,
	})

	reader := bufio.NewReader(f)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineOffset := offset + int64(len(line))
			incompleteTrailingLine := readErr == io.EOF && !strings.HasSuffix(string(line), "\n")
			indexed, skipped, failed, archiveID, err := s.indexArchiveLine(ctx, file, lineOffset, line, readErr == io.EOF, roots)
			if !incompleteTrailingLine || indexed > 0 || failed > 0 {
				offset = lineOffset
			}
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

func (s *TokenAnalysisService) indexArchiveLine(ctx context.Context, file string, offset int64, line []byte, eof bool, roots *tokenAnalysisProjectRoots) (indexed int64, skipped int64, failed int64, archiveID string, err error) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" {
		return 0, 1, 0, "", nil
	}
	var event tokenAnalysisArchiveEvent
	if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
		if eof && !strings.HasSuffix(string(line), "\n") {
			return 0, 1, 0, "", nil
		}
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

	// 项目归因: 直接命中则学习仓库根; Copilot 等只有路径线索时与已知根
	// 做前缀匹配。未归因留空, 由聚合层显式呈现为 unattributed。
	attribution := ExtractProjectAttribution(firstNonEmptyTokenAnalysisString(event.Endpoint, event.Path), event.UserAgent, event.Body)
	if attribution.Attributed() {
		if roots != nil {
			roots.learn(attribution.Workdir, attribution.Project)
		}
	} else if roots != nil && len(attribution.PathHints) > 0 {
		if root, project := roots.match(attribution.PathHints); project != "" {
			attribution.Workdir = root
			attribution.Project = project
			attribution.Source = ProjectAttributionSourceKnownRoot
		}
	}

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
		ClientWorkdir:        attribution.Workdir,
		ClientProject:        attribution.Project,
		ClientBranch:         attribution.Branch,
		AttributionSource:    attribution.Source,
		IndexedAt:            time.Now().UTC(),
		SourceFile:           filepath.Base(file),
		SourceOffset:         &offset,
	}
	if err := s.repo.UpsertRequestSummary(ctx, summary); err != nil {
		return 0, 0, 0, archiveID, err
	}
	// 净输入留存: 原始 JSONL 按保留期删除后, 全文仍可供"输入是否符合标准"类
	// 分析回溯; 质量字段由后续评估任务回填, 这里只存原料。
	if maxChars := tokenAnalysisInputStoreMaxChars(s.cfg); maxChars > 0 && strings.TrimSpace(bodySummary.LastUserText) != "" {
		// sha256 对"脱敏后未截断"文本计算: 同一人类输入跨 agent 轮次/跨截断配置
		// 稳定, 且不构成 secret 原文的可验证指纹(不可拿候选 secret 离线比对)。
		redacted := RedactTokenAnalysisInputText(bodySummary.LastUserText)
		redactedSum := sha256.Sum256([]byte(redacted))
		content, truncated := truncateTokenAnalysisRunes(redacted, maxChars)
		if err := s.repo.UpsertUserInput(ctx, &TokenAnalysisUserInput{
			ArchiveID:     event.ArchiveID,
			EventTime:     eventTime,
			UserID:        event.UserID,
			Content:       content,
			ContentSHA256: hex.EncodeToString(redactedSum[:]),
			Chars:         len([]rune(content)),
			Truncated:     truncated,
		}); err != nil {
			return 0, 0, 0, archiveID, err
		}
	}
	return 1, 0, 0, archiveID, nil
}

func tokenAnalysisInputStoreMaxChars(cfg *config.Config) int {
	if cfg == nil {
		return 0
	}
	return cfg.TokenAnalysis.InputStoreMaxChars
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

// archiveDir 返回归档目录: 优先运行态设置(后台可热切换, 与写入端同源),
// 未注入设置服务或取值为空时回退 config.yaml。
func (s *TokenAnalysisService) archiveDir(ctx context.Context) string {
	if s != nil && s.settings != nil {
		if dir := strings.TrimSpace(s.settings.GetRequestArchiveRuntimeConfig(ctx).Dir); dir != "" {
			return dir
		}
	}
	return tokenAnalysisArchiveDir(s.cfg)
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
