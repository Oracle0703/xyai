package service

import (
	"context"
	"strings"
	"time"
)

// summary_json 中记录索引时计入风险的同 body 重复数, 关联后重算风险沿用。
const tokenAnalysisSameBodyCountKey = "same_body_recent_count"

const (
	tokenAnalysisUsageLinkBatch = 500
	// 旧窗口关联被改判后释放的账单可能属于本轮已处理过的请求, 再跑一轮让它认领。
	tokenAnalysisUsageLinkMaxRounds = 3
)

// recordTokenAnalysisResponse 把 response 行的完成时间/状态/输出 token 写回对应
// 请求行, 供时序兜底约束; 时间无法解析时只是少了完成事实, 不算失败行。
func (s *TokenAnalysisService) recordTokenAnalysisResponse(ctx context.Context, event tokenAnalysisArchiveEvent) error {
	archiveID := sanitizeTokenAnalysisText(event.ArchiveID)
	completedAt, err := time.Parse(time.RFC3339Nano, event.Timestamp)
	if archiveID == "" || err != nil {
		return nil
	}
	return s.repo.RecordResponse(ctx, TokenAnalysisResponseFacts{
		ArchiveID:    archiveID,
		CompletedAt:  completedAt,
		Status:       event.Status,
		OutputTokens: tokenAnalysisArchiveOutputTokens(event.Usage),
		ResponseID:   sanitizeTokenAnalysisText(strings.TrimSpace(event.ResponseID)),
		Forward:      validTokenAnalysisForwardFacts(event.TokenAnalysis),
	})
}

// tokenAnalysisArchiveOutputTokens 取归档响应 usage 的输出 token(Responses/
// Messages 的 output_tokens, Chat 的 completion_tokens)作为账单指纹; Gemini
// 等口径不同的协议不取。
func tokenAnalysisArchiveOutputTokens(usage map[string]any) *int64 {
	for _, key := range []string{"output_tokens", "completion_tokens"} {
		if v, ok := usage[key].(float64); ok && v >= 0 {
			n := int64(v)
			return &n
		}
	}
	return nil
}

// linkUsageRange 为 [start,end) 内尚未按当前规则确认的请求关联计费记录: 未关联
// 行(请求当时未完成、账单晚到)每轮重试; 旧"开始时间 ±窗口"关联行复核一次,
// 转为确认或解除; 时序关联行在出现精确证据时被纠正。只读写库内行, 不重读 JSONL。
func (s *TokenAnalysisService) linkUsageRange(ctx context.Context, start, end time.Time) error {
	if err := s.repo.MergePendingResponses(ctx); err != nil {
		return err
	}
	window := tokenAnalysisUsageMatchWindow(s.cfg)
	for round := 0; round < tokenAnalysisUsageLinkMaxRounds; round++ {
		released := false
		var afterID int64
		for {
			batch, err := s.repo.ListUsageLinkCandidates(ctx, start, end, afterID, tokenAnalysisUsageLinkBatch)
			if err != nil {
				return err
			}
			for _, candidate := range batch {
				if err := ctx.Err(); err != nil {
					return err
				}
				afterID = candidate.ID
				freed, err := s.linkUsageCandidate(ctx, candidate, window)
				if err != nil {
					return err
				}
				released = released || freed
			}
			if len(batch) < tokenAnalysisUsageLinkBatch {
				break
			}
		}
		if !released {
			return nil
		}
	}
	return nil
}

// linkUsageCandidate 关联单行; 返回是否释放或夺回了账单(需要再跑一轮让他行认领)。
//   - 有精确证据(client_request_id / 上游 response.id)的账单总是优先, 可纠正弱关联。
//   - 已确认的时序关联只在出现精确证据时改变; 登记了新复核(完成事实变化)后则按
//     新事实完整复核, 例如晚到的失败响应会解除没有精确证据的时序关联。
//   - 带关联键的成功请求在复核截止时间前仍找不到账单时不确认复核, 后续每轮继续
//     不受日期范围限制地重试; 期满或已有确定结论才确认。
//   - 失败请求、有 response.id 或尚未完成的新请求只接受精确证据。
//   - 复核后仍无证据的旧关联/被夺回行解除关联并重算风险(保留 body 自身的风险)。
//   - 写入按查找前读到的关联状态做条件更新, 其他索引实例已改过时放弃。
func (s *TokenAnalysisService) linkUsageCandidate(ctx context.Context, c TokenAnalysisUsageLinkCandidate, window time.Duration) (bool, error) {
	failed := c.ResponseStatus != nil && (*c.ResponseStatus < 200 || *c.ResponseStatus > 299)
	settledTiming := c.Method == TokenAnalysisUsageMatchTiming && !c.LinkPending
	hasKeys := c.ClientRequestID != "" || c.ResponseID != ""
	exactOnly := settledTiming || failed || (c.ClientRequestID != "" && (c.ResponseID != "" || c.CompletedAt == nil))
	pending := c.UsageLogID == nil && c.Method != TokenAnalysisUsageMatchReleased
	expected := TokenAnalysisUsageLinkState{UsageLogID: c.UsageLogID, Method: c.Method, Version: c.LinkVersion}

	var match *TokenAnalysisUsageMatch
	if hasKeys || !exactOnly {
		var err error
		match, err = s.repo.FindUsageLink(ctx, TokenAnalysisUsageLinkQuery{
			ArchiveID:       c.ArchiveID,
			ClientRequestID: c.ClientRequestID,
			ResponseID:      c.ResponseID,
			StartedAt:       c.EventTime,
			CompletedAt:     c.CompletedAt,
			OutputTokens:    c.OutputTokens,
			UserID:          c.UserID,
			APIKeyID:        c.APIKeyID,
			Model:           c.Model,
			ExactOnly:       exactOnly,
			Window:          window,
		})
		if err != nil {
			return false, err
		}
	}
	// 账单可能晚到: 只对带关联键、未失败的请求在截止时间前保留复核任务。无关联键的
	// 存量行账单早已落库, 一次查不到就是确定结论。
	awaitingBill := match == nil && c.LinkPending && c.ClientRequestID != "" && !failed &&
		c.RetryUntil != nil && time.Now().Before(*c.RetryUntil)
	expected.KeepPending = awaitingBill
	switch {
	case match == nil && (pending || settledTiming),
		match != nil && c.UsageLogID != nil && *c.UsageLogID == match.UsageLogID && c.Method == match.Method:
		// 无需改动: 仍无证据, 已确认的时序关联没有精确证据, 或复核结果与现状一致。
		if c.LinkPending && !awaitingBill {
			return false, s.repo.ClearUsageLinkPending(ctx, c.ArchiveID, c.LinkVersion)
		}
		return false, nil
	}
	sameBody := 0
	if c.SameBodyRecentCount != nil {
		sameBody = *c.SameBodyRecentCount
	} else {
		// 旧行未记录索引时的重复数; 此时库里已含本行, 扣掉自身。
		count, err := s.repo.CountSameBodyRecent(ctx, c.BodySHA256, c.UserID, c.APIKeyID, c.EventTime, tokenAnalysisDuplicateWindow)
		if err != nil {
			return false, err
		}
		sameBody = max(count-1, 0)
	}
	usage := TokenAnalysisUsageSignals{}
	if match != nil {
		usage = match.UsageSignals()
	}
	score, reasons := ScoreTokenAnalysisRisk(c.Body, usage, TokenAnalysisDuplicateSignals{SameBodyRecentCount: sameBody})
	write, err := s.repo.SetUsageLink(ctx, c.ArchiveID, expected, match, score, reasons)
	if err != nil || !write.Applied {
		return false, err
	}
	freed := c.UsageLogID != nil && (match == nil || match.UsageLogID != *c.UsageLogID)
	return freed || write.Displaced, nil
}

// decorateTokenAnalysisUsageLink 填充对外的计费关联状态; 未关联行的 Token/费用
// 由 SQL 补 0, 页面必须按状态显示为未知而不是零消耗。
func decorateTokenAnalysisUsageLink(item *TokenAnalysisRequestItem) {
	switch {
	case item.UsageLogID != nil:
		item.UsageLinkStatus = TokenAnalysisUsageLinked
		if item.UsageMatchMethod == "" {
			item.UsageMatchMethod = TokenAnalysisUsageMatchLegacyWindow
		}
		return
	case tokenAnalysisIsCountTokensPath(item.Endpoint):
		item.UsageLinkStatus = TokenAnalysisUsageNotBillable
	case item.ResponseStatus != nil && (*item.ResponseStatus < 200 || *item.ResponseStatus > 299):
		item.UsageLinkStatus = TokenAnalysisUsageFailed
	default:
		item.UsageLinkStatus = TokenAnalysisUsageUnlinked
	}
	item.UsageMatchMethod = ""
}
