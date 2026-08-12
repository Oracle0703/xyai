# Subscription Daily Remaining Sort and Bulk Reset Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add organization filtering, database-wide daily remaining-ratio ordering, and a filtered one-click daily quota reset to admin subscription management.

**Architecture:** Introduce one normalized `SubscriptionAdminFilter` shared by the admin list and filtered reset paths. Keep the existing repository `List` method unchanged for subscription expiry maintenance, add admin-specific PostgreSQL ordering and atomic reset methods, and expose them through an idempotent admin endpoint. The Vue page sends the same applied filters to list and reset operations and freezes a copy when confirmation opens.

**Tech Stack:** Go, Gin, Ent, PostgreSQL, Ristretto/Redis cache invalidation, Vue 3 Composition API, TypeScript, Axios, Vitest, vue-test-utils, pnpm.

## Global Constraints

- Work on `feature/hy/10176_subscription_sort_daily_reset`, created from local `main@4b92eb2344d07259161e6027c527452fbe764b2a`.
- Organization API keys remain `xunyou` and `wsdashi`; UI labels are exactly `迅游` and `速宝`.
- Organization matching is case-insensitive exact email-domain matching; subdomains do not match.
- Every admin list is primarily ordered by ascending daily remaining ratio. Missing or non-positive daily limits sort last.
- Equal ratios keep the existing requested secondary order, then subscription ID provides deterministic order.
- Filtered reset affects every matching page but only subscriptions that are active, not deleted, and not expired when the write runs.
- Filtered reset changes only `daily_usage_usd` and `daily_window_start`; weekly/monthly usage and windows remain unchanged.
- Do not add a database field, migration, or dependency.
- Keep the existing single-subscription reset behavior and expiry-maintenance repository query behavior unchanged.
- Update required llm-wiki pages and `frontend/src/components/admin/README.md`; wiki files must use LF.

---

### Task 1: Shared Admin Filter and PostgreSQL List Ordering

**Files:**
- Create: `backend/internal/service/subscription_admin_filter.go`
- Create: `backend/internal/service/subscription_admin_filter_test.go`
- Modify: `backend/internal/service/user_subscription_port.go`
- Modify: `backend/internal/service/subscription_service.go`
- Modify: `backend/internal/repository/user_subscription_repo.go`
- Modify: repository/service stubs implementing `UserSubscriptionRepository`
- Test: `backend/internal/repository/user_subscription_repo_integration_test.go`

**Interfaces:**
- Produces: `SubscriptionAdminFilter`, `NormalizeSubscriptionAdminFilter`, repository `ListAdmin(..., now time.Time)`, and service `ListAdmin(...)`.
- Preserves: existing repository `List(...)` for `SubscriptionExpiryService`.

- [ ] **Step 1: Write failing filter normalization unit tests**

```go
func TestNormalizeSubscriptionAdminFilter(t *testing.T) {
    userID, groupID := int64(11), int64(21)
    got, err := NormalizeSubscriptionAdminFilter(SubscriptionAdminFilter{
        UserID: &userID, GroupID: &groupID, Status: " ACTIVE ",
        Platform: " OPENAI ", Organization: " XUNYOU ",
        SortBy: "expires_at", SortOrder: "asc",
    })
    require.NoError(t, err)
    require.Equal(t, "active", got.Status)
    require.Equal(t, "openai", got.Platform)
    require.Equal(t, SubscriptionOrganizationXunyou, got.Organization)
}

func TestNormalizeSubscriptionAdminFilterRejectsInvalidValues(t *testing.T) {
    for _, tc := range []SubscriptionAdminFilter{
        {Organization: "unknown"}, {Status: "unknown"},
        {Platform: "unknown"}, {SortBy: "unknown"}, {SortOrder: "sideways"},
    } {
        _, err := NormalizeSubscriptionAdminFilter(tc)
        require.Error(t, err)
    }
}
```

- [ ] **Step 2: Run the unit test and verify RED**

Run: `cd backend && go test -tags unit ./internal/service -run 'TestNormalizeSubscriptionAdminFilter' -count=1`

Expected: compile failure because the filter type and normalizer do not exist.

- [ ] **Step 3: Implement the shared normalized filter**

```go
const (
    SubscriptionOrganizationXunyou  = "xunyou"
    SubscriptionOrganizationWsdashi = "wsdashi"
)

type SubscriptionAdminFilter struct {
    UserID       *int64
    GroupID      *int64
    Status       string
    Platform     string
    Organization string
    SortBy       string
    SortOrder    string
}
```

Empty fields mean no filter. Accept statuses `active|expired|revoked|suspended`, existing group platforms, sort fields `created_at|expires_at|status`, and orders `asc|desc`; default to `created_at desc`. Return field-specific `infraerrors.BadRequest` values for invalid or non-positive IDs.

- [ ] **Step 4: Write failing repository integration tests**

Seed uppercase domains, subdomains, finite daily limits, zero/nil limits, expired daily windows, over-limit usage, one-time subscriptions, and equal ratios. Assert exact organization matching and cross-page ordering:

```go
items, page, err := s.repo.ListAdmin(s.ctx,
    pagination.PaginationParams{Page: 1, PageSize: 2},
    service.SubscriptionAdminFilter{
        Organization: service.SubscriptionOrganizationXunyou,
        SortBy: "created_at", SortOrder: "desc",
    },
    now,
)
require.NoError(s.T(), err)
require.Equal(s.T(), int64(3), page.Total)
require.Equal(s.T(), []int64{lowestRemainingID, nextPageBoundaryID}, subscriptionIDs(items))
```

The next page must continue the global order. `DEV@XUNYOU.COM` matches; `dev@team.xunyou.com` does not. Expired ordinary daily windows use zero effective usage; unexpired one-time subscriptions retain stored usage; over-limit usage sorts at 0%; nil/zero limits sort last; equal ratios use requested secondary order then ID.

- [ ] **Step 5: Run repository tests and verify RED**

Run: `cd backend && go test -tags integration ./internal/repository -run 'TestUserSubscriptionRepoSuite/TestListAdmin' -count=1`

Expected: compile failure because `ListAdmin` does not exist.

- [ ] **Step 6: Implement admin-only filtering and ordering**

Add `ListAdmin` without changing `List`. Build the Ent query with user/group relations, exact domain predicates using `LOWER(SPLIT_PART(email, '@', 2))`, and parameterized custom order expressions. Pass `now` and `timezone.StartOfDay(now)` from the service so SQL does not depend on database session timezone.

```sql
CASE WHEN groups.daily_limit_usd IS NULL OR groups.daily_limit_usd <= 0 THEN 1 ELSE 0 END ASC,
CASE WHEN groups.daily_limit_usd > 0 THEN
  GREATEST(groups.daily_limit_usd - effective_daily_usage, 0) / groups.daily_limit_usd
END ASC
```

`effective_daily_usage` is zero only when an ordinary subscription's daily window predates the supplied start of day. One-time subscriptions (`expires_at <= starts_at + interval '1 day'`) retain stored usage. Append the validated secondary order and ID tie-breaker before offset/limit.

- [ ] **Step 7: Wire the service and update interface stubs**

Normalize once in `SubscriptionService.ListAdmin`, call repository `ListAdmin(..., s.now())`, then retain `normalizeExpiredWindowsAt` and `normalizeSubscriptionStatus`. Add panic/no-op `ListAdmin` methods to stubs without routing expiry maintenance through it.

- [ ] **Step 8: Run Task 1 tests and verify GREEN**

```powershell
cd backend
go test -tags unit ./internal/service -run 'TestNormalizeSubscriptionAdminFilter' -count=1
go test -tags integration ./internal/repository -run 'TestUserSubscriptionRepoSuite/TestListAdmin' -count=1
```

Expected: all selected tests pass.

- [ ] **Step 9: Commit Task 1**

```powershell
git add backend/internal/service backend/internal/repository/user_subscription_repo.go backend/internal/repository/user_subscription_repo_integration_test.go backend/internal/server/api_contract_test.go backend/internal/server/middleware
git commit -m "feat(subscriptions): sort admin list by daily remaining quota"
```

---

### Task 2: Atomic Filtered Daily Reset and Cache Invalidation

**Files:**
- Modify: `backend/internal/service/user_subscription_port.go`
- Modify: `backend/internal/service/subscription_service.go`
- Modify: `backend/internal/repository/user_subscription_repo.go`
- Modify: repository/service stubs implementing `UserSubscriptionRepository`
- Test: `backend/internal/repository/user_subscription_repo_integration_test.go`
- Test: `backend/internal/service/subscription_reset_quota_test.go`

**Interfaces:**
- Consumes: normalized `SubscriptionAdminFilter` from Task 1.
- Produces: `SubscriptionCacheKey`, repository `ResetDailyFiltered(...)`, service `AdminResetDailyFiltered(...)`.

- [ ] **Step 1: Write failing repository integration tests**

Seed matching active, expired, revoked, other-organization, other-group, and other-platform subscriptions, then call:

```go
keys, err := s.repo.ResetDailyFiltered(s.ctx, service.SubscriptionAdminFilter{
    Status: "active", Organization: service.SubscriptionOrganizationXunyou,
}, now, timezone.StartOfDay(now))
```

Assert only active, not-deleted, not-expired xunyou rows get `daily_usage_usd=0` and the new daily window; weekly/monthly values and windows remain equal; returned `(user_id, group_id)` keys are unique. A zero-match call returns an empty slice and no error.

- [ ] **Step 2: Run repository test and verify RED**

Run: `cd backend && go test -tags integration ./internal/repository -run 'TestUserSubscriptionRepoSuite/TestResetDailyFiltered' -count=1`

Expected: compile failure because `ResetDailyFiltered` does not exist.

- [ ] **Step 3: Implement one-statement atomic reset**

Use parameterized PostgreSQL with a candidate CTE and `UPDATE ... FROM ... RETURNING user_id, group_id`. Reuse list predicates, then force `status='active'`, `deleted_at IS NULL`, and `expires_at > now`. Set only:

```sql
daily_usage_usd = 0,
daily_window_start = $start_of_day,
updated_at = $now
```

Deduplicate returned cache keys. Do not include weekly/monthly columns.

- [ ] **Step 4: Write failing service tests**

Extend the reset stub to capture the normalized filter and return keys. Assert configured start-of-day use, count, L1 invalidation, shared cache invalidation, and cross-instance publish for every key. Add zero-match and repository-error tests.

```go
count, err := svc.AdminResetDailyFiltered(context.Background(), SubscriptionAdminFilter{Organization: " XUNYOU "})
require.NoError(t, err)
require.Equal(t, 2, count)
require.Equal(t, SubscriptionOrganizationXunyou, stub.filter.Organization)
```

- [ ] **Step 5: Run service tests and verify RED**

Run: `cd backend && go test -tags unit ./internal/service -run 'TestAdminResetDailyFiltered' -count=1`

Expected: compile failure because `AdminResetDailyFiltered` does not exist.

- [ ] **Step 6: Implement service orchestration**

Normalize, call the repository once, then invalidate all affected keys. A committed database reset remains success when cache invalidation fails: log the key/error and continue. Publish cross-instance L1 invalidation and invalidate billing cache for each key.

- [ ] **Step 7: Run Task 2 tests and verify GREEN**

```powershell
cd backend
go test -tags unit ./internal/service -run 'TestAdminResetDailyFiltered' -count=1
go test -tags integration ./internal/repository -run 'TestUserSubscriptionRepoSuite/TestResetDailyFiltered' -count=1
```

Expected: all selected tests pass.

- [ ] **Step 8: Commit Task 2**

```powershell
git add backend/internal/service backend/internal/repository/user_subscription_repo.go backend/internal/repository/user_subscription_repo_integration_test.go backend/internal/server/api_contract_test.go backend/internal/server/middleware
git commit -m "feat(subscriptions): reset filtered daily quotas atomically"
```

---

### Task 3: Admin HTTP, Idempotency, Routing, and Permission Contract

**Files:**
- Modify: `backend/internal/handler/admin/subscription_handler.go`
- Create: `backend/internal/handler/admin/subscription_handler_test.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: `backend/internal/service/admin_permission.go`
- Test: `backend/internal/service/admin_permission_test.go`

**Interfaces:**
- Consumes: service `ListAdmin` and `AdminResetDailyFiltered`.
- Produces: list query `organization=xunyou|wsdashi`.
- Produces: `POST /api/v1/admin/subscriptions/reset-daily-filtered`, response `{ "reset_count": number }`, idempotency scope `admin.subscriptions.reset_daily_filtered`.

- [ ] **Step 1: Write failing handler tests**

Introduce a narrow handler service interface so tests capture calls without a database. Cover list parsing/validation, reset success, zero count, malformed/non-positive IDs, invalid organization/status/platform, and idempotent replay. Success request:

```http
POST /api/v1/admin/subscriptions/reset-daily-filtered
Idempotency-Key: subscriptions-reset-20260812-1
Content-Type: application/json

{"status":"active","organization":"xunyou","group_id":21}
```

Assert the service receives those filters and response data is `{"reset_count":12}`.

- [ ] **Step 2: Run handler tests and verify RED**

Run: `cd backend && go test ./internal/handler/admin -run 'TestSubscriptionHandler_(List|ResetDailyFiltered)' -count=1`

Expected: compile failure or 404 because the handler contract does not exist.

- [ ] **Step 3: Implement strict parsing and idempotent handler**

Replace ignored `strconv.ParseInt` errors on the admin list with a helper rejecting malformed/non-positive IDs. Build `SubscriptionAdminFilter` and call service `ListAdmin`.

```go
type ResetDailyFilteredRequest struct {
    Status       string `json:"status"`
    UserID       *int64 `json:"user_id"`
    GroupID      *int64 `json:"group_id"`
    Platform     string `json:"platform"`
    Organization string `json:"organization"`
}

type ResetDailyFilteredResponse struct {
    ResetCount int `json:"reset_count"`
}
```

Execute reset through `executeAdminIdempotentJSON` using the request payload and `service.DefaultWriteIdempotencyTTL()`.

- [ ] **Step 4: Add route permission test and verify RED**

Add an allowed `admin.subscriptions` case for `POST /api/v1/admin/subscriptions/reset-daily-filtered`.

Run: `cd backend && go test -tags unit ./internal/service -run 'TestAdminPermission' -count=1`

Expected: permission assertion fails before the route rule is added.

- [ ] **Step 5: Register route and permission**

Register the static POST route before `/:id` routes. Add only this write route to `AdminPermissionSubscriptions`; assignment, adjustment, revoke, and restore stay denied.

- [ ] **Step 6: Run Task 3 tests and verify GREEN**

```powershell
cd backend
go test ./internal/handler/admin -run 'TestSubscriptionHandler_(List|ResetDailyFiltered)' -count=1
go test -tags unit ./internal/service -run 'TestAdminPermission' -count=1
```

Expected: all selected tests pass.

- [ ] **Step 7: Commit Task 3**

```powershell
git add backend/internal/handler/admin/subscription_handler.go backend/internal/handler/admin/subscription_handler_test.go backend/internal/server/routes/admin.go backend/internal/service/admin_permission.go backend/internal/service/admin_permission_test.go
git commit -m "feat(subscriptions): expose filtered daily reset endpoint"
```

---

### Task 4: Vue Organization Filter and One-Click Reset Workflow

**Files:**
- Modify: `frontend/src/api/admin/subscriptions.ts`
- Modify: `frontend/src/views/admin/SubscriptionsView.vue`
- Modify: `frontend/src/views/admin/__tests__/SubscriptionsView.spec.ts`
- Modify: `frontend/src/i18n/locales/zh/admin/channels.ts`
- Modify: `frontend/src/i18n/locales/en/admin/channels.ts`

**Interfaces:**
- Produces: `SubscriptionOrganization`, `SubscriptionAdminFilters`, and `resetDailyFiltered(filters, idempotencyKey)`.
- Consumes: existing `Select`, `ConfirmDialog`, stores, and subscription permissions.

- [ ] **Step 1: Write failing view/API tests**

Extend the API mock with `resetDailyFiltered` and make the `Select` stub emit model changes. Prove:

1. Selecting xunyou sends `organization: 'xunyou'` while showing `迅游`.
2. List requests retain `sort_by`/`sort_order` as secondary sorting parameters.
3. The bulk button follows “分配订阅” for admins and remains visible to subscription sub-admins.
4. Confirmation freezes `{status,user_id,group_id,platform,organization}`; later UI changes do not alter submission.
5. Confirmation calls `resetDailyFiltered(snapshot, stableIdempotencyKey)`, ignores a second pending click, refreshes on success, and distinguishes positive/zero results.
6. Failure keeps the dialog open and preserves filters.
7. Existing single-row daily/all reset assertions remain unchanged.

- [ ] **Step 2: Run view test and verify RED**

Run: `cd frontend && pnpm exec vitest run src/views/admin/__tests__/SubscriptionsView.spec.ts`

Expected: organization/bulk workflow assertions fail.

- [ ] **Step 3: Implement typed API contracts**

```ts
export type SubscriptionOrganization = 'xunyou' | 'wsdashi'

export interface SubscriptionAdminFilters {
  status?: 'active' | 'expired' | 'revoked' | 'suspended'
  user_id?: number
  group_id?: number
  platform?: string
  organization?: SubscriptionOrganization
  sort_by?: 'created_at' | 'expires_at' | 'status'
  sort_order?: 'asc' | 'desc'
}
```

`resetDailyFiltered` sends only five filter fields and the `Idempotency-Key` Axios header. Generate the key when confirmation opens using `crypto.randomUUID()` with timestamp/random fallback; reuse it for retries of the same open dialog and replace it only for a new confirmation.

- [ ] **Step 4: Implement page workflow and copy**

Add organization to filters/list calls and options with exact `迅游`/`速宝` labels. Put the bulk button after assign in DOM order. Snapshot last-applied filters, excluding page/sort, when opening the dedicated bulk confirmation. Failure leaves it open; success closes and refreshes. Add Chinese/English keys for button, confirmation, running, positive count, zero match, and failure. Do not alter unrelated controls.

- [ ] **Step 5: Run view test and verify GREEN**

Run: `cd frontend && pnpm exec vitest run src/views/admin/__tests__/SubscriptionsView.spec.ts`

Expected: all `SubscriptionsView` tests pass.

- [ ] **Step 6: Run type regression checks**

```powershell
cd frontend
pnpm exec vitest run src/views/admin/__tests__/SubscriptionsView.spec.ts
pnpm typecheck
```

Expected: view tests and typecheck pass.

- [ ] **Step 7: Commit Task 4**

```powershell
git add frontend/src/api/admin/subscriptions.ts frontend/src/views/admin/SubscriptionsView.vue frontend/src/views/admin/__tests__/SubscriptionsView.spec.ts frontend/src/i18n/locales/zh/admin/channels.ts frontend/src/i18n/locales/en/admin/channels.ts
git commit -m "feat(subscriptions): add organization filter and bulk daily reset"
```

---

### Task 5: Stable Knowledge, Full Verification, and Delivery Review

**Files:**
- Modify: `frontend/src/components/admin/README.md`
- Modify: `llm-wiki/wiki/backend.md`
- Modify: `llm-wiki/wiki/frontend.md`
- Modify: `llm-wiki/wiki/data-and-domain.md`
- Modify: `llm-wiki/wiki/security-and-reliability.md`
- Verify: all files changed since `main`

**Interfaces:**
- Documents: exact routes, filter keys, ratio semantics, active-only scope, idempotency, permission, and cache invalidation.

- [ ] **Step 1: Update stable documentation**

Record: backend list/reset routes; frontend organization options, applied-filter snapshot, and feedback; domain formula, expired-window normalization, unlimited-last order, and daily-only columns; security permission, required idempotency key, atomic write, and post-commit cache behavior. Preserve the component README compact group-search constraint.

- [ ] **Step 2: Run focused backend verification**

```powershell
cd backend
go test -tags unit ./internal/service -run 'Test(NormalizeSubscriptionAdminFilter|AdminResetDailyFiltered|AdminPermission)' -count=1
go test ./internal/handler/admin -run 'TestSubscriptionHandler_(List|ResetDailyFiltered)' -count=1
go test -tags integration ./internal/repository -run 'TestUserSubscriptionRepoSuite/(TestListAdmin|TestResetDailyFiltered)' -count=1
```

Expected: selected tests pass. If Docker is unavailable, report the integration harness skip as an unverified environment gate, not a pass.

- [ ] **Step 3: Run broad backend verification**

```powershell
cd backend
go test -tags unit ./internal/service ./internal/handler/admin -count=1
go test ./internal/server/routes ./internal/server/middleware -count=1
go test ./...
go build ./...
```

Expected: all exit 0. For Windows `.test.exe` locks, use fresh repo-local `GOCACHE`, `GOTMPDIR`, `TEMP`, `TMP`, `-p 1 -count=1` and rerun the same scope.

- [ ] **Step 4: Run full frontend verification**

```powershell
cd frontend
pnpm exec vitest run src/views/admin/__tests__/SubscriptionsView.spec.ts
pnpm test:run
pnpm typecheck
pnpm lint:check
pnpm build
```

Expected: zero failing tests/errors and exit 0.

- [ ] **Step 5: Run repository and knowledge checks**

```powershell
git diff --check main...HEAD
git status --short
tools\refresh-understand-wiki.cmd
tools\check-understand-status.cmd
git diff --check
```

Expected: no whitespace errors; wiki graph refresh succeeds; status is READY after tracked graph changes are included.

- [ ] **Step 6: Review final diff against approved design**

Check exact domain matching, cross-page primary order, tie behavior, unlimited-last, all-page filtered reset, active-only enforcement, daily-only columns, atomic count, permissions, idempotency, cache invalidation, success/zero/failure UI, single-row regressions, and absence of migration/dependency changes. Remove unrelated diff.

- [ ] **Step 7: Commit documentation and final corrections**

```powershell
git add frontend/src/components/admin/README.md llm-wiki/wiki/backend.md llm-wiki/wiki/frontend.md llm-wiki/wiki/data-and-domain.md llm-wiki/wiki/security-and-reliability.md llm-wiki/.understand-anything/knowledge-graph.json
git commit -m "docs: document subscription quota ordering and reset"
```

- [ ] **Step 8: Report final evidence**

Report branch, base SHA, commits, changed file count, focused/full verification, integration environment status, wiki graph status, and final Git status. Do not push or create a PR without explicit request.
