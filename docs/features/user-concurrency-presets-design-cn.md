# 用户并发预设设计

更新：2026-11-04。状态：**已合入 main（提交 `45ccd3eb6` 仓储、`1b871c96b` 应用与定时、`7e4c43717` 接口、`1330d12ef` 前端、`fbe235e85` 空列表修复），迁移 `142_user_concurrency_presets.sql` 已存在。本文按 HEAD `26f6363af` 的代码核对；原设计中“预设不存在返回 404”未实现（§5.3、§9）。**

本文是用户并发预设功能的唯一文档，包含需求、规则、接口、数据模型、代码位置、易错点和验证方式。原设计与执行计划已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:docs/superpowers/specs/2026-05-23-user-concurrency-presets-design.md`、`git show 26f6363af:docs/superpowers/plans/2026-05-23-user-concurrency-presets.md` 查看）。执行计划已全部落地，代码为准。

阅读顺序建议：
- 改应用或定时逻辑：先读 §3、§8；
- 改接口：读 §5；
- 定位代码：读 §7；
- 跑测试：读 §9。

## 1. 需求与范围

管理员保存“一组用户 + 一个目标并发值”的预设，可手动应用，也可每天在固定时间自动应用一次。预设只改所选用户的 `users.concurrency`。

| 包含 | 不包含 |
| --- | --- |
| 预设 CRUD（管理员） | 账号并发、用户 RPM |
| 目标用户以显式 user_id 列表保存 | 按标签、备注、API Key、用量规则自动选人 |
| 每个预设一个目标并发值 | cron 表达式、按周调度 |
| 手动应用；可选每日 `HH:mm` 定时 | 定时补跑（错过当分钟不补） |
| 每次应用写执行记录；成功后失效受影响用户的鉴权缓存 | |

## 2. 数据模型与迁移

迁移 `backend/migrations/142_user_concurrency_presets.sql`（受 checksum 校验，不可修改）：

`user_concurrency_presets`

| 列 | 说明 |
| --- | --- |
| `id`、`name`、`description` | `name` 非空由服务层校验；`description` 默认空串 |
| `target_concurrency` | `CHECK (>= 1)` |
| `user_ids` | JSONB 数组，默认 `[]` |
| `schedule_enabled`、`schedule_time` | `schedule_time` 为 `HH:mm` 文本，默认空串 |
| `last_scheduled_run_date` | DATE，最后一次**成功**定时应用的本地日期 |
| `created_at`、`updated_at` | |

部分索引 `idx_user_concurrency_presets_schedule (schedule_enabled, schedule_time) WHERE schedule_enabled = true`。

`user_concurrency_preset_runs`

| 列 | 说明 |
| --- | --- |
| `preset_id` | 外键，`ON DELETE CASCADE`（删除预设连带删除执行记录） |
| `trigger` | `manual` / `scheduled`（CHECK） |
| `target_concurrency`、`user_ids` | 应用时的快照（`user_ids` 为预设原始列表，不是过滤后的有效列表） |
| `affected_count` | `UPDATE` 实际影响行数 |
| `status` | `success` / `failed`（CHECK） |
| `error_message`、`started_at`、`finished_at`、`created_at` | |

索引 `idx_user_concurrency_preset_runs_preset_created (preset_id, created_at DESC)`。

## 3. 业务规则

### 3.1 校验（`validatePreset`，创建与更新）

- `name` 去空白后非空；`target_concurrency >= 1`；`user_ids` 归一化（`normalizeInt64IDs`）后非空；
- `schedule_enabled=true` 时 `schedule_time` 必须匹配 `HH:mm`（`userConcurrencyPresetScheduleTimePattern`）；
- 目标用户经 `validTargetUserIDs` 校验：查不到的用户（含已删除）跳过；**任一目标是管理员（`RoleAdmin`）则整体拒绝**；剩余有效用户为 0 时拒绝（`no valid target users`）。

### 3.2 应用（`applyPreset`）

1. 再次执行 `validTargetUserIDs`（用户状态可能在保存后变化）。失败则写一条 `failed` 执行记录并返回错误。
2. `userRepo.BatchSetConcurrency(validUserIDs, target)`：`UPDATE users SET concurrency = $1, updated_at = NOW() WHERE id = ANY($2) AND deleted_at IS NULL`。失败写 `failed` 记录。
3. 成功写 `success` 记录，然后对每个有效用户调用 `InvalidateAuthCacheByUserID`，使网关鉴权缓存中的并发值立即失效。
4. 部分目标缺失不算失败：只要还有至少一个有效非管理员用户就继续，缺失的用户体现在执行记录 `user_ids` 与 `affected_count` 的差异中（没有逐个列出缺失原因）。

应用不在事务中：用户更新成功而写执行记录失败时，返回记录写入错误，但并发值已经生效。

### 3.3 定时（`UserConcurrencyPresetRunner`）

- `ProvideUserConcurrencyPresetRunner` 构造并启动；`github.com/robfig/cron/v3` 每分钟一次（`* * * * *`），时区为服务的 `location`：配置 `timezone`（`cfg.Timezone`）可解析时用它，否则服务器本地时间。runner 在 `cmd/server/wire.go` 的停机清理列表中，`Stop` 最多等 3 秒。
- 每次 tick：`CurrentScheduleKey(now)` 得到本地 `HH:mm` 与本地日期 → `RunDueSchedules` → 仓储 `ListDue` 查 `schedule_enabled = true AND schedule_time = 当前分钟 AND (last_scheduled_run_date IS NULL OR last_scheduled_run_date < 今天)`。
- 只有执行记录 `status=success` 才 `MarkScheduledRun` 写入当天日期；失败的预设当天在同一分钟内不会再被选中（下一分钟 `schedule_time` 不再匹配），即**失败不重试、错过不补跑**。
- 单个预设失败不影响其他预设；`MarkScheduledRun` 失败会中止本轮剩余预设并记日志。runner 只记日志，不会让服务崩溃。每轮超时 1 分钟。

## 4. 前端

- 入口：`frontend/src/views/admin/UsersView.vue` 的“并发预设”操作，打开 `frontend/src/components/admin/user/UserConcurrencyPresetsDialog.vue`。
- 对话框：预设列表（含最近执行状态）、创建/编辑表单、用户选择器（复用现有管理员用户搜索 API，显示已选摘要）、目标并发数字输入、每日定时开关与 `HH:mm` 输入（前端先校验）、带确认的手动应用。
- API：`frontend/src/api/admin/userConcurrencyPresets.ts`（经 `frontend/src/api/admin/index.ts` 导出）。
- 列表接口返回 `null` 时按空列表处理（`fbe235e85` 修复；仓储 `List` 也保证返回非 nil 切片）。

## 5. 接口合同

基础路径 `/api/v1/admin/users/concurrency-presets`（`backend/internal/server/routes/admin.go` 的 `registerUserManagementRoutes`，位于 admin 路由组内）。

### 5.1 路由

| 方法与路径 | handler | 说明 |
| --- | --- | --- |
| `GET /` | `List` | 预设列表，按 `created_at DESC, id DESC` |
| `POST /` | `Create` | 创建 |
| `PUT /:id` | `Update` | 更新 |
| `DELETE /:id` | `Delete` | 删除（连带删除执行记录） |
| `POST /:id/apply` | `Apply` | 手动应用，返回执行记录 |
| `GET /:id/runs?limit=` | `ListRuns` | 最近执行记录，默认 20，`1–100` 之外的值忽略 |

### 5.2 请求体（创建/更新）

```json
{
  "name": "工作日白天",
  "description": "",
  "target_concurrency": 5,
  "user_ids": [12, 34],
  "schedule_enabled": true,
  "schedule_time": "09:00"
}
```

响应体字段与 `service.UserConcurrencyPreset` / `UserConcurrencyPresetRun` 的 JSON tag 一致（`id`、`name`、`description`、`target_concurrency`、`user_ids`、`schedule_enabled`、`schedule_time`、`last_scheduled_run_date`、`created_at`、`updated_at`；执行记录另含 `preset_id`、`trigger`、`affected_count`、`status`、`error_message`、`started_at`、`finished_at`）。

### 5.3 错误

| 场景 | 当前行为 |
| --- | --- |
| JSON 解析失败、创建/更新校验失败 | 400，消息为 `Invalid request: ...` 或服务层错误文本 |
| 非法 `:id` | 400 `Invalid preset ID` |
| 手动应用失败（含预设不存在、无有效用户、含管理员） | 400，消息为错误文本；失败执行记录已写入（预设不存在时除外） |
| 更新不存在的预设 | 仓储 `UPDATE ... RETURNING` 扫描得到原始 `sql.ErrNoRows`，handler 返回 400 |
| 查询不存在预设的执行记录 | 返回空列表 |
| 删除不存在的预设 | `DELETE` 影响 0 行也返回成功 |

原设计“预设不存在返回 404”未实现：所有不存在场景都是 400、空列表或成功。

## 6. 配置

无专用配置项。定时时区取全局 `timezone` 配置，未配置或无法解析时用服务器本地时间。

## 7. 代码地图

| 位置 | 内容 |
| --- | --- |
| `backend/migrations/142_user_concurrency_presets.sql` | 两张表与索引 |
| `backend/internal/service/user_concurrency_preset.go` | `UserConcurrencyPreset`、`UserConcurrencyPresetRun`、`UserConcurrencyPresetRepository`、trigger/status 常量 |
| `backend/internal/service/user_concurrency_preset_service.go` | `NewUserConcurrencyPresetService`、`ListPresets`、`CreatePreset`、`UpdatePreset`、`DeletePreset`、`ListRuns`、`ApplyPreset`、`RunDueSchedules`、`CurrentScheduleKey`、`applyPreset`、`validatePreset`、`validTargetUserIDs` |
| `backend/internal/service/user_concurrency_preset_runner.go` | `UserConcurrencyPresetRunner`（`Start`、`Stop`、`runOnce`） |
| `backend/internal/service/wire.go`、`backend/cmd/server/wire.go` | `ProvideUserConcurrencyPresetRunner` 与停机清理 |
| `backend/internal/repository/user_concurrency_preset_repo.go` | `NewUserConcurrencyPresetRepository`、`GetByID`、`List`、`ListDue`、`Delete`、`MarkScheduledRun`、`CreateRun`、`ListRuns`、`scanUserConcurrencyPreset` |
| `backend/internal/repository/user_repo.go` | `BatchSetConcurrency` |
| `backend/internal/handler/admin/user_concurrency_preset_handler.go` | `UserConcurrencyPresetHandler`（`List`、`Create`、`Update`、`Delete`、`Apply`、`ListRuns`）、`userConcurrencyPresetRequest`、`parsePresetID` |
| 前端 | `frontend/src/api/admin/userConcurrencyPresets.ts`、`frontend/src/components/admin/user/UserConcurrencyPresetsDialog.vue`、`frontend/src/views/admin/UsersView.vue` |

## 8. 关键约束与易错点

1. **管理员不能作为预设目标**，且是整体拒绝而非跳过。依据：预设会批量改并发，误把管理员纳入可能限制运维账号。
2. **应用成功后必须失效鉴权缓存**，否则网关继续使用缓存中的旧并发值，直到缓存过期。
3. **只有成功的定时应用才写 `last_scheduled_run_date`**；改这里要保证“每天最多一次”和“失败不标记”同时成立。
4. **定时只匹配当前分钟**：服务在设定分钟内停机、或该分钟 tick 被跳过，当天不会补跑；失败也不会在当天重试。需要补跑时手动应用。
5. **执行记录的 `user_ids` 是预设原始列表**，有效目标数要看 `affected_count`；`BatchSetConcurrency` 会再次排除 `deleted_at` 非空的用户。
6. **时区**：`CurrentScheduleKey` 与 cron 都使用服务 `location`；`ListDue` 用 `$2::date` 比较本地日期（有专门测试 `TestUserConcurrencyPresetRepositoryListDueUsesLocalDate`），不要改成 UTC 日期。
7. 预设之间没有互斥：两个预设覆盖同一用户时，后执行的生效；同一分钟内按 `id ASC` 执行。
8. 应用后用户在用户管理页手动改并发，会被下一次定时应用覆盖。

## 9. 验证

### 9.1 命令

```bash
cd backend
go test -tags=unit ./internal/service/ -run UserConcurrencyPreset
go test -tags=unit ./internal/repository/ -run UserConcurrencyPreset
go test -tags=unit ./internal/handler/admin/ -run UserConcurrencyPreset
pnpm --dir ../frontend exec vitest run src/components/admin/user/__tests__/UserConcurrencyPresetsDialog.spec.ts
```

测试文件没有 build tag，默认 tag 与 `unit` tag 下都会编译。

### 9.2 测试清单

- 服务：`TestUserConcurrencyPresetServiceCreateValidation`、`TestUserConcurrencyPresetServiceRejectsAdminTargets`、`TestUserConcurrencyPresetServiceApplyPresetSetsConcurrencyAndInvalidatesAuthCache`、`TestUserConcurrencyPresetServiceApplyPresetLogsFailure`、`TestUserConcurrencyPresetServiceRunDueSchedulesMarksOnlySuccessfulRuns`。
- 仓储（sqlmock）：`TestUserConcurrencyPresetRepositoryCreateAndGet`、`TestUserConcurrencyPresetRepositoryListDueUsesLocalDate`、`TestUserConcurrencyPresetRepositoryListEmptyReturnsNonNilSlice`、`TestUserConcurrencyPresetRepositoryRunLifecycle`、`TestUserConcurrencyPresetRepositoryGetByIDNotFound`（只断言返回错误，不断言 404 映射）。
- handler：`TestUserConcurrencyPresetHandlerCreateAndApply`、`TestUserConcurrencyPresetHandlerCreateValidation`、`TestUserConcurrencyPresetHandlerJSONShape`。
- 前端：创建预设（选中用户与每日定时）、`null` 列表按空处理、非法时间被拦截、确认后应用。

### 9.3 最近结果

本次文档合并未运行；没有保留的历史运行记录。

## 10. 状态与待办

- [ ] 待决定：预设不存在时是否按原设计返回 404（需在仓储把 `sql.ErrNoRows` 映射为 not-found 错误，并调整 `Apply` 不再一律返回 400）。
- [ ] （可选）执行记录逐个列出被跳过的用户及原因。
- [ ] （可选）定时错过补跑或失败重试。
