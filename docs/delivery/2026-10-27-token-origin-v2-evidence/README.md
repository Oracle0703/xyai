# 来源 v2 设计补证（只读、结构信息）

完整结论补在现有 `docs/features/token-analysis-request-origin-design-cn.md` 的“v2 修订 / 审计方补证与实施边界”，未另建 v2 设计文档。

## 适用窗口与材料缺口

审计窗口仍为北京时间 `[2026-10-26 09:00, 2026-10-27 09:00)`。2026-10-27 16:35 生产只读预检：部署仍为 `25bedc620d9783e065ec6efae45d56f984b6b01c`，指纹列未迁移，窗口计数仍为 60,065 条、人工 3,591 条、来源版本 1。原配置归档目录不再有 10-26 原文或压缩副本，本轮没有删除文件或追查未经确认的备份位置。

| 队列 | 总数 | 原文尚在、已读取 | 原文在缺失的 10-26 文件 |
| --- | ---: | ---: | ---: |
| 摘要续接候选 | 186 | 2 | 184 |
| 固定压缩模板 | 207 | 3 | 204 |
| hertz 纯图片 | 22 | 0 | 22 |
| AI Messages | 1,053 | 64 | 989 |

这些“184 条缺原文”和“184 条未计入原 478 下限”是不同集合，数量相同不能混用。原 186 摘要中有 2 条 `app_tool_send_message` 已计入原下限；本次可读的 2 条不是那 2 条。

## 实际补证

- 2 对摘要/压缩请求有相同 API Key 与 session/thread/turn/root_turn，父请求前项为工具输出，明确 `request_kind=compaction`、`compaction.trigger=auto`、`phase=mid_turn`。子请求除环境上下文变化、末条恢复摘要外，未发现父历史中未出现的普通文本块。这是结构关联证据，不能证明期间没有重复发送相同文字、图片或其他用户操作。
- 另 1 条压缩明确 `phase=pre_turn`。不能把所有摘要后的请求一律当成 AI 续执行；`turn_trigger=composer`、`history_initialization=new/cold_resume` 也不能单独决定。
- 64 条 AI Messages 的完整历史中，`tool_result` 与完整 `<system-reminder>` 同 user 消息为 0；当前末有效项同样为 0。因此本子集没有该混合形态中的排队消息实例。全天 1,053 条的数量仍未知。
- hertz 的 22 条仍只有先前保存的结构（21 条单图、1 条双图），前工具名和图片来源尚未补到。工具名本身也不足以证明图片来自工具，最好由客户端提供来源字段，或补调用 ID 配对/图片哈希相等布尔。
- 原数字需分母一致：全部 207 条压缩中前接工具结果 200；去掉 2 条元数据层重叠后是 205/198。291/255 仅是重复候选，未经完整请求指纹和操作来源验证。

## 文件

| 文件 | 内容 |
| --- | --- |
| `summary.json` | 队列覆盖、修正数字、读取量和未完成边界 |
| `resume-relationships.json` | 2 对关联事实、位置、SHA 和证据限制；无原始会话 ID |
| `compaction-metadata.json` | 3 条显式压缩用途元数据，含 mid_turn/pre_turn |
| `messages-reminder-facts.jsonl` | 64 条请求的 reminder/工具混合布尔、位置与 SHA |
| `target-locations.json` | 各待核队列的 archive_id、源文件和行末 offset |
| `archive-inventory.json` | 实际检查时间与归档目录文件清单 |
| `query-evidence.json` | 实际 SQL、耗时、只读/ROLLBACK 确认及生产版本；1053 行选择输出以摘要和定位列表留证 |
| `final-readonly-check.jsonl` | 收尾只读状态、固定窗口计数、无遗留审计事务与应用状态 |
| `SHA256SUMS` | 本目录文件校验和（不含自身） |

只读取 69 条唯一请求，补读 5 条元数据与文本块关系，共 74 次定点读，累计 121,058,550 bytes，提取错误 0。单行 64 MiB、每轮总预算 512 MiB，使用低优先级读取。所有行校验 request 事件、archive_id 和行尾 offset；新旧材料交集的行 SHA 一致。

请求身份在远端以单次随机 HMAC key 生成临时比较值，仅导出相等关系，不保留 key、原始会话/用户/API Key 值。正文、图片和工具输出不在本包。`source_offset` 为行末，复读必须向前定位行首并校验 SHA。

原始工作脚本和结构中间材料位于 `E:/tmp/origin-audit/2026-10-27-v2-review/`：`preflight.py`、`extract_available.py`、`extract_resume_detail.py`。本轮仅补文档和证据，未改业务代码或生产状态。当前生产无指纹列，未执行新归并查询的性能验收，也未执行旧归档重索引。
