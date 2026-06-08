# Request Archive 归档目录后台可配置(热切换)设计

## 背景

归档目录 `gateway.request_archive.dir` 此前由 config.yaml 写死(默认 `data/request-archive`, 落在 backend 部署盘), 修改需重启。归档 JSONL 增长很快(项目归因功能要求归档常开), 部署盘容量有限, 公司计划扩盘后把归档放到新的大磁盘。

需求: 管理后台 Settings 页的 Request Archive 区块把归档目录从只读改为可编辑, 保存时校验路径有效性(磁盘/卷存在、目录可创建、可写), 展示目标磁盘剩余空间, 保存后无需重启即切换写入位置。

## 产品决策

| 决策点 | 结论 |
| --- | --- |
| 历史文件迁移 | **不自动迁移**, 页面提示手动处理; token_analysis 索引按 archive_id 幂等, 手动移动后重新索引不会产生重复 |
| 磁盘剩余空间 | 接口返回当前生效目录所在磁盘总容量/剩余空间, 页面展示 |
| 并发复杂度 | 从简: 切换在深夜低峰执行, 可先关闭归档再切换; 写入端仅做"目录变化时轮转文件句柄" |
| 路径策略 | 自定义目录必须为绝对路径(相对路径含义随部署方式漂移); 空串 = 恢复 config 默认 |

## 方案

复用 `enabled/capture_response` 既有的运行时配置链路(DB key `request_archive_settings` → 5 秒 TTL 进程缓存 `GetRequestArchiveRuntimeConfig` → 中间件每请求读取), dir 走同一条链路:

### 1. 持久化与校验(`internal/service/setting_service.go`)

- `persistedRequestArchiveSettings` 增加 `dir`(空 = 沿用 config 默认)。
- `RequestArchiveSettings` 增加 `DefaultDir`(config 值)与 `DirCustomized`。
- `SetRequestArchiveSettings`: 自定义目录保存前经 `ValidateRequestArchiveDir`(`internal/service/request_archive_dir.go`)校验, 失败不落库:
  1. `filepath.IsAbs` — 必须绝对路径;
  2. 卷/磁盘存在 — `os.Stat(VolumeName + /)`(Windows 盘符填错/新盘未挂载的典型场景);
  3. 路径不能是已存在文件; 目录不存在则 `MkdirAll`;
  4. 写探针 — 创建并删除临时文件确认可写。
- 与 config 默认值相同的输入按"恢复默认"处理, 避免把默认值固化为自定义。

### 2. 磁盘容量(`internal/pkg/diskspace/`)

`Usage(path) (total, free, err)`: Windows 走 `GetDiskFreeSpaceEx`, 其余平台走 `unix.Statfs`, build tag 分文件。GET/PUT 响应中尽力附带当前生效目录的磁盘容量(查询失败为 0, 不阻塞接口)。

### 3. 写入端热切换(`internal/server/middleware/request_archive.go`)

- `asyncRequestArchiveWriter.dir` 由固化字符串改为 `atomic.Value` + `SetDir`(值未变化时一次原子读即返回)。
- 中间件每请求取运行态配置后调用 `writer.SetDir(runtimeCfg.Dir)`。
- `fileForToday` 轮转条件由"跨天"扩展为"跨天 或 目录变化", 切换后下一条记录即写入新目录。
- `mergeRequestArchiveRuntimeConfig` 放开 Dir 合并(原注释明确固化的限制随本次解除); QueueSize 仍固化。

### 4. 索引器跟随(`internal/service/token_analysis_indexer.go`)

`TokenAnalysisService` 注入 `*SettingService`, `archiveDir(ctx)` 优先取运行态目录, 与写入端同源; 未注入(测试)回退 config。

### 5. 接口与前端

- `GET/PUT /api/v1/admin/settings/request-archive`: DTO 增加 `default_dir` / `dir_customized` / `disk_total_bytes` / `disk_free_bytes`; PUT 请求体 `dir` 为可选指针(省略=不修改, 空串=恢复默认, 非空=自定义)。校验失败返回 400 + 具体原因(盘不存在/不可写/是文件/非绝对路径)。
- SettingsView: dir 输入框 + 「恢复默认」按钮 + 默认值与磁盘剩余展示 + 切换须知警告(历史文件不迁移、建议低峰先关归档)。

## 运维注意

- 切换不迁移历史 JSONL; 旧文件如需继续被 token_analysis 索引, 手动移动到新目录后重新触发索引(幂等)。
- 当日文件在切换瞬间会"分裂"在新旧两个目录, 索引只读新目录——这正是建议"先关归档、低峰切换"的原因。
- 服务重启后自定义目录仍生效(持久化在 DB), config.yaml 的 dir 仅作为默认值。

## 安全边界

- 目录配置为管理员专属接口(admin-only), 信任边界与 `enabled`/`capture_response` 一致: 能改这两个开关的人本就能让敏感请求体落盘, 目录可配置不扩大该权限的影响面。
- 自定义目录仅做"绝对路径 + 卷存在 + 可创建 + 可写"校验, **不做 allowlist/敏感目录黑名单**(从简决策, codex 审查中等级别建议, 暂不采纳): 管理员需自行避免把归档指向 Web 静态目录或系统目录。若后续要收紧, 可加 `gateway.request_archive.allowed_dirs` 配置级白名单。
- "保存设置"会产生文件系统副作用(创建目录 + 写探针), 校验失败不落库。

## 验证

| 验证 | 方式 |
| --- | --- |
| 设置服务 | `go test -tags unit ./internal/service/ -run RequestArchive`(自定义目录持久化/空串恢复默认/相对路径与文件路径拒绝/校验失败不污染) |
| 写入端热切换 | `go test ./internal/server/middleware/ -run TestRequestArchiveHotSwitchesDir`(provider 切目录后新记录落新目录、旧目录不增长) |
| 磁盘容量 | `go test ./internal/pkg/diskspace/` |
| 前端 | `vue-tsc --noEmit` |
| 端到端 | 后台改 dir 为他盘绝对路径 → 保存成功且显示剩余空间 → 发请求新目录出现 JSONL; 填不存在盘符/相对路径 → 400 带原因; 空串 → 恢复默认 |
