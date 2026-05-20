package service

func ScoreTokenAnalysisRisk(summary TokenAnalysisBodySummary, usage TokenAnalysisUsageSignals, duplicates TokenAnalysisDuplicateSignals) (int, []TokenAnalysisRiskReason) {
	reasons := make([]TokenAnalysisRiskReason, 0, 6)
	score := 0
	add := func(code, message string, points int, metrics map[string]any) {
		score += points
		reasons = append(reasons, TokenAnalysisRiskReason{
			Code:    code,
			Message: message,
			Score:   points,
			Metrics: metrics,
		})
	}

	if usage.InputTokens >= 100000 && usage.OutputTokens <= 200 {
		add(TokenAnalysisRiskHugeInputTinyOutput, "输入 token 极高但输出很短", 45, map[string]any{
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
			"total_cost":    usage.TotalCost,
			"actual_cost":   usage.ActualCost,
		})
	}
	if duplicates.SameBodyRecentCount >= 3 && usage.InputTokens >= 20000 && usage.CacheReadTokens < usage.InputTokens/10 {
		add(TokenAnalysisRiskRepeatUncachedBody, "短时间重复请求未有效命中缓存", 35, map[string]any{
			"same_body_recent_count": duplicates.SameBodyRecentCount,
			"input_tokens":           usage.InputTokens,
			"cache_read_tokens":      usage.CacheReadTokens,
		})
	}
	if usage.InputTokens >= 50000 && usage.CacheReadTokens < usage.InputTokens/20 {
		add(TokenAnalysisRiskLowCacheHitLargeInput, "大输入请求缓存命中偏低", 25, map[string]any{
			"input_tokens":      usage.InputTokens,
			"cache_read_tokens": usage.CacheReadTokens,
		})
	}
	if duplicates.SimilarRecentCount >= 5 && usage.InputTokens >= 10000 {
		add(TokenAnalysisRiskRapidSimilarRequests, "短时间内存在多次相似大请求", 20, map[string]any{
			"similar_recent_count": duplicates.SimilarRecentCount,
			"input_tokens":         usage.InputTokens,
		})
	}
	if summary.SystemChars >= 20000 || (summary.SystemChars > 0 && summary.SystemChars > summary.UserChars) {
		add(TokenAnalysisRiskOversizedSystemPrompt, "system 内容明显偏大", 20, map[string]any{
			"system_chars": summary.SystemChars,
			"user_chars":   summary.UserChars,
		})
	}
	if summary.ToolsCount >= 10 && usage.InputTokens >= 20000 && usage.OutputTokens <= 500 {
		add(TokenAnalysisRiskToolHeavyShortOutput, "工具定义较多但输出很短", 15, map[string]any{
			"tools_count":   summary.ToolsCount,
			"input_tokens":  usage.InputTokens,
			"output_tokens": usage.OutputTokens,
		})
	}

	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score, reasons
}
