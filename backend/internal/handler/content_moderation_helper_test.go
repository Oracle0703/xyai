package handler

import (
	"net/http/httptest"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildContentModerationInputCopiesPromptRiskJudgeHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set(service.PromptRiskJudgeHeaderName, "internal-token")
	ctx.Request = req

	input := buildContentModerationInput(ctx, nil, middleware2.AuthSubject{UserID: 42}, service.ContentModerationProtocolOpenAIChat, "gpt-5", []byte(`{"messages":[]}`))

	require.Equal(t, "internal-token", input.PromptRiskJudgeHeader)
}
