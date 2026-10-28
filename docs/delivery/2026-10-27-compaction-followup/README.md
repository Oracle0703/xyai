# 10-27 压缩后继结构补证

## 结论

本次固定快照识别到 286 条显式压缩请求：233 条 auto/mid_turn、53 条 auto/pre_turn，manual 为 0。213 条以恢复摘要前缀结尾的请求中，212 条能找到同 API Key/session/thread 的 mid_turn/auto 候选前驱，且同 turn/root_turn、前驱 HTTP 200 并在子请求开始前完成；另 1 条缺少 session/thread。

这些事实支持“已关联样本的恢复摘要与轮中自动压缩高度相关”，不能证明“仅看摘要前缀就一定是 AI”。manual 没有样本；同一文本块内是否追加新问题没有可靠结构分隔；1 条无标识请求无法判定前驱。建议优先使用显式 request_kind=compaction，恢复调用使用可靠前驱条件或客户端显式恢复标记；若不实现这些条件，保留现有推断边界，不把统计关联写成无状态的强保证。

## 固定范围与覆盖

- 文件：`/opt/sub2api/backend/data/request-archive/2026-10-27.jsonl`。
- 固定前 **110,200,243,008 bytes**，快照捕获于北京时间 **2026-10-27 18:17:24.690**。扫描时文件继续增长，未读取快照之后的新字节。这不是完整自然日验收。
- 生产版本 `25bedc620d9783e065ec6efae45d56f984b6b01c`。
- 完整扫描 99,536 行：49,781 request、49,755 response；外层 JSON 错误、超行上限、未读完整尾部均为 0。
- 63 个 body 无法按完整 JSON 提取：43 个截断的 Responses、20 个合法图改图 multipart。截断请求的压缩用途未知，不能当作已确认非压缩。
- 45,347 条请求具备完整 API Key/session/thread 关联标识；4,434 条不具备，含上述无法解析的 body。元数据解析错误与身份字段冲突均为 0。

## ① 显式压缩按 phase、trigger、版本计数

以下所有值的 trigger 均为 auto；manual 未观察到。版本为 UA 中 Codex 核心版本；桌面发行 build 另存于逐条材料的 desktop_app_version。

| 客户端 | UA 中的核心版本 | auto / mid_turn | auto / pre_turn |
| --- | --- | ---: | ---: |
| codex desktop | 0.153.4 | 1 | 0 |
| codex desktop | 0.154.0-alpha.6.2 | 2 | 0 |
| codex desktop | 0.162.0-alpha.17.2 | 11 | 1 |
| codex desktop | 0.163.0-alpha.10.2 | 22 | 3 |
| codex desktop | 0.163.0-alpha.11.1 | 6 | 0 |
| codex desktop | 0.163.0-alpha.11.2 | 5 | 1 |
| codex desktop | 0.164.0-alpha.11.1 | 120 | 35 |
| codex-tui | 0.164.0 | 62 | 13 |
| codex_vscode | 0.162.0-alpha.17.2 | 2 | 0 |
| codex_vscode | 0.164.0-alpha.11.1 | 2 | 0 |

合计：mid_turn 233，pre_turn 53，总计 286。

## ② 同身份时间上紧随的下一请求

按 `(timestamp 纳秒值, archive_id, start_offset)` 排序，在相同 API Key/session/thread 的可关联记录中取直接下一请求，不跳过失败请求或连续压缩。最后有效项跳过 system/developer 和完整纯环境包装。

| 父压缩 | 恢复摘要为有效末项 | 摘要之后有 user 正文 | 工具输出 | 其他 | 快照内无后继 | 合计 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| auto / mid_turn | 205 | 1 | 0 | 26 | 1 | 233 |
| auto / pre_turn | 0 | 19 | 1 | 31 | 2 | 53 |
| manual | — | — | — | — | — | 0（未覆盖） |

- “恢复摘要为有效末项”表示最后有效 user 文本以已知恢复前缀开头，且没有随后独立文本块/媒体；并不证明同一文本块内没有新增问题。所有这类记录显式保留 `summary_suffix_ambiguous=true`。
- “摘要之后有 user 正文”表示输入序列里存在恢复摘要，后面有非包装 user 文字；不证明该文字是人新发的，也不证明它相对本次父压缩从未出现。
- **32 条直接后继本身又是 compaction（mid/pre 各 16）归入“其他”**。其中 27 条原始末项形态会落在“历史有摘要、最后 user 有文字”（mid 11、pre 16），但最后文字实际是再次压缩指令，不能当作新用户正文。原始形态统计保存在 `raw_successors_by_phase_trigger`，逐条同时保存 `raw_tail_category` 和最终 `outcome`；没有删除或去重这些请求。
- “其他”还包括 compaction/compaction_trigger/agent_message 条目、未形成“摘要后 user”结构的普通 user 尾项。它不是统一的 AI 或人工类别。
- 缺少身份或无法解析的请求可能无法插入关联序列；“下一条”限于可关联记录。快照内找不到后继不代表永远没有后继。

HTTP 完成事实：233 条 mid_turn 均为 200；53 条 pre_turn 为 47 个 200、6 个 404。另有 2 个按开始时间排序的直接后继早于父请求 completed_at，且 turn 不同，不能视为父压缩完成后的因果续接；它们不属于上表的 205 个摘要尾项。详情在 statistics.json 的 response_validation 中。

## ③ 反查恢复摘要尾项

| 检查 | 数量 |
| --- | ---: |
| 恢复摘要为最后有效文本前缀 | 213 |
| 找到最近 mid_turn/auto 压缩候选前驱 | 212 |
| 其中是压缩的直接下一请求 | 205 |
| 其中中间还有其他请求 | 7 |
| 这 212 条同 turn、同 root_turn | 212 |
| 这 212 条前驱 HTTP 200 且先完成 | 212 |
| 缺 session/thread，无法关联 | 1 |
| 身份完整却在窗口内找不到前驱 | 0 |
| 找到 pre_turn/manual 前驱 | 0（manual 未覆盖） |

最近候选前驱不等于独立证明“摘要正文由它产出”；没有保存/比较模型响应正文，也没有用户操作日志。身份缺失项为 `f7e10bc24c22882bbb2fb7ade098f3b0`，应保持来源推断边界。

## ④ 固定压缩模板的显式元数据覆盖

共 268 条有效末项完整匹配已核验的固定模板（仅正规化 CRLF 和首尾空白），267 条带 `request_kind=compaction`，覆盖 **99.63%**。

| 客户端 | 核心版本 | 固定模板数 | 带显式 compaction 元数据 |
| --- | --- | ---: | ---: |
| codex desktop | 0.153.4 | 1 | 1 |
| codex desktop | 0.154.0-alpha.6.2 | 2 | 2 |
| codex desktop | 0.162.0-alpha.17.2 | 12 | 12 |
| codex desktop | 0.163.0-alpha.10.2 | 25 | 25 |
| codex desktop | 0.163.0-alpha.11.1 | 6 | 6 |
| codex desktop | 0.163.0-alpha.11.2 | 4 | 4 |
| codex desktop | 0.164.0-alpha.11.1 | 138 | 138 |
| codex-tui | 0.164.0 | 75 | 75 |
| codex_vscode | 0.162.0-alpha.17.2 | 2 | 2 |
| codex_vscode | 0.164.0-alpha.11.1 | 2 | 2 |
| unknown | unknown | 1 | 0 |

无显式标记项为 `e9d72fa484257df605a0ed67054b02c1`，客户端版本、session/thread 均不可用。它说明仅靠元数据不能覆盖本窗口所有固定模板形态，但模板形态本身也不是物理发起来源证明。另有 **19 条显式 compaction 请求不使用该固定模板**；显式字段识别应优先，模板保留为有边界的兼容方向。

## 材料与核验

| 文件 | 内容 |
| --- | --- |
| `statistics.json` | 四项聚合、原始形态/最终分类、响应时间边界与扫描汇总 |
| `compaction-events.jsonl` | 286 条压缩的结构、版本和行定位 |
| `compaction-successors.json` | 286 组直接后继关系及 HTTP 完成事实 |
| `resume-events.jsonl` / `resume-predecessors.json` | 213 条恢复尾项及候选前驱关系 |
| `template-events.jsonl` | 268 条固定模板形态及显式标记覆盖 |
| `tail-refinement-facts.jsonl` | 55 条涉及媒体的有效尾项原文布尔补查；分类改变 0 |
| `response-facts.json` / `database-query-evidence.json` | 609 条请求的只读响应事实；查询 SQL、耗时与 ROLLBACK 证明 |
| `preflight.jsonl` / `execution-manifest.json` | 生产版本、服务状态、文件快照、扫描二进制 SHA 与执行参数 |
| `SHA256SUMS` | 本目录文件校验和（不含自身） |

包内没有正文、图片、工具输出或真实用户/API Key/session/thread ID。只导出身份相等布尔，关联用的临时 HMAC key 未落盘。完整结构中间结果和扫描/分析脚本位于 `E:/tmp/origin-audit/2026-10-27-compaction-followup/`，不需要分享整段请求内容。

扫描单行上限 128 MiB、4 个低优先级解析 worker、在途内存预算约 512 MiB、Go 软内存上限 1 GiB；远端通过 memfd 执行，没有安装软件或落地扫描程序。完整扫描耗时 690.968 秒；两次 256 MiB 试读加 55 条定点补查后，实际归档读取总量 **110,977,270,359 bytes**。此前独立 Python 读取的 5 条记录与本次 SHA、阶段、模板和身份配对一致；合成解析/排序/分组校验、Go 测试/vet、JSON/去重/计数/哈希检查通过。

数据库每段使用 default_transaction_read_only=on、BEGIN READ ONLY、45 秒 statement timeout、2 秒 lock timeout、ROLLBACK。609 条响应事实均找到，收尾无其他审计连接或打开事务。sub2api 仍 active，PID 3522741、启动时间未变。本轮没有生产写入、部署、服务重启、重索引或更改归档保留策略。
