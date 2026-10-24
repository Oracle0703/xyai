# Token Analysis 历史索引错误定位

日期：2026-10-25（Asia/Shanghai）。范围：复用右侧已登录浏览器，只读检查页面及自身轮询响应，对照本地源码并离线复现；未修改生产、未触发索引、未发送模型请求。业务代码未改动。

## 线上事实

页面：`http://118.25.112.30:52997/admin/token-analysis`。读取页面自身的 `GET /api/v1/admin/token-analysis/index/status` 和 `GET /api/v1/admin/token-analysis/archive-files` 响应，均为 HTTP 200。

- 标签上的 **4 是现存归档文件数**，来自 `archiveFiles.length`，不是错误数量。
- 状态接口返回最近更新的 50 个文件水位，合计已处理 1,950,638 行、失败 11 行；这个合计不是当前日期筛选的统计，也不保证覆盖全部历史。
- 当前目录列出的 `2026-10-25.jsonl`、`2026-10-24.jsonl`、`2026-08-09.jsonl`、`2026-05-28.jsonl` 全部 `failed_rows=0`、`last_error=""`。
- 页面顶层的 `invalid character 'm'` 来自 `2026-10-22.jsonl` 的历史 `last_error`。该文件最后更新水位为 2026-10-23 01:16:31；水位更新时间不等于错误发生时间。

| 历史文件 | 累计失败行 | 该文件最后记录的错误 |
| --- | ---: | --- |
| 2026-10-22.jsonl | 1 | `invalid character 'm' looking for beginning of object key string` |
| 2026-10-21.jsonl | 1 | `invalid character '(' looking for beginning of value` |
| 2026-10-14.jsonl | 4 | `invalid character '\x00' in string` |
| 2026-09-09.jsonl | 2 | `invalid character 'm' looking for beginning of object key string` |
| 2026-09-07.jsonl | 3 | `invalid character '\ufeff' looking for beginning of value` |

每个文件只有一个最后错误，不能据此断言该文件全部失败行均为同一原因。这 5 个文件均未出现在当前目录列表；是否已经删除、迁移或有备份，本轮未核实。没有读取到这些历史失败请求的原文，不能确定具体调用方、完整请求或返回状态。

## 触发链与影响

1. `backend/internal/server/middleware/request_archive.go` 在 `c.Next()` 前捕获原始请求体，并以字符串字段写入 JSONL；因此后续处理不接受的请求体也可能已被归档。
2. `backend/internal/service/token_analysis_summary.go#SummarizeTokenAnalysisRequest` 对归档行内的 `body` 执行 `json.Unmarshal`。`parse request body:` 前缀来自这里，表示内层请求正文解析失败，不能据此认定整个 JSONL 文件损坏或归档落盘失败。
3. `backend/internal/service/token_analysis_indexer.go#indexArchiveLine` 已有 multipart 和 `body_truncated` 的处理；未截断且非 multipart 的非法 JSON 返回错误，不写入该请求摘要。
4. `indexArchiveFile` 累加失败数、保存错误并前移 offset，继续处理后续行。重新点击增量索引不会重试这条旧记录。
5. `backend/internal/repository/token_analysis_repo.go#UpdateIndexState` 使用 `COALESCE(NULLIF(...), last_error)` 保留历史错误；`GetIndexStatus` 在最近更新的 50 个水位中取首个非空错误作为顶层 `last_error`。
6. `frontend/src/components/admin/token-analysis/tokenAnalysisFormat.ts` 在非运行态只要看到 `last_error` 就判为 `error`；`TokenAnalysisIndexPanel.vue` 无条件显示错误，同时最近水位只显示前 5 个文件；View 又把现存文件数与异常标记并排显示，容易误解为“当前 4 个文件出错”。

`{model:"example","input":"hello"}` 可精确复现本次 `'m'` 错误，合法写法为 `{"model":"example","input":"hello"}`。这证明该语法形态能触发错误，不证明历史原文一定如此；对象中间缺少字段名引号等其他位置同样可能触发。

影响位于离线分析：该失败请求缺少分析摘要，后续正常记录继续索引。这条索引错误不等于在线模型请求失败，也不能用于推断当时是否计费。

## 建议的最小修改

| 优先级 | 修改 | 约束 |
| --- | --- | --- |
| 1 | 前端明确显示“归档文件 4”“历史失败 11”；单独列出有失败的历史文件及错误，错误旁显示源文件；文案说明统计覆盖最近 50 个水位 | 可复用当前接口；不要把历史错误显示为当前轮次故障，不隐藏或清空历史记录 |
| 1 | 补充真正的本轮执行状态/错误字段，区分运行中、正常结束、有坏请求行、任务执行失败 | 不能仅凭旧 `last_error` 判断任务健康；实际本轮错误需要后端提供权威状态 |
| 2 | 保存失败定位信息：源文件、该行起始 byte offset、archive_id、错误分类、失败时间、JSON 错误位置 | 现有 `last_offset/last_archive_id` 会被后续成功行覆盖，不能用作失败行位置；避免记录完整请求正文或密钥 |
| 2 | 查到原始调用方后修正 JSON 序列化方式，使用标准序列化库，避免手工拼接或 shell 引号丢失 | 不自动给坏 JSON 补引号，不把 `body_truncated=true` 当成掩盖任意格式错误的手段 |
| 后续按需 | 若需要在请求明细中保留非法请求，可设计“仅元数据摘要 + 明确 parse_error 标记” | 来源保持 unknown，不能伪造用户正文/风险结论或覆盖历史评分；需单独测试计费关联和失败计数合同 |

历史恢复依赖原始文件或备份。不要直接清空索引水位或重扫所有大文件来消除提示；应先找到失败行并按明确方案受控重处理。原始非法请求不会因再次运行相同解析器自动变好。

## 本地验证

使用 Go overlay 在忽略目录下临时追加诊断测试，没有改写仓库测试文件。合成归档包含一条缺少字段名引号的请求和随后一条合法请求，验证错误字符串、失败计数、后续成功入库与水位推进至 EOF。配套检查既有截断、multipart、真实坏正文、历史错误保留和历史中断水位测试。

命令使用仓库规定的缓存目录与独立 GOTMPDIR：

```text
go test -overlay .gocache/diagnose-archive-20261025/overlay.json -p 1 -count=1 ./internal/service ./internal/repository -run 'TestArchiveMalformedBodyDiagnosis|TestTokenAnalysisIndexerCountsUntruncatedBadBodyAsFailed|TestTokenAnalysisIndexerDegradesTruncatedBodyToMetadata|TestTokenAnalysisIndexerIndexesMultipartImageEditRow|TestTokenAnalysisRepositoryUpdateIndexStateKeepsLastError|TestTokenAnalysisRepositoryGetIndexStatusIgnoresStaleUnfinishedRows' -v
```

结果：6 项检查全部通过，service 与 repository 两包退出码均为 0；合成坏请求计失败 1 行，随后合法请求成功索引 1 行，水位推进至文件末尾 494 bytes。

本机复现与测试日志：`backend/.gocache/diagnose-archive-20261025/`（gitignore，本机保留）。这是合成数据的本地验证，不是历史原文回放或线上修复验收。
