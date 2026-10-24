# Token Analysis 计费关联修复：第四次独立复核

日期：2026-10-25（Asia/Shanghai）。审核对象：`feature/hy/10218_organization_usage_rollup`、HEAD `bc45a7e03298d0a1643452b8cf12629edefa8eb3` 上 Claude 最新未提交修改。源码 SHA-256 见交付目录 `usage-link-review4-source-hashes.json`。

## 结论

**本地审核通过。本轮未发现新的实质遗漏，上一轮两类问题的三个原始失败场景全部通过。** 当前永久回归覆盖前几轮的关联准确性、并发认领、跨日恢复、失败请求及风险重算；本轮实际运行均通过。

该结论针对上述源码快照与本机隔离验证。未连接生产、未部署、未执行历史回填，尚不能把它写成生产原始样本已恢复。

## 上轮问题闭环证据

| 问题 | 当前实现 | 独立复跑结果 |
| --- | --- | --- |
| T1 晚到账单失去跨日期重试 | 完成事实变化或账单被夺回时登记版本及 48 小时截止时间；未找到精确账单的带 client key、未失败请求保留待复核状态 | 响应先索引时 pending=true；账单后到，再索引响应日/无关日期可正确关联 |
| T2a 旧 timing 覆盖较晚到达的失败响应 | Set 同时比较旧关联状态与读取时复核版本，关联和清空分支采用相同条件 | 首次 502 到达后旧写入 Applied=false、pending=true；正常复核后没有错误账单 |
| T2b 旧确认吞掉精确夺回产生的新任务 | Clear 只确认读取版本；夺回为被释放行递增版本 | 旧确认不消除新任务；后续索引将旧行的账单风险从 70 分降为 0 |

关键位置：`backend/internal/service/token_analysis_usage_link.go:101`、`:125`、`:133`、`:153`；`backend/internal/repository/token_analysis_repo.go:174`、`:216`、`:447`、`:517`、`:529`。行号对应本轮源码快照。

原始复现仅适配新接口/列合同，未改变触发顺序和预期：

- 读取 pending 布尔列改为比较 `usage_link_version <> usage_link_checked_version`。
- 旧 Clear 传入 A 当时读取的版本。
- T2b 中合法认领方 B 使用写回自身 response 后、查找账单前读取的状态；T2a 的旧状态仍为响应到达前的版本 0。

## 重试和迁移合同

- 48 小时从登记复核事件的数据库当前时间起算，历史请求也能获得完整窗口；相同完成事实重放不反复续期。
- 带关联键、未失败请求在期限内查不到精确账单时保留跨日期重试。期满仍无证据则确认未关联；以后明确选择请求日期仍可重新查询，不表示零费用。
- 无 client key 的历史行按既定合同完成一次复核，不进入长期跨日期重试。新归档路由先运行 ClientRequestID，正常新请求具有此键。
- migration 247 的三个复核字段、248 的版本差部分索引、runner 的无效索引清理已接通。本轮新空集群自动应用迁移成功；247/248 尚未部署，不涉及修改已在生产执行的 migration。
- 版本条件保护 Set 和 Clear；精确认领的目标条件更新成功后才释放他行，失败会回滚，不会因陈旧目标误释放别人的账单。

## 验证

| 检查 | 结果 |
| --- | --- |
| 上轮原始失败复现 | 3/3 通过 |
| 正式 PostgreSQL 专项 | 17 个顶层用例通过（16 个关联/来源用例及 1 个输入质量用例） |
| 数据库测试合计 | 20 个顶层用例全部通过，无数据库跳过 |
| Go TokenAnalysis / RequestArchive 专项 | service、repository、handler/admin、server/middleware 四包通过；默认 vet 随 go test 执行 |
| 前端页面/API/locale 专项 | 4 文件、49 测试通过 |
| 前端 typecheck、相关 ESLint | 通过 |
| git diff --check、证据校验、wiki LF | 通过 |

正式回归还覆盖重试期限结束后的确认，以及跨日风险重算时保留 body 自身风险。前端既有 Browserslist 提示与测试 stub 的 Vue emits 警告没有导致失败。本轮没有执行生产负载或线上浏览器验收。

主要命令：

```text
go test -p 1 -count=1 ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/middleware -run 'TokenAnalysis|RequestArchive'
go test -tags integration -p 1 -count=1 ./internal/repository -run 'TokenAnalysisUsageLink|TokenAnalysisListRequestsFiltersByOrigin|TokenAnalysisUserInputQuality|^TestReview3' -v
```

## 证据与剩余操作

目录：`docs/delivery/2026-10-24-token-origin-human-input/`。

| 文件 | 内容 |
| --- | --- |
| `usage-link-review4-repros.go.txt` | 适配新接口后的上轮 3 个原始复现 |
| `usage-link-review4-integration-results.txt` | 20 项数据库测试通过输出 |
| `usage-link-review4-unit-results.txt` | Go 四包专项通过输出 |
| `usage-link-review4-frontend-results.txt` | 前端 49 项通过输出 |
| `usage-link-review4-source-hashes.json` | 审核源码快照 |

复跑原始场景时把 .go.txt 临时复制到 `backend/internal/repository/token_analysis_review4_temp_test.go`，使用独立测试 DSN 运行 `-run '^TestReview3'`。harness 会自动迁移并写测试数据，不得指向生产或未知数据库。

本轮新建 PostgreSQL 18.3 空集群仅监听 127.0.0.1:55487，已停止；临时 .go 测试文件已移除。未修改业务代码、未提交、未推送。生产部署、受控历史回填和原始 archive 样本的上线后验收仍未执行。
