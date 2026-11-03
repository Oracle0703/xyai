# 未合并分支与工作区：Codex 复核及 Claude 交叉评审问题

| 项目 | 说明 |
| --- | --- |
| 日期 / 审核方 | 2026-11-03（北京时间）/ Codex |
| 状态 | **待 Claude 独立复审、待用户裁定；本文不是合并、删除或实施授权** |
| 用户目标 | 判断遗留分支和未提交内容是否仍有价值，避免重复合并、能力倒退和误删独有修复 |
| 前置材料 | [Grok 未合并工作清单](2026-11-03-unmerged-work-triage-for-codex-cn.md)，沿用 U1–U7、W1–W2 编号 |
| 核对基线 | 本地 `main` 与本机远端跟踪引用 `github/main` 均为 `7d0f760fc4a3fa8e52d4e99fe36787423f7e1cdf` |
| 基线 tree | `07941ebfe5ec6c38cfe927ca6dcdaefc6b279e67` |
| 用户已明确 | 先扫描和评审，拿不准的再问用户；确定后才操作。随后授权把 Codex 审核问题写成文档，由用户交给 Claude |
| 用户尚未确定 | 哪些分支删除、哪些修复实施、时区改写是否保留、排障工具是否入库、自分配订阅的业务规则 |

## 1. 复审目标与证据边界

请 Claude 独立验证下面的判断，尤其寻找反证：当前是否已有不同名称的等价实现；旧补丁是否会破坏后来新增的权限或模型能力；所谓“缺陷”是否其实依赖尚未确认的业务规则。无需为了同意 Codex 而合并内容，也不要因提交很旧就认定可以删除。

| 本轮已做 | 本轮未做 |
| --- | --- |
| 核对本地分支、远端跟踪引用、工作区及未提交状态 | 未刷新远端引用，未独立复核 GitHub 当前 PR 状态 |
| 读取每条独有提交，核对当前源码与相关 wiki | 未访问生产、真实账号、数据库或上游模型服务 |
| 对 U1–U7 执行 `git merge-tree --write-tree --name-only`，核对冲突和结果树 | 未实际 merge、cherry-pick、checkout、commit、push 或删除分支/工作区 |
| 用 `git show --remerge-diff` 检查 U2 当时的冲突解决 | 未运行新的业务测试、浏览器验收或并发复现 |
| 核对 W1 文件存在性、设计与历史验收，核对 W2 未提交 diff | W1 历史测试结果不代表工具今天仍通过真实环境验收 |

这里的“已覆盖”分为两种：U6 有结果树完全一致的证据；U2、U7、W2 是对目标行为的源码等价判断，不等于它们整条分支与 main 的树相同。下列行号均对应上述基线，复审时以函数与提交定位为准。

## 2. 候选处置总表

这些是 Codex 建议，**没有一项已获得用户最终确认**。

| ID | 分支 / 工作区 | 尖端 | 建议 | 判断依据 / 保留条件 |
| --- | --- | --- | --- | --- |
| U1 | `feature/hy/10177_fork_governance_validation` | `fc1a0b22e` | 保留，拆分仍成立的生命周期修复 | 治理文档与实际代码混在一个提交；部分停止/重复启停保护未进入 main，不能整包当成过期文档删除 |
| U2 | `feature/hy/10178_merge_sub2api_178` | `e558c091f` | 不合并，列清理候选 | 0.1.178 已由后续同步包含；核对的冲突解决行为已覆盖。独有旧计划是否留档需与业务代码分开决定 |
| U3 | `feature/hy/10207_department_usage` | `ce6ce97c9` | 保留，优先拆分审核修复 | 有尚未落地的权限/响应处理；旧补丁又不认识新增成员管理权限，直接合入可能产生回归 |
| U4 | `feature/hy/10212_sub4api_request_locale` | `82dd34428` | 暂不合并，先确认需求 | 时区改写未进入 main；分支自身记录的缓存竞争降级问题未闭环，真实上游效果未经本轮验证 |
| U5 | `hotfix/hy/0000_limit_codex_context_272k` | `493252341` | 不合并，列清理候选 | 会把现有目录能力上限从 872k 收缩到 272k，且常量复用范围较当时扩大 |
| U6 | `github/chore/remove-merge-artifacts-20260930` | `b02bb7046` | 不合并，列清理候选 | 预演结果树与 main 完全相同；删除结果及忽略规则均已存在 |
| U7 | `github/codex/server-runtime-fixes-review` | `ad8d5fac1` | 不合并，列清理候选 | 两处目标 usage 路径修复已有；附带价格及配置样例断言不符合当前实现 |
| W1 | `feature/hy/10219_request_diagnostics` 的旧工作区 | `534b1b404` | 保护未提交文件，确认是否迁入主线 | 分支提交已在 main，工具源码等文件仍独有且未提交，不能据分支已合入而清空工作区 |
| W2 | `hotfix/hy/10210_preserve_plugin_manager_wiring` 的旧工作区 | `d120d499e` | 冗余补丁及工作区列清理候选 | 当前 provider、ProviderSet、生成代码均已保留插件账号目录接线 |

清理候选共 5 项：U2、U5、U6、U7、W2；需保留并拆审 2 项：U1、U3；主要依赖用户用途判断 2 项：U4、W1。这里的计数是条目数，不是待删除分支副本数。

## 3. U3：部门审核提交，优先复审

独有提交为 `ce6ce97c96394f399ab9ed6deae135b517d0f5f6`。父提交 `273b76c35` 已在 main，部门主体已合入；不能因此推断后续审核修复也已合入。

### 3.1 已确认的代码差异与待裁定行为

| 编号 | 当前源码事实 | 旧提交的处理 | Codex 判断 / Claude 需要回答 |
| --- | --- | --- | --- |
| D1 | `subscription_handler.go` 使用 `UserSubscriptionFromServiceAdmin`，经 `adminSubscriptionResponse` 返回；该 wrapper 在 `adminaccess.FromContext == nil` 时原样返回 | 为子管理员单独建立精简 DTO，隐藏完整用户/分组对象及分配元数据 | 不是因为当前出现“裁剪函数”就已解决。JWT 子管理员与受限管理密钥是不同身份链；请核对各响应入口实际可见字段及预期权限 |
| D2 | 单人/批量分配入口未比较目标用户与登录操作者；`admin.subscriptions` 白名单包含这两个入口 | 子管理员给自己分配时拒绝，批量含自己时整体拒绝 | **业务规则待用户确定**。代码差异存在，但不能未经用户确认就把“允许自分配”定性成必须改变的规则 |
| D3 | `user_repo.go` 在角色离开 sub_admin 时清 grants；同角色撤掉部门权限后，没有旧提交所加的对应清理分支 | 无部门权限时清理旧 grants，防止后续重新授权复活旧范围 | 有保留价值，但必须按当前三种部门权限判断；见 3.2 的回归风险 |
| D4 | `DepartmentService.SetAccess` 未把 nil 部门列表归一成空列表，也未按最终权限集合清除 grants；仓储 DELETE 使用 `department_id=ANY($2)` | nil 改空数组；两项旧权限均关闭时清空列表 | 需要分别核对“省略字段”的接口约定、SQL NULL 与空数组行为、当前第三项权限。不能直接套用旧条件 |
| D5 | `assignSubscriptionWithReuse` 根据 ID 读取分组并校验订阅类型，未显式检查 active | 管理端分配拒绝停用分组；支付/兑换履约沿另一条路径保持原行为 | 需审查直接传入停用分组 ID 的行为与业务政策；不能只依赖 UI 筛选，也不能误伤既有支付履约 |
| D6 | `GetSubscriptionProgress` 将仓储任意错误转成 `ErrSubscriptionNotFound` | 保留原错误 | 错误语义修复候选，应核对范围拒绝、基础设施错误是否被错误改写 |
| D7 | 组织用量分页校验缺少旧补丁的乘法溢出上界检查 | `page-1 > math.MaxInt/pageSize` 时拒绝 | 仍有代码增量；复审应沿 handler→service→repository 确认输入上界与实际影响 |
| D8 | 导出从 `currentQuery()` 建立请求，旧提交额外要求已有 `snapshotAsOf` 并显式传入页面快照时间；订阅重置错误处理也有独有增量 | 固定页面导出快照；遇到权限/范围失败清旧确认状态并重载 | 后续页面已发生改动，需要独立确认今天的导出合同及错误恢复，不能只按旧 diff 移植 |

关键证据入口：

- `backend/internal/handler/admin/subscription_handler.go:93`：List、GetByID、Assign、BulkAssign、ResetQuota、ListByUser/ListByGroup。
- `backend/internal/handler/admin/scoped_admin_subscription_response.go:18`：受限管理密钥的响应裁剪。
- `backend/internal/server/middleware/admin_auth.go:234`：JWT 设置 DepartmentActor 和子管理员账号操作限制；`:261`：受限管理密钥才注入 `adminaccess.Principal`。
- `backend/internal/handler/dto/mappers.go:12`、`:885`、`:898`：用户 DTO 含余额、权限、允许分组、通知设置等；订阅 DTO 含分配备注与分配人等元数据。这里并非指密码或令牌被返回。
- `backend/internal/service/admin_permission.go:55`、`:67`：部门订阅权限与全站订阅权限的路由差异；只有后者提供分配入口。
- `backend/internal/repository/user_repo.go:372`、`backend/internal/service/department_service.go:492`、`backend/internal/repository/department_repo.go:607`：授权更新链。
- `backend/internal/service/subscription_service.go:507`、`:1256`：分配时校验、进度错误映射。
- `backend/internal/service/organization_usage_service.go:479`、`frontend/src/views/admin/OrganizationUsageView.vue:557`：分页及导出。

补充纠偏：分组选择列表通过 `adminService.GetAllGroups` → `groupRepo.ListActive` 已过滤停用分组，见 `backend/internal/service/admin_group.go:52`。因此旧提交在 `SubscriptionAssignmentGroups` 额外加 `IsActive()` 属于重复防御，不能宣称当前选择列表会列出停用分组。D5 指的是**提交分配时的服务端校验**。

### 3.2 不能直接合并：旧授权清理会误伤成员管理权限

当前已有第三项部门权限 `admin.department_members`，可以单独授予，不必同时授予报表或订阅额度重置。当前 `DepartmentAccessInput` 包含 `ManageMembers`，仓储会保留或设置它。

旧提交的两个判断仍只认识报表和部门订阅：

```go
// ce6ce97c9 的新增逻辑，不能直接移植。
if !in.Report && !in.ResetQuota {
    in.DepartmentIDs = []int64{}
}

// 同一提交的 hasDepartmentScopedPermission 也只认两项旧权限。
```

当用户仅保留成员管理权限时，原样应用前一条件会清掉合法部门范围；后一 helper 在通用用户权限更新中也有同类风险。旧弹窗文案“两项都取消就清全部授权”同样已经不适用。

请 Claude 明确裁定：是按最终三项权限统一清 grants，还是由用户定义保留休眠 grants 的规则；`ManageMembers == nil` 的兼容保留语义也要考虑。不能只在前端加第三个勾选项，后端仍按两个权限判断。

### 3.3 需要从 U3 拆开的其他内容

| 内容 | 处置建议 |
| --- | --- |
| 当时 NO-GO / CI 为红的审核文档 | 仅作为 2026-09-29 的历史证据，不写成今天的上线结论 |
| GPT 额度测试、零散 i18n 修正 | 按当前实现逐项判断，不随权限修复一并自动接受 |
| `applyDefaultOpenAIReasoningEffort` 对站点默认 `none` 的特殊处理 | 与部门主题无关；需核对现行网关参数合同，不能以“顺手修测试”为由带入 |
| 旧 wiki、图谱及测试装配 | 保留当前结构，只补经复审采纳的行为与测试，不覆盖旧文件 |

## 4. U1：治理文档与生命周期修复必须分开

独有提交为 `fc1a0b22e26ded90c6a50f8e875029c1d3741dd6`，提交标题虽然是 docs，实际修改了 Go 代码。设计候选文档写着当时未授权业务实现，而验证报告又记录了行为失败和最小修复；这些历史文字都不能代替今天的实施授权。

| 编号 | 当前 main 的事实 | 旧提交增加的行为 | 复审重点 |
| --- | --- | --- | --- |
| L1 | `provideCleanup` 已先等待服务停止，再关闭 Redis/Ent；但顶层 cleanup 没有旧补丁的 `sync.Once` | cleanup 重复调用保护 | “停机先后顺序”已有实现，不应算全新缺口；确认重复调用是否真实可达、是否值得独立修复 |
| L2 | Token Analysis 手动异步任务直接 `backgroundWG.Add(1)`；自动启动无 StartOnce；Stop 取消并 Wait，但未用共同门闩禁止之后的新任务 | 停止后拒绝新手动任务、自动启动幂等、同步 Add/Wait 的入口 | 代码增量成立；需区分服务 API 合同与实际 HTTP 停机顺序下的可达性，尚无本轮并发复现 |
| L3 | quota flusher 的 Stop 标记 stopped、Cancel 后直接最终 flush，没有等待已进入 tick 的 flush | 等待在途 tick，再最终 flush，并增加 Start/Stop 幂等及 nil 保护 | 确认并发写回可能产生的后果、时间预算及现有调度保证，不能直接声称已发生计费损失 |
| L4 | TimingWheel 的 recurring callback 在 `fn()` 后重新 SetTimer；Cancel 只 RemoveTimer | 用取消状态阻止已运行的 callback 再注册 | 检查 Cancel 与执行中 callback 的交错；该服务是共享能力，影响面不止额度刷新 |
| L5 | Prompt Metrics 禁用时仍构造 publisher；publisher Stop 没有旧补丁的 once/done 复用 | 禁用时不构造 publisher，Stop 幂等 | “创建 publisher”不等于本轮证明已启动多余 worker；需要结合池实现及测试确认价值 |

证据入口：`backend/cmd/server/wire.go:142`、`:440`；`backend/internal/service/token_analysis_service.go:145`；`backend/internal/service/token_analysis_auto_index.go:23`、`:49`；`backend/internal/service/user_platform_quota_flusher.go:243`、`:259`；`backend/internal/service/timing_wheel_service.go:56`、`:70`；`backend/internal/service/promptmetrics/extension.go:22`、`publisher.go:52`。

Codex 建议：先保留分支，将 L1–L5 按必要性独立审查。旧治理设计、债务审计、实施计划仅作历史材料；是否继续治理重构另行决定。分支验证报告中的历史 `97/97` 和 race PASS 未在当前 main 重跑，不能当成本轮通过。

## 5. U5：272k 限制建议停止采用

| 事实 | 说明 |
| --- | --- |
| 旧补丁内容 | `configuredCodexGPT56MaxContext` 从 `872_000` 改为 fallback `272_000`，配套修改测试和 wiki |
| 当前行为 | `openai_codex_models_service.go:332` fallback 为 272k，`:336` 最大值常量为 872k；两者用途不同 |
| 影响扩大 | `:517` 的 GPT-5.6 与 GPT-6 Sol/Luna 分支复用该上限；GPT-6.1 Sol 随后在 `:537` 重新加载专用 metadata，不能仅看到条件包含 6.1 就断言它最终也被改成 272k |
| 作用边界 | 此补丁改的是本地生成模型目录的能力描述，不是网关请求 token 准入拦截，也不是长上下文计费开关 |
| 外部事实边界 | 用户指出官方已支持 872k；本轮没有独立访问官方实时文档，不把仓库常量当成所有账号/模型实际可用上下文的证明 |

Codex 建议不合并。Claude 需要确认是否还有用户明确要求的成本控制或客户端兼容原因；如确有需求，应重新定义模型/账号范围及控制方式，而不是复活这个共享常量的全局收缩补丁。

## 6. U2、U6、U7、W2：已覆盖或过时的候选项

| ID | 独立核对结果 | 证据入口 | 复审余项 |
| --- | --- | --- | --- |
| U2 | 上游 `49504adc` 已是 main 祖先；后续 0.1.183 同步记录覆盖 0.1.178–0.1.183。核对旧 merge 的 Wire 双方服务接线、归档/拦截与 CN 配置并存、tool adaptation 后清洗 body、账号 Key placeholder 等解决行为，当前均有对应实现 | `git show --remerge-diff e558c091f`；`docs/upstream-sync/merge-log.md:1770`；`backend/cmd/server/wire_gen.go:317`；`backend/internal/config/config.go:1166`；`backend/internal/service/openai_gateway_passthrough.go:240`；`frontend/src/components/account/CreateAccountModal.vue:4311` | 旧 `docs/superpowers/plans/2026-08-19-sub2api-v0.1.178-sync.md` 不在当前树；需要留档时单独保存，不回放旧台账。不声称整分支 tree 等于 main |
| U6 | 预演退出码 0，结果 tree 等于 `07941ebfe...`；10 个点名的旧中间文件当前均不存在，精确忽略项和 `__pycache__/` 规则均命中 | `.gitignore`；`llm-wiki/wiki/ai-workflow.md`；`git merge-tree` / `git check-ignore` | 这是最明确的无新增内容候选；执行清理前仍按当时引用重新核验 |
| U7 | buffered reader 的 EOF Finish 与正常 terminal 两个读取点，都按实际结构选择 `usage` / `response.usage` 并补齐明细；旧补丁的目标已覆盖 | `backend/internal/service/openai_gateway_messages.go:916`、`:974`；现行代码也覆盖 streaming 分支 | 旧价格断言为 5/30/0.5 美元每百万 token，当前测试为 0.2/1.2/0.02；这是代码断言比较，不是官方定价核验。`UseKeyModal` 当前按场景使用 `OpenAI` / `sub2api`，不能自动改成 `xunyou` |
| W2 | 旧 diff 将手动 NewPluginManager + SetAccountDirectory 收入 provider；当前 `ProvidePluginManager` 正是这一行为，ProviderSet 和生成代码都在使用它 | `backend/internal/service/wire.go:196`；`backend/cmd/server/wire_gen.go:220` | 工作区在 `E:/allsite/xyai-plugin-wiring`。文件整体 hash 不同源于旧基线，不证明该补丁有新功能；清理前确认没有后来新增的本地文件 |

U7 附带 `TokenAnalysisView.vue` 仅有分号格式改动，不能作为保留业务分支的理由。主线现有相关测试只被读取，未在本轮运行。

## 7. U4、W1：主要需要明确用途

### U4 请求时区改写

- 当前 tracked 后端、前端、wiki 中没有 `enable_openai_request_timezone_rewrite` / `applyOpenAIRequestLocale` 这套实现；它确实是未进入 main 的功能。
- 旧实现需要全局与账号双开关；默认目标 `America/Los_Angeles`，可选择其他允许时区。修改的是结构化 `environment_context.timezone` 与 Web Search `user_location.timezone`，不是后台显示时间或 `Accept-Language`。
- 分支的 `setting_gateway_runtime.go` 在三次 CAS 竞争耗尽、且无有效新快照时返回最后读到的 `loaded.result()`；分支 wiki 明确把“可能返回旧开启结果”列为启用灰度前需修问题。
- 请求内容改写会改变用户传给模型/搜索的时区语义；本轮没有验证它能解决哪一种现存故障，也未验证 Codex CLI、compact、web_search、prompt cache 或 WS 的实际效果。

请 Claude 先说明该功能解决的具体问题与现有替代方式，再建议保留或归档。需要用户最终回答的是“现在还要这个行为吗”，而不是仅让用户判断某个 CAS 实现是否正确。若继续，应基于当前 HTTP/WS 和设置缓存接入，旧分支并非可直接上线版本。

### W1 本机请求排障台

| 项目 | 事实 / 建议 |
| --- | --- |
| 工作区 | `C:/Users/Admin/.codex/worktrees/request-diagnostics/xyai` |
| 独有内容 | `tools/request-diagnostics/` 源码、前端、测试、README；`docs/features/local-request-diagnostics-design-cn.md`；`docs/delivery/2026-10-23-request-diagnostics/acceptance.md`；两页 wiki 的未提交增量 |
| 功能 | 本机网页通过已有 SSH 查询日志数据库，支持 ID / 状态码 / 错误检索、账号聚合和本机确定性分类；不依赖外部模型 |
| 证据边界 | 设计与历史验收材料可读取；今天的数据库兼容性、连接、性能和实际使用频率未验证 |
| Codex 倾向 | 先保护源码；仍使用则迁入当前主线并复验，已不用则先决定可恢复留档方式再清理 |
| 迁移约束 | 分支已合入，继续 merge 分支不会带入这些 untracked 文件；不能用旧 wiki 整文件覆盖 main。连接配置、私钥及 `__pycache__` 不属于源码交付 |

## 8. 请 Claude 返回的结果

建议复审顺序：U3 → U1 → U5 → U2/U6/U7/W2 → U4/W1。优先验证“是否误删有效修复”和“旧补丁是否引入新回归”。

| 返回项 | 期望内容 |
| --- | --- |
| 逐项结论 | U1–U7、W1–W2 分别给出：赞同 / 部分赞同 / 不赞同 / 证据不足；列明源码证据 |
| 纠错 | 指出 Codex 或 Grok 哪个判断不成立，尤其区分同名符号缺失、等价实现、已失效需求、业务政策尚未确定 |
| U3 修复拆分 | 哪些是现有合同下的缺陷，哪些需要业务授权；怎样兼容 `admin.department_members` 与受限管理密钥裁剪 |
| U1 必要性 | 启停问题是否可达、影响是什么、需什么最小测试；不能把旧报告 PASS 当当前验证 |
| 清理清单 | 只有在无独有必要内容或明确放弃后才建议删除；区分本地分支、远端分支、工作区未提交内容 |
| 用户决策 | 用业务语言列少量问题，提供建议和取舍理由；不要把所有源码不确定性转交给用户 |

可使用以下骨架，另写 Claude 独立复审文档，保留本文和 Grok 原始快照：

| ID / 问题 | Claude 裁定 | 支持 / 反证及基线 | 尚缺验证 | 建议处置 | 需用户确认的业务选择 |
| --- | --- | --- | --- | --- | --- |
| U3 / D1–D8 | 待填 | 待填 | 待填 | 待填 | 待填 |
| U1 / L1–L5 | 待填 | 待填 | 待填 | 待填 | 待填 |
| U2 / U4 / U5 / U6 / U7 / W1 / W2（分别列行） | 待填 | 待填 | 待填 | 待填 | 待填 |

## 9. 本地复核命令与操作边界

在仓库根目录运行；比较独有提交用 `main..<ref>` 和提交自身补丁，不把 `git diff main <旧分支>` 的全部历史差异当成缺失内容。

```powershell
git status --short --branch
git worktree list --porcelain
git rev-parse main github/main 'main^{tree}'
git log --oneline main..feature/hy/10207_department_usage
git show --format= --stat ce6ce97c9
git show ce6ce97c9 -- backend/internal/service/department_service.go backend/internal/repository/user_admin_access.go
git show --remerge-diff e558c091f -- backend/cmd/server/wire.go backend/internal/service/openai_gateway_passthrough.go
git merge-tree --write-tree --name-only main github/chore/remove-merge-artifacts-20260930
git -C 'C:/Users/Admin/.codex/worktrees/request-diagnostics/xyai' status --short
git -C 'E:/allsite/xyai-plugin-wiring' diff -- backend/internal/service/wire.go backend/cmd/server/wire_gen.go
```

`merge-tree --write-tree` 会计算并写入 Git 对象，但不会更新分支、索引或检出文件；冲突时退出码 1 是预演结果。U1–U5、U7 本轮预演有冲突，只有 U6 无冲突且结果树与 main 完全一致。

本地远端跟踪引用不能证明服务器此刻状态。后续真正清理前，需要复查引用和工作区变化；内网 `origin/main`、上游 `Wei-Shaw/sub2api` 分支不属于本次候选合并或清理对象。

本轮新增的是评审文档，不修改业务代码。未采纳建议不回写为 wiki 已实现合同；现有历史 wiki 中的“未提交/未合入”状态也不能替代本次 Git 基线判断。用户确认后，再按采纳内容更新现行设计和对应 wiki。
