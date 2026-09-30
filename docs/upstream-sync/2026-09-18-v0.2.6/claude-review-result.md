# Sub2API 0.2.6 Claude 独立复审结果与风险登记

日期：2026-09-19。来源为用户在当前会话提供的 Claude 独立复审结论；下文区分 Claude 报告的测试结果与 GPT 本轮补做的源码/索引核对。本轮没有重新执行业务测试，也没有修改源码、测试或运行配置。

## 结论与授权状态

**Claude 结论：GO；未发现本轮合并引入的 finding。** 适用范围是固定 SHA、保留本地独有功能、只解决冲突。候选可交由用户决定是否提交；GO 不构成 commit 授权，不代表生产上线验收通过。

| 基线 | 值 |
| --- | --- |
| 工作分支 | `feature/hy/10206_merge_sub2api_206` |
| HEAD / main | `5ec57e4fc51a9052e8812f4cb925565c984856cc` |
| MERGE_HEAD | `8b69738d782ccaa7fd26511e1cca26ba8d1b58db` |
| merge base | `881f3202694c6bc932446931a30c27d9675178b9` |
| 源码范围索引 SHA256 | `a5306abf9ebbb14838b08102d59dc0d9fd18eafc8ca3c368f56fa76231811a34` |
| 指纹口径 | 对 `git ls-files --stage -z -- .gitignore backend frontend deploy` 的原始字节计算 SHA256 |
| Claude 审查时 | 149 个暂存路径；报告称复审前后无 tracked 文件变动，源码指纹一致 |
| GPT 本轮接收时 | Git 基线与上述源码指纹现场核对一致；工作区另有并行的部门报表设计文档，未纳入本轮暂存 |

## 四个文本冲突：Claude 全部通过

| 文件 | 复审证据摘要 |
| --- | --- |
| `.gitignore` | 本地规则保留并加入上游唯一新增例外；Claude 用 `git check-ignore` 核对 features/superpowers/playbook/Antigravity 文档可跟踪，缓存、report、fixtures 仍忽略 |
| `backend/go.mod` | 与上游差异仅为 x/sys、x/text 的 direct 属性，实际本地 import 存在；go.sum 与固定上游一致，无 Wire 工具 checksum 副作用 |
| `backend/internal/service/setting_service.go` | 本地回调与上游 ticket caches 为并集；沿 `service/wire.go` 的注册、`setting_update.go` 的消费及归档运行态消费者确认接线 |
| `backend/cmd/server/wire_gen.go` | 哈希与原记录一致；Claude 同时核对 cleanup 参数中的 Token Analysis、并发预设、Prompt Metrics、quota flusher 与 ticket harvester，无漏接或重复 |

Claude 报告对 31 个双方修改路径做行级并集核对：28 个零误差，另外 3 个为 Wire 单行实参串、go.mod direct/indirect 块移动、SettingService 的 gofmt 对齐，均逐行确认。GPT 本轮没有重做该行级校验，继续保留其证据来源。

## 文件集合与功能证据

| 范围 | Claude 报告及本轮现场核对 |
| --- | --- |
| 101 个仅上游路径 | 候选 index blob 与固定上游一致，包含 49 个测试文件 |
| 495 个仅本地路径 | 479 个存在且 blob 与第一父一致；`backend/internal/server/middleware/admin_only.go` 在第一父和候选均保持删除；15 个差异为合并台账、组件 README、wiki 和图谱 |
| 349 个仅本地非 Markdown 且不在 llm-wiki 的路径 | 保持第一父状态，沿用原审核报告的集合口径 |
| 本地路由/中间件 | 上游本轮未改 `backend/internal/server/routes/gateway.go`、`routes/admin.go`、`router.go` 及全部 middleware；归档、拦截、子管理员准入链继续保留 |
| ticket 模型门控 | Claude 核对调度与注入模型解析同源，compact 兜底顺序一致；开关读取实现同源，影子账号豁免成立 |
| 账号票据更新 | Claude 核对管理服务与 repository 双层 MergeOpenAICodexTicketExtra；repository 在 FOR NO KEY UPDATE 下读取当前 extra |
| 前端设置 | Claude 核对显式提交载荷只写 ticket enabled/proxy URL，不误提交只读 configured；代理掩码回环由上游实现和对应测试覆盖 |

### 接收结论时纠正的两处表述

1. **不能说两个 scheduler 文件“上游零改动”。** `backend/internal/service/openai_account_scheduler.go` 上游 diff 为 `+2/-2`，`backend/internal/service/openai_gateway_scheduling.go` 为 `+5/-5`，共 7 处将 `requireCompact` 传给 runtime-block 判定。两文件的当前 index blob 均与固定上游一致；这是上游已有变更，没有本轮额外改写。路由/middleware 零改动的结论不受此更正影响。
2. **F2 的“关闭时无 DB”不成立。** 关闭时确实不执行 ListByPlatform 账号查询或打票探测，但判断开关本身调用 `SettingService.GetOpenAICodexTicketEnabled`。其缓存 TTL 为 5 秒，过期会经 `settingRepo.GetValue` 读 settings；默认周期完成后等待 6 秒，因此关闭且缓存到期时仍可能产生 settings 读取。这里只做静态链路核对，没有测量实际运行期查询频率。

以上是复审说明的口径修正，不是新增合并回归，也没有据此修改源码。

## 三条风险：登记，当前合并不修复

| ID / 等级 / 归属 | 已知行为与证据 | 影响边界及后续验证 |
| --- | --- | --- |
| F1 / P2 / 上游固有 | `openai_codex_ticket.go#applyOpenAICodexTicket` 在 enabled、目标账号/模型、无有效票且 fail_closed 时返回 `ErrOpenAICodexTicketUnavailable`；普通 Responses 的 `openai_gateway_forward.go` 构造请求阶段直接返回该 error，未包装为 `UpstreamFailoverError`；当前未找到该 sentinel 的生产分类处理 | 调度已经过滤无票账号，但选中后票据失效等情况仍可能在构造阶段中止，缺少此错误的专门换号路径。默认关闭时不触发该错误分支。未来开启前单独评估；Claude 提议 fail_closed=false 灰度，这会放开缺票限制，尚未执行，也不等于当前配置建议已获采纳 |
| F2 / P3 / 上游固有 | `openai_gateway_service.go` 无条件启动 harvester；loop 每周期完成后等待默认 6 秒。关闭时跳过账号查询，但过期的开关缓存仍可能读 settings；开启时每周期 `ListByPlatform(PlatformOpenAI)` 全量取号，再按账号/模型处理 | 生命周期幂等守卫和 Stop 等待退出已由 Claude 核对；规模开销尚未实测。不能写成关闭时完全无数据库访问，本轮不改启动方式、轮询周期或查询实现 |
| F3 / 未确认 | `custom_group_usage_rollup_repo.go#readGroupUsageRollupSnapshot` 将 retained_from 的日期换算从 SQL `AT TIME ZONE` 移到 Go `GroupUsageDate()`；closed_before 的 ISO 文本比较由 Claude 判断等价 | 因 Docker 不可用、repository integration 整包跳过，retained_from 的真实 PostgreSQL/服务时区边界尚未验证。保留待确认状态，不能称为缺陷或已通过；本地组织用量链路未被替代 |

F1/F2 的 ticket 实现与固定上游一致；两个 scheduler 文件、开关读取实现亦已现场逐 blob 核对。本轮只记录上述边界，不扩大冲突修复范围。

## 已知失败与风险等级

| 项目 | Claude 复核结果 |
| --- | --- |
| `/auth/me` golden | 退出 1；第一父 DTO 已有无 omitempty 的 AdminPermissions，而第一父 golden 未包含 admin_permissions，确认第一父既有；不删字段、不改 golden |
| Ollama CAS | 精确 count=3，3/3 失败；ratelimit 主实现、429 实现和测试四方 blob 一致，确认非本轮引入 |
| PgDumper | 在 PATH 含动态定位的 sh.exe 的 Git Bash 中退出 0，确认环境限制；不改测试或生产实现 |
| SubscriptionsView.userUsageLink | 6/6 失败；页面与该 spec 的 index blob 均等于第一父，确认既有装配问题 |

Ollama 的 flaky 标签仅有 0.2.5 台账记录的“1 次通过 / 2 次失败”提供历史支撑。09-18 合并验证和 09-19 Claude 复审中的精确三次重跑均没有绿灯；不得以 flaky 标签降低风险，也不得因未来某次转绿就宣称问题已修复。

## Claude 报告实际执行的验证

以下结果来自用户提供的复审结论，本轮 GPT 未重跑，也未新增这些执行记录的原始日志。09-18 的原始日志仍在 `.git/codex-merge-026/`。

| 命令范围 | Claude 报告结果 |
| --- | --- |
| service、handler、handler/admin、handler/dto、server/routes 聚焦 Go；过滤涵盖 ticket/turn/affinity/Gemini/DeepSeek/role/archive/intercept/risk/cache/权限 | 5/5 packages 通过，退出 0 |
| `go build -p 1 ./...` | 退出 0 |
| server：`TestAPIContracts/GET_/api/v1/auth/me` | 退出 1，既有 golden 差异 |
| service：Ollama CAS，count=3 | 退出 1，3 次失败 |
| repository：`^TestPgDumper` | 退出 0 |
| SettingsView + AccountUsageCell + SubscriptionsView.bulkActions | 3 files / 92 tests 通过，退出 0 |
| SubscriptionsView.userUsageLink | 6 个失败，既有 Pinia/auth fixture 问题 |

Claude 未重跑 Go default/unit/integration 全量、完整 Vitest、golangci-lint、go mod tidy -diff、前端 lint/typecheck/build、Wire 生成。上述沿用 09-18 结果；源码指纹一致支持复用，但既有缺口仍存在：17 个显式 skip，以及 repository 的 Docker 整包跳过。没有运行期服务/真实上游验收，也未逐文件语义阅读全部 495 个仅本地路径。

## 本轮文档落地与工作区边界

- 更新原审核报告的最终态计数，追加本条复审记录到合并台账；将 F1/F2 和 F3 的未验证边界补入相应 wiki。历史台账不改写。
- 源码、测试、配置、依赖、Wire 生成物保持不变；未 commit、未 push、未部署。
- 接收结论时已有 `llm-wiki/wiki/README.md` 的未暂存部门设计索引，以及未跟踪的 `llm-wiki/wiki/department-report-design.md`、`docs/features/organization-department-usage-design-cn.md`。这些是独立工作，按原状保留，不纳入本轮文档暂存或合并候选图谱。
- Wiki 图谱按暂存候选单独刷新。当前工作树包含上述独立设计，工作树级图谱状态与暂存候选不必相同；不能把上一轮 READY 无条件描述成当前整个工作树 READY。

本轮文档校验：`git diff --cached --check` 通过；暂存 wiki 源 hash 与图谱 meta 一致（`66fb56e3a1e2...`），33 nodes / 67 edges 且边引用完整。当前 150 个暂存路径、0 unmerged；另外 1 个既有未暂存 README 和 2 个既有未跟踪设计文档原样保留。对整个工作树执行 `tools/check-understand-status.cmd -AllowDirtyWiki` 返回 PARTIAL，唯一差异为上述并行设计导致工作树 wikiSourceHash 与暂存候选不同；没有把它计为整个工作树 READY。

仍等待用户明确决定是否 commit；复审 GO 与本轮文档登记均不代替该决定。
