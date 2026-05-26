package promptmetrics

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractorOpenAIChatOnlyCapturesUserMessages(t *testing.T) {
	extractor := NewExtractor()
	got := extractor.Extract("/v1/chat/completions", []byte(`{
		"model":"gpt-test",
		"messages":[
			{"role":"system","content":"hidden"},
			{"role":"user","content":"hello"},
			{"role":"assistant","content":"answer"},
			{"role":"user","content":[{"type":"text","text":"second"},{"type":"image_url","image_url":{"url":"x"}}]}
		]
	}`))

	require.Equal(t, "openai_chat", got.SourceProtocol)
	require.Equal(t, "gpt-test", got.RequestedModel)
	require.Equal(t, "hello\n\nsecond", got.Text)
	require.Equal(t, 2, got.Segments)
}

func TestExtractorResponsesStringInput(t *testing.T) {
	extractor := NewExtractor()
	got := extractor.Extract("/responses", []byte(`{"model":"gpt-5","input":"explain this code"}`))

	require.Equal(t, "openai_responses", got.SourceProtocol)
	require.Equal(t, "explain this code", got.Text)
	require.Equal(t, 1, got.Segments)
}

func TestExtractorResponsesArrayRequiresUserRole(t *testing.T) {
	extractor := NewExtractor()
	got := extractor.Extract("/responses", []byte(`{
		"input":[
			{"type":"message","content":[{"type":"input_text","text":"missing role"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"assistant answer"}]},
			{"type":"tool_call","role":"user","text":"tool payload"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"real user"}]}
		]
	}`))

	require.Equal(t, "openai_responses", got.SourceProtocol)
	require.Equal(t, "real user", got.Text)
	require.Equal(t, 1, got.Segments)
}

func TestExtractorRejectsInvalidJSONWithoutTruncatedFallback(t *testing.T) {
	extractor := NewExtractor()
	body := []byte(`{"messages":[{"role":"user","content":"` + strings.Repeat("x", 1500) + `"}`)
	truncated := body[:1024]

	got := extractor.Extract("/v1/chat/completions", truncated)

	require.Empty(t, got.Text)
	require.False(t, got.PromptTruncated)
}

func TestExtractorTruncatedOpenAIChatKeepsUserPrompt(t *testing.T) {
	extractor := NewExtractor()
	body := []byte(`{"messages":[{"role":"user","content":"` + strings.Repeat("x", 1500) + `"}`)
	truncated := body[:1024]

	got := extractor.ExtractTruncated("/v1/chat/completions", truncated)

	require.Equal(t, "openai_chat", got.SourceProtocol)
	require.True(t, got.PromptTruncated)
	require.NotEmpty(t, got.Text)
	require.True(t, strings.HasPrefix(strings.Repeat("x", 1500), got.Text))
	require.Equal(t, 1, got.Segments)
}

func TestExtractorTruncatedResponsesStringInput(t *testing.T) {
	extractor := NewExtractor()
	body := []byte(`{"input":"` + strings.Repeat("write tests ", 200) + `"}`)

	got := extractor.ExtractTruncated("/responses", body[:512])

	require.Equal(t, "openai_responses", got.SourceProtocol)
	require.True(t, got.PromptTruncated)
	require.NotEmpty(t, got.Text)
	require.Contains(t, got.Text, "write tests")
}

func TestExtractorTruncatedResponsesArrayKeepsOnlyUserMessage(t *testing.T) {
	extractor := NewExtractor()
	body := []byte(`{"input":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"assistant"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"` + strings.Repeat("user text ", 80) + `"}`)

	got := extractor.ExtractTruncated("/responses", body[:512])

	require.Equal(t, "openai_responses", got.SourceProtocol)
	require.True(t, got.PromptTruncated)
	require.Contains(t, got.Text, "user text")
	require.NotContains(t, got.Text, "assistant")
}

func TestExtractorTruncatedImagesPrompt(t *testing.T) {
	extractor := NewExtractor()
	body := []byte(`{"prompt":"` + strings.Repeat("cat ", 400) + `"}`)

	got := extractor.ExtractTruncated("/images/generations", body[:256])

	require.Equal(t, "openai_images", got.SourceProtocol)
	require.True(t, got.PromptTruncated)
	require.NotEmpty(t, got.Text)
	require.Contains(t, got.Text, "cat")
}

func TestExtractorTruncatedGeminiRequiresUserRole(t *testing.T) {
	extractor := NewExtractor()
	body := []byte(`{"contents":[{"role":"model","parts":[{"text":"model text"}]},{"role":"user","parts":[{"text":"` + strings.Repeat("gemini ", 100) + `"}`)

	got := extractor.ExtractTruncated("/v1beta/models/gemini:generateContent", body[:512])

	require.Equal(t, "gemini", got.SourceProtocol)
	require.True(t, got.PromptTruncated)
	require.Contains(t, got.Text, "gemini")
	require.NotContains(t, got.Text, "model text")
}

func TestExtractorTruncatedDefaultInputFallback(t *testing.T) {
	extractor := NewExtractor()
	body := []byte(`{"input":"` + strings.Repeat("fallback ", 200) + `"}`)

	got := extractor.ExtractTruncated("/unknown", body[:256])

	require.Equal(t, "input", got.SourceProtocol)
	require.True(t, got.PromptTruncated)
	require.NotEmpty(t, got.Text)
	require.Contains(t, got.Text, "fallback")
}

func TestExtractorTruncatedUnquotesEscapedFragment(t *testing.T) {
	extractor := NewExtractor()
	body := []byte(`{"messages":[{"role":"user","content":"hello\nworld\"quote` + strings.Repeat("x", 200) + `"}`)

	got := extractor.ExtractTruncated("/v1/chat/completions", body[:120])

	require.Equal(t, "openai_chat", got.SourceProtocol)
	require.Contains(t, got.Text, "hello\nworld\"quote")
}

func TestExtractorGeminiCapturesUserParts(t *testing.T) {
	extractor := NewExtractor()
	got := extractor.Extract("/v1beta/models/gemini:generateContent", []byte(`{
		"contents":[
			{"role":"model","parts":[{"text":"ignore"}]},
			{"role":"user","parts":[{"text":"first"},{"text":"second"}]}
		]
	}`))

	require.Equal(t, "gemini", got.SourceProtocol)
	require.Equal(t, "first\n\nsecond", got.Text)
}
