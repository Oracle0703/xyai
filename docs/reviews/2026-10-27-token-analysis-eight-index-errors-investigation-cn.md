# Token Analysis「失败 8」复核与根因

日期：2026-10-27（Asia/Shanghai）；线上取证时间约 11:10–11:15。范围：右侧浏览器读取页面及自身 GET 响应，SSH 只读核对索引状态表、文件目录与生产源码；未触发索引、修改生产数据、重启或部署。

## 结论

页面的「失败 8」是最近更新的 50 个文件水位中的累计失败行数，不是本次索引新增 8 次失败，也不是 8 行都发生同一种 `'m'` 错误。当前目录 4 个归档文件的失败数全部为 0。顶层错误来自 `2026-10-22.jsonl` 保留的历史错误。

问题分为两层：历史归档中存在无法按 JSON 解析的请求正文；状态展示又把保留的历史错误作为当前「索引异常」。此外，失败合计只覆盖最近 50 个水位，旧文件退出窗口后数字会下降，并不代表修复。

## 线上证据

`GET /api/v1/admin/token-analysis/index/status?timezone=Asia%2FShanghai` 返回 HTTP 200：`running=false`、`processed_rows=1969577`、`failed_rows=8`、50 个文件水位，`updated_at=2026-10-27T11:10:39.932878+08:00`。

| 页面计数涉及的历史文件 | 累计失败行 | 该文件最后保留的错误 | 水位最后更新时间（北京时间） |
| --- | ---: | --- | --- |
| `2026-10-22.jsonl` | 1 | `parse request body: invalid character 'm' looking for beginning of object key string` | 10-23 01:16:31 |
| `2026-10-21.jsonl` | 1 | `parse request body: invalid character '(' looking for beginning of value` | 10-22 00:11:31 |
| `2026-10-14.jsonl` | 4 | `parse request body: invalid character '\x00' in string` | 10-15 09:44:46 |
| `2026-09-09.jsonl` | 2 | `parse request body: invalid character 'm' looking for beginning of object key string` | 09-10 11:44:30 |
| 合计 | **8** | 每个文件仅存最后一个错误，不能证明同文件全部失败行都由该错误造成 | 水位更新时间不是错误发生时间 |

归档列表接口也为 HTTP 200：当前 `2026-10-27.jsonl`、`2026-10-26.jsonl`、`2026-08-09.jsonl`、`2026-05-28.jsonl` 均为 `failed_rows=0`、空 `last_error`。其中 10-26 已处理 59,951 行并读至 EOF；10-27 已处理 14,283 行，仍在写入。标签旁的 4 是现存文件数。

只读查询 `token_analysis_index_state` 全表共 **146** 条水位、累计 **14** 条失败。另有 6 条在页面 50 条窗口之外：

| 其他历史文件 | 累计失败行 | 最后错误 |
| --- | ---: | --- |
| `2026-09-07.jsonl` | 3 | 请求体开头存在 `\ufeff`，JSON 解析失败 |
| `2026-06-10.jsonl` | 2 | 摘要入库时 PostgreSQL 拒绝 NUL：`invalid byte sequence for encoding "UTF8": 0x00` |
| `2026-06-04.jsonl` | 1 | 用户输入入库时 PostgreSQL 拒绝 NUL |

10-25 调查时显示 11，当前显示 8：`2026-09-07.jsonl` 现在位于按 `updated_at DESC` 排序的第 51 位，其 3 条失败退出接口窗口。数据库仍保留这些失败，未因数字减少而恢复。两个六月错误与本次请求体 JSON 解析错误属于不同阶段，不混作同一根因。

## 源码链路

生产 HEAD 为 `25bedc620d9783e065ec6efae45d56f984b6b01c`。下列摘要、索引器、仓储、前端状态函数及归档中间件的生产工作文件均与该提交逐文件 SHA-256 相同；当前本地风险分类分支另有未提交改动，不将其视为已部署代码。

| 环节 | 源码与行为 |
| --- | --- |
| 捕获 | `backend/internal/server/middleware/request_archive.go` 在 `c.Next()` 前捕获原始请求体；外层记录通过 `json.Marshal` 写入 JSONL。外层 JSON 合法，不代表其中字符串形式的 `body` 也是合法 JSON |
| 解析 | 生产提交的 `backend/internal/service/token_analysis_summary.go:39` 对 `body` 执行 `json.Unmarshal`，失败加上 `parse request body:` 前缀 |
| 跳过 | `backend/internal/service/token_analysis_indexer.go#indexArchiveLine` 对非 multipart、未截断且解析失败的请求返回错误；`indexArchiveFile` 累加失败数，推进 offset，继续后续行 |
| 留存 | `backend/internal/repository/token_analysis_repo.go#UpdateIndexState` 通过 `COALESCE(NULLIF(EXCLUDED.last_error, ''), ...last_error)` 保留旧错误；成功行和正常收尾不会清除它 |
| 汇总 | `GetIndexStatus` 只取 `ORDER BY updated_at DESC LIMIT 50`，累计其中失败数，首个非空旧错误成为顶层 `last_error`；不随页面日期范围过滤 |
| 展示 | `frontend/src/components/admin/token-analysis/tokenAnalysisFormat.ts:113` 在非运行态只要有 `last_error` 就返回 `error`；面板直接显示该错误，最近水位仅展示前 5 条，因而看不到实际出错历史文件 |

`'m' looking for beginning of object key string` 表示解析器在期待带双引号的对象字段名时遇到了 `m`。合成例子 `{model:"example","input":"hello"}` 可以精确复现；合法形式是 `{"model":"example","input":"hello"}`。这是语法机制验证，不是历史请求原文，不能据此断言一定是 `model` 字段或某个调用客户端。

## 定位边界与影响

- 线上当前归档目录已无这四个出错文件，也无这些日期的同名前缀压缩副本。已检查的 `/opt/sub2api/backups`、`/opt/backups` 不存在；未做全盘或外部备份搜索，不能断言所有备份均不存在。
- 状态表仅保存文件累计失败数与最后错误；后续成功行覆盖 `last_offset/last_archive_id`，无法从现有水位确定失败请求 ID、调用方或原始正文。
- 对正文解析失败行，缺少 Token Analysis 摘要，后续合法行继续处理。这个错误本身不能证明在线模型响应或计费失败。
- 当前按钮执行增量续读，既不会重新处理已越过的失败行，也不会清除历史错误。重复点击不能修复原始非法 JSON；不应为消除提示而清空水位或失败统计。

## 建议处理

| 优先级 | 处理 |
| --- | --- |
| 1 | 区分本轮任务状态与历史坏行；历史错误展示源文件、失败计数及统计覆盖范围，保留诊断记录 |
| 1 | 将全历史失败总数独立聚合，或明确标注「最近 50 个文件累计失败」；不能让随列表滚动而减少的数值被理解成已修复 |
| 2 | 持久化失败事件的源文件、行起始偏移、archive_id、失败时间与 JSON 错误位置，避免后续成功行覆盖定位信息；不记录完整正文或凭据 |
| 2 | 原始归档/备份可恢复时，定点核对发送端序列化；使用标准 JSON 序列化，避免手拼 JSON 或 shell 引号丢失。不要在索引器中猜测补引号 |

本轮仅定位，以上产品修复未实施。

## 验证

- 线上两个 GET 接口成功；数据库会话显式 `default_transaction_read_only=on`、`BEGIN READ ONLY`、15 秒 statement timeout、2 秒 lock timeout，`ROLLBACK` 结束，只读状态返回 `on`。
- 生产服务仍为 active，PID `3522741`，启动于 2026-10-26 09:55:27；本轮没有服务操作。
- 本机标准库 Go 合成复现：缺少字段名双引号得到精确 `'m'` 错误；合法 JSON 正常解析。未向生产发送合成请求。
- 只新增调查文档和更新 wiki 诊断约束，执行对应文档差异/LF 检查；未重跑全量业务测试。此前索引跳过、继续推进及历史错误留存的合成回归见 `docs/reviews/2026-10-25-token-analysis-index-errors-investigation-cn.md`。
- 本机只读取证脚本/输出：`E:/tmp/token-index-diagnosis/2026-10-27/`；未导出请求正文或认证凭据。
