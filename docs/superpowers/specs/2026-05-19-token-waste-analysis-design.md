# 部门用量与 Token 浪费分析设计

## 背景

一期已经上线本地请求归档能力，AI 网关请求会按天写入 `data/request-archive/YYYY-MM-DD.jsonl`，并用 `archive_id` 关联 request/response 事件。`usage_logs` 已经记录每次成功请求的 token、缓存命中、成本、用户、API Key、模型、端点和耗时等结构化数据。

二期目标是在后台提供一个面向部门运营和审计的页面，让管理员能直观看到所有用户的用量、缓存命中情况、疑似浪费 token 的请求，以及请求参数摘要。用户确认首版采用 B 方案：展示请求参数摘要，不展示完整响应正文。

## 目标

| 目标 | 说明 |
|---|---|
| 部门总览 | 展示选定时间范围内的总请求数、总 tokens、输入/输出 tokens、缓存读写 tokens、成本和疑似浪费请求数 |
| 用户/API Key 排行 | 按用户和 API Key 聚合 token、成本、缓存命中率、平均输入/输出、疑似浪费比例 |
| 可疑请求定位 | 列出疑似浪费请求，支持按用户、API Key、模型、分组、时间、风险类型过滤 |
| 请求参数摘要 | 展示模型、消息数量、system 长度、最后用户消息预览、tools 数量、body 截断状态等摘要 |
| 可解释规则 | 每条可疑请求展示命中的规则和关键指标，避免黑盒评分 |
| 为后续分析留接口 | 为后续相似度聚类、重复提示词分析、洗 token 风险评分保留数据结构 |

## 非目标

| 非目标 | 原因 |
|---|---|
| 首版展示完整响应正文 | 响应可能包含敏感内容，且文件体积和权限风险高 |
| 每次打开页面全量扫描 JSONL | 数据量增长后性能不可控，容易拖慢后台页面 |
| 改动请求热路径同步写审计表 | 会增加线上请求路径风险，首版先用异步索引任务 |
| 自动判定用户恶意 | 首版只给“疑似浪费”信号和证据，不做封禁或扣费动作 |

## 现有数据基础

| 数据源 | 已有能力 | 本功能使用方式 |
|---|---|---|
| `usage_logs` | token、成本、缓存读写、用户、API Key、账号、分组、模型、端点、耗时 | 作为统计、排行和成本分析的主数据源 |
| `request_archive` JSONL | 请求/响应体、headers allowlist、身份字段、状态、耗时、截断标记 | 只读取 request 事件，提取请求参数摘要 |
| 用户/API Key 表 | 用户邮箱、API Key 名称、归属关系 | 页面展示和筛选 |

当前缺口是 `usage_logs` 与 `request_archive` 没有稳定的一对一关联字段。首版采用“近似关联 + 记录关联置信度”：优先使用 `archive_id` 摘要中的 `user_id/api_key_id/model/timestamp`，在时间窗口内匹配最接近的 `usage_logs`；无法可靠匹配时，摘要仍可展示为未关联归档记录。

## 数据模型

新增一张请求审计摘要表，用于保存从 JSONL 中提取出的轻量摘要，避免后台页面反复扫描文件。

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | bigserial | 主键 |
| `archive_id` | text unique | 归档请求 ID |
| `usage_log_id` | bigint nullable | 近似匹配到的 `usage_logs.id` |
| `match_confidence` | smallint | 0 未匹配，1 低，2 中，3 高 |
| `event_time` | timestamptz | request 事件时间 |
| `user_id` | bigint nullable | 用户 ID |
| `api_key_id` | bigint nullable | API Key ID |
| `account_id` | bigint nullable | 上游账号 ID |
| `group_id` | bigint nullable | 分组 ID |
| `model` | text | 请求模型 |
| `endpoint` | text | 归一化端点 |
| `method` | text | HTTP method |
| `request_body_size` | bigint | 原始请求体大小 |
| `request_body_truncated` | boolean | 请求体是否被归档截断 |
| `body_sha256` | text | 请求体 hash，用于重复检测 |
| `message_count` | integer | messages/input 数量 |
| `system_chars` | integer | system/developer 指令字符数 |
| `user_chars` | integer | 用户消息字符数估算 |
| `last_user_preview` | text | 最后一条用户消息预览，限制长度 |
| `tools_count` | integer | tools/functions 数量 |
| `image_count` | integer | 图片输入数量估算 |
| `summary_json` | jsonb | 不同协议的轻量摘要扩展 |
| `risk_score` | integer | 0-100 的疑似浪费分 |
| `risk_reasons` | jsonb | 命中规则列表和指标 |
| `indexed_at` | timestamptz | 索引入库时间 |
| `source_file` | text | 来源 JSONL 文件名 |
| `source_offset` | bigint nullable | 文件偏移，便于排查 |

索引：

| 索引 | 用途 |
|---|---|
| `(event_time DESC, id DESC)` | 默认请求列表 |
| `(user_id, event_time DESC)` | 用户维度筛选和排行下钻 |
| `(api_key_id, event_time DESC)` | API Key 维度筛选 |
| `(group_id, event_time DESC)` | 部门/分组过滤 |
| `(risk_score DESC, event_time DESC)` | 可疑请求列表 |
| `(body_sha256, event_time DESC)` | 重复请求检测 |
| `usage_log_id` | 与 `usage_logs` join |

## 摘要提取规则

只处理 request 事件，不保存完整 body。索引任务读取 `body` 后立即提取摘要，落库后丢弃原文。

| 协议/字段 | 提取内容 |
|---|---|
| OpenAI Chat Completions `messages` | 消息数量、system/developer 字符数、user 字符数、最后用户消息预览、tools 数量 |
| OpenAI Responses `input` | 输入项数量、文本字符数、最后用户文本预览、tools 数量 |
| Claude Messages `system/messages/tools` | system 字符数、消息数量、最后 user 文本预览、tools 数量 |
| Gemini `contents/system_instruction/tools` | contents 数量、system 字符数、最后用户文本预览、tools 数量 |
| 图片请求 | prompt 预览、图片数量、尺寸字段 |

预览字段默认只保存前 300 个字符，并做基础脱敏：去掉明显的 API Key、Bearer token、Cookie、长 Base64 串和超长连续无空白片段。`summary_json` 只存结构化统计和短预览，不存完整消息数组。

## 疑似浪费规则

首版使用可解释的规则评分，每条规则输出 reason code、命中指标和分值。

| 规则 | 条件示例 | 目的 |
|---|---|---|
| `huge_input_tiny_output` | 输入 tokens 高，输出 tokens 很低，且非 count_tokens/图片请求 | 找超长上下文但几乎无有效输出的请求 |
| `repeat_uncached_body` | 同一 `body_sha256` 在短时间内多次出现，但 `cache_read_tokens` 很低 | 找重复大请求但没有命中缓存的场景 |
| `low_cache_hit_large_input` | 输入 tokens 高，缓存命中率低 | 找应该利用缓存但没有命中的请求 |
| `rapid_similar_requests` | 同用户/API Key 在短时间内多次发送相近长度和相同模型请求 | 找循环调用或脚本重复请求 |
| `oversized_system_prompt` | system/developer 字符占比过高 | 找长期携带超大固定提示词的请求 |
| `tool_heavy_short_output` | tools 很多、输入很大、输出很短 | 找工具定义过重但实际没有使用收益的请求 |

风险评分只用于排序和提示，不作为惩罚依据。规则阈值先在后端常量中保守设置，后续可迁移为后台配置。

## 后端接口

新增后台审计接口，保持和现有 `/admin/usage` 分开，避免污染现有用量记录页面。

| 方法 | 路径 | 返回 |
|---|---|---|
| `GET` | `/api/v1/admin/token-analysis/summary` | 顶部汇总、缓存命中率、风险请求数 |
| `GET` | `/api/v1/admin/token-analysis/users` | 用户/API Key 聚合排行 |
| `GET` | `/api/v1/admin/token-analysis/requests` | 可疑请求列表，包含 usage 指标和请求摘要 |
| `POST` | `/api/v1/admin/token-analysis/index` | 管理员手动触发指定日期范围的归档摘要索引 |
| `GET` | `/api/v1/admin/token-analysis/index/status` | 索引进度、最近处理文件、错误信息 |

通用过滤参数：

| 参数 | 说明 |
|---|---|
| `start_date/end_date/timezone` | 时间范围 |
| `user_id/api_key_id/account_id/group_id` | 维度过滤 |
| `model/endpoint` | 模型和端点过滤 |
| `risk_min/risk_reason` | 风险过滤 |
| `include_unmatched` | 是否包含未匹配到 `usage_logs` 的归档摘要 |
| `page/page_size/sort_by/sort_order` | 分页和排序 |

## 索引任务

索引任务以后台服务形式运行，启动后可自动增量索引最近归档，也允许管理员手动触发。

| 行为 | 设计 |
|---|---|
| 增量记录 | 保存每个 JSONL 文件已处理的 offset 或最后 `archive_id`，重复执行要幂等 |
| 幂等写入 | `archive_id` 唯一冲突时更新摘要和风险字段，不重复插入 |
| 关联 usage | 在 `event_time ± 10s` 内按 `user_id/api_key_id/model` 匹配最近 usage log，匹配不到则 `usage_log_id=NULL` |
| 错误处理 | 单行解析失败记录任务错误计数，不中断整个文件 |
| 限速 | 每批处理固定行数，避免大文件索引时占满数据库 |
| 可观测 | 暴露最近索引时间、处理文件、成功/失败行数 |

## 前端页面

新增后台页面 `/admin/token-analysis`，菜单名称建议为“Token 分析”或“用量审计”。界面按运营后台设计，信息密度高，优先筛选和表格，不做营销式展示。

| 区域 | 内容 |
|---|---|
| 汇总卡片 | 总 tokens、总成本、缓存命中 tokens、缓存命中率、疑似浪费请求数、疑似浪费成本 |
| 筛选栏 | 日期、用户、API Key、分组、模型、风险类型、风险分下限、是否显示未匹配记录 |
| 用户排行表 | 用户邮箱、用户 ID、API Key 数、请求数、总 tokens、缓存命中率、成本、风险请求占比 |
| 可疑请求表 | 时间、用户、API Key、模型、tokens、缓存读写、成本、风险分、命中规则、请求摘要 |
| 请求摘要抽屉 | 点击请求后展示结构化摘要：endpoint、模型、消息统计、最后用户消息预览、截断状态、关联置信度 |
| 索引状态 | 最近索引时间、处理进度、手动触发按钮、错误摘要 |

页面不提供“查看完整响应正文”按钮。若后续确实需要完整正文，应单独做权限确认、审计日志和脱敏展示。

## 权限与隐私

| 边界 | 处理 |
|---|---|
| 访问权限 | 仅管理员可访问，复用现有 admin 路由守卫和后端 admin middleware |
| 内容最小化 | 只保存请求摘要和短预览，不保存完整请求数组，不保存完整响应 |
| 脱敏 | 对 token、Cookie、长密钥、长 Base64 串做基础脱敏 |
| 操作审计 | 手动触发索引记录 operator user_id、时间范围和结果 |
| 默认展示 | 列表默认展示摘要；更详细内容放入抽屉，降低误浏览敏感信息的概率 |

## 测试策略

| 层级 | 测试点 |
|---|---|
| Go 单元测试 | 各协议请求体摘要提取、脱敏、风险规则评分、重复 body 检测 |
| Repository 测试 | 摘要表插入幂等、过滤、分页、聚合排行、usage 近似匹配 |
| Handler 测试 | admin 权限、参数校验、summary/users/requests/status 返回结构 |
| 索引任务测试 | JSONL 文件增量处理、坏行跳过、offset 恢复、重复执行不重复插入 |
| 前端测试 | API 类型、筛选参数、表格渲染、摘要抽屉、索引状态展示 |
| 回归测试 | 现有 usage 页面和 request archive 测试继续通过 |

## 分阶段交付

| 阶段 | 交付 |
|---|---|
| 第一阶段 | 数据库迁移、摘要提取器、风险规则、索引任务、后端查询接口 |
| 第二阶段 | 后台 Token 分析页面、筛选、汇总卡片、用户排行、可疑请求表、摘要抽屉 |
| 第三阶段 | 根据真实样本调整规则阈值，增加重复请求趋势和缓存优化建议 |

## 风险与缓解

| 风险 | 缓解 |
|---|---|
| JSONL 文件很大，索引耗时 | 增量 offset、批处理、限速、手动触发范围限制 |
| usage 与 archive 关联不准 | 返回 `match_confidence`，页面标注“未匹配/低置信度”，后续再增强统一关联字段 |
| 请求摘要仍可能包含敏感信息 | 短预览、脱敏、管理员限定、默认不展示完整 body/response |
| 规则误报 | 展示命中规则和指标，不自动处理用户；后续基于样本调阈值 |
| 新表增长过快 | 摘要比原文小得多；后续可加保留周期和清理任务 |

## 成功标准

| 标准 | 验收方式 |
|---|---|
| 管理员能看到部门整体 token/cost/cache 情况 | 打开 `/admin/token-analysis` 后汇总卡片正常显示 |
| 能定位高消耗用户和 API Key | 用户排行按 tokens/成本排序可用 |
| 能看到可疑请求和请求摘要 | 可疑请求表展示风险原因和短预览 |
| 不展示完整响应正文 | 页面和接口响应不返回 response body |
| 不影响线上请求链路 | 索引任务异步执行，请求路径不新增同步数据库写入 |
