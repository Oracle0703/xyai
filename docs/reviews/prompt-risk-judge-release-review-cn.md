# Prompt Risk Judge 上线前审查报告

日期: 2026-06-26

## 审查范围

| 项目 | 内容 |
| --- | --- |
| 工作分支 | `feature/hy/0621_敏感词过滤` |
| 对比基线 | `github/feature/hy/0621_敏感词过滤` |
| 当前 HEAD | `57b460c68 feat: 添加用户请求内容显示及提示信息到Token分析视图` |
| 新增提交 | `babab70ec feat: implement LLM semantic review for prompt risk assessment`、`57b460c68 feat: 添加用户请求内容显示及提示信息到Token分析视图` |
| 主要变更 | Prompt Risk 增加 LLM Semantic Review / judge；管理端 PromptRiskPanel 增加 judge 配置；TokenAnalysisView 抽屉增加 `<userRequest>` 摘要展示 |

## 总体结论

| 结论 | 判断 |
| --- | --- |
| 是否可部署 | 可以部署当前 commit |
| 是否建议立即启用 judge | 不建议 |
| 建议上线方式 | 允许代码随版本上线，但生产配置保持 `judge.enabled=false`；待递归/回环防护补强后再灰度开启 judge |
| 最大风险 | judge 指向本网关时，当前 `context` 防递归不能跨 HTTP 入站请求生效；如果管理员未配置专属 API Key 豁免，可能出现自触发递归、超时、额外请求压力或 fail-open 放行 |

## 发现的问题

| 级别 | 问题 | 证据 | 影响 | 建议 |
| --- | --- | --- | --- | --- |
| P1 | judge 防递归机制只在同一 Go 调用链内有效，不能覆盖真实 HTTP 回环 | `backend/internal/service/content_moderation.go:1481` 只检查 `ctxHasPromptRiskJudgeInFlight(ctx)`；`backend/internal/service/prompt_risk_judge.go:164` 只把标记写进 judge 请求自己的 `context`；新入站 HTTP 请求不会继承这个 context | judge `base_url` 若配置为当前网关，judge 请求体含原始高危 prompt，可能再次触发 Prompt Risk | 后端增加可跨 HTTP 的硬防护，例如专用 header + server-side 校验、按 API Key 强制豁免、或保存配置时禁止 judge API Key 落在 Prompt Risk 作用域内 |
| P2 | 管理端 Prompt Risk 在线测试器不经过 judge | `backend/internal/service/content_moderation.go:1694` 的 `TestPromptRisk` 调用 `GetPromptRiskConfig` 后只执行 `EvaluatePromptRisk` | UI 测试结论与真实请求启用 judge 后的结论不一致，管理员可能误判灰度效果 | 测试器明确标注“仅关键词规则”，或新增真实 judge dry-run 测试接口 |
| P2 | 本次高风险逻辑变更没有同步 `llm-wiki` | `AGENTS.md` 要求新增/修改认证、权限、限流、网关边界等高风险逻辑时更新 `llm-wiki/wiki/` | 后续 AI/人工维护读取 wiki 时看不到 judge 的配置边界、fail-open 语义和递归风险 | 补充 `llm-wiki/wiki/security-and-reliability.md` 或 `backend.md`：Prompt Risk judge 默认关闭、失败放行、必须配置专属 API Key 豁免、测试器口径 |

## 风险细节

### P1: judge HTTP 回环不会继承 context 标记

当前实现里，递归标记通过 `context.WithValue` 注入:

```go
reqCtx, cancel := context.WithTimeout(withPromptRiskJudgeInFlight(ctx), timeout)
req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
```

这只能影响当前进程内 `client.Do(req)` 这条 outbound request 的取消和本地上下文值。若 `endpoint` 指向本网关，服务端收到的是一条新的 HTTP 入站请求，Gin/request context 是新建的，不会带 `promptRiskJudgeContextKey`。

因此这条检查:

```go
if prCfg.Judge.Enabled && prCfg.Judge.triggersLevel(d.Level) && !ctxHasPromptRiskJudgeInFlight(ctx) {
    judge = s.runPromptRiskJudge(ctx, prCfg, content.Text, d.Reasons)
}
```

在真实 HTTP 回环里仍可能为 true。

前端已有黄色提示要求使用专属 API Key 并加豁免，这是必要操作，但目前只是人工提示，不是后端强约束。上线后如果管理员漏配，会把高风险行为暴露到运行期。

## 已验证项

| 命令 | 结果 | 备注 |
| --- | --- | --- |
| `git diff --check github/feature/hy/0621_敏感词过滤...HEAD` | 通过 | 无 whitespace/error marker 问题 |
| `go test -tags=unit -p 1 -count=1 ./internal/service -run 'TestPromptRisk\|TestPromptRiskJudge\|TestRunPromptRiskJudge\|TestContentModeration.*PromptRisk\|TestRecordPreBlockSyncMetric_PromptRiskBlockCountsBlocked'` | 通过 | 覆盖 judge payload、解析、失败放行、融合逻辑、API Key 掩码/沿用 |
| `go test -tags=unit -p 1 -count=1 ./internal/handler/admin -run 'ContentModeration\|PromptRisk'` | 通过 | 包内无匹配测试 |
| `frontend` 目录下 `vitest run src/views/admin/__tests__/TokenAnalysisView.spec.ts` | 通过 | 7 个测试通过 |
| `frontend` 目录下 `vue-tsc --noEmit` | 通过 | 类型检查通过 |
| `pnpm.cmd --dir frontend run build` | 通过 | 仅有既有 Vite chunk/dynamic import 与 Browserslist 过期警告 |

## 未完整验证

| 项目 | 状态 | 说明 |
| --- | --- | --- |
| `go test -tags=unit -p 1 -count=1 ./internal/service` 全包 | 未通过完整验证 | 本地运行 180 秒超时，不能作为通过证据；已用定向测试覆盖本次改动核心路径 |
| 真实网关回环场景 | 未验证通过 | 当前单测 `TestPromptRiskJudge_ContextInFlightSkips` 是直接传入带标记的 context，不等价于 HTTP 打回本网关 |

## 建议 Claude 重点复核的问题

| 序号 | 问题 |
| --- | --- |
| 1 | 是否同意 P1 判断：`context` 防递归无法跨 HTTP 入站请求，当前实现不能防真实本网关回环？ |
| 2 | `judge` 调用失败统一 fail-open 是否符合当前业务目标？如果 judge 端点配置错，所有本应拦截的高危请求都会降级为观察放行。 |
| 3 | 是否需要把 `judge.enabled=true` 的配置保存改为更强约束：必须填写专属 `api_key_id` 豁免，或后端自动校验该 key 不在 Prompt Risk 阻断作用域？ |
| 4 | 管理端测试器是否必须展示 judge 参与后的真实结论，避免管理员按关键词测试结果误灰度？ |
| 5 | 这次是否应补 `llm-wiki/wiki/security-and-reliability.md`，把 judge 默认关闭、失败放行、递归边界和灰度要求固化为项目知识？ |

## 上线建议

| 场景 | 建议 |
| --- | --- |
| 仅合入代码，生产不启用 judge | 可以上线 |
| 上线后启用 Prompt Risk 关键词拦截，但 judge 仍关闭 | 可以灰度，但继续按既有 Prompt Risk 配置谨慎放量 |
| 上线后启用 judge，并让 judge 调用当前网关 | 暂缓；先修 P1 |
| 上线后启用 judge，并调用外部独立 OpenAI-compatible endpoint | 风险较低，但仍建议先补测试器口径和 wiki，并在小流量灰度中观察 judge 错误率 |
