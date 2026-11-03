# 2026-11-02 Token Analysis 生产补录验收

用户授权连接 118 补录，并明确“恢复不了就算了，能恢复的就恢复”。仅恢复可由原文确认的分析数据；未重发上游请求、重新计费、删除原始归档、重启服务或部署在线代码。

## 结果（北京时间 21:10）

| 项目 | 结果 |
| --- | --- |
| 原始归档 | `2026-10-31.jsonl`，78,020,680,200 bytes；扫描前后 size/mtime 不变 |
| 完整扫描 | 25,238 条正常 request、25,224 条正常 response、5 条损坏物理行；耗时 883 秒，低 CPU/IO 优先级 |
| 坏行抢救 | 每条均为截断 request + 后续完整 JSON；找回后续 2 条 request、3 条 response，按实际字节位置和哈希重放 |
| 新增摘要/用户输入 | 92 条摘要及 92 条净输入（包含上述 2 条抢救请求） |
| 恢复响应事实 | 86 条（包含上述 3 条抢救响应）；另 1 条候选已在 pending 表，未重复写入 |
| 排除误修 | 388 条无用户输入的既有摘要经当前解析规则验证属正常，不修改 |
| 计费关联 | 新增摘要中 71 条按 request_id 精确关联既有账单；21 条仍无已确认账单，不推测补造 |
| 不完整证据 | 5 条 request 正文已截断，无法恢复；新增摘要中 14 条无完成响应事实；3 条可读 response 因请求正文缺失保留在 pending 表 |
| 全文件对账 | 可读/抢救后的 25,240 条 request 全部有摘要，source offset/body hash 冲突 0；25,227 条 response 全部有完成事实或 pending 记录，缺失 0 |
| 再次预检 | 178 项全部 skip，新增写入候选 0；没有再次执行写入来测试幂等 |
| 状态校正 | 10-31 processed：25,148 → 25,240；failed：178 → 5；last_error 改为截断残留说明。最近 50 水位失败合计：184 → 11（5 + 更早的 6） |
| 运行验证 | sub2api/PostgreSQL active，`/health` 返回 ok；生产工作树 clean，版本仍为 `70e90d706` |

178 次补录操作与原失败计数相同只是本次结果：其中有坏行后缀抢救，不应将错误计数解释为请求数。原始正文丢失和缺响应不等于网关调用失败；这里只恢复归档分析证据。

## 方法与保护

- 生产原始文件位于 `/opt/sub2api/backend/data/request-archive/2026-10-31.jsonl`。操作及私有证据目录为 `/var/tmp/token-analysis-repair-20261102`（目录 0700）；原始正文始终留在服务器。
- 从生产 `HEAD` 用 `git archive` 建独立源码副本；补录桥接代码只加入该副本，不改在线进程源码。复用现有解析、脱敏、指纹、候选边、response 和 usage-link 实现及运行时配置。指纹密钥仅在服务器内从既有 security_secrets 读取，不新建、不输出。
- `scan.py` 生成行位置/哈希清单；`salvage_suffixes.py` 仅认可独立可解析的完整后缀；`build_plan.py` 对照只读数据库快照生成候选；临时 Go helper 默认使用 `default_transaction_read_only=on` 预检。
- 正式执行只保留预检的 92 request + 86 response；现有请求候选的 body hash/source file/source offset 已核对无冲突。每项写前将既有派生记录落入 0600 的 journal 并 fsync。补录会拒绝源文件大小/mtime/行哈希或身份变化、已存在的冲突响应；重复项跳过。用户原始 JSONL 不改写。
- 补录后用 `verify_repair.py` 对全部可读事件核对，再次运行只读 helper 确认所有 178 项 skip。最后以原 checkpoint 的 offset/processed/failed/last_error 作 CAS，仅校正 10-31 水位的 processed、failed、last_error、updated_at；未重置 offset 或删除水位。
- 恢复计费关联只选本次修复 archive_id；正常 `MergePendingResponses` 同时运行其已有合并/过期清理逻辑。分析关联不创建 usage_logs、不改用户余额。3 条孤立 pending 沿用已有 7 天 TTL，原始文件仍保留。
- 原水位：`state-before.json`；结果：`state-after.json`；逐项前像/应用日志：`apply-*.jsonl`；前后只读审计：`audit-*.jsonl`；完整偏移清单、不可恢复片段和 SQL 快照均保留在服务器私有目录。不要将含用户输入的前像 journal 发布入库，也不要将其全表覆盖恢复到线上。

## 验证与复现

隔离源码基线为生产 `70e90d706`。把本目录 `repair_bridge.go.txt`、`repair_bridge_test.go.txt` 分别复制为 `internal/service/token_analysis_repair_bridge.go`、`internal/service/token_analysis_repair_bridge_test.go`；把 `repair_main.go.txt` 复制为 `cmd/token-repair/main.go`，均相对于隔离 backend 根目录。

执行 `GOMAXPROCS=2 go test -p 2 ./internal/service -run '^TestRepair' -count=1 -v`：5 项通过。

| 测试 | 验证内容 |
| --- | --- |
| ReaderRejectsDriftAndWrongIdentity | 原文变化、错误身份、偏移不一致拒绝 |
| PartialWriteResumesWithoutRewritingSummaryOrWatermark | 摘要已写而输入失败，可仅补输入，不重写摘要或水位 |
| ResponseFailureCanRetryAndPreservesFacts | response 写入故障可重试，输出 token/响应 ID 保留 |
| CharacterizesDeployedIndexerSkippingFailedResponse | 复现现网缺陷：response 失败后仍返回成功并越过水位；该测试不是长期修复已完成的证明 |
| LinksOnlySelectedRows | 计费关联只处理本次选择的 archive_id |

构建临时 helper：`GOMAXPROCS=2 go build -p 2 -o <private-dir>/token-repair ./cmd/token-repair`。`run_helper.py` 默认只读；`--apply` 是写入开关。本目录脚本固定本次日期/路径，只用于复现审查，不是已接入产品的通用重试入口；不要无审查地对其他日期运行。

浏览器复核未完成：iab 返回 `browser.user is unavailable`；按规则回退本地 Chrome DevTools 后，连接在 60 秒超时。11 次失败及恢复数据由生产只读 SQL 验证，未将浏览器连接失败冒充页面验证成功。

长期修复及故障注入矩阵见 `docs/features/token-analysis-recovery-design-cn.md`；尚未修改/部署在线归档 writer、索引器和 UI。
