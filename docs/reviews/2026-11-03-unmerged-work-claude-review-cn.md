# 未合并分支与工作区：Claude 独立复审

| 项目 | 说明 |
| --- | --- |
| 日期 / 审核方 | 2026-11-03（北京时间）/ Claude |
| 状态 | **用户已裁定业务问题（第 8 节）；实施与删除仍待授权** |
| 输入 | [Codex 复核](2026-11-03-unmerged-work-codex-review-for-claude-cn.md)、[Grok 清单](2026-11-03-unmerged-work-triage-for-codex-cn.md)，沿用 U1–U7、W1–W2、D1–D8、L1–L5 编号 |
| 核对基线 | `main` = `github/main` = `7d0f760fc`，tree `07941ebfe`；未刷新远端引用 |
| 方法 | 只读：`git show`/`log`/`merge-tree`、读源码；U1 用 `go test -overlay` 在临时目录加并发用例，未改受控文件；U3 §3.3 单测在 main 上实际运行 |
| 未做 | 未访问生产配置、数据库或上游；未运行 Vitest 与 `-race`；未逐条复核 U2 的 17 个冲突文件（抽查 4 个代码文件及文档） |

## 1. 结论总表

| ID | 裁定 | 关键证据 | 建议处置 | 需用户决定 |
| --- | --- | --- | --- | --- |
| U1 | 部分赞同 | L3、L4 在 main 上用例可复现失败；L1、L2、L5 不可达或无实际影响 | 不合并分支；L4 单独移植；L3 视生产配置；其余放弃 | 生产是否开启额度异步落库（L3） |
| U2 | 赞同 | 两个父提交均为 main 祖先；`main..e558c091f` 只有该 merge 本身 | 删除本地与 GitHub 分支；需要时先另存旧计划文档 | 无 |
| U3 | 部分赞同，补充纠偏 | 旧补丁在 `user_repo.go`、`department_service.go` **无冲突自动合入**，两权限判断会静默误伤成员管理权限 | 不合并；按 §2 拆三类：现行合同缺陷直接修、业务规则待定、§3.3 单列 | D1、D2、D3、D5 四条规则 |
| U4 | 赞同 | 需求来源只有“对标 sub4api”，无故障/工单记录；main 无替代实现 | 无明确需求则归档（保留 GitHub 分支，删本地）；要做则基于 main 重做 | 还要不要时区伪装 |
| U5 | 赞同，补充 | 共享常量还会把 GPT-6 Sol/Luna 收缩到 272k | 删除本地、GitHub、内网 origin 三处分支 | 无（成本控制需求另行设计） |
| U6 | 赞同 | `git merge-tree` 退出码 0，结果 tree = `07941ebfe` | 删除 GitHub 分支 | 无 |
| U7 | 赞同，证据更强 | `dff279f26`（同父、同日）是其超集，3 个文件逐字节相同 | 删除 GitHub 分支 | 无 |
| W1 | 赞同 | 11 个源码文件约 270K 仅存在于未跟踪区；`.localdev/` 含连接配置与请求样本 | 先保护；仍用则迁入 main 复验 | 工具还用不用 |
| W2 | 赞同 | 工作区只有 2 个修改文件，内容即 main 的 `ProvidePluginManager`；无其他未跟踪文件 | 移除工作区并删除已合并本地分支 | 无 |

## 2. U3 部门审核提交

### 2.1 逐项裁定

| ID | 裁定 | 当前 main 事实 | 性质 | 建议 |
| --- | --- | --- | --- | --- |
| D1 | 部分赞同，风险比 Codex 描述更具体 | `adminSubscriptionResponse` 仅在受限管理密钥（`adminaccess.Principal`）时裁剪（`scoped_admin_subscription_response.go:18-20`）；Principal 只在管理员创建的密钥链注入（`admin_auth.go:257-263`）。JWT 登录的子管理员拿到完整 DTO（`mappers.go:12-36`、`:885-896`），含成员余额、冻结余额、累计充值、允许分组、通知邮箱、并发/RPM、角色，以及 `notes` 与 `assigned_by_user`（分配人自身的完整用户对象）。不含密码或令牌。页面实际只用邮箱/用户名与分组名称、平台、类型、倍率、额度（`SubscriptionsView.vue:247-296`） | 信息暴露；但设计 `:258` 写“非部门子管理员保持原合同”，未约定字段边界，属业务规则 | 推荐裁剪。基于当前 handler 重写精简 DTO，叠加在受限密钥裁剪之前（受限密钥属管理员，两者不重叠）；测试：子管理员响应无 `balance`/`notes`/`assigned_by_user`，管理员不变 |
| D2 | 赞同 | 单人/批量分配未比较操作者（`subscription_handler.go:196-208`、`:223-235`）；分配入口只在全站 `admin.subscriptions` 下（`admin_permission.go:68-82`），部门经理够不到 | 业务规则 | 待定。注意：只拦自分配是弱控制，该子管理员仍可给任意他人分配 |
| D3 | 部分赞同 | 仅在角色离开 sub_admin 时清 grants（`user_repo.go:373`），符合设计 `:136-137`；弹窗文案写“其他部门授权保留”（`zh/departments.ts:66`） | 现行合同下不是缺陷；是否“撤权即清范围”属业务规则 | 见 2.2；旧两权限 helper 不得移植 |
| D4 | 部分赞同，**旧修法不可取** | lib/pq 将 nil `[]int64` 绑定为 SQL NULL；`NOT (department_id = ANY(NULL))` 为 NULL，DELETE 不删任何行（`department_repo.go:707`）。前端总是发送数组（`DepartmentAccessDialog.vue:112`） | 接口合同模糊，非安全问题，仅直调 API 可达 | 旧补丁“nil 当空数组”会把“省略字段”变成“全部清空”，与 `manage_members` 省略即保留的语义相反。建议改为省略 `department_ids` 返回 400；`[]` 才清空。未实跑 SQL，按 PG 语义推断 |
| D5 | 部分赞同，**旧修法有回归** | 分配时未查 active（`subscription_service.go:513`）；分组列表已只列启用分组（`admin_group.go:52`，前端 `SubscriptionsView.vue:1257` 也过滤）；支付履约另走 `payment_fulfillment.go:643` | 业务规则（UI 暗示只能选启用分组，服务端未强制） | 旧检查位于“已存在订阅”查找之前，会连带阻断停用分组中既有订阅的续期/重复分配。若采纳，只拦新建 |
| D6 | 赞同 | `GetSubscriptionProgress` 把任意仓储错误改写为 `ErrSubscriptionNotFound`（`subscription_service.go:1257-1259`，本轮已读源码确认）；设计 `:454` 要求范围拒绝返回 403，同 ID 的 `GetByID` 已返回 403 | 现行合同缺陷（轻微，不泄露信息） | 直接 `return nil, err`，仓储已映射 not-found（`user_subscription_repo.go:91`）；测试：越范围返回 403 |
| D7 | 赞同 | handler 接受至 MaxInt64 的页码（`organization_usage_handler.go:105`），service 只校验 ≥1（`organization_usage_service.go:476-481`），offset = `(Page-1)*PageSize`（`organization_usage_repo.go:97,202`） | 现行合同缺陷（健壮性） | 超大页码导致负 OFFSET 报 500 或回绕取错页；范围过滤在 WHERE，不越权。加上界校验，测试 `page=math.MaxInt` 返回 400。`department_query_scope.go:116` 与 `department_repo.go:165` 同模式，未核对上界 |
| D8 | 部分赞同 | 导出只带页面 `scope_version`，不带 `as_of`（`OrganizationUsageView.vue:221-237,567`），`fetchAll` 用点击时刻（`organizationUsage.ts:262`）；两处重置额度在 403/409 后保留旧弹窗、快照、幂等键与列表（`SubscriptionsView.vue:1708-1770`） | 现行合同缺陷：设计 §7.4 要求导出复用页面快照，`:454` 要求 403 清除受保护数据 | 按当前页面重写；旧弹窗改动依赖 D3 且有冲突，不移植 |

### 2.2 授权清理规则（§3.2）

若用户选择“撤销全部部门权限即清空部门范围”：以**最终权限集合**判断，只要包含 `admin.organization_usage`、`admin.department_subscriptions`、`admin.department_members` 任一项即保留 grants；全站 `admin.subscriptions` 不使用 grants（`department_query_scope.go:209-210`）。由于 `ManageMembers == nil` 表示沿用原权限，判断必须放在仓储 `SetAccess` 读到旧权限之后，而不是 service 层；`user_repo.go:373` 之后用同一三权限判断。

最小测试：旧客户端省略 `manage_members`、报表与重置均为 false 的“仅成员管理”经理保留 grants；显式 `manage_members=false` 且另两项 false 时清空；用户编辑器保存仅成员管理经理时保留 grants。

若用户选择保留休眠 grants，则维持现状，只需把弹窗与 wiki 写明“重新授权会恢复原部门范围”。

### 2.3 §3.3 推理强度：main 当前单测失败，单列修复

`go test -tags unit -count=1 -run 'TestApplyDefaultOpenAIReasoningEffort$' ./internal/service/` 在基线上**失败**（子用例 `config none normalizes to empty -> disabled`，`openai_default_reasoning_effort_test.go:68`）。原因：`b8d52fad3`（2026-09-20）后归一化保留 `none`（`openai_gateway_request_body.go:2595`），于是会注入 `reasoning_effort:"none"`（`openai_gateway_chat_completions.go:47-67`）。配置校验接受 `none`（`config.go:3540`），默认值为空，生产只在显式配置 `none` 时受影响（未核对生产配置）。与部门主题无关，应作为独立小修复，不随 U3 带入。

### 2.4 拆分建议

| 批次 | 内容 | 前置 |
| --- | --- | --- |
| A：现行合同缺陷 | D6、D7、D8，及 D4 改为“省略返回 400” | 无需业务授权，需实施授权 |
| B：业务规则 | D1、D2、D3/§2.2、D5 | 用户裁定后实现 |
| C：独立修复 | §2.3 推理强度单测 | 实施授权 |
| 不移植 | 旧 NO-GO 审核文档（仅历史证据）、旧弹窗文案、旧 wiki/图谱整文件 | — |

## 3. U1 生命周期修复

关机顺序（main）：`main.go:155` 唯一一次 `defer app.Cleanup()`；`:185-189` 先 `Server.Shutdown`（5 秒）再执行 cleanup；cleanup（`wire.go:482-483`）先并行停服务，再依次关 Redis、Ent。

| ID | 裁定 | 可达性 / 影响 | 建议 |
| --- | --- | --- | --- |
| L1 | 部分赞同：缺口真实，风险不成立 | 二次调用 cleanup 会在 `PricingService.Stop`、`EmailQueueService.Stop` 触发 close of closed channel；但 cleanup 只有一个 defer 调用点，`log.Fatalf` 不执行 defer | 放弃 |
| L2 | 部分赞同：代码缺口真实，实际几乎不可达 | 监听关闭后无新管理请求；只有关停前已接受、超过 5 秒才到达 `IndexRangeAsync` 的请求会命中。`StartAutoIndex` 只有一个调用方（`service/wire.go:450`），幂等保护无对象。旧补丁也未覆盖 10-27 新增的 `startClassificationWithInterval`（`token_analysis_classification_job.go:59`），旧测试签名已过期 | 低优先；如做须在 main 上重写 |
| L3 | 赞同，影响有条件 | 仅在 `database.user_platform_quota_flusher_enabled=true` 时存在，默认 false（`config.go:2322`），仓库内无开启配置。写入为绝对值 `SET usage = v.usage`（`user_platform_quota_repo.go:439`），不会重复计数；可能是旧快照覆盖新快照，或在途 tick 在 Redis/Ent 关闭后写入失败、键离开脏集合。Redis 仍是准数据源，DB 副本落后至该用户下次请求 | 生产开启则移植“等待在途 tick”（tickWG + tick 内加锁），不移植 Start/Stop once 与 nil 保护。最小测试：tick 阻塞时 Stop 不得先返回（临时用例在 main 上失败） |
| L4 | 赞同，真实缺陷但当前影响低 | go-zero 触发时已从表中移除定时器，回调执行期间 `Cancel` 无效（`timing_wheel_service.go:70`），包装函数随后重新 `SetTimer`（`:58-62`）。现有 Cancel 调用方的回调都已自检停止状态，只是空转到进程退出；对共享服务的后续使用者是陷阱 | 原样移植并带测试（文件自分支基线未变；注意补丁使 `Schedule()` 同名时取消 recurring，属小幅行为变化） |
| L5 | 不赞同 | Prompt Metrics 默认开启；禁用时中间件不采集；pond v2 创建池不启动 goroutine，提交任务才启动 worker | 放弃 |

**报告外发现（未评估影响）**：`DeferredService`、`TimingWheelService`、`DashboardAggregationService` 不在 `provideCleanup` 中。`DeferredService.Stop` 的 `last_used` 最终批量写回（10 秒批）在关机时不会执行；时间轮回调可能在 Redis/Ent 关闭后继续触发。这比 L1–L5 更值得单独评估。

分支文档（治理设计、债务审计、验证报告）不在 main，属历史材料；在用户决定是否继续治理重构前保留 GitHub 分支即可，`technical-debt-board-cn.md` 已变化，不得整段回放。

## 4. U4 请求时区改写

- **是什么**：全局与账号双开关都开启时，把 Codex 隐藏环境块 `environment_context.timezone` 与 Web Search `user_location.timezone` 改成 `America/Los_Angeles`（或所选时区）后再发上游。
- **为什么做**：仓库内唯一来源是 `docs/reviews/2026-09-29-sub4api-request-locale-review-cn.md:7`“对标 MACOS-DO/sub4api 的账号请求时区”；无需求方、工单或故障记录。推测目的是降低共享账号暴露中国时区的风控信号——**此为推断，未验证**。
- **main 有无替代**：无。main 只读取 `environment_context`（归因、风险、来源判定），不改写时区。
- **风险**：模型以为用户本地时间偏差 15–16 小时；`current_date` 不改，日期与时区可能自相矛盾；Chat Completions、Messages、Alpha Search、input-tokens 端点不改写，覆盖不一致；会话中途切换会打断 prompt cache；归档与上游所见时区不同；未证明能降低封号，时区相对 IP 与出口代理是弱信号。
- **Codex 的缓存问题成立但窄**：分支 `setting_gateway_runtime.go:892-908` 三次 CAS 均失败且快照过期时返回 `loaded.result()`，可能是关闭前读到的开启值；只在保存成功但重读失败写入过期标记时才连续三次命中，影响单实例一批请求，不缓存。修法一行：重试耗尽返回“不改写”。补充：该改造替换的是所有网关设置共用的缓存（指纹、cch、dateline），不只时区。
- **重做成本**：`merge-tree` 6 处小冲突（`routes/admin.go` 相邻路由、WS v2 adapter 改名、两个账号弹窗测试、两页 wiki）；挂点仍在（`openai_gateway_forward.go:37`）；账号弹窗变动约 1500 行可自动合并但需 UI 复测。估计半天到一天，不含真实上游验证。

## 5. U2、U5、U6、U7、W1、W2

| ID | 补充核对 |
| --- | --- |
| U2 | 0.1.183 合并 `48b74c5d5` 同第一父、同 merge base 且包含 `49504adc`；抽查 `openai_gateway_passthrough.go:239-249`、`wire.go:293,297`、`config.go:1166-1171`、`CreateAccountModal.vue:4311-4314` 与 main 行为一致。分支独有：62 行旧计划 `docs/superpowers/plans/2026-08-19-sub2api-v0.1.178-sync.md`、已过期的台账条目与合并中途 wiki 文字 |
| U5 | 旧提交意图（其 wiki 行）：让遵守 catalog 的 Codex 客户端保持 272k 标准上下文与自动压缩口径。main 中该常量同时用于 GPT-5.6 与 GPT-6 Sol/Luna（`openai_codex_models_service.go:517-518`）；GPT-6.1 Sol 随后由专用 metadata 覆盖（`:537` 起）。若仍有成本控制需求，应按分组/模型配置，不复活全局常量。分支存在于本地、GitHub、内网 origin 三处，tip 相同 |
| U6 | 被删的 10 个中间文件与 `dump.rdb`、`.pyc` 在 main 均不存在，忽略规则已在 main |
| U7 | 两处 usage 修复在 `openai_gateway_messages.go:914-931`、`:971-989`，流式分支 `:1112-1129` 也已覆盖。旧价格断言 5/30/0.5 已被 `f54e9827a` 有意替换为 0.2/1.2/0.02（与价格目录一致，非官方价格核验）；`UseKeyModal` 无 `xunyou`，旧测试改动会失败 |
| W1 | `tools/request-diagnostics/` 11 个文件（最后修改 2026-10-23），另有设计文档、验收文档与 `backend.md`/`ops.md` 两段未提交增量。`.localdev/` 下有连接配置、请求样本 JSON 与截图，不属于源码交付；main 的 `.gitignore` 已忽略 `.localdev/` 与 `__pycache__/`。分支 ahead=0，删除工作区会永久丢失这些文件 |
| W2 | `git status --ignored` 只有 `wire.go`、`wire_gen.go` 两个修改；main `service/wire.go:196-199`、`cmd/server/wire_gen.go:220` 已有相同接线 |

## 6. 对 Codex / Grok 判断的纠偏

| 来源 | 原判断 | 复审结论 |
| --- | --- | --- |
| Codex D1 | “需核对各响应入口实际可见字段” | 已核对：JWT 子管理员可见成员余额、通知邮箱、备注与分配人完整对象，属实际暴露，建议优先请用户裁定 |
| Codex D4 | 列出旧处理“nil 改空数组” | 旧修法会反转省略语义，不应采用；建议 400 |
| Codex D5 | 需审查停用分组分配 | 补充：旧检查会误伤停用分组内既有订阅续期 |
| Codex §3.2 | 旧补丁不能直接合并 | 补充：`merge-tree` 显示这两个文件无冲突自动合入，回归不会被冲突提示拦下 |
| Codex §3.3 | 需核对现行合同 | main 上对应单测当前失败，是独立待修问题 |
| Codex U1 | 保留分支、L1–L5 逐项审查 | 只有 L4 无条件值得移植，L3 有条件；另发现 cleanup 未覆盖 3 个服务 |
| Codex U7 | 目标修复“已有” | 更强：`dff279f26` 是同日超集提交 |

## 7. 清理清单（待授权，执行前先 `git fetch --prune` 复核 tip）

| 对象 | 位置 | 前提 |
| --- | --- | --- |
| U2 | 本地 + `github` | 如需留档先另存旧计划文档 |
| U5 | 本地 + `github` + `origin`（内网） | 无 |
| U6、U7 | 仅 `github` | 无 |
| W2 | 工作区 `E:/allsite/xyai-plugin-wiring` + 本地分支 `hotfix/hy/10210_preserve_plugin_manager_wiring` | `git worktree remove` 前再看一次 `status --ignored` |
| U1、U3、U4 本地分支 | 本地 | GitHub 有同 tip 副本，删本地不丢提交；远端保留至对应修复移植完成或用户放弃 |
| W1 | 工作区 `C:/Users/Admin/.codex/worktrees/request-diagnostics/xyai` + 本地分支 `feature/hy/10219_request_diagnostics` | 已归档（第 8 节）；分支 ahead=0 |

## 8. 用户裁定（2026-11-03）

| 问题 | 用户答复 | 落地方式 |
| --- | --- | --- |
| U4 时区改写 | 先保留 | 本地与 GitHub 分支都保留，不合并、不重做 |
| W1 排障台 | 已不使用 | 未跟踪源码、设计、验收文档与 wiki 未提交 diff 已打包到 `E:/allsite/_archive/xyai-request-diagnostics-20261103.zip`（15 项，zip 校验通过）；未打包 `.localdev/`（连接配置、请求样本、截图）与 `__pycache__`。工作区待删除授权 |
| D1 子管理员可见字段 | 按建议隐藏 | JWT 子管理员订阅响应改为精简 DTO：保留邮箱/用户名与套餐、用量字段，不返回余额、充值、通知邮箱、备注、分配人；管理员不变 |
| D2 自分配 | 按建议允许 | 维持现状，不改代码 |
| D3 撤权后部门范围 | 保留 | 维持现状（休眠 grants 在重新授权时恢复）；只补弹窗与 wiki 说明，不移植旧清理逻辑 |
| D5 停用分组 | 按建议 | 服务端拒绝在停用分组**新建**订阅；已有订阅的续期与重复分配不受影响；支付履约路径不变 |
| L3 额度异步落库 | 用户认为未开启，采用默认配置 | 用户陈述，未核对生产配置；暂不移植 L3。将来开启 `database.user_platform_quota_flusher_enabled` 前须先补“等待在途 tick” |

据此，待实施清单为：U3 的 D1、D4（省略返回 400）、D5、D6、D7、D8 与 D3 文案；§2.3 推理强度单测修复；U1 的 L4。关机清理未覆盖 3 个服务（第 3 节）另行评估。

### 8.1 实施结果（2026-11-03）

| PR | 分支 | 内容 |
| --- | --- | --- |
| [#45](https://github.com/Oracle0703/xyai/pull/45) | `hotfix/hy/0000_reasoning_effort_none` | §2.3：默认 reasoning_effort 为 `none` 时不注入，修复 `-tags=unit` 失败用例 |
| [#46](https://github.com/Oracle0703/xyai/pull/46) | `hotfix/hy/0000_timing_wheel_cancel_recurring` | L4：移植 `fc1a0b22e` 的时间轮取消补丁及用例（main 实现上失败，修复后通过） |
| [#47](https://github.com/Oracle0703/xyai/pull/47) | `hotfix/hy/0000_department_review_followups` | U3：D1、D4、D5、D6、D7、D8 与 D3 文案；另修 `internal/handler/admin` 在 `-tags=unit` 下的编译失败 |

基线 `main` 的 `go test -tags unit ./...` 另有与本轮无关的失败：仓储 `TestDashboardAggregationRepositoryCleanupUsageLogsPartitioned…` 两项；`TestCommandCodeGatewayPassesThroughCatalogProtocols` 在不同运行中时过时不过。均未处理。三个分支两两 `merge-tree` 无冲突。仓储集成测试与浏览器验收未运行。
