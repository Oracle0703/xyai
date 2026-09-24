# GPT 账号额度展示（设计待最终审核）

- 完整设计：docs/features/gpt-account-quota-display-design-cn.md（2026-09-24 修订）。尚未实施；用户明确要求设计审核通过后再编码。
- 已确认：只展示 GPT/OpenAI 上游账号；通过现有系统模式门禁的登录用户看到同一列表；账号原名 TrimSpace 后按 c-/d- 前缀归类为迅游/速宝，桌面左右双列，移动端上下排列。管理员选择展示账号，其他前缀不展示；与组织、分组授权无关。
- 数据隔离：用户只读独立持久化快照；缺失、过期、失败或反复轮询都不得触发上游。主动采集不写 accounts.extra，不写调度键，不改变账号错误、暂停、可调度状态，不触发自动重置或 reset-credit。
- 数据源：专用只读 OpenAI wham/usage 查询，只解析 rate_limit.primary_window/secondary_window，忽略 additional_rate_limits；不复用会发模型探测的 getOpenAIUsage，不直接复用完整 QueryUsage 副作用链。
- 首版资格：OpenAI OAuth 普通主账号、未软删除；排除 shadow、PAT、Agent Identity、Setup Token。暂停或错误账号仍可展示已有快照。
- 调度：每天 Asia/Shanghai 09:30—18:00，30/60 分钟；60 分钟模式默认包含 18:00 收尾；使用 leader lock、单行槽位条件更新、账号级 singleflight/60 秒冷却；不用通用任务表、租约、代次和任务查询接口。页面轮询固定 15 分钟。
- 公开开关关闭时隐藏用户菜单；路由服从 JWT、BackendModeUserGuard、审计和现有面板限制。用户 DTO 只含采样时间与 stale，管理端才返回最近尝试状态。
- 综合审核结论：已吸收 Claude/Grok 共同意见，当前仍待最终审核；未经用户明确通过，不得生成业务代码或数据库迁移。
