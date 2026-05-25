# User Concurrency Presets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an admin-only feature for saving user concurrency presets, applying them manually, and applying enabled presets once per day at a configured `HH:mm` time.

**Architecture:** Add raw-SQL migrations and repository methods, matching the existing scheduled test pattern. Keep domain logic in a dedicated service, expose routes under admin users, and add a compact dialog on the existing Users page.

**Tech Stack:** Go, PostgreSQL migrations, Gin handlers, Vue 3, TypeScript, Vitest, Go unit tests.

---

## File Map

| File | Responsibility |
|---|---|
| `backend/migrations/142_user_concurrency_presets.sql` | Create preset and run-log tables. |
| `backend/internal/service/user_concurrency_preset.go` | Domain models, repository interface, validation helpers. |
| `backend/internal/repository/user_concurrency_preset_repo.go` | Raw SQL persistence for presets and runs. |
| `backend/internal/service/user_concurrency_preset_service.go` | CRUD, manual apply, scheduled apply, auth cache invalidation. |
| `backend/internal/service/user_concurrency_preset_runner.go` | Once-per-minute background schedule runner. |
| `backend/internal/handler/admin/user_concurrency_preset_handler.go` | Admin HTTP endpoints. |
| `backend/internal/server/routes/admin.go` | Route registration. |
| `backend/internal/service/wire.go` and generated wire files | Dependency wiring and runner startup. |
| `frontend/src/api/admin/userConcurrencyPresets.ts` | Frontend API client. |
| `frontend/src/views/admin/UsersView.vue` | Entry button and dialog integration. |
| `frontend/src/views/admin/components/UserConcurrencyPresetsDialog.vue` | Preset list, edit form, apply action, run log. |

---

### Task 1: Database Migration

**Files:**
- Create: `backend/migrations/142_user_concurrency_presets.sql`

- [ ] **Step 1: Add migration**

Create `backend/migrations/142_user_concurrency_presets.sql`:

```sql
CREATE TABLE IF NOT EXISTS user_concurrency_presets (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    target_concurrency INTEGER NOT NULL CHECK (target_concurrency >= 1),
    user_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    schedule_enabled BOOLEAN NOT NULL DEFAULT false,
    schedule_time TEXT NOT NULL DEFAULT '',
    last_scheduled_run_date DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_concurrency_presets_schedule
    ON user_concurrency_presets(schedule_enabled, schedule_time)
    WHERE schedule_enabled = true;

CREATE TABLE IF NOT EXISTS user_concurrency_preset_runs (
    id BIGSERIAL PRIMARY KEY,
    preset_id BIGINT NOT NULL REFERENCES user_concurrency_presets(id) ON DELETE CASCADE,
    trigger TEXT NOT NULL CHECK (trigger IN ('manual', 'scheduled')),
    target_concurrency INTEGER NOT NULL CHECK (target_concurrency >= 1),
    user_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    affected_count INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('success', 'failed')),
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_concurrency_preset_runs_preset_created
    ON user_concurrency_preset_runs(preset_id, created_at DESC);
```

- [ ] **Step 2: Run migration validation test**

Run:

```powershell
cd E:\allsite\xyai\backend
go test -count=1 ./internal/repository -run TestMigrationsRunner
```

Expected: repository migration tests pass.

- [ ] **Step 3: Commit**

```powershell
git add backend/migrations/142_user_concurrency_presets.sql
git commit -m "feat: add user concurrency preset tables"
```

---

### Task 2: Backend Domain And Repository

**Files:**
- Create: `backend/internal/service/user_concurrency_preset.go`
- Create: `backend/internal/repository/user_concurrency_preset_repo.go`
- Test: `backend/internal/repository/user_concurrency_preset_repo_test.go`

- [ ] **Step 1: Write repository tests**

Create tests covering create, update, list, due-list, run-log creation, and marking scheduled success.

Run:

```powershell
cd E:\allsite\xyai\backend
go test -count=1 ./internal/repository -run UserConcurrencyPreset
```

Expected before implementation: compile failure because repository types are missing.

- [ ] **Step 2: Add domain models and repository interface**

Create `backend/internal/service/user_concurrency_preset.go` with:

```go
package service

import (
	"context"
	"time"
)

const (
	UserConcurrencyPresetTriggerManual    = "manual"
	UserConcurrencyPresetTriggerScheduled = "scheduled"
	UserConcurrencyPresetRunSuccess       = "success"
	UserConcurrencyPresetRunFailed        = "failed"
)

type UserConcurrencyPreset struct {
	ID                   int64      `json:"id"`
	Name                 string     `json:"name"`
	Description          string     `json:"description"`
	TargetConcurrency    int        `json:"target_concurrency"`
	UserIDs              []int64    `json:"user_ids"`
	ScheduleEnabled      bool       `json:"schedule_enabled"`
	ScheduleTime         string     `json:"schedule_time"`
	LastScheduledRunDate *time.Time `json:"last_scheduled_run_date"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type UserConcurrencyPresetRun struct {
	ID                int64     `json:"id"`
	PresetID          int64     `json:"preset_id"`
	Trigger           string    `json:"trigger"`
	TargetConcurrency int       `json:"target_concurrency"`
	UserIDs           []int64   `json:"user_ids"`
	AffectedCount     int       `json:"affected_count"`
	Status            string    `json:"status"`
	ErrorMessage      string    `json:"error_message"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at"`
	CreatedAt         time.Time `json:"created_at"`
}

type UserConcurrencyPresetRepository interface {
	Create(ctx context.Context, preset *UserConcurrencyPreset) (*UserConcurrencyPreset, error)
	GetByID(ctx context.Context, id int64) (*UserConcurrencyPreset, error)
	List(ctx context.Context) ([]*UserConcurrencyPreset, error)
	ListDue(ctx context.Context, scheduleTime string, runDate time.Time) ([]*UserConcurrencyPreset, error)
	Update(ctx context.Context, preset *UserConcurrencyPreset) (*UserConcurrencyPreset, error)
	Delete(ctx context.Context, id int64) error
	MarkScheduledRun(ctx context.Context, id int64, runDate time.Time) error
	CreateRun(ctx context.Context, run *UserConcurrencyPresetRun) (*UserConcurrencyPresetRun, error)
	ListRuns(ctx context.Context, presetID int64, limit int) ([]*UserConcurrencyPresetRun, error)
}
```

- [ ] **Step 3: Implement raw SQL repository**

Create `backend/internal/repository/user_concurrency_preset_repo.go`. Use `json.Marshal` and `json.Unmarshal` for `user_ids`. Use `sql.NullTime` for `last_scheduled_run_date`.

Important query for due schedules:

```sql
SELECT id, name, description, target_concurrency, user_ids, schedule_enabled, schedule_time,
       last_scheduled_run_date, created_at, updated_at
FROM user_concurrency_presets
WHERE schedule_enabled = true
  AND schedule_time = $1
  AND (last_scheduled_run_date IS NULL OR last_scheduled_run_date < $2::date)
ORDER BY id ASC
```

- [ ] **Step 4: Run repository tests**

Run:

```powershell
cd E:\allsite\xyai\backend
go test -count=1 ./internal/repository -run UserConcurrencyPreset
```

Expected: pass.

- [ ] **Step 5: Commit**

```powershell
git add backend/internal/service/user_concurrency_preset.go backend/internal/repository/user_concurrency_preset_repo.go backend/internal/repository/user_concurrency_preset_repo_test.go
git commit -m "feat: add user concurrency preset repository"
```

---

### Task 3: Backend Service And Scheduler

**Files:**
- Create: `backend/internal/service/user_concurrency_preset_service.go`
- Create: `backend/internal/service/user_concurrency_preset_runner.go`
- Test: `backend/internal/service/user_concurrency_preset_service_test.go`

- [ ] **Step 1: Write service tests**

Cover:

- Reject empty name.
- Reject `target_concurrency < 1`.
- Reject empty target user list.
- Reject admin users.
- Manual apply sets selected user concurrency.
- Manual apply writes success run log.
- Failed apply writes failed run log.
- Scheduled apply marks run date only after success.

Run:

```powershell
cd E:\allsite\xyai\backend
go test -count=1 ./internal/service -run UserConcurrencyPreset
```

Expected before implementation: compile failure because service is missing.

- [ ] **Step 2: Implement service**

Create `NewUserConcurrencyPresetService(repo UserConcurrencyPresetRepository, userRepo UserRepository, authCacheInvalidator APIKeyAuthCacheInvalidator, cfg *config.Config)`.

Core apply flow:

```go
func (s *UserConcurrencyPresetService) ApplyPreset(ctx context.Context, id int64, trigger string) (*UserConcurrencyPresetRun, error) {
	started := time.Now()
	preset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	validUserIDs, validateErr := s.validTargetUserIDs(ctx, preset.UserIDs)
	run := &UserConcurrencyPresetRun{
		PresetID:          preset.ID,
		Trigger:           trigger,
		TargetConcurrency: preset.TargetConcurrency,
		UserIDs:           preset.UserIDs,
		StartedAt:         started,
		FinishedAt:        time.Now(),
	}
	if validateErr != nil {
		run.Status = UserConcurrencyPresetRunFailed
		run.ErrorMessage = validateErr.Error()
		created, _ := s.repo.CreateRun(ctx, run)
		return created, validateErr
	}
	affected, err := s.userRepo.BatchSetConcurrency(ctx, validUserIDs, preset.TargetConcurrency)
	run.FinishedAt = time.Now()
	run.AffectedCount = affected
	if err != nil {
		run.Status = UserConcurrencyPresetRunFailed
		run.ErrorMessage = err.Error()
		created, _ := s.repo.CreateRun(ctx, run)
		return created, err
	}
	run.Status = UserConcurrencyPresetRunSuccess
	created, err := s.repo.CreateRun(ctx, run)
	if s.authCacheInvalidator != nil {
		for _, uid := range validUserIDs {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, uid)
		}
	}
	return created, err
}
```

- [ ] **Step 3: Implement runner**

Create a runner like `ScheduledTestRunnerService`, with one cron entry `* * * * *`. It formats local time as `15:04`, calls `RunDueSchedules`, and logs errors without panicking.

- [ ] **Step 4: Run service tests**

Run:

```powershell
cd E:\allsite\xyai\backend
go test -count=1 ./internal/service -run UserConcurrencyPreset
```

Expected: pass.

- [ ] **Step 5: Commit**

```powershell
git add backend/internal/service/user_concurrency_preset_service.go backend/internal/service/user_concurrency_preset_runner.go backend/internal/service/user_concurrency_preset_service_test.go
git commit -m "feat: apply user concurrency presets"
```

---

### Task 4: Admin API

**Files:**
- Create: `backend/internal/handler/admin/user_concurrency_preset_handler.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: service wiring files after Wire generation
- Test: `backend/internal/handler/admin/user_concurrency_preset_handler_test.go`

- [ ] **Step 1: Write handler tests**

Cover list, create validation, update, apply, run list, and delete routing.

Run:

```powershell
cd E:\allsite\xyai\backend
go test -count=1 ./internal/handler/admin -run UserConcurrencyPreset
```

Expected before implementation: compile failure because handler is missing.

- [ ] **Step 2: Implement handler**

Expose:

```text
GET    /api/v1/admin/users/concurrency-presets
POST   /api/v1/admin/users/concurrency-presets
PUT    /api/v1/admin/users/concurrency-presets/:id
DELETE /api/v1/admin/users/concurrency-presets/:id
POST   /api/v1/admin/users/concurrency-presets/:id/apply
GET    /api/v1/admin/users/concurrency-presets/:id/runs
```

Request body fields:

```json
{
  "name": "daytime",
  "description": "",
  "target_concurrency": 12,
  "user_ids": [1, 2, 3],
  "schedule_enabled": true,
  "schedule_time": "09:00"
}
```

- [ ] **Step 3: Register routes before `/:id` user routes**

In `registerUserManagementRoutes`, register `/concurrency-presets` routes before `users.GET("/:id", ...)`, otherwise Gin may parse `concurrency-presets` as `:id`.

- [ ] **Step 4: Wire dependencies**

Add providers for repository, service, runner, and handler. Regenerate Wire if the project requires generated files:

```powershell
cd E:\allsite\xyai\backend
go generate ./cmd/server
```

If the repo uses checked-in generated wire files, include generated diffs.

- [ ] **Step 5: Run API tests**

Run:

```powershell
cd E:\allsite\xyai\backend
go test -count=1 ./internal/handler/admin ./internal/server -run "UserConcurrencyPreset|APIContract"
```

Expected: pass.

- [ ] **Step 6: Commit**

```powershell
git add backend/internal/handler/admin/user_concurrency_preset_handler.go backend/internal/server/routes/admin.go backend/internal/service/wire.go backend/cmd/server/wire_gen.go backend/internal/handler/admin/user_concurrency_preset_handler_test.go
git commit -m "feat: expose user concurrency preset APIs"
```

---

### Task 5: Frontend API And Dialog

**Files:**
- Create: `frontend/src/api/admin/userConcurrencyPresets.ts`
- Create: `frontend/src/views/admin/components/UserConcurrencyPresetsDialog.vue`
- Modify: `frontend/src/views/admin/UsersView.vue`
- Test: `frontend/src/views/admin/__tests__/UserConcurrencyPresetsDialog.spec.ts`

- [ ] **Step 1: Write frontend tests**

Cover:

- Dialog loads presets.
- Create payload contains `target_concurrency`, `user_ids`, `schedule_enabled`, and `schedule_time`.
- Apply button calls `/apply` after confirmation.
- Invalid schedule time blocks save.

Run:

```powershell
cd E:\allsite\xyai\frontend
pnpm vitest run src/views/admin/__tests__/UserConcurrencyPresetsDialog.spec.ts
```

Expected before implementation: fail because component is missing.

- [ ] **Step 2: Add API client**

Create functions:

```ts
export async function listPresets(): Promise<UserConcurrencyPreset[]>
export async function createPreset(payload: UserConcurrencyPresetPayload): Promise<UserConcurrencyPreset>
export async function updatePreset(id: number, payload: UserConcurrencyPresetPayload): Promise<UserConcurrencyPreset>
export async function deletePreset(id: number): Promise<void>
export async function applyPreset(id: number): Promise<UserConcurrencyPresetRun>
export async function listPresetRuns(id: number, limit = 20): Promise<UserConcurrencyPresetRun[]>
```

- [ ] **Step 3: Add dialog component**

Build one compact modal with:

- Left side preset list.
- Right side form.
- User search using existing `admin/users` list API.
- Selected user chips.
- `target_concurrency` numeric input.
- Schedule toggle and `HH:mm` input.
- Save, Apply, Delete actions.

- [ ] **Step 4: Add Users page entry**

Add a toolbar button named `并发方案` beside existing user actions. Open the dialog without changing the table behavior.

- [ ] **Step 5: Run frontend tests**

Run:

```powershell
cd E:\allsite\xyai\frontend
pnpm vitest run src/views/admin/__tests__/UserConcurrencyPresetsDialog.spec.ts src/views/admin/__tests__/UsersView.spec.ts
```

Expected: pass.

- [ ] **Step 6: Commit**

```powershell
git add frontend/src/api/admin/userConcurrencyPresets.ts frontend/src/views/admin/components/UserConcurrencyPresetsDialog.vue frontend/src/views/admin/UsersView.vue frontend/src/views/admin/__tests__/UserConcurrencyPresetsDialog.spec.ts
git commit -m "feat: add user concurrency preset UI"
```

---

### Task 6: Final Verification And Push

**Files:**
- No new files unless tests reveal fixes.

- [ ] **Step 1: Run backend focused tests**

```powershell
cd E:\allsite\xyai\backend
go test -count=1 ./internal/repository ./internal/service ./internal/handler/admin ./internal/server
```

Expected: pass.

- [ ] **Step 2: Run frontend focused tests**

```powershell
cd E:\allsite\xyai\frontend
pnpm vitest run src/views/admin/__tests__/UserConcurrencyPresetsDialog.spec.ts src/views/admin/__tests__/UsersView.spec.ts
```

Expected: pass.

- [ ] **Step 3: Run conflict and whitespace checks**

```powershell
cd E:\allsite\xyai
git diff --check
rg -n "<<<<<<<|>>>>>>>|^=======$" . -S
```

Expected: `git diff --check` exits 0. `rg` exits 1 with no output because no conflict markers exist.

- [ ] **Step 4: Push branch**

```powershell
git status --short --branch
git push origin codex/user-concurrency-presets
```

Expected: branch pushed to origin.

