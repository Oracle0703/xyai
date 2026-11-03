# 未合并分支与未提交改动清单

日期：2026-11-03。核对基线：本地 `main` = `github/main` = `7d0f760fc4a3fa8e52d4e99fe36787423f7e1cdf`（`feat(image-gen): add catalog-based model selection and optional key memory`，2026-11-03 14:19 +0800）。

这份清单只记录当时还没进入 `main` 的分支，以及两个工作区里尚未提交的文件。目的是让 Codex 逐项判断要不要把内容合进当前 `main`。本文不是合并授权，也不是删除授权。

## 给 Codex 的判定方法

先读完本节再看各条目。

1. 用 `git log main..<ref>` 和 `git show <唯一提交>` 看分支自己多出来的补丁。不要用 `git diff main <ref>`。两两对比会把该分支分叉之后 `main` 上的全部后续历史都算成差异，不能说明“还缺什么”。
2. 用 `git merge-tree --write-tree --name-only main <ref>` 看现在合进去会碰到哪些文件。退出码 0 且结果树等于 `main^{tree}`，表示合进去不改变当前树。退出码 1 表示有冲突，冲突文件列在各条目里。
3. 同名符号在当前 `main` 搜不到，只说明这套名字没有落地。后来的实现可能换了名字。要对照当前源码判断行为是否已经存在，不能只凭符号缺失就合并。
4. 工作区未提交 diff 是相对该工作区 `HEAD`，不是相对当前 `main`。两个工作区的 `HEAD` 都已经是 `main` 的祖先。把工作区文件覆盖到 `main` 会退回旧树。
5. 不要合并 `upstream`（`Wei-Shaw/sub2api`）的分支。不要把内网 `origin/main` 合并进 GitHub `main`。

复现命令（在 `E:\allsite\xyai`）：

```powershell
git rev-parse main
git log --oneline main..<ref>
git show --stat <sha>
git merge-tree --write-tree --name-only main <ref>
git grep -n <符号> main -- <路径>
```

## 总表

| ID | 位置 | 尖端 | 未包含在 main 的提交 | 2026-11-03 合并预演 | 已核实、还不能当裁决的事实 |
| --- | --- | --- | --- | --- | --- |
| U1 | 本地 + `github/feature/hy/10177_fork_governance_validation` | `fc1a0b22e` | 1 | 冲突 | 设计文档不在 main；文内写明当时未授权改业务代码，但提交里有生命周期代码 |
| U2 | 本地 + `github/feature/hy/10178_merge_sub2api_178` | `e558c091f` | 1 个 merge commit | 冲突 | 第二父 `49504adc` 已是 main 祖先；这是 0.1.178 的中间合并提交 |
| U3 | 本地 + `github/feature/hy/10207_department_usage` | `ce6ce97c9` | 1 | 冲突 | 父提交已在 main；审核后的子管理员订阅投影符号不在当前 main |
| U4 | 本地 + `github/feature/hy/10212_sub4api_request_locale` | `82dd34428` | 1 | 冲突 | OpenAI 请求时区改写源码不在当前 main |
| U5 | 本地 + `github` + `origin` 的 `hotfix/hy/0000_limit_codex_context_272k` | `493252341` | 1 | 冲突 | main 上 GPT-5.6 catalog 上限仍是 `872_000` |
| U6 | 仅 `github/chore/remove-merge-artifacts-20260930` | `b02bb7046` | 1 | 干净，结果树等于 main | 忽略规则和被删文件的结果已经在 main |
| U7 | 仅 `github/codex/server-runtime-fixes-review` | `ad8d5fac1` | 1 | 冲突 | usage 路径修复在当前 messages 文件里已有同名调用；价格数字和当前 main 不同 |
| W1 | 工作区 `C:/Users/Admin/.codex/worktrees/request-diagnostics/xyai` | 分支 `534b1b404` 已在 main | 无新提交 | 不适用 | 未跟踪的排障工具和两份文档不在 main |
| W2 | 工作区 `E:/allsite/xyai-plugin-wiring` | 分支 `d120d499e` 已在 main | 无新提交 | 不适用 | 未提交 diff 是在 2026-09-20 旧树上补 `ProvidePluginManager`；当前 main 已有该函数，文件哈希不同 |

没有打开的 GitHub PR。

## U1 `feature/hy/10177_fork_governance_validation`

- 跟踪：`github/feature/hy/10177_fork_governance_validation`，与本地尖端相同。
- 唯一提交：`fc1a0b22e26ded90c6a50f8e875029c1d3741dd6`，2026-08-13 10:43 +0800。
- 说明：`docs: add fork governance validation and debt audit review`。正文写的是记录 Codex CX-TD 审核、Grok 复核、fork 治理设计和效果标准，以及用来验证降低合并成本的本地扩展生命周期合同。
- 父提交 `8784a4084268b532ab653774c0dc3999e24ff7c9` 是与 main 的 merge-base，已在 main。
- 补丁规模：26 文件，`+2579/-147`。
- `git cherry -v main <ref>` 标记为 `+`，patch-id 不在 main。

提交同时包含文档和 Go 改动。设计文档开头写的是“设计候选，待用户评审批准后进入实施计划；尚未授权修改业务代码”，日期 2026-08-12，对应债务 `TD-009`。当前 main 没有这些文件：

- `docs/features/codex-full-repo-technical-debt-audit-cn.md`
- `docs/features/upstream-fork-governance-design-cn.md`
- `docs/features/upstream-fork-governance-effectiveness-test-standard-cn.md`
- `docs/features/upstream-fork-governance-validation-report-cn.md`
- `docs/superpowers/plans/2026-08-12-fork-governance-validation.md`

`docs/features/technical-debt-board-cn.md` 在 main 上存在，合并会冲突。

Go 侧新增或改动的符号在当前 `backend/` 中搜不到，包括 `TokenAnalysisService.beginBackgroundTask`、`TimingWheelService.cancelRecurringLocked`、`TestProvideCleanup_LocalTasksFinishBeforeInfrastructureAndCleanupIsIdempotent`。补丁还改了 `wire.go`、`wire_gen.go`、`promptmetrics`、`user_platform_quota_flusher` 的启动/停止测试。这些名字缺失不证明当前停机顺序没有等价实现。

合并预演冲突：

- `backend/cmd/server/wire_gen_test.go`
- `backend/internal/service/token_analysis_service.go`
- `docs/features/technical-debt-board-cn.md`
- `llm-wiki/.understand-anything/knowledge-graph.json`
- `llm-wiki/.understand-anything/meta.json`
- `llm-wiki/wiki/README.md`
- `llm-wiki/wiki/backend.md`

需要 Codex 分开裁定：

1. 四份治理/债务文档是否仍要进入当前文档树，还是只作 2026-08 历史快照。
2. 生命周期与 cleanup 代码是否仍是缺口。若要合，只能摘当前仍成立的行为，不能把 2026-08-13 的 Wire 测试整文件合进来。
3. 图谱 JSON 和当时的 wiki 段落不要当作待合并内容。

## U2 `feature/hy/10178_merge_sub2api_178`

- 跟踪：`github/feature/hy/10178_merge_sub2api_178`。
- 唯一未包含提交：`e558c091fe7ff5432227655e953efb06b75e6754`，2026-08-19 15:17 +0800。
- 说明：`Merge commit '49504adc98d2b6d539491e865a340e644548979e' into feature/hy/10178_merge_sub2api_178`。无额外正文。
- 父提交：`0747c896abf8202dff9935a30f020317ebdc91fc` 和 `49504adc98d2b6d539491e865a340e644548979e`。
- `49504adc` 的说明是 `chore: sync VERSION to 0.1.178`，2026-08-18。`git merge-base --is-ancestor 49504adc main` 退出码 0，该上游尖端已经在 main 历史里。
- 相对第一父的 diff 是 311 文件、`+20022/-1028`，这是当时整轮 0.1.178 合并结果，不是后来又写的一小段功能。
- 三方 diff 对 main 有多个 merge-base，警告使用 `0747c896`。不要用这个三方 diff 的文件清单判断“main 缺 0.1.178”。

合并预演冲突：

- `backend/cmd/server/wire_gen.go`
- `docs/upstream-sync/merge-log.md`
- `frontend/src/components/account/CreateAccountModal.vue`
- `frontend/src/components/account/__tests__/CreateAccountModal.grok.spec.ts`
- `llm-wiki` 图谱与 `README.md`、`backend.md`、`data-and-domain.md`、`frontend.md`、`ops.md`、`security-and-reliability.md`

需要 Codex 裁定：这个 merge commit 里是否还有后来各轮上游同步没有带入的冲突解决。预期应对照冲突文件的当时解决结果和当前 main，而不是再合并整分支。`merge-log.md` 是只追加台账，不能用旧版本覆盖。

## U3 `feature/hy/10207_department_usage`

- 跟踪：`github/feature/hy/10207_department_usage`。
- 唯一提交：`ce6ce97c96394f399ab9ed6deae135b517d0f5f6`，2026-09-29 11:25 +0800。
- 说明只有 `feat:第一次完成审核`，无正文。
- 父提交 `273b76c3558e2f3a63d35d851e0e9783f7a66adb` 已在 main。部门功能主体此前已经合入；本提交是其后的审核修复。
- 补丁：35 文件，`+1021/-70`。

当前 main 没有这两份文档：

- `docs/features/feature-10207-release-audit-adjudication-cn.md`
- `docs/features/organization-department-usage-design-implementation-audit-cn.md`

裁决文档开头写的基线是 2026-09-29 的 `273b76c35`，结论是当时维持 NO-GO、不部署生产。阻断理由包括当时 CI 为红、集成测试未在 CI 执行。该结论只描述那天的门禁，不能直接当成今天的部署结论。

代码侧新增了子管理员订阅响应投影，例如 `isSubAdminRequest`、`subscriptionResponse`、`dto.SubAdminUserSubscriptionFromService`、`service.ErrSubscriptionSelfAssignDenied`。2026-11-03 在当前 `backend/` 搜索这些符号没有命中。同一次提交还改了部门授权弹窗、组织用量、订阅分配测试和 GPT 额度测试。

合并预演冲突：

- `backend/internal/handler/admin/subscription_handler.go`
- `backend/internal/service/gpt_quota_display_test.go`
- `frontend/src/components/admin/department/DepartmentAccessDialog.vue`
- `frontend/src/components/admin/department/__tests__/DepartmentManagement.spec.ts`
- `llm-wiki/wiki/README.md`
- `llm-wiki/wiki/backend.md`
- `llm-wiki/wiki/security-and-reliability.md`

需要 Codex 裁定：

1. 子管理员看不到完整订阅 DTO、不能给自己分配订阅，这两条在当前子管理员权限里是否已经用别的实现覆盖。
2. 若没有覆盖，是否只移植仍成立的处理器和测试，而不是回放 2026-09-29 的 wiki。
3. 两份审核文档若保留，应放在 `docs/reviews/` 还是保持原路径；它们描述的是当天 NO-GO，不是现行行为合同。

## U4 `feature/hy/10212_sub4api_request_locale`

- 跟踪：`github/feature/hy/10212_sub4api_request_locale`。
- 唯一提交：`82dd344283da344ae478fd6e39a6bf2b259ba9b6`，2026-09-30 12:50 +0800。
- 说明只有 `feat:提测`，无正文。
- 父提交 `bdf31e87223058200c614a807e24169e12bb31ea` 已在 main。
- 补丁：42 文件，`+2668/-109`。

当前 main 没有这些路径，仓库里也搜不到 `enable_openai_request_timezone_rewrite` 或 `applyOpenAIRequestLocale`：

- `backend/internal/service/openai_request_locale.go`
- `backend/internal/service/openai_request_locale_test.go`
- `backend/internal/service/openai_request_timezone.go`
- `backend/internal/service/openai_request_timezones.txt`
- `frontend/src/components/account/OpenAIRequestTimezoneField.vue`
- `frontend/src/components/account/__tests__/OpenAIRequestTimezoneField.spec.ts`

分支上的 wiki 合同（相对父提交的新增段落，不是当前 wiki）摘要：

- 全局 `enable_openai_request_timezone_rewrite` 默认 `false`，账号 `extra.openai_request_timezone_rewrite_enabled` 仅 JSON `true` 算开启，两者同时开启且平台为 OpenAI 才改写。
- 只改写用户输入里结构化 `environment_context` 的 `timezone`，以及 Web Search `user_location.timezone`。不改 `Accept-Language`。
- 默认目标时区 `America/Los_Angeles`，白名单校验。Spark 影子跟随母账号。
- 写入失败整体回退原 body。日志不记客户端时区值或正文。
- 分支自己的备注：2026-09-30 二轮复核仍有“三次 CAS 冲突耗尽后可能返回旧的开启结果”；默认全关可以部署，开启灰度前要先修这个降级。真实上游（Codex CLI、compact、web_search、prompt cache）当时未验收。
- 本地整栈脚本写在 `E:/tmp/e2e-tz`，不在仓库里。

合并预演冲突：

- `backend/internal/server/routes/admin.go`
- `backend/internal/service/openai_ws_v2_passthrough_adapter.go`
- `frontend/src/components/account/__tests__/CreateAccountModal.spec.ts`
- `frontend/src/components/account/__tests__/EditAccountModal.spec.ts`
- `llm-wiki/wiki/backend.md`
- `llm-wiki/wiki/frontend.md`

其余账号弹窗、设置页、handler 和 `setting_gateway_runtime.go` 在预演里显示为自动合并。自动合并不等于语义仍正确，WS 和设置缓存后来又改过很多轮。

需要 Codex 裁定：这是不是仍要上线的功能。若要，应按当前网关转发、WS v2 和设置缓存重做接入，并先处理分支自己写下的 CAS 耗尽降级。不要直接合并 2026-09-30 的 wiki 段落覆盖现行合同。

## U5 `hotfix/hy/0000_limit_codex_context_272k`

- 本地跟踪 `origin/hotfix/hy/0000_limit_codex_context_272k`。
- `github/hotfix/hy/0000_limit_codex_context_272k` 与 `origin` 上的同名分支都是 `493252341505d8f3ff6aed5e8ede8c3f325557f5`。
- 父提交 `ba4fdebb4b01c0088d387f2b649c46c38ef7b707` 已在 main。
- 补丁 3 文件，`+3/-2`。

相对父提交的代码改动：

```diff
-	configuredCodexGPT56MaxContext     = 872_000
+	configuredCodexGPT56MaxContext     = configuredCodexFallbackContext
```

测试从 `require.Equal(t, int64(872_000), gpt56.MaxContextWindow)` 改为 `272_000`。wiki 增加一句：本地生成的 GPT-5.6 Codex descriptor 将 `max_context_window` 固定为 fallback `272000`，与默认 `context_window` 一致；该字段是客户端能力合同，不是网关请求前 token 准入限制，也不改变长上下文计费开关。

当前 main 仍是：

- `backend/internal/service/openai_codex_models_service.go:332` `configuredCodexFallbackContext = 272_000`
- 同文件 `:336` `configuredCodexGPT56MaxContext = 872_000`
- `:518` `descriptor.MaxContextWindow = configuredCodexGPT56MaxContext`
- 当前 `llm-wiki/wiki/backend.md` 没有这句 272000 合同。

合并预演只冲突 `backend/internal/service/openai_codex_models_service.go`。常量块的行号已经变过，所以冲突是上下文漂移，不是另一套业务改动叠在同一行上。

需要 Codex 裁定：现在是否仍要把 GPT-5.6 catalog 的 `max_context_window` 从 `872_000` 收成 `272_000`。这是行为变更。合的时候只改这个常量和对应测试、wiki 合同，不要用旧文件覆盖。

## U6 `github/chore/remove-merge-artifacts-20260930`

- 没有本地分支。远端尖端 `b02bb7046`，说明 `chore: remove merge artifacts and local caches`。
- 父提交 `ba53ec78d557ba2aa08efa5e28f2dde869b011da` 已在 main。
- 提交本身不是 main 的祖先，`git cherry` 不会把它标成已有补丁。

2026-11-03 复核的结果状态已经在 main：

- `git merge-tree --write-tree main github/chore/remove-merge-artifacts-20260930` 退出码 0。
- 结果树 `07941ebfe5ec6c38cfe927ca6dcdaefc6b279e67` 等于 `main^{tree}`。
- `.gitignore` 已有同一段精确忽略：`/_conflict_oai.diff`、`/_local_oai.diff`、`/_merged_oai.diff`、`/_merged_test.diff`、`/_upstream_oai.diff`、`/ours_numstat.txt`、`/theirs_numstat.txt`、`/dump.rdb`、`/backend/_local_compat.go`、`/backend/_upstream_fallback.go`、`__pycache__/`。
- 这些路径在 main 上不存在。
- `llm-wiki/wiki/ai-workflow.md` 已有对应“不入库”句子。

需要 Codex 确认：没有残留文件或忽略规则漏掉。确认后这条不必再合并，可以删远端分支。本文不执行删除。

## U7 `github/codex/server-runtime-fixes-review`

- 没有本地分支。远端尖端 `ad8d5fac1`，说明 `fix: preserve usage details in OpenAI messages bridge`。
- 父提交 `cbb781c0009dc84ed31b423a65d69430fe95f6d2` 已在 main。
- 补丁 5 文件，`+22/-7`。

四块改动要分开看：

1. `openai_gateway_messages.go`：buffered terminal 读取里，按事件实际 JSON 路径调用 `applyOpenAICompatibleResponsesUsageDetailsFromJSON` 和 `applyOpenAICompatibleCacheUsageFromJSON`。当 `event.Response.Usage == event.Usage` 时路径用 `usage`，否则用 `response.usage`。当前 main 的同名函数已经在 `openai_gateway_messages.go` 的 916、928、974、986、1114、1126 行附近按 `usagePath` 调用。合并预演仍然冲突，因为旧补丁对不上现在的函数体。
2. `pricing_service_test.go`：把当时的 `codex-auto-review` 期望从 `2.5e-6 / 1.5e-5 / 2.5e-7` 改成 `5e-6 / 3e-5 / 5e-7`。当前 main 同名模型的测试期望是 `0.2e-6 / 1.2e-6 / 0.02e-6`（`pricing_service_test.go` 约 868 行）。三套数字都不同。
3. `UseKeyModal.spec.ts`：把一处 `model_provider = "OpenAI"` 改成 `"xunyou"`。当前 main 同时存在 `sub2api` 和 `OpenAI` 断言。这处不在冲突列表里，预演会自动套上，套上之前要看它会不会改错现行配置样例。
4. `TokenAnalysisView.vue`：只删了一个分号，属于格式。wiki 加了一句 usage 路径说明。

合并预演冲突：

- `backend/internal/service/openai_gateway_messages.go`
- `backend/internal/service/pricing_service_test.go`
- `frontend/src/views/admin/TokenAnalysisView.vue`
- `llm-wiki/wiki/backend.md`

需要 Codex 裁定：messages 桥的 cached token 路径在当前函数里是否已经覆盖分支想修的两个读取点。价格和 `xunyou` 样例不要随这个修复一起合，除非单独证明现行合同错了。

## W1 请求排障工作区未提交文件

- 路径：`C:/Users/Admin/.codex/worktrees/request-diagnostics/xyai`
- 分支：`feature/hy/10219_request_diagnostics`
- 尖端 `534b1b404` 已是 main 祖先。分支本身没有待合并提交。
- 未提交内容是在这个旧 `HEAD` 上改出来的，不是相对当前 main 的 diff。

已跟踪改动：

- `llm-wiki/wiki/backend.md` `+8`
- `llm-wiki/wiki/ops.md` `+10`

当前 main 没有“请求排障的 ID 与状态口径”和“本机请求排障工具”这两节。段落内容是：计费 request ID、上游 request ID、Ops 状态码不能当成同一条全链路 ID；本机工具入口、只读查询边界和验证命令。

未跟踪、且 main 上不存在：

| 路径 | 字节 |
| --- | --- |
| `tools/request-diagnostics/server.py` | 13763 |
| `tools/request-diagnostics/queries.py` | 15980 |
| `tools/request-diagnostics/diagnostics.py` | 13379 |
| `tools/request-diagnostics/app.js` | 19007 |
| `tools/request-diagnostics/index.html` | 7872 |
| `tools/request-diagnostics/style.css` | 12765 |
| `tools/request-diagnostics/test_classification.py` | 6378 |
| `tools/request-diagnostics/test_diagnostics.py` | 17336 |
| `tools/request-diagnostics/README.md` | 8539 |
| `tools/request-diagnostics/start.cmd` | 53 |
| `tools/request-diagnostics/.gitignore` | 19 |
| `docs/features/local-request-diagnostics-design-cn.md` | 7705 |
| `docs/delivery/2026-10-23-request-diagnostics/acceptance.md` | 5507 |

同目录 `__pycache__/*.pyc` 不要入库。连接配置若出现在 `.localdev/request-diagnostics.json`，该路径按工具说明应保持 gitignore，不要写进文档或提交。

需要 Codex 裁定：这个本机排障工具是否还要保留。若要，从工作区把源码、测试、设计和验收抄进当前 main 上的新提交，并按当前 wiki 结构补合同。不要合并该分支，也不要把工作区的旧 `backend.md` / `ops.md` 整文件覆盖到 main。

## W2 插件接线工作区未提交 diff

- 路径：`E:/allsite/xyai-plugin-wiring`
- 分支：`hotfix/hy/10210_preserve_plugin_manager_wiring`
- 尖端 `d120d499e` 已是 main 祖先。说明是 2026-09-20 的 `Merge commit 'fbb9006...' into feature/hy/10207_merge_sub2api_207`。没有待合并提交。

相对该旧 `HEAD` 的未提交 diff：

- `backend/cmd/server/wire_gen.go`：`NewPluginManager` 加 `SetAccountDirectory` 改成 `ProvidePluginManager(...)`。
- `backend/internal/service/wire.go`：新增 `ProvidePluginManager`，`ProviderSet` 从 `NewPluginManager` 换成它。

当前 main 已有：

- `backend/cmd/server/wire_gen.go:220` 调用 `service.ProvidePluginManager(...)`
- `backend/internal/service/wire.go:196` 函数 `ProvidePluginManager`，注释是 `preserves account-directory wiring when regenerating Wire`，参数名是 `gateway`
- `ProviderSet` 已包含 `ProvidePluginManager`

工作区文件哈希和 main 不同：

| 文件 | 工作区工作树 | 工作区 HEAD | main |
| --- | --- | --- | --- |
| `backend/cmd/server/wire_gen.go` | `508be3583` | `8b3c9a4b9` | `6ebd9e8ed` |
| `backend/internal/service/wire.go` | `ceb5a5815` | `b1e1419ff` | `549c7eef9` |

需要 Codex 确认当前 `ProvidePluginManager` 是否已经保住账号目录接线。预期这条未提交 diff 是旧工作区上的局部补丁，不能覆盖 main。确认后工作区可以留着或丢掉，不必为它开合并。

## 不要纳入本次合并判断的远端

内网 `origin` 是 `http://192.168.1.23/orig-app/orig.xy.sub2api.git`。`origin/main` 是 `89eeca221`，2026-05-29 的 `Merge branch 'codex/upstream-sync' into 'main'`。它有 1 个提交不在 GitHub `main`，GitHub `main` 有 4398 个提交不在它上面。这是落后的内网主分支，不是待挑选的功能分支。不要把它合并进 `github/main`。

`upstream` 是 `Wei-Shaw/sub2api`。它的分支即使尖端能从本地 main 走到，也不属于本仓库清理或合并范围。

## 建议的阅读顺序

1. U5：补丁小，行为问题明确，先决定 catalog 上限。
2. U4：整段功能不在 main，先决定还要不要这个时区改写。
3. U3：只审审核提交里的子管理员订阅投影，不重审已经在 main 的部门主体。
4. W1：决定本机排障工具是否入库。
5. U1：文档和生命周期代码分开决定。
6. U7：只核对 messages usage 路径是否已覆盖，价格和品牌样例单独放下。
7. U2、U6、W2：先确认没有独有残留，再决定删除分支或工作区。
