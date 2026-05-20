package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenAnalysisRiskHugeInputTinyOutput(t *testing.T) {
	usage := TokenAnalysisUsageSignals{
		InputTokens:         220000,
		OutputTokens:        32,
		CacheReadTokens:     0,
		CacheCreationTokens: 0,
		TotalCost:           1.5,
	}
	summary := TokenAnalysisBodySummary{MessageCount: 3, UserChars: 20000}

	score, reasons := ScoreTokenAnalysisRisk(summary, usage, TokenAnalysisDuplicateSignals{})

	require.GreaterOrEqual(t, score, 40)
	requireRiskReason(t, reasons, TokenAnalysisRiskHugeInputTinyOutput)
}

func TestTokenAnalysisRiskRepeatUncachedBody(t *testing.T) {
	usage := TokenAnalysisUsageSignals{
		InputTokens:         90000,
		OutputTokens:        900,
		CacheReadTokens:     0,
		CacheCreationTokens: 100,
	}
	summary := TokenAnalysisBodySummary{MessageCount: 2, UserChars: 10000}
	dupe := TokenAnalysisDuplicateSignals{SameBodyRecentCount: 4}

	score, reasons := ScoreTokenAnalysisRisk(summary, usage, dupe)

	require.GreaterOrEqual(t, score, 30)
	requireRiskReason(t, reasons, TokenAnalysisRiskRepeatUncachedBody)
}

func TestTokenAnalysisRiskCapsAt100(t *testing.T) {
	usage := TokenAnalysisUsageSignals{InputTokens: 500000, OutputTokens: 1, CacheReadTokens: 0}
	summary := TokenAnalysisBodySummary{SystemChars: 60000, UserChars: 100000, ToolsCount: 25}
	dupe := TokenAnalysisDuplicateSignals{SameBodyRecentCount: 20}

	score, _ := ScoreTokenAnalysisRisk(summary, usage, dupe)

	require.Equal(t, 100, score)
}

func requireRiskReason(t *testing.T, reasons []TokenAnalysisRiskReason, code string) {
	t.Helper()
	for _, reason := range reasons {
		if reason.Code == code {
			return
		}
	}
	require.Failf(t, "missing risk reason", "code=%s reasons=%v", code, reasons)
}
