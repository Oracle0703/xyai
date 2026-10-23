# Claude Code reminder 生产只读补充检查

日期：2026-10-24。目的：核验 `<system-reminder>` 结构的真实出现范围，不做生产重索引，不修改业务代码，不将 Python 近似分类当作 Go 执行结果。

## 执行方式

原 `E:/tmp/origin-audit/audit_messages.py` 遍历整份文件，并近似旧版分类逻辑，还输出 system prompt 前缀。本次改用 `audit_messages_bounded.py`：优先从两个已知 Messages 行偏移开始，再使用均匀字节锚点；总读取硬上限 1 GiB、单窗口 64 MiB、单行 24 MiB、时间上限 45 秒。遇到读取预算内不完整行即停该窗口，不将其计为归档损坏。预筛选依赖归档当前的紧凑 JSON 格式和字段位置，不能当全量扫描。

脚本通过 SSH stdin 交给 Python，服务器不落脚本文件；输出仅含结构和定位信息。本机原审计脚本未修改。

## 实测结果

| 项目 | 结果 |
| --- | --- |
| 目标 | `2026-10-23.jsonl`，与原 272 条证据包相同文件 |
| 主采样实际读取 | 1,073,741,824 bytes（1 GiB，含行边界对齐） |
| 执行耗时 | 4.352 秒（当次观测，不作性能承诺） |
| 完整行 | 去重后 1,246 行；重复偏移跳过 20 次 |
| Messages 请求 | 11 条，UA 均匹配 `claude-cli/` 前缀 |
| 有效尾部为 user tool_result | 10 条，未携带纯 reminder 文本块 |
| 有效尾部为普通 user 文本 | 1 条（只说明结构，不证明手动发起） |
| tool_result 与纯 reminder 同处 user | **0 条命中**，该组合仍未经生产实证 |
| 源文件 | 读取前后 size、mtime_ns、inode 一致 |

原始结构结果见 `messages-audit.json`。本次是有界、偏向已知 Messages 时段的定向检查，不能推算全日占比，也不能以 0 命中证明不存在。

## 完整历史中的 reminder

仅为补充检查历史包装，从上述样本中选 5 条精确行，校验其 SHA-256 后检查整个 messages 数组。每次读取合计 11,608,060 bytes；最终可复现脚本为 `audit_message_histories.py`，结果为 `messages-history-audit.json`。这些历史彼此重叠，不得将各请求里的重复历史累计成独立请求或用户轮次。

| 原行 offset | 原始 messages 索引（0 起始） | 结构事实 |
| ---: | ---: | --- |
| 107745012942 | 0 | 三个纯 reminder 文本块 + 一个 90 字符普通文本块 |
| 107745012942 | 488 | 单个 925 字符纯 reminder 文本块；前一消息为 assistant |
| 107745012942 | 620 | 848 字符纯 reminder + 49 字符普通文本 |
| 107745012942 | 689 | 单个 929 字符纯 reminder 文本块 |
| 124298870556 | 282 | 两个纯 reminder 文本块 + 一个 8 字符普通文本块 |
| 124298870556 | 318、324、330、336、342 | 各自是仅含纯 reminder 的 user 消息，前一消息为 assistant |

其他两条相邻请求重复携带上述历史；早些的 `offset=8334277225` 也含 reminder。五条完整历史里均未发现 reminder 与 tool_result 同处一条 user 消息。

因此可以确认生产请求存在纯 reminder 和 reminder+普通文本结构；**不能升级为“已验证 tool_result+reminder 同条混合”**。也不能仅凭包装就证明内部永远没有用户意图；设计已记录客户端可能将排队用户消息包在 reminder 内的识别限制。

## 与当前代码及交付的关系

- 已核对代码：`x-openai-subagent` 与 `subagent_kind` 分别匹配；system/developer 先于 item type 跳过。15 个生产案例形态及 `additional_tools` 回归已存在。
- 272 条脱敏结构回放证明规则对重建结构的一致性，不是对 272 份原始请求正文的直接执行，也不是人工来源准确率评估。
- 本轮只运行只读证据采集和数据完整性检查，不重复执行此前已通过的业务测试套件；先前测试结果沿用复核者提供的记录。
- `tool_result+reminder` 的真实末尾混合实例仍待命中；历史回填仍待受控生产写入安排。未 commit、未部署、未执行生产索引。
