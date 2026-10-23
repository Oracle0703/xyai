# Token Analysis 页面组件

- `TokenAnalysisView.vue` 保留筛选、分页、请求加载和索引轮询；本目录组件负责概览、排行、趋势、请求表、详情抽屉与索引展示。
- `TokenAnalysisRequestTable.vue` 的请求来源列与 `TokenAnalysisRequestDrawer.vue` 的解释共用 `requestOrigin.ts`，只读取列表响应的 `summary_json.request_origin`，不新增 API 请求。
- 版本 1 合同为 `{ kind, reason, confidence, version: 1 }`。`human/user_input/medium` 显示“人工”；明确子 Agent、自动化、工具结果、Agent 消息、助手续执行和压缩标记为 `ai/high`，显示“AI”。`unknown/none`（含 `count_tokens` 探测行）、旧记录、未知版本及不合法字段显示 `—`，不是第三个业务分类。新增 reason 需同步 `requestOrigin.ts` 映射、API 类型与双语 `requestOrigin.reasons`。
- “人工”只表示当前轮次存在新的用户输入结构，不能证明实际由本人手动发送。来源不从 `last_user_preview`、净输入全文、`duplicate_count`、工具数量或用户账号推测。抽屉展示规则依据、置信度及版本。
- 用户趋势继续使用计费日志，归档排行与计费趋势统计口径独立；子管理员只读，不能触发索引。
- 验证入口：`pnpm --dir frontend exec vitest run src/views/admin/__tests__/TokenAnalysisView.spec.ts src/api/__tests__/admin.tokenAnalysis.spec.ts src/i18n/__tests__/localeKeyCompleteness.spec.ts src/i18n/__tests__/localesMessageCompile.spec.ts`；同步检查 typecheck 和本目录 ESLint。
