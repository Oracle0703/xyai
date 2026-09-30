# 设计规格

## 摘要

| 字段 | 内容 |
|---|---|
| 需求来源 | `requirements.md` |
| 交付目标 | 形成一个只到 0.1.153 固定 SHA、保留本地扩展且可验证的合并分支 |
| 主要验收标准 | 父链边界、冲突语义、文档完整性、全量验证 |

## 当前状态

| 区域 | 当前行为 / 证据 |
|---|---|
| 分支 | `feature/hy/10153_同步sub2api主线` |
| 合并拓扑 | `0d65f65a` parents=`5e6e85568 55ed0ab0d` |
| 上游边界 | `55ed0ab0d` 是祖先；`7d239d62e` 不是祖先 |
| 冲突 | 6 个冲突已解决，无 unresolved index 或冲突标记 |
| 收口状态 | `backend/go.sum` 依赖清理、稳定知识与验证证据已完成；最终复核修订纳入文档收口提交 |

## 目标行为

| ID | 行为 | 用户 / 系统影响 |
|---|---|---|
| TB-1 | Responses→Chat options adapter 接入 `EffectiveResponsesTools` | 保留本地第三方过滤，同时支持 Codex `additional_tools` |
| TB-2 | 网关保留 archive/intercept 并增加 Grok 视频编辑/扩展 | 本地审计链与上游 media 能力并存 |
| TB-3 | 并发服务同时保留 `ConcurrencyCacheError` 与 WS ingress lease | Redis 故障保持 503 语义，长连接获得 API Key 级容量与空闲保护 |
| TB-4 | 账号创建同时保留 OpenAI-compatible preset 与 Grok API Key | 两侧 UI 能力均可使用 |
| TB-5 | 数据、支付、静态缓存和前端体验更新进入稳定文档 | 后续维护者可快速定位配置、migration 与组件契约 |

## 接口

| 接口 | 变更 | 兼容性说明 |
|---|---|---|
| `gateway.openai_ws.ingress_inter_turn_idle_timeout_seconds` | 新增，默认 300 秒，0 关闭 | 非负校验；只限制完成 turn 之间的客户端空闲 |
| `gateway.openai_ws.max_ingress_connections_per_api_key` | 新增，默认 64，0 关闭 | Redis 分布式 lease，独立于单 turn 并发槽 |
| `/v1/videos/edits`、`/v1/videos/extensions` 及根级别名 | 新增 Grok 路由 | 非 Grok 仍本地 404 + business-limited |
| `groups.web_search_price_per_call` | 新增可空 decimal | NULL 使用默认 0.01 USD/次，只对成功 alpha search 计费 |
| `UseKeyModal` Grok CLI / OpenCode | 新增配置生成 | 使用当前 API Key 与网关 base URL |
| 旧 payment channels 接口 | 删除 | 不再向用户端泄露内部渠道配置 |

## 数据契约

| 字段 / Payload / 格式 | 必需规则 | 兼容性 / 消费方说明 |
|---|---|---|
| `web_search_price_per_call` | NULL=0.01 USD/次；非空使用分组覆盖 | Ent schema、migration、DTO、管理 UI 与 billing 同步 |
| `additional_tools` input item | 合并 top-level tools 后再转换 | custom/namespace/tool_search 可逆映射和 tool_choice 校验继续生效 |
| WS ingress lease | key=`concurrency:openai_ws_ingress:api_key:{id}`，TTL 60 秒，20 秒刷新 | lease 丢失时关闭连接；非正数限制关闭保护 |
| Grok OAuth `base_url` | 官方 API URL归一到 CLI proxy；自定义 URL 仅在可信校验通过时使用 | API Key 无自定义时仍用官方 `api.x.ai/v1` |

## 数据流

| 步骤 | 输入 | 处理 | 输出 |
|---|---|---|---|
| 1 | 151 基线与固定上游 SHA | 三方 merge，逐文件解决冲突 | merge commit `0d65f65a` |
| 2 | merged source | tidy、静态扫描、聚焦与全量验证 | 可复查验证证据 |
| 3 | 源码事实与 merge metadata | 追加 ledger、review、wiki、组件 README | 稳定知识入口 |
| 4 | 最终工作树 | 父链、祖先、版本、diff 与状态审计 | 仅到目标 SHA 的交付分支 |

## 失败模式

| 场景 | 预期处理 | 需要的证据 |
|---|---|---|
| 后续上游提交误入 | 停止并修正父链，不继续文档收口 | `merge-base --is-ancestor 7d239d62e HEAD` 返回 1 |
| 冲突整侧覆盖 | 对比两父与 combined diff，补回缺失语义 | 6 文件 resolution notes 与聚焦测试 |
| Windows Go `.test.exe` 锁 | 使用新的 repo-local `GOTMPDIR` 串行重跑 | 完整成功尝试日志，不改业务代码 |
| 全仓 lint 失败 | 区分基线与本次差异，不误报 | full lint 29 项；`--new-from-rev HEAD^1` 为 0 issues |

## 验收标准映射

| 成功标准 | 规格覆盖 | 验证方式 |
|---|---|---|
| SC-1、SC-2 | 合并拓扑与上游边界 | `rev-list --parents`、ancestor checks |
| SC-3、SC-4 | TB-1 至 TB-4 | combined diff、入口扫描、聚焦测试和全量测试 |
| SC-5 | TB-5 | 文档路径与 append-only diff |
| SC-6 | 失败模式与验证矩阵 | `test-review.md` 中的最终命令记录 |
