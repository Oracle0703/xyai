# 给 Claude 的请求来源识别抽样证据

2026-10-24 02:10:43（Asia/Shanghai）重新只读采样。此目录供复核，不修改分类器、生产文件或数据库。

## 先看结论边界

已重现上一轮的 **272 条请求、259/8/5 端点分布和全部末尾结构计数**。这是一组真实归档的脱敏结构证据，不是人工逐条标注的真值集，也没有在本轮把当前正在修改的 Go 分类器跑出的结果当作真值。

**优先看样本 #193**：普通 user 文本 + 图片，JSON 内容带 `stage=preflight` 和合成预检目标，却没有自动任务元数据。现有 user-input 规则倾向标人工，但实际发起者不能仅凭结构确认。#175 的翻译模板也有同类歧义。不能据此宣布识别准确率，更不能把 28 条 user 末尾请求直接称为 28 次人工输入。

## 文件入口

| 文件 | 用途 |
| --- | --- |
| `review-cases.json` | 15 条优先复核案例；每条包含原始定位信息、最后 6 个事件的脱敏结构、关键 metadata 和复核提示 |
| `samples.json` | 全部 272 条及 16 个采样窗口、源文件前后 stat、精确计数 |
| `sample_archive.py` | 原地只读采样脚本，仅向 stdout 输出脱敏 JSON；未在服务器创建脚本文件 |
| `checksums.json` | 上述三个文件的 SHA-256，用于固定这次交付 |

`sample` 编号从 1 开始，`offset` 为源 JSONL **0 起始字节偏移**；`tail[].index` 为原请求数组 **0 起始索引**。`line_bytes_including_lf` 与 `line_sha256_including_lf` 均包含行末 LF。可按 offset 直接 seek 回查，不需要扫描 123 GiB 文件。归档 ID 保留供精确关联，但没有导出用户 ID、邮箱、IP、headers、metadata 中的会话 ID。

正文、工具参数/结果、加密内容、图片字节均未导出。`text` 对象是从原文计算的长度、标签和整段包装等事实，不是请求原 JSON。`review_assessment` / `review_note` 是人工复核提示，不是运行当前分类器的输出。`known_prefix` 的固定前缀定义可在脚本中核对。

## 源文件及采样方法

| 项目 | 实测值 |
| --- | --- |
| 源文件 | `/opt/sub2api/backend/data/request-archive/2026-10-23.jsonl`（不是 `.json1`） |
| 文件大小 | 132,582,213,280 bytes |
| inode | 1062370 |
| 采样前后 stat | size、mtime_ns、inode 完全相同，完整值保存在 `samples.json` |
| 锚点 | `int(file_size * i / 16)`，i=0..15；非零位置先丢弃至下一换行的片段 |
| 每窗口 | 最多 20 条 request；循环开始前检查累计读入是否小于 32 MiB |
| 单次 readline 上限 | 20 MiB；不带 LF 即终止该窗口，不把残片作为 JSON 行 |
| 计数选择 | 只计 `event=request`；读取的 response 行不进入样本 |
| 采样正文循环读取量 | 463,449,925 bytes，与上一轮完全一致 |
| 对齐行边界时丢弃的读取量 | 68,768,109 bytes |
| 本次实际读取量 | **532,218,034 bytes**，即以上两者之和 |

**口径更正**：上一轮文档中的 463,449,925 bytes 只统计了正文循环，没有计入 seek 后的行边界对齐读取，不能称为全部 I/O。32 MiB 是循环开始前的预算检查，最后一行可使实际窗口读入量超出预算；不是每窗口严格最多 32 MiB。

窗口 #10 在偏移 `82,868,811,620` 读满 20 MiB 后仍未遇到行尾，停止该窗口。该处距离文件尾尚远，这是本次采样的行长限制，不能据此说原归档损坏或请求本身被截断。已选入的 272 条中 `body_truncated=true` 为 **0**。

各窗口请求数（i=0..15）：`19,20,14,9,20,19,12,15,20,20,6,20,18,20,20,20`，合计 272。

该方法不是按请求均匀随机抽样：字节锚点、时段密度、长行停止和窗口预算均会影响选中概率。因此不可外推当天人工/AI 占比。

## 精确结构计数（互斥，合计 272）

| 原始请求最后一项 | 条数 | 可支持的结论 |
| --- | ---: | --- |
| Responses `custom_tool_call_output` | 156 | 明确工具结果 |
| Responses `function_call_output` | 54 | 明确工具结果 |
| Chat `role=tool` | 6 | 明确工具结果 |
| Responses `agent_message` | 22 | 明确 Agent 消息 |
| Messages `role=system` | 5 | 5 条的前一条均为 user，内容块全部是 `tool_result` |
| Responses `role=developer` | 1 | 图片缩放通知；前一条是工具输出 |
| `role=user` | 28 | 包括环境注入、审批、定时任务和普通用户输入，不能直接算人工 |

端点：Responses 259、Chat Completions 8、Messages 5。`client_metadata.x-openai-subagent`：`collab_spawn=88`、`guardian=6`、`memory_consolidation=3`、其余无标记 175；此分组与末尾结构表重叠，不能相加。

## 15 条重点案例

时间及行哈希见 JSON。下列“建议判断”用于审核规则，不声称已经确认真实操作者。

| sample | 字节偏移 | 事实 | 建议判断 / 审核点 |
| ---: | ---: | --- | --- |
| 1 | 0 | `collab_spawn`、`composer`，末尾 `custom_tool_call_output` | AI；composer 不是当前请求人工凭证 |
| 2 | 488477 | `goal`，工具结果后追加纯 environment user 消息 | AI；环境注入应跳过 |
| 5 | 2264277 | `composer/user`，agent_message 后追加纯 environment user 消息 | AI；不能只看 role=user |
| 9 | 4608281 | 普通 user 正文，`automation_cron_scheduled` | 自动任务 |
| 15 | 23943546 | `thread_source=user`，末尾 `agent_message` | Agent 通信 |
| 23 | 8296888915 | `composer/user`，末尾 `function_call_output` | AI 工具续执行 |
| 32 | 8305117097 | `guardian/guardian_review`，末尾 15 个 input_text 块，role=user | 自动审批，不能判人工 |
| 153 | 74598984812 | 工具输出后追加 developer 图片缩放提示 | 跳过 developer 后为 AI |
| 175 | 91151684578 | 单条 user 翻译模板，无机器元数据 | 只能推断；可能手动，也可能程序生成 |
| 184 | 91156051244 | `guardian_classifier`，user 文本 + 图片 | 图片不代表人工，属于自动审批 |
| **193** | **91169004481** | user 文本 + 图片；JSON 含 preflight、goal、设备/画面字段；无机器元数据 | **重要歧义：普通 user 结构不足以确认人工** |
| 210 | 99471474288 | 历史 tool/assistant 后新增 user 包装，`userRequest` 114 字符 | 新用户输入结构；不能因历史工具判 AI |
| 219 | 107727454269 | 最新 `userRequest` 23 字符，上一条 14 字符；其余大部分是附件 | 应识别最新输入，不能按总长度/重复附件猜 |
| 230 | 107745012942 | assistant tool_use → user tool_result → system 尾注 | AI 工具续执行 |
| 259 | 124309436659 | user 正文，`turn_trigger=thread_title` | 自动标题任务 |

## 本样本没有证明的范围

- 没有 Gemini 请求、multipart 请求、`count_tokens` 请求，且没有已选入的 body_truncated 请求。这些实现边界依靠构造测试，不是这 272 条真实样本验证通过。
- 导出的最后 6 个事件中没有发现“纯 `<system-reminder>` 文本块”的样本；不能引用本包来宣称已验证 tool_result 与 system-reminder 混合的生产实例。
- `x-codex-turn-metadata` 在 252 条中是字符串。另 20 条 `turn_metadata_encoding=dict` 是采集器对缺失/空值的 `{}` 兜底，不能当成生产对象格式的验证证据。
- 实际观察到 `x-openai-subagent=collab_spawn` 时，嵌套 `subagent_kind` 可为 **thread_spawn**；字段不是同一套值域，不应未经检查互相替换。
- 当前工作区正在由 Claude 复核修改。本包独立记录原始证据，不固定当前代码的行为，也没有覆盖正在进行的代码更改。

## 重现与逐条核验

用已有 SSH 登录通道，把 `sample_archive.py` 的内容作为 `python3 -` 的 stdin；stdout 保存到本机 JSON。这样无需在服务器写入脚本。脚本默认目标就是上述 2026-10-23 文件，也接受显式路径参数。

如果只核验一个案例，在服务器 Python 中执行以下只读代码。先从本机 JSON 复制该条 offset/length/hash 常量，勿把脱敏 tail 当成原始 body 回放：

```python
import hashlib, json
path = '/opt/sub2api/backend/data/request-archive/2026-10-23.jsonl'
offset = 91169004481  # sample 193
length = 9681
with open(path, 'rb') as f:
    f.seek(offset)
    raw = f.read(length)
assert raw.endswith(b'\n')
print(hashlib.sha256(raw).hexdigest())  # 与 line_sha256_including_lf 精确比较
row = json.loads(raw)
assert row['archive_id'] == 'ed96e18ebf86058c48a8f15bc142bd98'
# 按复核需要在本机/授权终端检查 json.loads(row['body'])；勿将整行含正文日志发布出去。
```

本轮已核验：源 stat 稳定、272 个 offset 无重复、端点计数和末尾结构计数闭合、选中 15 例均来自同一 272 条样本；脚本输出通过 JSON 解析，交付文件 SHA-256 已固定。生产全程只读。

## 后续 Messages 定向检查

追加证据见 [messages-audit.md](messages-audit.md)：另做 1 GiB 有界读取，命中 11 条 Claude CLI Messages 请求；其末尾没有 tool_result+纯 reminder 混合实例。对其中 5 条完整历史的检查确认存在纯 reminder 以及 reminder+普通文本，但同条工具结果混合仍未实证。该追加结果不改写上面的原 272 条采样或其校验和；新增文件单独由 `messages-checksums.json` 固定。
