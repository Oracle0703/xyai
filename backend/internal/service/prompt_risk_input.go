package service

import (
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

// extractPromptRiskInput 为 Prompt 风险审查抽取用户意图文本。与内容审核的
// ExtractContentModerationInput(只取最后一项)不同,它按 scope 抽取:
//   - newest(默认):只取**最新一轮**用户意图——定位最后一个 user item,连同其前面相邻的
//     user item 一并收集(覆盖 Codex 把 environment_context / 真实问题拆成多 item 的同一轮),
//     并跳过尾部的工具调用/工具输出/模型 item;不回溯更早的历史轮次,避免多轮会话里历史高危
//     输入污染后续普通请求(下一轮只说"谢谢"也被旧轮触发)。
//   - full:扫描请求体内**所有** user turn(显式 opt-in,用于需要全上下文审计的场景)。
//
// 两种 scope 都会剥离编码客户端注入的环境上下文包裹,只留真正的用户意图。
func extractPromptRiskInput(protocol string, body []byte, scope string) ContentModerationInput {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ContentModerationInput{}
	}
	full := normalizePromptRiskScope(scope) == PromptRiskScopeFull
	var parts []string
	var images []string

	collectMsgs := func(msgs gjson.Result, anthropic bool) {
		if full {
			collectAllUserMessages(msgs, anthropic, &parts, &images)
		} else {
			collectNewestUserMessages(msgs, anthropic, &parts, &images)
		}
	}
	collectResp := func(in gjson.Result) {
		if full {
			collectAllResponsesUserInput(in, &parts, &images)
		} else {
			collectNewestResponsesUserInput(in, &parts, &images)
		}
	}
	collectGem := func(c gjson.Result) {
		if full {
			collectAllGeminiUserContent(c, &parts, &images)
		} else {
			collectNewestGeminiUserContent(c, &parts, &images)
		}
	}

	switch protocol {
	case ContentModerationProtocolAnthropicMessages:
		collectMsgs(gjson.GetBytes(body, "messages"), true)
	case ContentModerationProtocolOpenAIChat:
		collectMsgs(gjson.GetBytes(body, "messages"), false)
	case ContentModerationProtocolOpenAIResponses:
		collectResp(gjson.GetBytes(body, "input"))
	case ContentModerationProtocolGemini:
		collectGem(gjson.GetBytes(body, "contents"))
	case ContentModerationProtocolOpenAIImages:
		addModerationText(&parts, gjson.GetBytes(body, "prompt").String())
		collectContentValue(gjson.GetBytes(body, "images"), &parts, &images)
	default:
		collectResp(gjson.GetBytes(body, "input"))
		collectMsgs(gjson.GetBytes(body, "messages"), false)
		collectGem(gjson.GetBytes(body, "contents"))
	}
	text := stripPromptRiskWrappers(strings.Join(parts, "\n"))
	out := ContentModerationInput{
		Text:   normalizeContentModerationText(text),
		Images: normalizeModerationImages(images),
	}
	out.Normalize()
	return out
}

// newestUserRange 在数组里定位"最新一轮用户意图"的连续 user 段 [start,end)。
// 做法:从末尾向前找到最后一个 user item(跳过尾部工具/模型 item),再把它前面相邻的
// 连续 user item 一并纳入(同一轮的多 item);遇到第一个非 user item 即停,不回溯更早轮次。
func newestUserRange(arr []gjson.Result, isUser func(gjson.Result) bool) (int, int) {
	last := -1
	for i := len(arr) - 1; i >= 0; i-- {
		if isUser(arr[i]) {
			last = i
			break
		}
	}
	if last < 0 {
		return 0, 0
	}
	start := last
	for start-1 >= 0 && isUser(arr[start-1]) {
		start--
	}
	return start, last + 1
}

func isChatUserMessage(msg gjson.Result) bool {
	return strings.ToLower(strings.TrimSpace(msg.Get("role").String())) == "user"
}

// isResponsesUserItem 判断 Responses input item 是否为用户意图项(非工具/模型 item)。
func isResponsesUserItem(item gjson.Result) bool {
	if role := strings.ToLower(strings.TrimSpace(item.Get("role").String())); role != "" && role != "user" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(item.Get("type").String())) {
	case "function_call", "function_call_output", "reasoning", "computer_call",
		"computer_call_output", "web_search_call", "file_search_call", "item_reference":
		return false
	}
	return true
}

func isGeminiUserContent(item gjson.Result) bool {
	role := strings.ToLower(strings.TrimSpace(item.Get("role").String()))
	return role == "" || role == "user"
}

// ---- newest scope(默认):仅最新一轮 ----

func collectNewestUserMessages(messages gjson.Result, anthropic bool, parts *[]string, images *[]string) {
	if !messages.IsArray() {
		return
	}
	arr := messages.Array()
	start, end := newestUserRange(arr, isChatUserMessage)
	for i := start; i < end; i++ {
		if anthropic {
			collectAnthropicUserContentValue(arr[i].Get("content"), parts, images)
		} else {
			collectContentValue(arr[i].Get("content"), parts, images)
		}
	}
}

func collectNewestResponsesUserInput(input gjson.Result, parts *[]string, images *[]string) {
	switch {
	case !input.Exists():
		return
	case input.Type == gjson.String:
		addModerationText(parts, input.String())
	case input.IsObject():
		collectResponsesUserItem(input, parts, images)
	case input.IsArray():
		arr := input.Array()
		start, end := newestUserRange(arr, isResponsesUserItem)
		for i := start; i < end; i++ {
			collectResponsesUserItem(arr[i], parts, images)
		}
	}
}

func collectNewestGeminiUserContent(contents gjson.Result, parts *[]string, images *[]string) {
	if !contents.IsArray() {
		return
	}
	arr := contents.Array()
	start, end := newestUserRange(arr, isGeminiUserContent)
	for i := start; i < end; i++ {
		if pa := arr[i].Get("parts"); pa.IsArray() {
			pa.ForEach(func(_, part gjson.Result) bool {
				addModerationText(parts, part.Get("text").String())
				addGeminiModerationImage(images, part)
				return true
			})
		}
	}
}

// ---- full scope(显式 opt-in):所有历史 user turn ----

func collectAllUserMessages(messages gjson.Result, anthropic bool, parts *[]string, images *[]string) {
	if !messages.IsArray() {
		return
	}
	for _, msg := range messages.Array() {
		if !isChatUserMessage(msg) {
			continue
		}
		if anthropic {
			collectAnthropicUserContentValue(msg.Get("content"), parts, images)
		} else {
			collectContentValue(msg.Get("content"), parts, images)
		}
	}
}

func collectAllResponsesUserInput(input gjson.Result, parts *[]string, images *[]string) {
	switch {
	case !input.Exists():
		return
	case input.Type == gjson.String:
		addModerationText(parts, input.String())
	case input.IsArray():
		for _, item := range input.Array() {
			collectResponsesUserItem(item, parts, images)
		}
	case input.IsObject():
		collectResponsesUserItem(input, parts, images)
	}
}

func collectResponsesUserItem(item gjson.Result, parts *[]string, images *[]string) {
	if !isResponsesUserItem(item) {
		return
	}
	collectContentValue(item.Get("content"), parts, images)
	if item.Get("type").String() == "input_text" || item.Get("text").Exists() {
		collectContentValue(item, parts, images)
	}
}

func collectAllGeminiUserContent(contents gjson.Result, parts *[]string, images *[]string) {
	if !contents.IsArray() {
		return
	}
	for _, item := range contents.Array() {
		if !isGeminiUserContent(item) {
			continue
		}
		if arr := item.Get("parts"); arr.IsArray() {
			arr.ForEach(func(_, part gjson.Result) bool {
				addModerationText(parts, part.Get("text").String())
				addGeminiModerationImage(images, part)
				return true
			})
		}
	}
}

// promptRiskWrapperRegexes 剥离编码客户端注入的上下文包裹,只留真正的用户意图。
var promptRiskWrapperRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<environment_context>.*?</environment_context>`),
	regexp.MustCompile(`(?is)<user_instructions>.*?</user_instructions>`),
	regexp.MustCompile(`(?is)<system-reminder>.*?</system-reminder>`),
}

func stripPromptRiskWrappers(text string) string {
	for _, re := range promptRiskWrapperRegexes {
		text = re.ReplaceAllString(text, " ")
	}
	return text
}
