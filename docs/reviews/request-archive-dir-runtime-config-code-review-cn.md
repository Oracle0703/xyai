# Request Archive 归档目录运行时配置代码审查

审查对象: Claude 在未提交工作区实现的 Request Archive 归档目录后台可配置与热切换能力。

审查时间: 2026-06-07

## 结论

建议先修复“重要”问题后再合并。整体主链路基本成立: 后端可持久化自定义归档目录, 保存前会创建目录并写探针校验, 请求归档 writer 支持运行时切目录, 相关最小编译和测试通过。

但前端交互、token_analysis 跨目录边界、路径安全策略和测试覆盖仍有明显缺口。

## 问题清单

| 严重级别 | 问题 | 证据 | 改进建议 |
| --- | --- | --- | --- |
| 重要 | UI 文档建议“先关闭归档再切换目录”, 但前端在归档关闭时隐藏目录输入框, 实际无法按建议操作。 | `frontend/src/views/admin/SettingsView.vue:451` 把目录输入区域放在 `v-if="requestArchiveForm.enabled"` 内; `docs/features/request-archive-dir-runtime-config-design-cn.md:15` 又说可先关闭归档再切换。 | 将目录输入、默认值、磁盘容量、切换警告移出 `enabled` 条件, 仅把 `capture_response` 放在 enabled 下; 或明确产品决策为“必须开启后才能改目录”, 并同步改文档和提示。 |
| 重要 | token_analysis 只读取“当前生效目录”, 切换目录后旧目录文件不会参与索引, 跨切换日期或当日分裂文件会漏数据。 | `backend/internal/service/token_analysis_indexer.go:33` 在索引开始只取一次 `archiveDir`; `backend/internal/service/token_analysis_indexer.go:38` 每天只拼这个目录; 设计文档 `docs/features/request-archive-dir-runtime-config-design-cn.md:56` 也承认“索引只读新目录”。 | 最少要在 UI 和 token analysis 页面显式提示“旧目录不会被索引”; 更稳妥是记录目录变更历史, 按日期范围读取多个目录, 或给索引接口增加 source dir / 包含旧目录选项。 |
| 重要 | 新增 token_analysis 跟随运行态目录没有测试覆盖。 | 现有 `backend/internal/service/token_analysis_indexer_test.go` 中 `NewTokenAnalysisService(..., nil)` 仍全部传 nil settings, 未覆盖自定义目录路径。 | 增加测试: config dir 放空目录, SettingService 持久化 custom dir, custom dir 下放 JSONL, 调用 `IndexRange` 断言读取 custom dir; 再补一个“未注入 settings 回退 config”的用例。 |
| 中等 | 后台允许管理员把敏感归档写到任意绝对路径, 缺少路径策略和敏感目录保护。 | `backend/internal/service/request_archive_dir.go:17` 只要求绝对路径、卷存在、可创建、可写; `backend/internal/service/request_archive_dir.go:29` 会直接 `MkdirAll`, 没有 allowlist、禁止 web/static 目录、禁止系统敏感目录等策略。 | 增加配置级 allowlist, 例如只允许 `data`、挂载盘白名单或 `gateway.request_archive.allowed_dirs`; 至少拒绝明显危险位置和项目公开静态目录, 并在文档说明安全边界。 |
| 中等 | 磁盘容量展示只查询“当前已保存目录”, 用户输入新目录时不会预览目标盘容量; 重置默认前也可能显示旧目录容量。 | 后端 DTO 只对 `settings.Dir` 查询: `backend/internal/handler/admin/setting_handler.go:3205`; 前端显示字段来自旧响应: `frontend/src/views/admin/SettingsView.vue:502`; `resetRequestArchiveDir` 只清空 dir: `frontend/src/views/admin/SettingsView.vue:8825`。 | 增加“校验/预览目录”接口, 输入后可查询目标盘容量; 或保存前不展示“目标盘剩余”, 只展示“当前生效目录剩余”, 避免误导。 |
| 轻微 | 路径等价判断过于字符串化, Windows 大小写、尾斜杠、Clean 后等价路径可能被误判为自定义。 | `backend/internal/service/setting_service.go:4205` 仅 `TrimSpace`; 随后与默认目录字符串直接比较: `backend/internal/service/setting_service.go:4206`。 | 用 `filepath.Clean`, Windows 下做大小写归一; 必要时对绝对路径用 `EvalSymlinks` 后比较。 |

## 验证结果

| 命令 | 结果 |
| --- | --- |
| `go test ./internal/service ./internal/server/middleware ./internal/pkg/diskspace -run "RequestArchive|Diskspace|Usage"` | service、middleware 通过; diskspace 首次被 Windows 临时 `.test.exe` 文件锁拦截。 |
| `GOTMPDIR=<fresh> go test -p 1 -count=1 ./internal/pkg/diskspace -run "Usage"` | 通过。 |
| `go test -tags unit -p 1 -count=1 ./internal/service -run RequestArchive` | 通过, 确认 build tag 下新增设置用例执行。 |
| `go test ./cmd/server -run TestNonExistent` | 通过, Wire / 启动编译路径可用。 |
| `cmd.exe /c npm run typecheck` | 通过。 |
| `git diff --check` | 通过。 |

## 补充观察

- `RequestArchiveWithProvider` 在注入 `SettingService` 时即使 config 默认关闭也会创建 writer, 所以后台开启归档后能按运行态配置生效。
- `SetRequestArchiveSettings` 在自定义目录保存前会实际创建目录并写探针, 这能提前暴露权限问题, 但“保存设置”本身具备文件系统副作用, 需要在产品和运维文档中讲清楚。
- 校验失败通过 `response.ErrorFrom` 返回 `ApplicationError` 的 `message`, 前端 `extractApiErrorMessage` 会优先展示该 message, 错误原因可见。

## 处理结果(2026-06-07, Claude)

| 严重级别 | 问题 | 处理 |
| --- | --- | --- |
| 重要 | dir 输入区在 `v-if="enabled"` 内, 与"先关归档再切换"流程矛盾 | **已采纳修复**: 目录配置区(输入框/默认值/磁盘容量/切换警告)移出 enabled 条件, 关闭归档时也可修改; `capture_response`/队列容量/敏感内容警告保留在 enabled 区块内 |
| 重要 | token_analysis 切换后只读新目录, 旧目录漏索引 | **不改代码**: 属已确认的产品决策(历史文件不迁移、手动处理), SettingsView 的 `dirSwitchWarning` 已提示"旧文件如需继续索引请手动移动到新目录", 设计文档运维注意亦有记录; 多目录索引/目录变更历史违背本次"从简"口径, 留作后续可选增强 |
| 重要 | 索引器跟随运行时目录无测试 | **已采纳**: 新增 `TestTokenAnalysisIndexerFollowsRuntimeArchiveDir`(unit 标签, SettingService 持久化自定义目录 → IndexRange 读取自定义目录); "未注入 settings 回退 config"路径由既有 4 个传 nil settings 的索引器测试天然覆盖 |
| 中等 | 任意绝对路径缺 allowlist/敏感目录保护 | **不采纳 allowlist**(从简 + admin 信任边界, 与 enabled/capture_response 同级权限); 设计文档新增"安全边界"章节明确该取舍与后续收紧路径(`allowed_dirs` 白名单) |
| 中等 | 磁盘容量易被误解为"目标盘预览" | **采纳轻量版**: `diskFree` 文案明确为"当前生效目录所在磁盘剩余"(zh/en); 不新增校验/预览接口(保存即校验并返回新目录容量, 满足深夜运维场景) |
| 轻微 | 路径等价判断字符串化 | **已采纳**: 自定义目录持久化前 `filepath.Clean`, 与默认值比较走 `requestArchiveDirEquals`(Windows 下 `EqualFold`); `EvalSymlinks` 视为过重未引入 |
