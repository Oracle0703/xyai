package securityaudit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLegacyModerationInputPreservesPromptRiskJudgeHeader(t *testing.T) {
	input := legacyModerationInput(Request{PromptRiskJudgeHeader: "signed-judge-request"})

	require.Equal(t, "signed-judge-request", input.PromptRiskJudgeHeader)
}
