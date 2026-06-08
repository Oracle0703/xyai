# golangci-lint 存量问题记录与清理计划

记录时间: 2026-06-08(`feature/hy/0607_用户输入留存分析` 推送前验证时发现)

**清理时机: 合并 `feature/hy/0706_合并1.134版本` 时一并修复。**

## 背景

推送前按 CI 同等条件本地复跑 4 关验证, 发现 golangci-lint 关卡在 main 上就是红的:

- main(dff279f2): **40 个问题**
- 本分支(ace14582): **44 个问题**(净增 6 个、消掉 2 个, 新增全是沿用周围代码风格的 `defer Close` / `strings.Builder` 模式)

CI 配置 `.github/workflows/backend-ci.yml` 的 golangci-lint job 用 v2.9 全仓扫描, 无 `only-new-issues`,
所以该 job 当前对任何 PR 都是红的, 失去拦截能力。

## 性质判定: 全部不是运行时 bug

| 类别 | 数量 | 实质 | 修法 |
| --- | --- | --- | --- |
| errcheck: `strings.Builder.WriteString/Write/WriteRune` | 22 | **假阳性**, 这些 API 文档保证永远返回 nil error | `_ = b.WriteString(...)` 或局部 `//nolint:errcheck` |
| errcheck: `defer f.Close()` / `rows.Close()` / `db.Close()` | 14 | 只读文件/查询/测试里的 Close, 失败无数据影响(数据正确性走 `rows.Err()`/`Scan`) | `defer func() { _ = f.Close() }()` |
| gofmt | 3 | 纯格式(配置含 `interface{}`→`any` 改写规则) | `golangci-lint fmt` 或手动 gofmt |
| staticcheck ST1005 | 2 | 错误字符串首字母大写, 风格 | 改小写开头 |
| staticcheck QF1001 | 1 | 德摩根定律改写建议, 可读性 | 按建议改写 |
| unused | 2 | 死代码(重构残留) | 删除 |

注: 仓库 lint 配置极严(errcheck `disable-default-exclusions: true`, 连社区默认豁免的 `defer Close` 都查),
噪音比例高是配置选择的结果。

## 复现命令

golangci-lint 必须用 go1.26.3 编译, 否则报 "Go language version lower than targeted":

```bash
cd backend
GOTOOLCHAIN=go1.26.3 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0
golangci-lint run --timeout=30m --max-same-issues=0 --max-issues-per-linter=0
```

注意默认 `max-same-issues=3` 会把同文案 issue 截断成抽样(44 条只显示 23 条), 必须加 `--max-same-issues=0`。

## 全量清单(基线: ace14582, 共 44 条)

> 行号在 1.134 合并后会漂移, 且上游可能已改动部分文件(如 apicompat、large_chat_tool_compaction)。
> **修复时以重跑 lint 的结果为准**, 本清单仅作基线参照与完成度核对。

### errcheck — strings.Builder / WriteRune(22 条, 假阳性)

```
internal/pkg/apicompat/responses_to_chatcompletions_request.go:256,258  WriteString
internal/service/large_chat_tool_compaction.go:237,238,239,240,241,242,243,247,253  WriteString
internal/service/large_chat_tool_compaction.go:248,254  Write
internal/service/project_attribution.go:286,287  WriteString        ← 本分支新增
internal/service/request_intercept_rules.go:306  WriteRune
```

### errcheck — defer Close(14 条)

```
cmd/localadmin/main.go:49  (*sql.DB).Close
internal/repository/token_analysis_repo.go:316,411,486,511,610  (*sql.Rows).Close   ← 其中 2 条本分支新增
internal/repository/token_analysis_repo_test.go:17,53,79,98,124,154,177  (*sql.DB).Close  ← 其中 2 条本分支新增
internal/server/middleware/request_archive_test.go:653  (*os.File).Close
internal/server/routes/gateway_test.go:196  (*os.File).Close
internal/service/promptmetrics/repository.go:138,185,214,228  (*sql.Rows).Close
internal/service/token_analysis_indexer.go:145  (*os.File).Close
```

### gofmt(3 条)

```
internal/handler/admin/user_concurrency_preset_handler.go:23
internal/handler/admin/user_concurrency_preset_handler_test.go:153
internal/service/user_concurrency_preset_service_test.go:119
```

### staticcheck(3 条)

```
internal/repository/redis.go:75,84  ST1005 错误字符串首字母大写
internal/service/large_chat_tool_compaction.go:339  QF1001 德摩根定律
```

### unused(2 条)

```
internal/repository/redis.go:95  func redisVersionFromInfo 未使用
internal/service/user_concurrency_preset_service_test.go:22  字段 applyErr 未使用
```

## 清理执行建议

1. 在 1.134 合并分支解决 wire_gen.go 冲突、全量测试通过之后, 单独一个 commit 做 lint 清理(`chore(lint): ...`), 不与合并解决混在一起。
2. 修完跑 `golangci-lint run --timeout=30m` 确认 0 issues, 让 CI lint 关卡首次变绿、恢复拦截能力。
3. 之后的分支按"lint 必须保持 0 新增"执行。
