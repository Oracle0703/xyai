# sub2api 上游 v0.1.146 合并复核报告

> 复核对象：codex 将上游 `upstream/main`（Wei-Shaw/sub2api）合并进本地功能分支后的**未提交**冲突解决结果。
> 复核目的：核对是否存在「冲突未解决干净」或「把我方新增内容合并/丢弃掉」的情况。
> 复核方式：只读审计，未改动任何文件。

---

## 0. 合并元信息

| 项 | 值 |
|---|---|
| 合并方向 | `upstream/main` → `feature/hy/0621_敏感词过滤` |
| 我方 (ours) HEAD | `62e6a2e99` — feat: 更新用户使用列表… |
| 上游 (theirs) MERGE_HEAD | `6f43986c3` — Merge PR #3811 (upstream/main) |
| 合并基 (merge-base) | `a5638a4e5` |
| 版本 | 本地 `0.1.143` → 上游 `0.1.146` |
| 当前状态 | **合并进行中、已 staged、尚未 `git commit`**（`.git/MERGE_HEAD` 存在） |
| 冲突文件 | 11 个（见 `.git/MERGE_MSG`） |

上游此次带来一次大规模**「纯移动拆分」重构**（提交 `d9e514f98` / `bb5d2e84a` / `50043b117` / `4d23ad4ba` 等）：把多个巨型文件拆成小文件——
`setting_handler.go`(3957→468)、`setting_service.go`(5471→263)、`openai_gateway_service.go`(4872→1095)、以及把 `i18n` 的 `en.ts`/`zh.ts` 拆成按域划分的模块。**这正是我方新增内容最容易在合并中「被拆没」的高风险区**，也是本次复核的重点。

---

## 1. 总体结论

**codex 的冲突解决整体质量高，后端零丢失、可编译、相关测试通过。但前端 i18n（多语言文案）存在真实的内容丢失，且存在一处会导致「提交即丢文件」的暂存区完整性问题。以下问题必须在 `git commit` 结束合并之前修复。**

| 级别 | 问题 | 影响 |
|---|---|---|
| 🔴 P0 | 6 个新增 i18n 模块文件 + 1 个测试文件**未加入暂存区（untracked）** | 直接 `git commit` 会把它们漏掉 → 提交后前端编译失败、tokenAnalysis/requestIntercept/promptMetrics 三个功能文案全丢 |
| 🔴 P1 | 我方 `admin.riskControl.promptRisk` 整个子树（敏感词/Prompt 风险面板文案，约 74 键 + `judge` 子树 + 4 个兄弟键）在拆分迁移中**被丢弃** | `PromptRiskPanel.vue`（73 处引用）全部落空，敏感词管理面板显示原始 key，等于该核心功能界面「未翻译/破损」 |
| 🟠 P2 | `resetDailyQuota*`（4 键）丢失 | `SubscriptionsView.vue`「重置日限」相关按钮文案破损 |
| 🟠 P3 | `compatibleProvider*`（2 键）丢失 | `CreateAccountModal.vue`「OpenAI 兼容服务商」文案破损 |

**后端（Go）判定：干净。** 11 个冲突文件中的 6 个后端文件、以及全部我方后端新增文件，内容均已正确保留/迁移；`go build ./...` 通过、全量测试编译通过、冲突相关测试通过。**问题全部集中在前端 i18n。**

---

## 2. ✅ 已正确解决的部分（附证据）

### 2.1 无残留冲突标记
全仓（含 11 个冲突文件）未发现 `<<<<<<<` / `=======` / `>>>>>>>` 冲突标记；`git diff --diff-filter=U`（未合并路径）为空。

### 2.2 后端拆分文件——我方新增内容已完整迁移
逐一提取我方在冲突文件里新增的符号（函数/类型/常量），再在合并后全树检索其存续：

| 冲突文件（我方新增） | 结果 | 迁移去向（上游拆分后的新文件） |
|---|---|---|
| `openai_gateway_service.go`（+173） | ✅ 全部存续 | `sanitizeOpenAIResponsesOfficialRequestBody` 等 → `openai_gateway_request_body.go`；`applyOpenAICompatibleCacheUsageFromJSON` → 多个 CC 管线文件；`zeroCostBreakdown`/`usageCostTotal` 被**正确改名/内联**为 `openAIUsageZeroCostBreakdown` + `cost.TotalCost`（`openai_gateway_usage.go`），且保留了我方「图片计费模式」特殊逻辑 |
| `setting_handler.go`（+93） | ✅ 全部存续 | 请求归档设置处理器 → `setting_handler_runtime.go`；路由挂载保留于 `routes/admin.go:512-513` |
| `setting_service.go`（+214） | ✅ 全部存续 | 请求归档设置服务 + 缓存类型 → 新文件 `setting_request_archive.go`（无重复定义） |
| `endpoint.go`（+7） | ✅ 正确融合 | 我方 `/chat/completions`、裸 `/responses`、`/backend-api/codex/responses` 别名被上游更完善的 `EndpointResponsesCompact` 拆分方案**吸收覆盖**，路由意图完全保留 |
| `openai_gateway_responses_chat_fallback.go`（+44） | ✅ 全部存续 | `shouldDropOpenAICompatibleResponsesTemperature`、`openai_compat_drop_temperature` 等保留原位 |
| `concurrency_service_test.go`（+17） | ✅ 存续并通过 | `TestAcquireUserSlot_CacheError` / `TestAcquireAccountSlot_CacheError`（`-tags unit` 下 PASS） |

### 2.3 我方新增文件——零丢失
`base..HEAD` 新增的全部文件（后端 `prompt_risk*.go`、`token_analysis_*.go`、`promptmetrics/*`、`user_concurrency_preset*.go`、`request_archive*.go` 等；前端各视图/store/api），在合并后工作区中**全部存在且已被 index 跟踪**（缺失计数 = 0）。

### 2.4 go.mod / go.sum——双方改动都在
- 上游侧：AWS SDK 升级（`aws-sdk-go-v2 v1.41.5`、`s3 v1.97.3`、`eventstream v1.7.8`）、`tiktoken-go/tokenizer` 提为直接依赖 ✅
- 我方侧：`golang.org/x/sys`、`golang.org/x/text` 提为直接依赖、`smithy-go` 降为间接 ✅
- `go.sum` 含全部上游升级版本 ✅

### 2.5 依赖注入（wire）——完整
`wire_gen.go` 冲突已正确解决：我方全部服务经其 `Provide*` 包装器完成实例化——
`ProvideTokenAnalysisService`(L244)、`ProvideUserConcurrencyPresetService`(L193)、`ProvideUserConcurrencyPresetRunner`(L302)，并把 `UserConcurrencyPresetRunner` 注册为后台任务(L524)；`RequestIntercept*`、`promptmetrics.ProviderSet` 等均在。

### 2.6 构建与测试证据
- `go build ./...` → **exit 0**（DI、go.mod、迁移后符号全部一致）
- `go test -tags unit ./... -run '^$'`（编译全部测试）→ 仅 `logger`、`proxyutil` 两包报错，均为 **Windows 下测试 .exe 被占用/拒绝访问的环境问题**（`Access is denied` / `being used by another process`），**非编译或合并错误**
- 冲突区相关测试全部 PASS：`internal/handler`（endpoint/responses）、responses-compat、sanitize、cache-usage、default-reasoning-effort、concurrency
- 前端 `vue-tsc --noEmit` → **exit 0**
- 前端 i18n 守卫测试（5 文件 15 用例）→ 全部 PASS

> ⚠️ 注意：前端类型检查与 i18n 守卫测试**通过**并不能证明文案完整——vue-i18n 的 key 是运行期字符串查找，缺 key 只会回显原始 key，不会导致 `vue-tsc` 或现有守卫测试失败。第 3 节的丢失正是从这个盲区里漏过去的。

---

## 3. 🔴 发现的问题（提交前必须处理）

### P0 — 6 个新 i18n 模块文件 + 1 个测试文件未加入暂存区
`git status` 显示以下文件为 **`??`（untracked）**：

```
?? frontend/src/i18n/locales/en/admin/tokenAnalysis.ts
?? frontend/src/i18n/locales/en/admin/requestIntercept.ts
?? frontend/src/i18n/locales/en/admin/promptMetrics.ts
?? frontend/src/i18n/locales/zh/admin/tokenAnalysis.ts
?? frontend/src/i18n/locales/zh/admin/requestIntercept.ts
?? frontend/src/i18n/locales/zh/admin/promptMetrics.ts
?? frontend/src/i18n/__tests__/adminMergeKeys.spec.ts
```

而已暂存的 `en/admin/index.ts`、`zh/admin/index.ts`（状态 `AM`）**已经 import 了这些模块**。当前工作区能编译/测试通过，仅仅因为文件在磁盘上；**一旦执行 `git commit` 结束合并，untracked 文件不会进入提交**，结果是：提交后的树里 `admin/index.ts` 引用了不存在的模块 → 前端构建失败，且 tokenAnalysis / requestIntercept / promptMetrics 三个功能的全部文案丢失。

**修复：提交前 `git add` 上述 7 个文件（或 `git add frontend/src/i18n`）。**

### P1 — `admin.riskControl.promptRisk` 整个子树丢失（最大的一处内容丢失）
我方在 `en.ts` / `zh.ts` 中位于 `admin.riskControl` 下的 **`promptRisk` 子树被拆分迁移时整体遗漏**，合并后全 i18n 树中 `promptRisk` 出现次数 = 0，而组件仍在引用：

- 来源（我方 HEAD）：`en.ts` 第 **3097–3172** 行、`zh.ts` 第 **3175–3250** 行（各约 76 行）
- 组件引用：`frontend/src/views/admin/PromptRiskPanel.vue` 共 **73 处** `t('admin.riskControl.promptRisk.*')`，该面板经 `RiskControlView.vue:1044` 渲染（用户可见）
- 现有 `admin.riskControl` 块本身已迁移到 `en/admin/channels.ts` + `en/admin/settings.ts`（zh 同），**唯独其中的 `promptRisk` 子块没带过来**

丢失的键包括（en，zh 对称）：
- 配置子树 `admin.riskControl.promptRisk.*`（约 57 个直接叶子键）：`intro, enabled, enabledHint, mode, modeHint, modeOff, modeObserve, modeBlock, inputScope, inputScopeHint, scopeNewest, scopeFull, allGroups, allGroupsHint, groupIds, idsPlaceholder, threshold, thresholdHint, blockStatus, blockMessage, rewriteSuggestion, keywordSets, addSet, level, matchMode, score, keywords, keywordsPlaceholder, matchContains, matchWord, matchRegex, levelLow, levelMedium, levelHigh, exemptions, exemptionsHint, addExemption, userIds, apiKeyIds, maxLevel, save, saved, saveFailed, loadFailed, tester, testerRuleOnlyHint, testerPlaceholder, runTest, testFailed, actionBlock, actionObserve, actionAllow, testerBlocked, testerPassed, testerObserveNote, testerWouldReturn`
- 嵌套 `judge` 子树（LLM 语义复核，约 16 键）：`title, hint, enabled, baseUrl, model, apiKey, apiKeyHint, timeoutMs, triggerLevels, triggerLevelsHint, promptTemplate, promptTemplatePlaceholder, recursionTitle, recursionHint, addExemption, exemptionAdded`
- 4 个 `riskControl` 下的兄弟标签键：`tabs.promptRisk`(=`Prompt Risk`/`Prompt 风险`)、`result.observe`(=`Observe`/`观察`)、`action.promptRiskBlock`、`action.promptRiskObserve`

**修复：从 `HEAD:en.ts` 3097–3172 / `HEAD:zh.ts` 3175–3250 取回 `promptRisk` 子树，连同 4 个兄弟键，补回合并后的 `en/admin/channels.ts`+`settings.ts`（zh 同）中对应的 `riskControl` 结构里。**

### P2 — `resetDailyQuota*`（重置每日配额，4 键）丢失
- 键：`resetDailyQuota`、`resetDailyQuotaTitle`、`resetDailyQuotaConfirm`、`resetDailyQuotaDesc`
- 合并后 i18n 树中出现次数 = 0；组件 `frontend/src/views/admin/SubscriptionsView.vue` 仍引用
- 属于「部分对象丢失」：上游同名兄弟键 `resetQuota*` 保留了下来（在 `en/admin/accounts.ts`），但合并取了上游版对象、丢了我方追加的这 4 个兄弟键
- **修复：补回 `en/admin/accounts.ts` + `zh/admin/accounts.ts` 中 `resetQuota*` 所在对象。**

### P3 — `compatibleProvider*`（OpenAI 兼容服务商，2 键）丢失
- 键：`compatibleProvider`、`compatibleProviderHint`
- 合并后 i18n 树中出现次数 = 0；组件 `frontend/src/components/account/CreateAccountModal.vue` 仍引用
- **修复：补回 `en/admin/accounts.ts` + `zh/admin/accounts.ts` 中 `accounts.openai` 对象（`baseUrlHint`/`apiKeyHint` 所在处）。**

> 说明：三处丢失在 EN / ZH 两个语种上是**对称**的（两边丢的是同一批键）。另有一处曾疑似丢失的 `requestArchive.maxRequestBodyHint` 经核实**并未丢失**，仅是 EN 值里的破折号 `—` 被规范成 `-`，属文案微调，非丢键（假阳性，已排除）。

---

## 4. 修复建议清单（按优先级）

1. **[P0]** `git add` 6 个未跟踪的 i18n 模块文件（en/zh × tokenAnalysis/requestIntercept/promptMetrics）与 `adminMergeKeys.spec.ts`——**这一步不做，后面全白做**。
2. **[P1]** 恢复 `admin.riskControl.promptRisk` 子树（含 `judge`）+ 4 个兄弟标签键。
3. **[P2]** 恢复 `resetDailyQuota*` 4 键至 `accounts.ts`。
4. **[P3]** 恢复 `compatibleProvider*` 2 键至 `accounts.ts` 的 `openai` 对象。
5. **[验证]** 修复后：
   - `git status` 确认无 `??` 遗留于 `frontend/src/i18n`；
   - 重新 `vue-tsc --noEmit`；
   - 建议补一条守卫用例（现有 i18n 守卫测试**没有**覆盖到这三处，无法拦截此类丢失）：断言 `PromptRiskPanel.vue` / `SubscriptionsView.vue` / `CreateAccountModal.vue` 引用的关键 key 在 en/zh 中均存在；
   - 全部修复到位后再 `git commit` 结束合并。

---

## 5. 附录：复现/取证命令

```bash
BASE=$(git merge-base HEAD 6f43986c3)          # a5638a4e5

# P0：未跟踪的 i18n 模块
git status --porcelain -- frontend/src/i18n/ | grep '^??'

# P1：promptRisk 子树在合并后全树是否为 0
git grep -n 'promptRisk' -- frontend/src/i18n/           # 期望：空（=已丢）
git grep -c 'promptRisk\.' -- frontend/src/views/admin/PromptRiskPanel.vue   # 73（仍在引用）
# 取回来源：
git show HEAD:frontend/src/i18n/locales/en.ts | sed -n '3097,3172p'
git show HEAD:frontend/src/i18n/locales/zh.ts | sed -n '3175,3250p'

# P2 / P3
git grep -n 'resetDailyQuota'   -- frontend/src/i18n/    # 空
git grep -n 'compatibleProvider' -- frontend/src/i18n/   # 空

# 后端「干净」佐证
cd backend && go build ./...                              # exit 0
go test -tags unit ./internal/service/ -run 'TestAcquireUserSlot_CacheError|ResponsesCompat|SanitizeOpenAI'
```

---

*复核完成时间：2026-07-08。复核仅覆盖冲突解决的正确性与我方内容存续；未对上游新功能本身做代码审查。*
