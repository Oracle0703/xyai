# 部门负责人管理本部门成员：实施任务卡

面向执行编码的 AI 代理。唯一需求依据是 [设计文档](department-member-management-design-cn.md)（下称"设计"）；本文只规定执行顺序、固定接口、锁定测试、门禁和交付格式，不重复业务规则。两者冲突时停下报告，不要自行取舍。

分支：`feature/hy/10226_department_member_management`。不推送、不合并、不部署。

## 0. 开工前必须阅读

1. `AGENTS.md`、`llm-wiki/wiki/README.md`，以及 `backend.md`、`frontend.md`、`security-and-reliability.md`、`ops.md` 中的部门章节。
2. 设计全文，重点是第 2、3、3.1、4、5 节和第 12 节审核结论 R1–R4。
3. 现有实现：`backend/internal/repository/department_repo.go`、`department_query_scope.go`、`department_subscription_scope.go`，以及 `backend/internal/service/department_service.go`、`admin_permission.go`、`frontend/src/components/admin/department/DepartmentAccessDialog.vue`。

## 1. 硬性规则

| 规则 | 说明 |
| --- | --- |
| 锁定测试不可改 | 第 3 节列出的测试文件不得删除、跳过、放宽断言或改名。唯一允许的修改是：阶段 2 开始时删除 `department_member_management_integration_test.go` 构建行里的 ` && deptmembers_pending`。若认为某条锁定断言与设计矛盾，停止并在报告中说明，不要自行修改 |
| 只改清单内文件 | 只改设计第 8 节和本文第 4 节列出的文件，以及因签名变化必须机械更新的调用点和测试。改动清单外的文件，需在报告中逐个说明理由 |
| 复用，不新建层 | 必须复用 `loadDepartmentActor`、`lockDepartmentUsers`、`departmentAudit`、`departmentPredicates`、`departmentMemberFrom`、`organizationUsageOrganizationExpression`、`organizationUsageSearchPattern`、`normalizeDepartmentList`、现有严格 JSON 解码、`departmentErrorKey`。禁止新建 helper 包、通用框架、DTO 转换层或"以后可能用到"的参数 |
| 不顺手重构 | 不改无关代码的格式、命名或结构；不修复设计范围外的问题，发现时写进报告 |
| 不照搬 `Assign` 的空操作规则 | 负责人路径的重复操作语义以设计第 4.1 节为准（R4） |
| 迁移只新增 | 不改已应用迁移；新迁移编号取实现时的最大值 +1（当前为 `261`），文件名 `261_department_member_management.sql` |
| 不提交密钥 | 测试和文档中不得出现真实凭据 |

## 2. 动手前先交两份清单

阶段 1 写代码前，先在报告草稿中列出以下两份清单，并逐条标注处理方式。写完清单再开始编码，不需要等待确认。

1. **所有写 `users.status` 的位置**：Go、原生 SQL、迁移、导入、测试辅助。在 `backend/internal`、`backend/cmd`、`backend/migrations` 里搜索 `SetStatus(`、`Status: true`、`status=`、`status =`。每一处标明它最终是否经过 `UserRepository.Update(fields.Status)` 或第 3 节的触发器兜底。
2. **所有改 `users.department_id` 的位置**：确认都会触发备注清空触发器。

## 3. 锁定测试（当前全部失败，完成后必须全部通过）

| 文件 | 构建标签 | 覆盖 | 当前状态 |
| --- | --- | --- | --- |
| `backend/internal/repository/department_member_schema_integration_test.go` | `integration` | 停用来源触发器、`UserRepository.Update` 显式停用写 other（R1）、负责人备注清空与审计、长度 CHECK | 3 个用例因缺列失败 |
| `backend/internal/service/content_moderation_department_disable_test.go` | 默认 | 已停用用户达到封禁阈值时仍须写入 | 失败 |
| `backend/internal/repository/department_member_management_integration_test.go` | `integration && deptmembers_pending` | 权限组合、加入/移出与重试（R4）、版本 CAS（R2）、启用边界（R1）、资料与备注、名单可见性、候选搜索 | 新接口未定义，编译失败 |
| `frontend/src/components/admin/department/__tests__/DepartmentAccessManageMembers.spec.ts` | — | 授权弹窗不隐式加权限（R3） | 4 个用例失败 |

第一个文件中的 `departmentMemberFixture` 由第三个文件共用，不要复制或改动它。

## 4. 固定接口（锁定测试直接依赖，名称和语义不得改变）

### 4.1 数据库

| 对象 | 约定 |
| --- | --- |
| `users.disabled_source` | `VARCHAR(32) NULL`；CHECK：`status='disabled'` 时取值为 `department_manager` 或 `other`，否则必须为 NULL；迁移内回填既有 disabled 为 `other` |
| 来源触发器 | `BEFORE INSERT OR UPDATE OF status, disabled_source`：转为非 disabled 时清空；非 disabled → disabled 时，未显式写 `department_manager` 一律记为 `other`；disabled → disabled 时允许 `department_manager` → `other`，`other` → `department_manager` 必须 `RAISE`（SQLSTATE 23514）；其他列的更新不改变来源 |
| `users.department_note` | `TEXT NOT NULL DEFAULT ''`，CHECK `char_length(department_note) <= 1000` |
| 备注触发器 | 名称 `users_department_note_guard`，`BEFORE UPDATE OF email, department_id`；`NEW.department_id IS DISTINCT FROM OLD.department_id` 时置空；旧值非空时插入审计：`action='department.note_cleared'`、`auth_method='database_guard'`、`method='INTERNAL'`、`status_code=200`，`extra` 包含 `user_id`、`before_department_id`、`after_department_id`、`department_note`（旧值） |

### 4.2 Go（`backend/internal/service`）

```go
const AdminPermissionDepartmentMembers = "admin.department_members" // 目录项 MenuKey "department_members"，Route "/admin/my-department"

var ErrDepartmentMemberEnableDenied = infraerrors.Forbidden("DEPARTMENT_MEMBER_ENABLE_DENIED", "member was disabled by an administrator or the system")

// DepartmentAccessInput 增加字段；nil 表示保留现状，非 nil 按值授予或撤销。
ManageMembers *bool `json:"manage_members"`

type ManagedDepartmentMember struct {
	ID                int64   `json:"id"`
	Email             string  `json:"email"`
	Username          string  `json:"username"`
	DepartmentNote    string  `json:"department_note"` // 仅 Manageable=true 时填充
	Status            string  `json:"status"`
	DisabledSource    *string `json:"disabled_source"`
	CanEnable         bool    `json:"can_enable"`
	Manageable        bool    `json:"manageable"`
	DepartmentID      int64   `json:"department_id"`
	DepartmentName    string  `json:"department_name"`
	DepartmentVersion int64   `json:"department_version"`
}

type ManagedDepartmentMemberList struct {
	Items    []ManagedDepartmentMember `json:"items"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
}

type ManagedMemberUpdateInput struct {
	ExpectedDepartmentID      *int64  `json:"expected_department_id"`
	ExpectedDepartmentVersion *int64  `json:"expected_department_version"`
	Username                  *string `json:"username"`
	DepartmentNote            *string `json:"department_note"`
	Status                    *string `json:"status"`
}

func NewDepartmentService(repo DepartmentRepository, authCacheInvalidator APIKeyAuthCacheInvalidator) *DepartmentService
func (s *DepartmentService) ManagedScope(ctx context.Context) (*DepartmentScope, error)
func (s *DepartmentService) ManagedMembers(ctx context.Context, f DepartmentListFilter) (*ManagedDepartmentMemberList, error) // DepartmentID 为 "all" 或数字，不要求 Organization
func (s *DepartmentService) ManagedCandidates(ctx context.Context, departmentID int64, q string) ([]DepartmentMember, error)
func (s *DepartmentService) ManagedAssign(ctx context.Context, in DepartmentAssignInput) (int, error)
func (s *DepartmentService) UpdateManagedMember(ctx context.Context, userID int64, in ManagedMemberUpdateInput) (*ManagedDepartmentMember, error)
```

- `NewDepartmentService` 改为两个参数后，机械更新所有现有调用点（测试传 `nil`）并重新生成 Wire。`authCacheInvalidator` 为 nil 时跳过失效调用。
- 错误映射：越权、目标不可管理、无权限或无授权返回 `ErrDepartmentScopeDenied`；版本冲突返回 `ErrDepartmentConflict`；参数非法（缺版本、用户名超过 100 个 UTF-8 字节即 `len(username) > 100`、备注超过 1000 个 Unicode 字符即 `utf8.RuneCountInString > 1000`、非法 status、`q` 去空格后少于 2 个字符）返回 `ErrDepartmentInvalid`；加入停用部门返回 `ErrDepartmentInactive`；启用非负责人停用的成员返回 `ErrDepartmentMemberEnableDenied`。
- 审计 action：`department.manager_assign_members`、`department.manager_update_member`，由 `departmentAudit` 写入，`actor_user_id` 为操作者。无实际变化时不写。

### 4.3 HTTP 与前端

- 路由与白名单按设计第 4 节；`PUT /admin/department-members/:id` 的请求体即 `ManagedMemberUpdateInput`（严格 JSON）。
- `DepartmentAccessDialog.vue` 新增复选框 `data-testid="access-manage-members"`，保存时始终发送 `manage_members` 布尔值。现有 `DepartmentManagement.spec.ts` 中对 `setAccess` 的精确断言补上 `manage_members` 字段属于机械更新，不算放宽。
- 新页面 `frontend/src/views/admin/MyDepartmentView.vue`、路由 `/admin/my-department`、API 放在现有 `frontend/src/api/admin/departments.ts` 内，不新建 API 文件。

## 5. 分阶段执行与门禁

每个阶段结束时，门禁全部通过才能提交，然后进入下一阶段。每个阶段一个提交，提交信息格式：`feat(department): <阶段内容>`。

| 阶段 | 内容 |
| --- | --- |
| 1 数据与通用写入 | 迁移 261；Ent 两个字段并重新生成；`user_repo.go` 在 `fields.Status` 且目标为 disabled 时同时写 `disabled_source='other'`（含原本已 disabled 的情况；来源不进入通用字段掩码）；内容审计对已停用用户也执行写入；`AdminUser` DTO 只读返回两个字段 |
| 2 后端接口 | 删除 pending 标签；权限码与白名单；`SetAccess` 处理 `ManageMembers`；5 个服务方法及仓储、handler、路由；构造函数与 Wire；严格 JSON；审计；提交后失效认证缓存 |
| 3 前端 | 授权弹窗；「我的部门」页面与弹窗；`UsersView.vue` 的"负责人停用"提示与负责人备注只读展示；移出/转岗确认文案；侧栏、路由、`adminPermissions.ts` 落地顺序、类型、中英文 i18n |
| 4 收尾 | 按设计第 9 节逐项补齐缺失的测试；更新 wiki（backend/frontend/data-and-domain/security 的部门章节与 README 入口）；按 `ops.md` 跑全量门禁 |

Go 命令在 `backend/` 下执行，环境变量见第 6 节。各阶段门禁（全部通过才提交）：

阶段 1：

```bash
go test -tags integration -p 1 -count=1 ./internal/repository -run 'TestDepartmentMemberSchemaIntegration|TestDepartmentRepositoryIntegration|TestUserAdminAccessIntegration|Migration' -v
go test -p 1 -count=1 ./internal/service -run 'ContentModeration' -v
go generate ./ent && git status --short ent   # 再次生成无漂移
```

阶段 2：

```bash
go test -tags integration -p 1 -count=1 ./internal/repository -run 'TestDepartmentMember|TestDepartmentRepositoryIntegration|TestDepartmentSubscriptionsIntegration|TestOrganizationUsageDepartmentIntegration|TestDepartmentQueryCountIntegration|TestUserAdminAccessIntegration' -v
go test -p 1 -count=1 ./internal/service ./internal/handler/admin ./internal/server/... -run 'Department|AdminPermission|Route'
go build ./...
```

另需为 handler 补单测：严格 JSON、请求体含 `notes` 返回 400、路由已加入白名单。

阶段 3（仓库根目录）：

```bash
pnpm --dir frontend exec vitest run src/components/admin/department src/views/admin src/utils
pnpm --dir frontend run lint:check
pnpm --dir frontend run typecheck
```

另需为新页面补测试：单部门自动选中、`can_enable` 控制按钮、403/409 分码处理、切换部门后迟到响应被丢弃。

阶段 4：按 `llm-wiki/wiki/ops.md` 跑 Go default/unit/integration、前端完整 Vitest 与 build，并分别记录结果。

## 6. 本机环境（不照做会卡住或产生假结论）

- Go 工具链：`go.mod` 要求 1.27.3。使用 `GOROOT=backend/.gomodcache/golang.org/toolchain@v0.0.1-go1.27.3.windows-amd64`、`GOTOOLCHAIN=local`、repo 内的 `GOCACHE=backend/.gocache` 与 `GOMODCACHE=backend/.gomodcache`、每条命令独立的 `GOTMPDIR`，PATH 加入 Git `usr/bin`。
- 真实 PostgreSQL：本机无 Docker。删除 `E:/tmp/epg/STOP` 后以 `EPG_VERSION=18` 运行 `E:/tmp/epg/epg.exe`（端口 55418），日志出现 READY 后，用 `E:/tmp/epg/sqlrun.exe` 新建一个空库，设置 `SUB2API_POSTGRES_ONLY_INTEGRATION_DSN="postgres://postgres:postgres@127.0.0.1:55418/<新库>?sslmode=disable&TimeZone=UTC"`。用完创建 `E:/tmp/epg/STOP` 停库。不要复用旧库做回归判断。
- 代码生成：Ent 用 `cd backend && go generate ./ent`，Wire 用 `cd backend/cmd/server && go run -mod=mod github.com/google/wire/cmd/wire`。两者都带 `-mod=mod`，生成后要还原 `go.mod`/`go.sum` 中由此新增的条目，最终 diff 不应包含这两个文件。
- golangci-lint：本机旧版本对 go1.27 的结论不可信，报告中注明"未执行可信 lint"即可，不要把旧版本的 `0 issues` 当作通过。
- `go test -tags unit ./internal/service` 有两个既有失败（见 `ops.md`），与本任务无关，不要修。
- 显式 skip 或因缺 Docker/Redis 而跳过的测试，不能算通过。

## 7. 交付报告（每个阶段一份，最终汇总一份）

报告写在你的最终回复中，不写进仓库。内容依次为：

1. 第 2 节的两份清单及每条的处理方式。
2. 设计第 9 节逐行对照表，每一行填三列：覆盖它的测试名（含文件）、执行命令、实际结果（PASS/FAIL/未执行及原因）。没有对应测试的行要明确写"缺失"。
3. 第 3 节四个锁定测试文件的执行结果原文（关键行）。
4. 改动文件清单；清单外文件逐个说明理由。
5. 与设计不一致或不确定之处、发现但未修的范围外问题。
6. 各阶段提交哈希。

不要写"已全部完成""应该没问题"之类没有对应命令输出支撑的结论。

完成后由 Claude 对照设计与本任务卡进行代码审核；审核时只看代码、测试和命令输出，不以报告中的自我评价为依据。
