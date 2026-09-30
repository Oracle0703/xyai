# 设计规格

## 摘要

| 字段 | 内容 |
|---|---|
| 需求来源 | `requirements.md` |
| 交付目标 | 形成可审核、未提交的 0.1.173 merge 快照。 |
| 主要验收标准 | 精确提交边界、冲突清零、本地功能保留、文档完整、验证证据真实。 |

## 当前状态

| 区域 | 当前行为 / 证据 |
|---|---|
| 分支基线 | `feature/hy/10173_merge_sub2api_173` 的第一父为 `ddbb0426...`，与 `github/main` 一致。 |
| 上游边界 | `MERGE_HEAD=48eb3766...`，merge base 为 `aac53afe...`。 |
| 三方变化 | 仅本地 389 路径、仅上游 414 路径、双方修改 79 路径；9 个文本冲突。 |
| 本地功能 | `docs/features/` 合并前 23 个 tracked 文件均为本地新增，上游未修改或删除。 |
| 提交状态 | merge 处于 `--no-commit --no-ff` 状态，未 commit、未 push。 |

## 目标行为

| ID | 行为 | 用户 / 系统影响 |
|---|---|---|
| TB-1 | 采用上游 0.1.173 的 Grok 完整集成、Channel Monitor V2、响应模型审计、邮箱主域额度等增量。 | 获得固定上游版本能力。 |
| TB-2 | RequestArchive/Intercept、Prompt Metrics/Risk、Token Analysis、组织用量、子管理员、compatible cache、默认 reasoning、并发 preset 等本地能力继续存在。 | 避免同步上游时回退本地功能。 |
| TB-3 | 根级 video/voice/realtime/web_search 别名显式挂本地 archive/intercept；Responses guard 顺序不回退。 | 新入口不绕过本地审计与拦截链。 |
| TB-4 | Channel Monitor V2 与本地后台服务同时注入并正确清理。 | Wire 生命周期完整。 |
| TB-5 | 上游缺陷原样保留并记录风险。 | 不制造难以回溯的本地 fork 修复。 |

## 接口

| 接口 | 变更 | 兼容性说明 |
|---|---|---|
| Gateway routes | 合入 Grok video/audio/voice/realtime/web search 路由和根别名。 | 根别名继续执行 API Key、composite/group gate、RequestArchive、RequestIntercept。 |
| Settings | 合入 Grok、Channel Monitor V2、邮箱主域额度等运行时设置。 | 本地 archive/intercept 与风险控制回调继续 round-trip。 |
| UsageLog | 增加 `upstream_response_model` 及 mismatch 三态口径。 | 与本地 `upstream_model`、cache usage 互斥桶并存，不混为同一字段。 |
| Channel Monitor V2 | 增加 admin/user API、repository、service、前端视图与 feature gate。 | 默认仍为 V1，V2 显式启用。 |
| Grok OAuth session | Redis 单次消费 session。 | 先校验 state/redirect/client，再消费；远端 miss 不复活本地旧 session。 |

## 数据契约

| 字段 / 格式 | 必需规则 | 兼容性 / 消费方说明 |
|---|---|---|
| `upstream_model` | 表示发送给上游的映射模型。 | 保留现有管理/用户用量消费方。 |
| `upstream_response_model` | 表示上游实际返回模型；mismatch 为 `NULL/false/true` 三态。 | `false` 筛选不得包含 `NULL`。 |
| Channel Monitor V2 migrations | 双 `194_`、双 `195_` 按完整文件名排序执行。 | 不为数字前缀重复而重命名。 |
| Grok media/search pricing | video/audio/voice/search 独立字段与计费元数据。 | 不复用图片定价覆盖视频/音频/搜索。 |

## 数据流

| 步骤 | 输入 | 处理 | 输出 |
|---|---|---|---|
| 1 | 本地 `main@ddbb0426...`、上游 `48eb3766...` | Git 三方 merge | 暂存的合并树。 |
| 2 | 9 个文本冲突 | 按语义取并集，重叠能力优先上游 | 冲突索引清零。 |
| 3 | schema/provider 源定义 | 运行 Ent/Wire generate | 可重复生成的生成物。 |
| 4 | 合并树 | focused/full tests、lint、build、静态审计 | 可审核验证证据。 |
| 5 | 已验证事实 | 更新 wiki、图谱、ledger | 后续 AI 和维护者可复用的知识。 |

## 失败模式

| 场景 | 预期处理 | 需要的证据 |
|---|---|---|
| 未合并索引或冲突标记残留 | 不进入用户审核点。 | `git ls-files -u`、`rg` 扫描。 |
| 生成物漂移 | 以源定义重新生成并审查差异。 | generate 后 Git diff。 |
| 测试发现接口失配 | 仅做合并所必需的测试桩/调用签名适配。 | focused test 前后证据。 |
| 测试暴露上游 bug | 记录为残余风险，不修改生产逻辑。 | 与固定上游或合并前基线对照。 |
| GitHub 无同名分支 | 保持本地待提交，不擅自 push。 | `git ls-remote github`。 |

## 验收标准映射

| 成功标准 | 规格覆盖 | 验证方式 |
|---|---|---|
| SC-1 | 基线与上游固定 SHA | `rev-parse`、`ls-remote`。 |
| SC-2 | 三方 merge 与冲突处理 | index/marker/whitespace 审计。 |
| SC-3 | TB-2/TB-3/TB-4 | 重叠文件审查、focused tests、features 清单。 |
| SC-4 | TB-5 | 额外编辑与固定上游 blob 对照。 |
| SC-5 | 文档数据流 | wiki diff、ledger、图谱状态。 |
| SC-6 | 测试失败模式 | 完整测试矩阵。 |
| SC-7 | 提交状态 | `MERGE_HEAD` 与 Git 状态。 |
