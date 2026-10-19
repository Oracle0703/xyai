# Sub2API 0.2.17 + PR #8139 固定提交合并审核

## 合并边界

| 项目 | 结果 |
| --- | --- |
| 日期 / 新分支 | 2026-10-19 / `feature/hy/10217_merge_sub2api_217` |
| 本地 main / 第一父 | `ded494b6dbcd6841818e494d0f694d8a96dd2b90` |
| 上游仓库 / 分支 | `https://github.com/Wei-Shaw/sub2api` / `main` |
| 固定上游提交 | `b0a02a3304a0a78cdd003ddda1dd23ba02c03084` |
| 共同祖先 | `3a6fd1c9db07203ca308aaba69e502bc1f35b307` |
| 版本 / 增量 | 0.2.17；127 commits / 332 paths / +20917 / -1531 |
| 合并提交 | 尚未创建，HEAD 保持第一父，MERGE_HEAD 固定目标；等待用户审核 |
| 范围 | 0.2.17 部分仅解决冲突；另按用户明确指定叠加 PR #8139，保留本地独有功能，其余上游/基线 bug 不修复 |
| 追加 PR | [#8139](https://github.com/Wei-Shaw/sub2api/pull/8139)，head `e068c3d02ff8664c1142d5f7c51a05a82f75ad3d`；核验时 open / 未合并 |

初始 main 与工作区干净，基于本地 main 建分支，没有更新本地 main 指针。fetch 时上游 main 正好等于指定目标，后续始终固定 SHA。首次执行长时间无输出时中断重试，已备份并恢复本轮产生的部分文件及孤立锁，重新执行正常 merge；没有带入其它工作区改动。

## 冲突裁决

| 文件 | 处理与理由 |
| --- | --- |
| `backend/internal/handler/gemini_v1beta_handler.go` | 保留本地统一 `service.ParseGeminiModelActionPath`，不恢复被替代的私有 pathParseError；采用上游 googleErrorWithType/Ops 分类、显式缓存绑定和 Composite→Antigravity Gemini 行为。并发错误复用既有映射，保留本地缓存故障 503。 |
| `backend/internal/handler/gemini_v1beta_handler_test.go` | 同时保留本地缓存失败 503 用例和上游 Composite Gemini 路由用例，仅解决文件末尾添加冲突，不改断言。 |
| `backend/internal/server/routes/gateway.go` | models POST 保留本地 guard→requestIntercept→handler，新增 cachedContents 五个上游 CRUD 路由原样合入；API Key/group gate、归档中间件既有顺序不变。 |
| `backend/cmd/server/wire_gen.go` | 从已合并 provider / wire.go 生成，cleanup 同时保留本地 Token Analysis、Prompt Metrics、并发预设、GPT 额度及上游 GeminiCachedContentService；保留组织/部门、订阅自助重置、归档/拦截与插件账号目录接线。 |

## 0.2.17 阶段三方审查

| 范围 | 结论 |
| --- | --- |
| 287 个仅上游改动路径 | 初始合并 index blob 全部等于固定目标；不加入 bug 修补。 |
| 673 个仅本地改动路径 | 文档更新前全部等于第一父；收尾只允许 wiki/图谱、组件 README、合并记录发生额外变化。 |
| 45 个双方修改路径 | 4 个文本冲突按上述裁决；其余 41 个文件的 HEAD→index 增删行序列全部等于共同祖先→目标，不丢失任一方独有增量。 |
| 本地功能文档 | 29 个 `docs/features` tracked 文件全部保留，逐 blob 等于第一父。设计文档存在不代表尚未实现的方案已上线。 |

语义核对覆盖：配置默认值/校验，DTO 与 API Key 用户权限字段，Wire/Ent 本地组织和权限字段，网关路由顺序，Prompt Risk 热更新，Responses→Chat 参数过滤/reasoning 缓存，默认 effort、大请求保护、空流检查、compatible cache usage、图片零费用分类，以及前端账号 preset、角色权限类型与 locale。上游最小输出配置、created_at、web_search、结构化 input 安全回退、订阅长上下文口径与本地独有能力并存；没有识别到需要整项删除的本地功能。

## 新合同与覆盖边界

| 事项 | 合并结果 |
| --- | --- |
| Gemini 显式缓存 | `/v1beta/cachedContents` CRUD、本地资源绑定与持有账号固定、输入/存储计费、修改租约及恢复循环；原生 Gemini 非 Composite 分组，沿上游实现。 |
| 本地新入口覆盖 | cachedContents 不在本地 archive / Prompt Metrics 路径白名单，路由没有额外挂 requestIntercept；不宣称其已覆盖本地 Token Analysis/拦截/Prompt Metrics。创建调用既有 checkSecurityAudit。本轮不扩展适配。 |
| schema / migration | 追加 242_group_i18n、243/244/245_gemini_cached_contents；旧迁移保持不变，242 与既有平台/自助重置事件迁移、243 与本地 GPT 额度迁移按完整文件名区分，保留所有原名。 |
| 配置 | gateway.min_output_tokens 默认 128、最低 16；gemini_cached_content_max_ttl_seconds 默认 86400，非正数由服务回退；示例同步。 |
| 分组译文 | 用户 DTO 使用首选 Accept-Language，缺字段回退原值；管理端保留原字段与译文表，唯一性/搜索/日志仍用原名。 |
| 上游网关 / 支付 | Gemini 4 Argon、Vertex API Key/thinking、Haiku 5.5、GPT-5.5 订阅退休、GPT-6.1 Sol 订阅长上下文豁免、EasyPay 通知二次确认及退款 pending 处理等直接采用目标代码。 |
| 既有边界 | TypeSafe 入口及本地新平台筛选缺口沿上一轮报告保留；不扩大本轮冲突修复范围。 |

## 0.2.17 阶段验证（叠加 PR 前）

日志保存在忽略目录 `backend/.gocache/merge217-*`。Go 使用 repository-local cache、fresh GOTMPDIR、Git usr/bin PATH 和 `-p 1 -count=1`；default/unit/integration 串行执行，避免 Ent schema 测试共享 `.entc` 工作目录竞争。

| 命令 | 结果 |
| --- | --- |
| Ent / Wire 各两轮 generate | 通过，第二轮漂移 0；Ent 首轮与自动合并一致，Wire 从源图解决冲突。工具临时增加的 12 行 go.sum checksum 已恢复，业务依赖未改变。 |
| `go test -p 1 -count=1 -v ./...` | 退出 0，54 包通过，15 显式 skip。 |
| `go test -tags=unit -p 1 -count=1 -v ./...` | 退出 1，59 包通过，15 显式 skip；service 被第一父既有 ptrFloat 重复声明阻塞，无额外失败包。 |
| service unit overlay 补跑 | 8437 顶层 PASS / 含子用例 16827 PASS，4 skip；Command Code 目录用例与默认 effort none 用例失败，退出 1。 |
| overlay 聚焦复跑 | Command Code 通过；默认 effort none 仍失败。 |
| 指定上游原始源码 Command Code，count=3 | 2 pass / 1 fail，与合并结果相同位置 `model_protocol_catalog_test.go:374`；没有本地源码或 overlay。确认目标上游测试波动，未修复。 |
| `go test -tags=integration -p 1 -count=1 -v ./...` | 退出 0，54 包返回 ok、19 个显式 skip；repository 因 Docker 不可用整包跳过，不能视为完整 Docker/Redis 集成通过。另有隔离 PG 专项证据。 |
| 独立 PostgreSQL 18.1 专项 | 30 顶层用例 / 含子用例 76 PASS，0 skip；覆盖缓存仓储生命周期/修改租约、分组译文、分组删除竞争、本地部门授权与重置、订阅自助重置、GPT 额度及平台校验。启动应用全部迁移成功，测试库已停止。 |
| `go mod tidy -diff` | 退出 1，仅删除 go.sum 中 9 个旧版本的 18 行 checksum；go.sum 与第一父及指定上游一致，没有 go.mod 差异，按范围不清理。 |
| Go normal build | 通过。 |
| Go embed build | 通过，使用本轮 frontend build 产物。 |
| golangci-lint 2.14 `run --new-from-rev=HEAD --timeout=30m ./...` | 通过，0 issues。 |
| pnpm frozen install / lint:check / typecheck | 均通过。 |
| 前端完整 Vitest / build | 411 files / 3178 tests 全部通过；build 通过（i18n 3/3，1152 modules），保留 Browserslist、动态 import 与 chunk size 提示。 |

失败归属与验证边界：

- `payment_config_plans_validation_test.go:137` 与 `gpt_quota_display_test.go:287` 的重复 ptrFloat 已存在于第一父，两文件本轮未变。正式 unit 结果保留；补跑只在忽略目录通过 Go overlay 重命名一个 helper，不改断言或仓库源码，不将补跑当正式 unit 全绿。
- `openai_default_reasoning_effort_test.go:68` 的 none 断言、applyDefaultOpenAIReasoningEffort 与 normalizeOpenAIReasoningEffort 函数都与第一父完全相同。聚焦复跑仍失败，是既有本地合同失配，本轮不修。
- Command Code 原始上游导出使用 `git archive b0a02a3304a0a78cdd003ddda1dd23ba02c03084 backend`，只有目标源码。当前分支聚焦复跑通过、原始上游三次复跑一败，保留波动证据，不修改生产实现或测试。
- PostgreSQL 使用已有 embedded-postgres 二进制、新建随机目录与 loopback 随机端口，不读取业务数据库；pg_ctl fast stop 成功。Redis 由相关用例的 miniredis 覆盖，不代表真实 Redis/Docker 集成或生产验收。
- 显式 skip 与包退出码分别记录；未调用真实模型/支付上游，也未部署或运行生产浏览器验收。

## PR #8139 审查与应用

| 项目 | 结论 |
| --- | --- |
| 来源 | `Wei-Shaw/sub2api#8139`；作者仓库 `zenggang/my_sub2api`，分支 `contribute/openai-preoutput-metadata-20261016` |
| 精确 head / 父提交 | `e068c3d02ff8664c1142d5f7c51a05a82f75ad3d` / `455d317d6a87d8519a52c8769a0727824c403171` |
| 祖先关系 | PR 只有 1 个提交，父提交已是指定 0.2.17 的祖先；不拉入后续 main 或其他 PR |
| 应用方式 / 冲突 | 在已有未提交 merge 上 `git apply --3way --index` 应用精确提交 diff；无冲突，3 文件 +15/-1 |
| 内容与拓扑 | PR 内容已纳入 index，未创建 commit；MERGE_HEAD 仍只指向 `b0a02a3...`，PR head 未加入合并父链，来源由本报告及追加台账记录；不是宣称 GitHub PR 已合并 |
| 生产改动 | `backend/internal/service/openai_gateway_passthrough.go` 只扩充共享 metadata 分类，保留本地及 0.2.17 其他变更；逐文件增删行序列与 PR 完全相同 |
| 测试改动 | `openai_capacity_shed_test.go`、`openai_ws_http_bridge_test.go` 两文件 blob 与 PR head 完全相同 |
| 行为 | queued / metadata 后 overload 保留 native SSE、passthrough、WS→HTTP 的原重试能力，元数据不提前触发语义 TTFT 或解除 native 首输出超时 |
| 保留边界 | 未知事件仍保守，真实文本/tool arguments/encrypted reasoning 输出后不放宽重放；不修改重试预算、调度、其他 bug、schema 或前端 |

## PR #8139 叠加后验证

日志前缀 `backend/.gocache/merge217-pr8139-*`，保留原 0.2.17 日志，不覆盖前轮证据。

| 命令 / 范围 | 结果 |
| --- | --- |
| service 流式/容量恢复/首输出/WS HTTP 专项，`-count=3` | 130 个顶层用例 × 3 轮 = 390 顶层 PASS / 600 含子用例 PASS，0 fail / 0 skip；包含 PR 增补的 native、passthrough、bridge 场景和输出后禁止重放 |
| default：`go test -p 1 -count=1 -v ./internal/service ./internal/handler ./internal/server/routes` | 5061 顶层 / 10108 含子用例 PASS、5 skip；handler/routes 通过，service 的 OAuth 刷新超时用例失败，退出 1 |
| OAuth 刷新超时聚焦 `-count=10` | 10/10 通过，0 skip；保留此前失败，不用重跑抹去首次结果 |
| service unit overlay，全包 `-count=1` | 8438 顶层 / 16828 含子用例 PASS、4 skip；仅默认 effort none 既有断言失败，退出 1；Command Code 本轮通过 |
| normal / embed build | 均通过；embed 使用前轮前端构建产物，前端源码与产物未被 PR 改动 |
| golangci-lint 2.14，`--new-from-rev=HEAD` | 通过，0 issues |
| 未重复的验证 | 前端、PG/Docker integration、Ent/Wire、tidy 沿用前轮证据与限制：PR 不涉及这些路径或依赖；不是重新完成生产/真实上游验收 |

新增观察：`token_refresh_pool_health_test.go:949` 的 `TestTokenRefreshService_LateSuccessPastAttemptDeadlineIsRejected` 在受影响包全量默认测试中期望 timeout error 却得到 nil；其 10ms deadline / 30ms 延迟属于时序敏感场景。该测试文件及 `token_refresh_service.go` 的 blob 在第一父、指定 b0a 上游与当前 index 三方完全一致，PR 改动不涉及刷新调用链；同轮 unit 包通过，隔离聚焦 10 次通过，记录为未改动代码中的时序波动，不修复实现或放宽断言。

正式 unit 的重复 ptrFloat 编译限制继续沿用前轮证据；本轮 overlay 同样只在忽略目录重命名重复 helper，不更改测试断言/仓库源码。默认 effort none 的旧失败及所有先前集成/skip 边界仍然保留；本交付不宣称测试全绿或已获上线批准。

## 交付状态

| 门禁 | 结果 |
| --- | --- |
| 暂存范围 | 348 paths：332 个 0.2.17 路径 + PR 新增涉及的 2 个测试路径 + 14 个文档/图谱路径；额外生产/测试增量仅为指定 PR。 |
| HEAD / main / MERGE_HEAD | HEAD 和本地 main 均保持 `ded494b6dbcd6841818e494d0f694d8a96dd2b90`；MERGE_HEAD 为 `b0a02a3304a0a78cdd003ddda1dd23ba02c03084`。 |
| Git 检查 | 未解决冲突 0、残余源码冲突标记 0、未暂存 0、非忽略未跟踪 0；cached whitespace 通过。 |
| 本地功能 / 上游代码 | 29 份 features 文件逐 blob 等于第一父；287 个仅上游文件逐 blob 等于目标，41 个自动合并双方修改文件在叠加 PR 前增删行序列匹配 0.2.17；叠加后仅 passthrough 生产文件新增指定 PR 分类，另 40 个保持；本地独有源码未被文档维护改写。 |
| 合并台账 | 原内容保留为完整字节前缀，仅追加本轮记录。 |
| Wiki 图谱 | 35 nodes / 76 edges、60 wikilinks / 0 unresolved；source hash 对应本轮 wiki，`check-understand-status.cmd -AllowDirtyWiki` 为 READY。代码图谱未重建，该结果不是代码图谱或生产验收结论。 |

未 commit、未 push、未创建 PR、未合回 main、未部署。已完成 0.2.17 与指定 PR #8139 的内容合入、验证，停在提交前供用户审核；正式 unit、既有断言/时序波动、tidy 和集成覆盖边界明确保留，不宣称全绿。

0.2.17 阶段本机核对证据：`backend/.gocache/merge217-final-audit.json`、`merge217-blob-audit.json`、`merge217-test-summary.json` 与各命令日志（均忽略，不入库）。

PR 叠加后证据：`backend/.gocache/merge217-pr8139-final-audit.json`、`merge217-pr8139-test-summary.json`、`merge217-pr8139-refresh-baseline.json` 与同前缀日志。叠加前 index tree、原始 PR diff、merge 元数据保存在忽略目录 `backend/.gocache/pr8139-before/`。
