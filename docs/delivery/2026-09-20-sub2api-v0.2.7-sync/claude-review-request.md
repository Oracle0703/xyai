# Sub2API 0.2.7 合并复审任务书（Claude）

编写日期：2026-09-20。请独立审查当前暂存的合并候选，判断冲突处理、本地功能保留和范围控制是否正确。本文是审核输入，不是审核通过结论；已有报告中的结论需要用源码、索引和测试证据复核。

## 1. 用户要求与本次任务

| 项目 | 要求 |
| --- | --- |
| 原始目标 | 从最新本地 main 新建分支，合入用户指定的 Wei-Shaw/sub2api main 精确 SHA |
| 本地能力 | 保留 `docs/features/` 描述的本地独有功能；真正重叠的能力采用上游实现，需说明等价依据 |
| 变更范围 | 仅解决文本冲突和双方组合产生的语义冲突；不额外修复上游或第一父既有 bug |
| 已明确的补充选择 | 上游重写历史后撤下 Codex ticket，用户明确选择“跟随指定上游移除 Codex ticket” |
| 当前交付门槛 | 更新 wiki、追加台账、运行测试；停在 commit 前等待用户审核 |
| Claude 的任务 | 只读审查代码、暂存区、文档与测试证据；输出独立报告及最小修复建议，不自行修复 |
| 允许写入 | 新建本目录 `claude-review-result.md`；必要的测试日志写入独立的忽略目录，不覆盖原始证据 |
| Git 边界 | 不 commit/push/merge/rebase/reset/checkout/switch/restore/add/stash，不刷新或移动上游目标；保持当前索引和 MERGE_HEAD |
| 生成与自动修复 | 不在当前候选运行 go generate、无 -diff 的 go mod tidy、eslint --fix 或自动格式化；需要复现生成问题时使用独立副本 |
| 验证边界 | 可按需执行测试、构建和只读检查；不启动生产服务、不调用真实收费上游、不写真实业务数据 |

如后续用户明确授权修复，以新指令为准。本次审核结论不等于 commit、push 或上线授权。

## 2. 固定审核对象

| 项目 | 精确值 |
| --- | --- |
| 工作分支 | `feature/hy/10207_merge_sub2api_207` |
| 第一父 / HEAD / 本地 main | `de5a3e383cd8eb197c1a83f12a71fb04d9e4e049` |
| 固定上游 / MERGE_HEAD | `fbb9006adef852c46f0c7f18b0a8a740722cfac7` |
| 上游来源 | `https://github.com/Wei-Shaw/sub2api` 的 main |
| 共同祖先 | `efe9aab1e4ec89a42ba45e8dac20e882c5409a6a` |
| 上一轮已合入的纯上游版本 | `8b69738d782ccaa7fd26511e1cca26ba8d1b58db`（0.2.6） |
| 本次目标 VERSION | `0.2.7` |
| 目标相对共同祖先 | 34 commits、96 paths、+7652/-615 |
| 初始三方集合 | 30 双方修改、540 仅本地修改、66 仅上游修改路径 |
| 本文创建前的候选 | 164 个暂存路径，+8050/-3019；无 unmerged、unstaged、untracked |
| 本文创建前的候选根 tree | `7fa3eebd64b56382ad424986ec5b49ad156c64a3`（tree 对象，不是 commit） |
| backend 子树 | `4e25d22da54a7c602a679c1c3799d636b83d9003` |
| frontend 子树 | `31b1fc950acc10951f627c5108a304423d69c5ce` |
| deploy 子树 | `c3fd269d62e89f74000ba6436a5d02af5b9668b0` |

加入本文只新增审核文档，暂存路径由 164 变为 165；完整根 tree 随之变化，上述三个代码子树应保持一致。报告中“164 个文件”描述的是本文加入前的候选，不表示业务代码发生漂移。

先核对现场状态，再开始审核。若基线或代码子树有变化，记录实际值并重新确定审核范围，不自动恢复工作区。

```powershell
git status --short --branch
git branch --show-current
git rev-parse HEAD main MERGE_HEAD
git merge-base HEAD MERGE_HEAD
git diff --cached --stat
git diff --cached --check
git diff --name-only
git ls-files -u
git ls-files --others --exclude-standard

# 已保存的候选 tree 可直接用于比较，不改动索引。
$reviewSnapshot = '7fa3eebd64b56382ad424986ec5b49ad156c64a3'
git diff --cached $reviewSnapshot -- backend frontend deploy
git diff --cached --name-status $reviewSnapshot
```

预期第一条 snapshot diff 为空；第二条仅新增本文。已解决冲突的索引是 stage 0，普通 `git diff` 为空不能代替 `git diff --cached` 审核。

## 3. 阅读顺序与证据入口

| 顺序 | 路径 | 用途 |
| --- | --- | --- |
| 1 | `AGENTS.md`、`llm-wiki/wiki/README.md` | 项目规则与知识入口 |
| 2 | `llm-wiki/wiki/backend.md`、`frontend.md`、`ops.md`、`data-and-domain.md`、`security-and-reliability.md` | 本轮行为、配置与高风险边界 |
| 3 | `docs/upstream-merge-playbook.md`、`docs/features/` | 本地功能合同；与旧模板有差异时以本次用户范围和固定 SHA 为准 |
| 4 | 本目录 `review.md` | 合并说明、全部 30 个双方修改路径、测试和问题归属主张 |
| 5 | `docs/features/sub2api -merage-list.md` 的 2026-09-20 条目 | 核对仅追加、SHA/分支/冲突与验证记录一致 |
| 6 | `backend/.gocache/merge-207/` | 本机原始证据，受 gitignore 排除，不随提交分发 |

本机证据包括 `facts.json`、`preservation.json`、`test-summary.json`、`focused.log`、`default.log`、`unit.log`、`integration.log`、`build.log`、`embed.log`、`tidy.log`、`lint.log`、`frontend-*.log`、`frontend-results.json`、`baseline-auth.log`、`baseline-frontend.log`、`baseline-ollama.log`、`ollama.log` 和 `first-parent/` 源码快照。文件缺失时如实说明，不能假定已读。

`verify.ps1` 可用于了解原命令，但直接执行会覆盖同名原始日志；复跑请另存日志。部分 PowerShell 日志为 UTF-16，解析前检查 BOM。`focused-before-adapter.log` 是接口冲突修复前的历史失败，不是当前候选的测试结果。不要重放 `remove-ticket.patch`。

## 4. 首先复核三处关键处理

### 4.1 上游历史重写与 Codex ticket 移除

上游 fetch 显示 `8b69738d7...fbb9006ad main -> upstream/main (forced update)`。共同祖先 VERSION 是 0.2.5，旧 0.2.6 ticket PR #7315 及 VERSION 同步共 8 个提交不在目标历史。普通 merge 会把这些旧上游代码保留下来；本轮在用户明确选择后，用旧上游到共同祖先的精确反向差异移除 ticket。

| 核查项 | 预期 |
| --- | --- |
| 删除是否有依据 | 对照旧纯上游、共同祖先、固定目标；不能把“仅本地修改集合”中的旧上游 ticket 当作本地独有功能恢复 |
| 删除是否完整 | harvester、票据注入/调度 gate、settings/cache、DTO、账号状态、前端字段/展示及专属测试一并对齐目标 |
| 是否误删其他机制 | 普通 x-codex-turn-state echo guard、response affinity、指纹、模型选择与租户隔离继续存在 |
| SettingService 冲突 | 保留 `onRiskControlUpdate` 及注册/触发/消费链，仅删除 ticket 相关 cache/singleflight 字段 |
| 数据边界 | 没有新增 ticket 数据清理 migration、删除旧 settings/extra 或操作真实数据库 |

### 4.2 Prompt Risk 与新 collector 接口

上游 `content_moderation_input.go` 将三个自由函数迁为 `moderationTextCollector` 方法，本地 `prompt_risk_input.go` 原调用因此编译失败。本轮在本地文件增加三个薄转接，统一使用 `filterReminders=true`。

请比较第一父与候选，验证 `addModerationText`、`collectContentValue`、`collectAnthropicUserContentValue` 的输入抽取、递归、多模态和 reminder 行为等价；核对 newest/full、连续 user items、尾部 tool output 和包装内容过滤。上游关键词检测使用不过滤 reminder 的独立路径，不得被本地适配重新绕过。

同时沿 `content_moderation.go` 检查 TypeSafe/OpenAI 引擎切换和本地 Prompt Risk/LLM judge 前置阶段的组合：配置 hash、热更新、独立保存、回环保护、密钥掩码、action 统计、日志筛选和副作用排除不能丢失。两者不是等价功能，不能仅因都叫“风控”就删除本地阶段。

### 4.3 Wire 与插件账号目录

目标上游 `backend/cmd/server/wire_gen.go` 含 `pluginManager.SetAccountDirectory(openAIGatewayService)`，但 Wire 源图没有等价接线。实际重生成会删除该行；本轮重生成以去掉 ticket 参数/cleanup 后，保留了目标生成物中的该接线，未修复上游源图。

请独立确认归属，检查最终候选确实保留账号目录注入，以及 `PluginKVStore`、本地 provider、handler 和 cleanup 完整。不能为了“生成物一致”删除上游接线，也不能顺手补改上游源图。必要的 `go generate` 复现放在独立副本；说明未来重生成的风险。

## 5. 自动合并与本地功能审核矩阵

完整 30 路径清单在相邻 `review.md`，以下问题用于组织审查，不代表已经发现缺陷。

| 重点 | 源码入口 | 需要回答的问题 |
| --- | --- | --- |
| 新 Seedance 路由 | `backend/internal/server/routes/gateway.go`、`backend/internal/handler/seedance.go`、`backend/internal/service/seedance.go` | 四组别名是否共享 API Key/group/model allowlist、本地归档/拦截；查询/删除是否保持用户/API Key/分组所有权、原账号绑定与计费去重？ |
| Seedance 前端 | `frontend/src/components/account/{CreateAccountModal,EditAccountModal,BulkEditAccountModal}.vue` | seedance 是否显式 opt-in，默认 chat/embeddings 和本地 compatible preset 是否保留？ |
| 内容审计与本地风险 | `backend/internal/service/content_moderation*.go`、`prompt_risk*.go`、对应 handler/repository；`RiskControlView.vue`、`PromptRiskPanel.vue` | 引擎配置、输入边界、runtime snapshot 和本地前置审查是否互相覆盖？ |
| 模型名和协议兼容 | `backend/internal/service/openai_gateway_{cc_pipeline,chat_completions_raw,passthrough,request_body,response_handling,responses_chat_fallback}.go` | 上游 model 还原/DeepSeek reasoning 是否与本地 cache usage、thinking/options/schema 清洗、默认 reasoning、大请求保护和空响应 failover 共存？ |
| 插件 HostService | `backend/internal/service/plugin_host_services.go`、`openai_plugin_account_directory.go`、`plugin_manager.go`、`backend/internal/repository/plugin_kv_store.go` | KV 命名空间、manifest capability 范围与目录注入是否正确；上游 metadata 保留 Extra/Proxy，文档是否错误声称完全脱敏？ |
| 账号/网络状态 | `backend/internal/service/account_usage_service.go`、`ratelimit_cn_providers.go`、`backend/internal/repository/http_upstream.go` | refresh error 保留、quota 403 暂停、HTTP/2 keepalive 是否沿目标实现，同时避免 ticket 移除影响正常 transport？ |
| 权限 | 管理路由/middleware、`frontend/src/types/index.ts`、router/store/菜单/按钮 | sub_admin/AdminPermission 和默认拒绝白名单是否保持；新插件 status 是否绕过已有管理权限？ |
| 本地后台能力 | Wire、Prompt Metrics、Token Analysis、组织用量、并发预设、quota flusher | 不只看文件存在，沿 provider → 调用方 → API/页面 → runner/cleanup 检查完整调用链 |
| 数据与文档 | `backend/migrations/238b_content_moderation_engine_meta.sql`、wiki、features 台账 | 新 engine_meta 可空与历史兼容是否一致；旧 SQL 是否零改写；25 个 feature 文档是否保留、台账仅追加？ |

已有核对主张：66 个仅上游路径与固定目标逐 blob 一致；相对旧纯上游的 369 个 backend/frontend/deploy 本地增量路径，除 Wire ticket 参数、Prompt Risk 适配、两个组件 README 外，归一化增删行保持一致。该检查不是行为证明，尤其要检查上游改动影响本地调用方的情形。部门用量/额度重置文档是本地 main 的已有设计，本轮没有授权实现该功能。

## 6. 三方比较与问题归因

```powershell
$reviewBase = 'efe9aab1e4ec89a42ba45e8dac20e882c5409a6a'
$reviewLocal = 'de5a3e383cd8eb197c1a83f12a71fb04d9e4e049'
$reviewUpstream = 'fbb9006adef852c46f0c7f18b0a8a740722cfac7'
$reviewPrevious = '8b69738d782ccaa7fd26511e1cca26ba8d1b58db'
$reviewPath = 'backend/internal/service/prompt_risk_input.go'

git diff $reviewBase $reviewLocal -- $reviewPath
git diff $reviewBase $reviewUpstream -- $reviewPath
git diff --cached $reviewLocal -- $reviewPath
git diff --cached $reviewUpstream -- $reviewPath
git show ":$reviewPath"

# 区分旧上游 ticket 与真正本地增量。
git log --oneline "${reviewBase}..${reviewPrevious}"
git diff $reviewPrevious $reviewBase -- backend frontend deploy
git diff $reviewPrevious $reviewLocal -- backend frontend deploy
```

| 归属 | 证据要求 | 审核处理 |
| --- | --- | --- |
| 本轮合并引入 | 指出两侧原合同、组合后的具体破坏和触发条件，必要时给精确复现 | 列为本轮 finding，给最小冲突修复方向，不直接修改 |
| 上游固有 | 在固定上游复现，或结合源码与完整调用链证明早于本轮 | 单列风险、严重度和证据，不要求本轮顺手修复 |
| 第一父既有 | 第一父快照复现或有充分代码对照 | 单列，不改测试断言换取绿灯 |
| 环境限制 | Docker/权限/凭据等明确错误或 skip 原文 | 说明未覆盖范围，不当成功，也不直接当业务回归 |
| 待确认 | 缺少运行条件、合同或因果证据 | 列明假设与所需证据，不写成确定缺陷 |

单文件 blob 与上游一致，不代表本地调用链必然正确；自动合并成功或构建通过也不能单独证明功能完整。既有严重问题仍需报告，只是不能错归为本轮回归。先前 0.2.6 的 Claude GO 不适用于本次候选。

## 7. 已执行测试与已知边界

以下是 2026-09-20 合并执行阶段的记录。本任务书编写阶段只核对状态和文档，没有重跑业务测试。Claude 必须区分“读取的已有证据”和“本次独立执行”。

| 验证 | 已有结果 | 证据文件，均相对 `backend/.gocache/merge-207/` |
| --- | --- | --- |
| 后端专项/default | 专项通过；default 54 个测试包通过、14 个显式 skip，退出 0 | `focused.log`、`default.log` |
| 后端 unit | 58 个测试包通过、2 个包失败、18 个显式 skip；实际失败为 auth/me 与 Ollama CAS | `unit.log`、`test-summary.json` |
| 后端 integration | 53 个测试包通过；repository 的 TestMain 因无 Docker 在 CI=true 下失败；另有 18 个显式 skip | `integration.log`、`test-summary.json` |
| 构建/依赖/lint | normal/embed build、go mod tidy -diff 通过；golangci-lint v2.13.0/Go 1.27 增量 0 issues | `build.log`、`embed.log`、`tidy.log`、`lint.log` |
| 前端静态检查/构建 | lint、typecheck、build 通过（1089 modules） | `frontend-lint.log`、`frontend-typecheck.log`、`frontend-build.log` |
| 前端完整测试 | 325/326 files、2427/2433 tests；剩余 6 个订阅 Pinia 失败 | `frontend-test.log`、`frontend-results.json` |
| Wiki 图谱 | 34 nodes、72 edges、54 wikilinks、0 unresolved；允许待审 wiki dirty 时 READY | 当前图谱及 `tools/check-understand-status.cmd -AllowDirtyWiki` |

| 需要复核的失败/风险 | 已有证据与要求 |
| --- | --- |
| auth/me golden | `TestAPIContracts/GET_/api/v1/auth/me` 实际多 `admin_permissions:null`，第一父精确复跑同样失败；见 `baseline-auth.log`。父 TestAPIContracts 的 fail 不额外计作一个独立缺陷 |
| Ollama CAS | 当前 `-count=3` 为 3 失败，第一父为 1 通过/2 失败；见 `ollama.log`、`baseline-ollama.log`。对应 ratelimit_service_ollama_429 实现和测试四方 blob 一致；不能误写第一父 3/3 失败，也不能一次转绿就宣称修复 |
| 前端 Pinia | `SubscriptionsView.userUsageLink.spec.ts` 的 6 个失败在第一父同样复现；见 `baseline-frontend.log`。不要与之前已修的 bulkActions spec 混淆 |
| Docker | 原文 `docker is not available (CI=true); failing integration tests`；不是 repository 用例执行成功，也不是本轮业务代码失败 |
| integration 的 18 个 skip | DingTalk sentinel 1、symlink 权限 1、Docker 限流 3、TLS capture 1、Prompt Audit Redis 3/PostgreSQL 6、TypeSafe live 1、OpenAI key 1、插件 fixture 1；repository 整包失败另计 |
| Wire 源图漂移 | 目标已有 SetAccountDirectory 仅写入生成物；保留最终行为但未修源图。须独立确认，不能重生成后把新差异留在候选中 |

## 8. 按需复跑

先看已有证据，优先验证本地与上游交界。没有新疑点时无需重跑整套；若只复跑部分，明确覆盖范围。以下命令在仓库根目录的独立 PowerShell 进程执行，输出保存到新日志，不覆盖原始日志。

```powershell
$reviewRepo = (git rev-parse --show-toplevel).Trim()
$env:GOCACHE = Join-Path $reviewRepo 'backend/.gocache/review-cache'
$env:GOPATH = Join-Path $reviewRepo 'backend/.gocache/review-gopath'
$env:GOMODCACHE = Join-Path $env:GOPATH 'pkg/mod'
$reviewGitRoot = Split-Path (Split-Path (Get-Command git.exe).Source)
$reviewShell = Join-Path $reviewGitRoot 'usr/bin'
if (Test-Path (Join-Path $reviewShell 'sh.exe')) {
    $env:PATH = $reviewShell + ';' + $env:PATH
}
function New-ReviewGoTemp {
    $env:GOTMPDIR = Join-Path $reviewRepo ('backend/.gocache/run-tmp-claude-027-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Force $env:GOTMPDIR | Out-Null
}
Set-Location (Join-Path $reviewRepo 'backend')
New-ReviewGoTemp
go test -tags=unit -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes -run 'PromptRisk|ContentModeration|Seedance|PluginHost|PluginAccount|RequestArchive|RequestIntercept'

# 仅在需要重新验证前端交界时执行。
Set-Location (Join-Path $reviewRepo 'frontend')
pnpm.cmd exec vitest run src/views/admin/__tests__/RiskControlView.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts src/components/account/__tests__/BulkEditAccountModal.spec.ts
```

每个命令单独记录退出码；shell 最后一个命令成功不代表前面的命令成功。全量命令可参考相邻 `review.md`。重新做 integration 时明确 CI 标志；无 Docker 时不能通过取消 CI 使整包跳过，然后宣称原验证已通过。

## 9. 交付格式

新增 `docs/delivery/2026-09-20-sub2api-v0.2.7-sync/claude-review-result.md`，保持 LF，不暂存。建议包含以下内容：

| 部分 | 内容 |
| --- | --- |
| 结论 | 对“本次固定 SHA、仅冲突合并候选”给出 GO / NO-GO；若覆盖不足，明确未完成项，不以 GO 替代生产验收 |
| Findings 优先 | 按 P0–P3 排序；每项写归属、仓库相对路径与现场行号、触发条件、影响、三方证据、最小修复方向；没有发现则明确写无新增 finding |
| 本地功能 | 逐项说明保留、回归或未确认；不要仅写“文件都在” |
| ticket 与 Wire | 分别确认授权范围、移除完整性、普通 turn-state 保留、插件账号目录最终接线及生成风险 |
| 验证 | 分开列已有日志与自己执行的命令、退出码、失败和 skip；说明未执行的环境依赖验证 |
| 上游/第一父风险 | 独立列出，不能与本轮回归混记；严重风险仍须说明对部署的影响 |
| 文档准确性 | 核对 wiki、合并台账、review.md 是否有过时或过度声明；建议修正但不改已有文件 |
| 终态 | 记录分支、HEAD、MERGE_HEAD；确认索引/代码未改，唯一新增为复审报告；明确未 commit/push/部署 |

GO 只能说明在已验证范围内未发现阻断本次冲突合并的缺陷，不代表上游没有 bug、数据库集成全覆盖或允许自动提交。不要为满足输出数量而制造 finding。
