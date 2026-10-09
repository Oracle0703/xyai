# Sub2API 0.2.15 固定提交合并审核

## 边界

| 项目 | 结果 |
| --- | --- |
| 日期 / 工作分支 | 2026-10-09 / `feature/hy/10216_merge_sub2api_215` |
| 本地 main / 第一父 | `cdc5178d26f5b9a9300604bf09f74e5f3e1eb18a` |
| 上游分支 / 固定目标 | `Wei-Shaw/sub2api main@3a6fd1c9db07203ca308aaba69e502bc1f35b307` |
| 共同祖先 | `b8dece9000c68815a5b867ca5a1e6f236e173905` |
| 版本 / 上游增量 | `0.2.15`；154 commits / 301 paths / `+20051/-1402` |
| 合并提交 | 尚未创建；`HEAD` 保持第一父，`MERGE_HEAD` 固定目标，等待用户 commit 前审核 |
| 范围 | 仅解决冲突；本地独有能力保留，重叠实现采用上游，不修上游 bug |

本地 main 在操作前干净；从其新建分支后执行 `git merge --no-commit --no-ff <固定目标>`。此前 0.2.13 已由第一父 merge commit 提交并进入本地 main；不改写上轮历史审核报告或台账。

## 冲突裁决

| 文件 | 双方改动与处理 |
| --- | --- |
| `backend/go.mod` | 上游升级 Go 1.27.2 和 x/* 依赖，本地直接导入 x/sys 的 diskspace 与 x/text 的 request intercept。采用上游所有新版本，保留 x/sys 0.48.0、x/text 0.42.0 为直接依赖，避免丢失本地依赖合同。 |
| `frontend/src/api/admin/users.ts` | 上游新增 `listPlatformIds` / `AccountPlatform` 并将配额平台列表改为清单函数；本地增加 `AdminPermission` / `UserRole`。合并 import，保留本地权限目录、角色、组织/部门筛选及管理请求合同。 |

其它业务实现由 Git 自动合并；没有在上游独有生产实现或测试中加入 bug 修补。

## 三方核对与本地功能

| 核对范围 | 证据 |
| --- | --- |
| 270 个仅上游修改路径 | 合并结果 index blob 逐项等于固定目标。 |
| 685 个仅本地修改路径 | 文档沉淀前，index blob 逐项等于第一父；包含本地删除记录的正确保留。收尾只允许 wiki/图谱/组件 README/合并材料文档发生额外变化。 |
| 31 个双方修改路径 | 29 个路径的“第一父→结果”增加/删除行序列逐项等于“共同祖先→上游”；另 2 个仅包含上述冲突裁决。 |
| `docs/features` | 29 个 tracked 文件全部保留，逐 blob 等于第一父，无改写/删除。 |
| Ent / Wire | 连续两轮生成无漂移；核对插件账号目录及本地服务接线保留，工具附加的 12 行 go.sum checksum 已恢复到合并前生成工具执行基线。 |

本地 RequestArchive/RequestIntercept、Prompt Metrics/Risk、Token Analysis、生图页面、组织/部门报表、子管理员、订阅自助重置、GPT 额度展示、用户并发预设、quota flusher、OpenAI-compatible preset/default effort/参数过滤继续保留。三方核对证明源码保存；真实外部上游、生产浏览器和数据库覆盖以测试表的边界为准。

| 双方修改区域 | 路径与核对要点 |
| --- | --- |
| 领域 / 生成物 / 依赖 | `internal/domain/constants.go`、`internal/service/domain_constants.go`、Ent runtime、go.mod；新平台与清单来自上游，保留本地领域字段。 |
| handler / routes / auth | OpenAI handler 及其测试、gateway routes、API contract、auth service；保留本地归档/拦截/审计中间件顺序和权限数据，合入上游网关族分流及 WS turn 定价。 |
| 协议转换 | `pkg/apicompat/chatcompletions_to_responses.go` / `types.go`；本地 cache breakpoint 等字段保留，采用上游 developer、旧 function_call 配对、tool_choice、refusal、thinking signature。 |
| OpenAI service | Codex transform、Chat/Responses/Messages、passthrough、request body、response handling；采用上游统一协议路由和 web_search history 声明，保留本地默认 effort、参数过滤/缓存和空响应合同。 |
| 前端 | package/lock、settings/users client、CreateAccountModal、credentialsBuilder 测试、UsageTable、settings view、types、en/zh accounts locale；本地角色/权限和 compatible preset 保留。 |
| 根 README | 仅并入上游新安装随机管理员说明，本地架构/运维入口保留。 |

## 上游新合同与已知边界

| 事项 | 合并后的行为 / 边界 |
| --- | --- |
| 平台 / provider profile | `domain/platforms.go` 与 `provider_profile.go` 成为平台/端点与协议配置入口；新增 Command Code/Cline。前端使用内置 platform catalog JSON，不存在运行时拉取清单的新 API。 |
| migration 242 | 删除 user quota/composite route 的两处平台 CHECK，应用层/Ent 统一清单校验；渠道监控 CHECK 保留，旧迁移和本地订阅日限迁移未改写。 |
| WS 分组定价 | 后续 turn 重取同 Key/分组/平台/订阅类型的计费分组，失败回退建连快照；不重新加载所有授权/订阅/额度。 |
| 初始化 / EasyPay | 仅首次实际创建管理员时生成随机邮箱/密码，显式输入校验；EasyPay 通知拒绝未知参数，采用上游实现。 |
| 本地平台筛选 | `subscription_admin_filter.go:51`、`organization_usage_department.go:43`、`OrganizationUsageFilters.vue:218` 未纳入 TypeSafe/Command Code/Cline；订阅请求选择新平台仍会被本地校验拒绝，部门下拉未显示。仅记录，不扩展本地新平台支持。 |
| TypeSafe 既有边界 | `/v1/systemone` 的本地 Prompt Risk、归档/Token Analysis、拦截和 Prompt Metrics 覆盖缺口保持不变，详见 wiki security；此轮不修复。 |

## 验证

测试命令使用 `backend/.gocache/review-cache` / `review-gopath`、每命令独立 GOTMPDIR、Git `usr/bin` PATH、串行包 `-p 1 -count=1`。前端安装按 pnpm 9 frozen lockfile 执行；日志为本机忽略产物。

| 命令 | 结果 |
| --- | --- |
| `go generate ./ent` / `go generate ./cmd/server`（两轮） | 通过，两轮无漂移。 |
| `go test -p 1 -count=1 -v ./...` | 退出 0，54 包通过，15 个显式 skip。 |
| `go test -tags=unit -p 1 -count=1 -v ./...` | 退出 1：58 包通过；Ent 首次共享临时目录失败，独立复验通过；service 因 main 既有重复 ptrFloat 不能编译。15 个显式 skip。 |
| `go test -tags=unit -p 1 -count=1 -v ./ent/schema`（独立复验） | 通过，解决验证方式造成的共享目录竞争；没有改测试。 |
| `go test -overlay <本机临时JSON> -tags=unit -p 1 -count=1 -v ./internal/service` | 完整补跑，退出 1；8237 个顶层 PASS、含子用例 16039 条 PASS，4 skip。失败为默认 effort none 旧断言和上游 Command Code catalog 用例。两项聚焦复验中 Command Code 通过，none 仍失败。 |
| 指定上游原始源码 `go test -tags=unit -p 1 -count=3 -v ./internal/service -run '^TestCommandCodeGatewayPassesThroughCatalogProtocols$'` | 2 pass / 1 fail，与合并结果首次失败位置相同（model_protocol_catalog_test.go:374）；确认目标上游本身存在测试波动。使用 git archive 的原始源码，没有本地 feature 或 overlay。 |
| `go test -tags=integration -p 1 -count=1 -v ./...` | 退出 0，54 包通过，19 个显式 skip；repository 因 Docker 不可用整包跳过，非完整集成验收。 |
| 独立 PG18.1 + postgres-only repository 专项 | 通过，35 顶层用例 / 45 条 PASS（含子用例），0 skip；启动/停止均通过。覆盖 migration 242、user quota/composite target 拒绝未登记平台、UserPlatformQuota、Department/DepartmentSubscriptions/OrganizationUsageDepartment、Ops TPS 分布/查询期限、GPT 额度读写/并发。 |
| `go mod tidy -diff` | 退出 1，只报告固定上游 go.sum 的 18 行旧 checksum，应删除而尚未清理；未修改。 |
| normal / embed `go build` | 均通过；embed 使用本轮前端构建产物。 |
| golangci-lint 2.14 `run --new-from-rev=HEAD --timeout=30m ./...` | 通过，0 issues，与上游 CI 当前版本一致。 |
| `pnpm.cmd install --frozen-lockfile` | 通过，锁文件无漂移。 |
| `pnpm.cmd run lint:check` / `typecheck` | 均通过。 |
| `pnpm.cmd run test:run` | 401 files / 3101 tests 全部通过；旧平台断言随上游更新，无本地修补。 |
| `pnpm.cmd run build` | 通过，含 i18n 3/3 与 typecheck，1148 modules；保留 Browserslist、动态 import 与 chunk size 提示。 |
| 部署脚本静态检查 | docker-deploy shell syntax、Compose security/gateway env、runtime resources、Caddy cache 检查通过；Apple container 和 simple-mode 的本机环境失败见下文。 |
| Wiki 图谱 / Git 门禁 | 见交付状态；只表示文档与合并快照一致，不替代真实外部上游、浏览器或完整 Docker/Redis 验收。 |

失败归属说明：

- unit 的两个 `ptrFloat` helper 位于 `internal/service/payment_config_plans_validation_test.go:137` 和 `gpt_quota_display_test.go:287`；文件逐 blob 等于第一父，未改动。正式 unit 编译失败不能视为通过；补测 overlay 只在本机忽略目录替换一个 helper 名称，不改变断言或生产代码。
- 默认 effort none 的测试文件、`applyDefaultOpenAIReasoningEffort` 函数和 `normalizeOpenAIReasoningEffort` 函数均与第一父一致；当前聚焦复验仍在 `openai_default_reasoning_effort_test.go:68` 失败，保持既有基线问题。
- Command Code catalog 测试文件及目录实现逐 blob 等于指定上游，纯上游源码 3 次复验为 2 pass / 1 fail，排除本地 feature 合并导致该测试失败。用例以 UnixNano 构建 URL/cache key，Windows 时钟粒度可能令连续调用复用 key，而错误 store 保留先前目录；这是时序原因假设，未据此改生产实现或测试。
- 首次三组 Go 套件并行执行时，Ent 的 `load.Config.Load()` 共享工作目录 `.entc`，触发 `TestAuthIdentityFoundationSchemas` 临时文件被删除错误；随后独立 `go test -tags=unit -p 1 -count=1 -v ./ent/schema` 通过。初始失败日志保留，不计为合并源码缺陷。
- `go mod tidy -diff` 只要求删除 go.sum 中 9 个旧版本的 18 行 checksum，没有 go.mod 差异；当前 go.sum 逐 blob 等于指定上游，按冲突限定范围不清理。
- PG18.1 使用本机已有 embedded-postgres 二进制、新建 `backend/.gocache/merge215-pg-data-*` 数据目录和 loopback 随机端口；未使用已有业务数据目录。两组测试均已通过，pg_ctl fast stop 均退出 0。仓储命令先运行 `-run 'TestMigration242|TestUserPlatformQuota|TestCompositeModelRoute|TestOpsRepository.*|TestDepartmentRepositoryIntegration|TestDepartmentSubscriptionsIntegration|TestOrganizationUsageDepartmentIntegration|TestSubscriptionSelfReset|TestGptQuota'`，再补跑大小写/名称未匹配到的 `-run 'TestOpsOutputTPS|TestGPTQuotaDisplay'`；前者 29 顶层 / 36 PASS，后者 6 顶层 / 9 PASS。仅证明 PostgreSQL 专项，未安装/连接 Redis 或 Docker。
- Apple container 测试与第一父逐 blob 一致，Windows/Git Bash 的 GNU `stat` 不支持脚本使用的 macOS `%Lp` 参数，模式断言失败。simple-mode 脚本的 `python3` 命中 WindowsApps alias（退出 49）；用现有 Python 直接运行同一正文后，静态断言通过，因没有 Docker CLI 在 Compose 阶段报 WinError 2。未修改脚本、安装 Docker 或启动部署。

本机日志统一为 `backend/.gocache/merge215-*.log`；不提交缓存、overlay、测试二进制、临时目录或运行日志。

## 交付状态

| 最终门禁 | 结果 |
| --- | --- |
| 暂存范围 | 320 paths：301 个上游路径与 19 个必需文档/图谱路径；没有冲突之外的业务修补。 |
| 第一父 / MERGE_HEAD | `cdc5178d26f5b9a9300604bf09f74e5f3e1eb18a` / `3a6fd1c9db07203ca308aaba69e502bc1f35b307`；本地 main 未改变。 |
| Git 状态 | 未解决冲突 0、残余源码冲突标记 0、未暂存变更 0、非忽略未跟踪文件 0；cached whitespace 检查通过。 |
| 最终 blob 核对 | 270 上游独有路径全部等于目标；本地独有业务路径无变化；29 features 文件全部等于第一父。 |
| 文档完整性 | 合并台账既有 239054 bytes 前缀保持不变，只追加本条与复核补充；本轮编辑 wiki 保持 LF。 |
| Wiki 图谱 | 35 nodes / 76 edges，60 wikilinks / 0 unresolved；source hash 匹配当前正文，`check-understand-status.cmd -AllowDirtyWiki` 为 READY。 |

本机核对清单：`backend/.gocache/merge215-final-audit.json`；三方增量核对与测试日志均为忽略的本机证据，不加入 commit。正式 unit、基线断言、上游测试波动、tidy 差异和未覆盖的 Docker/Redis/生产环境边界均明确保留，不能将本轮交付称为全绿。

本轮未 commit、未 push、未创建 PR、未合回 main、未部署。合并限定范围已准备供审核；测试未通过或未覆盖事项不等于发布批准。
