# Sub2API 0.2.6 合并复审任务书（Claude）

编写日期：2026-09-19。请对当前待提交的合并结果做独立、只读复审，判断冲突处理是否正确、本地功能是否完整，以及是否混入了范围外修复。本文是复审输入，不是复审通过结论。

## 1. 原始需求与操作边界

| 项目 | 要求 |
| --- | --- |
| 合并方式 | 从最新本地 main 新建分支，合入用户指定的上游精确 SHA |
| 本地功能 | 保留 `docs/features/` 所描述的独有能力；真正重叠的能力可以采用上游实现，但要给出功能等价证据 |
| 修改范围 | 仅解决文本冲突及自动合并造成的语义冲突，不额外修复上游或第一父既有 bug |
| 交付要求 | 更新 wiki、追加合并台账、执行测试，停在 commit 前等待用户审核 |
| 本次 Claude 任务 | 检查源码、暂存区、测试和已有证据，输出复审报告；不要直接修复源码、测试或已有文档 |
| Git 状态 | 不执行 commit、push、merge、rebase、reset、checkout、switch、restore、add、stash，也不刷新或移动合并目标 |
| 生成及自动修复 | 不在当前候选中运行 `go generate`、`go mod tidy`（无 `-diff`）、`eslint --fix` 或自动格式化；必要的生成复现放在独立临时副本 |
| 允许的验证 | 只读 Git/源码核对、按需运行测试并写临时日志；不启动生产服务、不调用真实收费上游、不写真实业务数据 |

如用户之后明确授权修复，以新授权为准；在此之前只给结论和最小修复建议。不要因为本文件要求只读而省略必要的源码核对。

## 2. 固定审查基线

2026-09-19 编写本文时，已重新读取真实 Git 状态，仍处于未提交 merge。

| 项目 | 精确值 |
| --- | --- |
| 工作分支 | `feature/hy/10206_merge_sub2api_206` |
| 第一父 / 当前 HEAD / 本地 main | `5ec57e4fc51a9052e8812f4cb925565c984856cc` |
| 第二父候选 / MERGE_HEAD | `8b69738d782ccaa7fd26511e1cca26ba8d1b58db` |
| 上游来源 | `https://github.com/Wei-Shaw/sub2api` 的 main |
| merge base | `881f3202694c6bc932446931a30c27d9675178b9` |
| 固定目标 VERSION | `0.2.6` |
| 上游相对 merge base | 60 commits、132 paths、`+4836/-304` |
| 双方修改集合 | 31 个双方修改路径、495 个仅本地修改路径、101 个仅上游修改路径 |
| 本文加入前 | 148 个暂存路径、`+5069/-328`；无 unmerged、unstaged、untracked |
| 本文加入后 | 149 个暂存路径；仅新增本复审任务书，已有候选内容保持不变 |
| 源码范围索引指纹 | SHA256 `a5306abf9ebbb14838b08102d59dc0d9fd18eafc8ca3c368f56fa76231811a34`；口径见下方命令 |

`review.md` 中的 148 个文件是上一轮完成时的快照，不能因新增本文变为 149 个就判定业务代码发生变化。测试结果来自 2026-09-18，本次只重新核对候选状态并编写复审文档，没有重跑全量测试。

开始复审先执行以下命令。如果 HEAD、MERGE_HEAD、分支或源码指纹变化，先记录漂移并重新界定审查对象，不要套用本文旧结论。

```powershell
git status --short --branch
git branch --show-current
git rev-parse HEAD
git rev-parse main
git rev-parse MERGE_HEAD
git merge-base HEAD MERGE_HEAD
git diff --cached --stat
git diff --cached --check
git diff --name-only
git ls-files -u
git ls-files --others --exclude-standard
python -c 'import hashlib,subprocess; print(hashlib.sha256(subprocess.check_output(["git","ls-files","--stage","-z","--",".gitignore","backend","frontend","deploy"])).hexdigest())'
```

指纹覆盖这些路径下所有已跟踪文件的索引条目，包括源码、测试、依赖声明、配置和组件 README；不包含本文所在的 `docs/`。完整索引核对还应查看 `git diff --cached`。

## 3. 阅读顺序与证据入口

| 顺序 | 路径 | 用途 |
| --- | --- | --- |
| 1 | `AGENTS.md`、`llm-wiki/wiki/README.md` | 仓库规则和知识入口 |
| 2 | `llm-wiki/wiki/backend.md`、`frontend.md`、`ops.md`、`data-and-domain.md`、`security-and-reliability.md` | 本地能力与 0.2.6 数据流、配置、权限约束 |
| 3 | `docs/upstream-merge-playbook.md`、`docs/features/` | 本地长期维护功能和行为合同；分支命名及固定 SHA 以当前用户要求为准 |
| 4 | `docs/delivery/2026-09-18-sub2api-v0.2.6-sync/review.md` | 前一轮处理说明、完整 31 路径清单、测试结果和问题归属主张 |
| 5 | `docs/features/sub2api -merage-list.md` 的 2026-09-18 条目 | 合并记录，核查是否只追加而未改写历史 |
| 6 | `.git/codex-merge-026/` | 本机原始预演、三方清单、测试日志与第一父源码快照；此目录不随 Git 提交分发 |

已有报告的“已确认”“已保留”均是需要复核的主张。以当前索引、源码、测试和实际日志为准；文件仍存在、diff 自动合并成功、构建通过，都不足以独立证明功能保留。

不要直接执行 `.git/codex-merge-026/verify-backend.ps1`：它是前一轮执行记录，含代码生成及 `git restore --staged --worktree`，不符合此次只读复审边界。`update-docs.py`、`write-review.py` 同样只供查阅，不应重放。

## 4. 必须独立核对的四个文本冲突

| 冲突文件 | 当前处理主张 | 请重点验证 |
| --- | --- | --- |
| `.gitignore` | 保留本地文档跟踪例外，加入上游 `docs/ANTIGRAVITY_ATTRIBUTION_429.md` | features/reviews/superpowers/playbook 是否仍可跟踪；缓存、fixtures、运行报告是否仍被忽略 |
| `backend/go.mod` | 采用上游新版本，保留 x/sys、x/text 的本地直接依赖属性 | 本地实际 import 与 direct/indirect 是否一致；gRPC 1.83.2 和其余升级是否完整；`go.sum` 是否与固定上游一致，有无 Wire 工具副作用 |
| `backend/internal/service/setting_service.go` | 本地 `onRiskControlUpdate` 与上游 ticket cache/singleflight 字段共存 | 不能只检查字段存在；沿注册方、`setting_update.go#refreshCachedSettings`、运行态消费者确认本地风控热更新仍成立 |
| `backend/cmd/server/wire_gen.go` | 从合并后的 provider source 生成 | 对照 `backend/cmd/server/wire.go` 和 `backend/internal/handler/wire.go`，确认本地 provider/handler/cleanup 未丢；新增 SettingService 和 ticket harvester 生命周期无漏接或重复接线 |

前一轮 Wire 连续两次生成一致，文件 SHA256 为 `0ED69076DCE05B2A86919A6FD68D60B7882EB0D2DF9793D69BE86178265FF683`。这是生成一致性的证据，不替代 provider 图的语义检查。本次没有 SQL migration 或 Ent schema 增量。

## 5. 自动合并和本地功能复核重点

以下是核查问题，不是已经发现的新缺陷。完整双方修改路径见相邻 `review.md` 的逐项表；不要只审查出现过冲突标记的四个文件。

| 优先顺序 | 入口 | 需要回答的问题 |
| --- | --- | --- |
| 1 | `backend/internal/service/setting_service.go`、`setting_update.go`；两处 Wire source | 新增配置和后台任务是否覆盖了本地风控回调、Prompt Metrics、Token Analysis、组织用量、并发预设、quota flusher 的接线或清理？ |
| 2 | `backend/internal/service/openai_gateway_{forward,messages,passthrough,chat_completions_raw,request_body,response_handling}.go` | 上游 ticket 注入、Chat role 规范化、DeepSeek 媒体提取、affinity context 变更，是否与本地 thinking/options/schema 清洗、compatible cache usage、reasoning-only failover 共存？有没有重复处理、顺序变化或 usage 覆盖？ |
| 3 | `backend/internal/service/openai_codex_ticket.go`、`openai_account_runtime_block_fastpath.go`、`openai_account_scheduler.go`、WS forwarding 文件 | 调度与实际出站模型（尤其 compact）是否一致？开关关闭时是否保留既有路径？账号/模型隔离、fail-closed、影子账号豁免在合并中是否被改变？发现问题须先对照固定上游归因 |
| 4 | `backend/internal/server/routes/gateway.go`、`router.go`、相关 middleware | 本地归档、Responses subpath guard、请求拦截与 Prompt Metrics 顺序是否保持？上游新增行为是否另走了未挂本地能力的入口？ |
| 5 | `backend/internal/handler/dto/{mappers,types,settings}.go`、`backend/internal/handler/admin/account_*`、`backend/internal/repository/account_repo.go` | 票据脱敏/导出/更新是否保留本地字段与权限边界？旧账号编辑快照是否可能因合并覆盖新票据？对上游原样代码的缺陷单独列出 |
| 6 | `backend/internal/handler/gemini_v1beta_handler.go`、`gemini_v1beta_handler_test.go` | 混合模型列表是否兼容本地 path parser、模型白名单、并发基础设施错误分类及 fixture？ |
| 7 | `frontend/src/views/admin/SettingsView.vue`、`frontend/src/api/admin/settings.ts`、`frontend/src/types/index.ts`、中英文账号/设置文案 | ticket 读取、初始化、提交是否一致；只读 configured 是否误提交？本地 RequestArchive 目录/开关、auth-source fallback、UserRole/AdminPermission 是否保留？ |
| 8 | 子管理员路由/菜单/按钮；`frontend/src/views/admin/SubscriptionsView.vue` | 本地权限是否因新字段或组件行为被放宽？选中行批量动作仍只允许完整管理员，组织筛选和按筛选重置是否保持？ |
| 9 | `backend/internal/repository/custom_group_usage_rollup_repo.go` 与本地组织用量链路 | 上游分组汇总优化是否误替代本地组织统计？如发现水位/时区问题，需证明是上游已有还是双方组合引入 |
| 10 | `docs/features/`、本地测试和对应功能入口 | 24 个原有文档零删除只是基础证据；独有归档/拦截、Prompt Risk/LLM judge、Token Analysis、组织用量、生图工具、默认 reasoning、大请求保护、并发预设是否仍有完整调用链？ |

前一轮的逐 blob 核对结果：101 个仅上游路径与目标一致、349 个仅本地非 Markdown 且不在 `llm-wiki/` 的路径与第一父一致、49 个仅上游测试文件原样保留。495 是最初仅本地修改的全部路径数，包含本轮后来更新的文档，不能误解为最终 495 个路径都与第一父零差异。24 个 feature 文档中仅合并台账追加了内容。

## 6. 三方比较方法与归因标准

当前合并已经解决冲突并暂存，普通 `git diff` 可能为空；审查对象应使用 `--cached`。stage 1/2/3 已被解决后的 stage 0 替代，不要依赖 `git show :2:path` / `:3:path`。

```powershell
$reviewBase = '881f3202694c6bc932446931a30c27d9675178b9'
$reviewLocal = '5ec57e4fc51a9052e8812f4cb925565c984856cc'
$reviewUpstream = '8b69738d782ccaa7fd26511e1cca26ba8d1b58db'
$reviewPath = 'backend/internal/service/setting_service.go'

# 各自相对共同祖先的修改
git diff $reviewBase $reviewLocal -- $reviewPath
git diff $reviewBase $reviewUpstream -- $reviewPath

# 合并候选相对两侧的剩余差异
git diff --cached $reviewLocal -- $reviewPath
git diff --cached $reviewUpstream -- $reviewPath

# 需要精确内容或对象证据时
git show "${reviewBase}:$reviewPath"
git show "${reviewLocal}:$reviewPath"
git show "${reviewUpstream}:$reviewPath"
git show ":$reviewPath"
git rev-parse "${reviewLocal}:$reviewPath"
git rev-parse "${reviewUpstream}:$reviewPath"
git rev-parse ":$reviewPath"
```

| 归属 | 必须给出的依据 | 本次处理建议 |
| --- | --- | --- |
| 本轮合并引入 | 第一父和上游各自的合同或测试成立，组合后因漏字段、错顺序、接线或 fixture 不兼容而失效 | 作为本轮 finding，给出触发条件、影响和最小冲突修复方向；等待用户决定是否修 |
| 上游固有 | 在固定上游本身可复现，或相关代码/测试和完整依赖链足以证明问题早于合并 | 单独报告风险和证据，不提出在本轮顺手修复的要求 |
| 第一父既有 | 第一父快照可复现，相关变化不是本轮带入 | 单独报告，不改原有功能或断言以换取绿灯 |
| 环境限制 | 失败由缺少 shell、Docker、服务、权限或凭据导致，须有错误原文或复跑证据 | 报告覆盖缺口，不能当成功，也不能直接归为业务回归 |
| 尚未确认 | 仅有怀疑、无法完成对照或缺必要合同 | 明确待确认项与所需证据，不写成 confirmed defect |

即使某个入口文件与上游 blob 相同，也要考虑本地调用方、接口、权限或全局状态是否改变了运行语义；单文件相同不是普遍免责依据。同样，新增失败不能仅因“这是上游测试”就自动归责上游。

## 7. 已执行验证与需要复核的失败

下表均为 2026-09-18 的已有执行结果，本次任务书没有新增测试执行。

| 验证 | 已有结果 | 原始证据（相对 `.git/codex-merge-026/`） |
| --- | --- | --- |
| Go 聚焦 unit | 通过，覆盖冲突接线、新版特性和本地主要能力；部分包过滤后无测试，详见日志 | `focused.log`、`verify-backend.ps1` 中的参数，仅查阅脚本 |
| Go default 全量 | 首轮只有 repository 3 个缺 sh 的失败；补齐测试 PATH 后 repository 全包通过 | `default.log`、`repository-default.log`、`repository-results.txt` |
| Go unit 全量 | 首轮含上述环境失败，另有 auth/me golden 和 Ollama CAS；repository 环境复跑通过 | `unit.log`、`repository-unit.log` |
| Go integration 全量 | 退出 0，但有 17 个显式 skip，另有 repository 整包 Docker skip | `integration.log`、`repository-integration-evidence.log` |
| Go 普通/嵌入式构建 | 均通过 | `backend-results.txt`、`build.log`、`build-embed.log` |
| 依赖 / 增量 lint | `go mod tidy -diff` 无差异；golangci-lint v2.13.0/Go 1.27 增量为 0 issues | `tidy-diff.log`、`lint.log`、`lint-result.txt` |
| 前端 lint / typecheck / build | 均通过；build 含 i18n 前置检查，1089 modules | `frontend-results.txt` 及各同名日志 |
| 前端完整 Vitest | 325/326 files、2422/2428 tests 通过，6 个订阅 Pinia 装配失败 | `frontend-tests.log` |
| 第一父对照 | 订阅 6 个失败与 auth/me golden 在第一父源码快照均复现 | `frontend-baseline.log`、`backend-baseline.log`、`baseline/` |
| Wire / Wiki 图谱 | 两次生成一致；图谱 33 nodes / 67 edges，READY | `generate-1.log`、`generate-2.log`、`wiki-final-status.log` |

| 已知失败 | 已有归因与复审要求 |
| --- | --- |
| `TestAPIContracts/GET_/api/v1/auth/me` | 实际响应含 `admin_permissions:null`，旧 golden 缺字段。核对 `backend/internal/handler/dto/types.go`、API golden 和第一父精确复跑；不要删除权限字段或改 golden 来掩盖既有失败 |
| `TestOllamaProbeCallback_StaleLongDoesNotOverrideNewShort` | stale callback 的 CAS 计数为 1；本轮 unit 失败，精确 `-count=3` 复跑 3 次失败。相关 ratelimit 实现与测试四方一致；历史有时序波动，按 flaky 跟踪，不因一次转绿宣称修复 |
| `SubscriptionsView.userUsageLink.spec.ts` 6 个测试 | 页面调用 `useAuthStore()`，fixture 未初始化 Pinia/auth mock；第一父精确复跑同样 6 个失败。核对页面、fixture 和相关依赖；不要与此前已修的 `SubscriptionsView.bulkActions.spec.ts` 混淆 |
| PgDumper 3 个测试 | Windows 测试 PATH 没有 sh；本机 Git 位于 `F:/an/Git`，补入其 `usr/bin` 后通过。应动态定位 Git，不照搬默认安装路径 |

integration 的 17 个显式 skip 包括：3 个 Docker 限流测试、3 个 Prompt Audit Redis、6 个 Prompt Audit PostgreSQL、1 个符号链接权限测试、TLS capture/OpenAI API/本地插件包各 1 个，以及 1 个 DingTalk 哨兵测试。repository 的 `TestMain` 整包跳过不在这 17 个单项中。未完成真实数据库/在线验证，不能给出“完整集成全绿”或“已验证可直接上线”的结论。

`.git/codex-merge-026/final-state.json` 是末次补充报告文字之前保存的快照，其中 `+5066` 不是当前最终 diff 行数；本文开头的 `+5069` 来自 2026-09-19 现场核对。不要把记录时间差误判为代码改动。

## 8. 按需复现命令

先读已有日志；出现新疑点、归因不清或需要确认当前候选时，再运行对应测试。以下为只读源码验证，不要求无条件重跑全套。

Windows Go 在独立 PowerShell 进程中设置环境，避免污染用户终端；每次 `go test` 前调用 `New-ReviewGoTemp`：

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
    $env:GOTMPDIR = Join-Path $reviewRepo ('backend/.gocache/run-tmp-claude-026-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Force $env:GOTMPDIR | Out-Null
}
Set-Location (Join-Path $reviewRepo 'backend')

New-ReviewGoTemp
go test -tags=unit -p 1 -count=1 ./internal/service ./internal/handler ./internal/handler/admin ./internal/handler/dto ./internal/server/routes -run 'CodexTicket|CodexTurn|BindHTTPResponseAccount|Gemini.*Model|DeepSeek|Chat.*Role|RequestArchive|RequestIntercept|RiskControl|CompatibleCache|ReasoningOnly|AdminPermission|SubAdmin'

New-ReviewGoTemp
go test -tags=unit -p 1 -count=1 ./internal/server -run 'TestAPIContracts/GET_/api/v1/auth/me'

New-ReviewGoTemp
go test -tags=unit -p 1 -count=3 ./internal/service -run '^TestOllamaProbeCallback_StaleLongDoesNotOverrideNewShort$'

New-ReviewGoTemp
go test -tags=unit -p 1 -count=1 ./internal/repository -run '^TestPgDumper'
```

前端从仓库根目录执行：

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/views/admin/__tests__/SettingsView.spec.ts src/components/account/__tests__/AccountUsageCell.spec.ts src/views/admin/__tests__/SubscriptionsView.bulkActions.spec.ts
cmd.exe /c pnpm --dir frontend exec vitest run src/views/admin/__tests__/SubscriptionsView.userUsageLink.spec.ts
```

每条命令单独记录退出码。出现红灯不自动改测试；若需在第一父或上游复现，应使用隔离源码副本及相同依赖条件，不切换或改写当前合并工作区。复审前后比较索引与工作区，说明是否产生任何 tracked 文件变动。

## 9. 请 Claude 输出的复审结果

请使用中文，先列 findings，再给验证和结论。每个 finding 至少包含：

| 字段 | 要求 |
| --- | --- |
| 严重度与位置 | P0/P1/P2/P3，当前候选文件路径和准确行号 |
| 触发条件与影响 | 可复现的输入/配置/调用路径，以及实际行为偏差 |
| 归属 | 本轮合并引入 / 上游固有 / 第一父既有 / 环境 / 未确认 |
| 三方证据 | base、第一父、上游、候选之间的关键代码差异；必要时给最小复现 |
| 修复方向 | 仅描述解决该问题的最小范围，注明是否属于本轮允许的冲突修复 |
| 证据状态 | 已实际执行的命令、退出码；未运行或被阻断的部分不得写成通过 |

同时提供以下表格：

1. 四个文本冲突：逐项通过 / 不通过 / 未确认及理由。
2. 自动合并和本地功能：已复核范围、发现的问题、尚未覆盖的路径。
3. 已知失败归因：确认 / 推翻 / 证据不足；特别说明是否发现合并新增失败。
4. 文档与候选一致性：wiki、台账、原审核报告是否存在错误结论或遗漏。
5. 提交前结论：GO / NO-GO / INCOMPLETE，并说明真正的阻断项及残余风险。

GO 仅表示在本次“固定 SHA、保留独有功能、只解决冲突”的范围内可交由用户决定是否提交，不代表已获得 commit 授权或完成生产上线验收。没有发现问题时明确写“本次复审未发现新增 finding”，仍须说明覆盖范围与未验证部分。

本轮只输出复审结论；若需要将结论落盘，请先按用户后续指示处理。不要把本文的预设检查项直接抄成缺陷清单，也不要以测试总数、文件保留数代替行为证据。
