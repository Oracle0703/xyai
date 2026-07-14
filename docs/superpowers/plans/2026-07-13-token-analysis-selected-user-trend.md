# Token Analysis Selected User Trend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Token 分析页面支持选择最多 5 名用户，并按北京时间的日或小时展示权威计费用量总 Token 折线图。

**Architecture:** 兼容扩展现有 `/api/v1/admin/dashboard/users-trend`：无 `user_ids` 时保持 Top N，有 `user_ids` 时走 `usage_logs` 精确选人查询。前端在现有 `TokenAnalysisView.vue` 中维护跨分页选择、补齐时间轴并复用 Chart.js；不新增数据库迁移或页面路由。

**Tech Stack:** Go、Gin、PostgreSQL、lib/pq、Vue 3、TypeScript、Chart.js、vue-chartjs、Vitest、Go test

**Status:** 核心代码和文档已实现；Handler、Service、前端全量测试、typecheck/lint 与 Repository integration 编译已通过。真实 PostgreSQL integration/EXPLAIN 和浏览器验收仍待可用运行环境，当前分支按要求不提交。

---

## 文件结构

| 文件 | 职责 |
| --- | --- |
| `backend/internal/repository/usage_log_repo_trend.go` | 精确用户集合的日/小时 SQL 聚合，保留旧 Top N 分支 |
| `backend/internal/repository/usage_log_repo_integration_test.go` | PostgreSQL/Repository 选人、北京时间和 Token 求和合同 |
| `backend/internal/repository/user_usage_trend_explain_integration_test.go` | 选 1/5 人时的 opt-in PostgreSQL 执行计划基线 |
| `backend/internal/service/account_usage_service.go` | Repository 接口增加规范化 `userIDs` 参数 |
| `backend/internal/service/dashboard_service.go` | 透传精确用户集合 |
| `backend/internal/handler/admin/dashboard_handler.go` | 解析、规范化和验证 `user_ids`、日期范围与粒度 |
| `backend/internal/handler/admin/dashboard_query_cache.go` | 把规范化用户集合纳入缓存键 |
| `backend/internal/handler/admin/dashboard_snapshot_v2_handler.go` | 旧 snapshot 调用显式传空集合，保持 Top N 行为 |
| `backend/internal/handler/admin/dashboard_handler_cache_test.go` | Handler 参数与缓存隔离回归 |
| `frontend/src/api/admin/dashboard.ts` | `user_ids: number[]` 到逗号分隔查询参数的序列化 |
| `frontend/src/api/__tests__/admin.dashboard.spec.ts` | API 请求合同 |
| `frontend/src/views/admin/TokenAnalysisView.vue` | 用户选择、粒度、加载状态、零值补齐和折线图 |
| `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts` | 页面交互、图表数据、竞态和错误回归 |
| `frontend/src/i18n/locales/{zh,en}/admin/tokenAnalysis.ts` | 趋势图标题、模式、限制和错误文案 |
| `llm-wiki/wiki/{backend,frontend,data-and-domain}.md` | 稳定记录接口、页面数据流与 90 天存储边界 |

本计划不创建组件目录，因此不新增组件 `README.md`。

### Task 1: Repository 支持精确用户集合聚合

**Files:**
- Modify: `backend/internal/repository/usage_log_repo_trend.go:85`
- Modify: `backend/internal/repository/usage_log_repo_integration_test.go:1565`
- Modify: `backend/internal/service/account_usage_service.go:56`

- [x] **Step 1: 写 Repository 失败测试并同步旧调用签名**

先把现有调用改为显式传 `nil`，再增加精确选人测试。测试创建 3 名用户，在北京时间 `00:30`、`23:30` 和下一日 `00:30` 写入不同 Token；只选择前两名并断言未选择用户不存在、日分桶正确、总 Token 等于四类 Token 之和。

```go
func (s *UsageLogRepoSuite) TestGetUserUsageTrend_SelectedUsers_DailyShanghaiBuckets() {
	loc, err := time.LoadLocation("Asia/Shanghai")
	s.Require().NoError(err)
	selectedA := mustCreateUser(s.T(), s.client, &service.User{Email: "selected-a@test.com"})
	selectedB := mustCreateUser(s.T(), s.client, &service.User{Email: "selected-b@test.com"})
	unselected := mustCreateUser(s.T(), s.client, &service.User{Email: "unselected@test.com"})

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 2)
	fixtures := []struct {
		name                               string
		user                               *service.User
		createdAt                          time.Time
		input, output, cacheWrite, cacheHit int
	}{
		{"selected-a-day-1", selectedA, start.Add(30 * time.Minute), 10, 20, 30, 40},
		{"selected-b-day-1", selectedB, start.Add(23*time.Hour + 30*time.Minute), 1, 2, 3, 4},
		{"selected-a-day-2", selectedA, start.Add(24*time.Hour + 30*time.Minute), 5, 6, 7, 8},
		{"unselected-day-1", unselected, start.Add(time.Hour), 100, 100, 100, 100},
	}
	for _, fixture := range fixtures {
		apiKey := mustCreateApiKey(s.T(), s.client, &service.APIKey{
			UserID: fixture.user.ID,
			Key:    "sk-" + fixture.name,
			Name:   fixture.name,
		})
		account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "acc-" + fixture.name})
		_, err := s.repo.Create(s.ctx, &service.UsageLog{
			UserID:              fixture.user.ID,
			APIKeyID:            apiKey.ID,
			AccountID:           account.ID,
			RequestID:           uuid.NewString(),
			Model:               "claude-3",
			InputTokens:         fixture.input,
			OutputTokens:        fixture.output,
			CacheCreationTokens: fixture.cacheWrite,
			CacheReadTokens:     fixture.cacheHit,
			CreatedAt:           fixture.createdAt,
		})
		s.Require().NoError(err)
	}

	trend, err := s.repo.GetUserUsageTrend(
		s.ctx,
		start,
		end,
		"day",
		[]int64{selectedB.ID, selectedA.ID},
		0,
	)
	s.Require().NoError(err)
	s.Require().Len(trend, 3)
	tokens := make(map[string]int64, len(trend))
	for _, point := range trend {
		tokens[fmt.Sprintf("%d/%s", point.UserID, point.Date)] = point.Tokens
		s.Require().NotEqual(unselected.ID, point.UserID)
	}
	s.Require().Equal(int64(100), tokens[fmt.Sprintf("%d/2026-07-01", selectedA.ID)])
	s.Require().Equal(int64(10), tokens[fmt.Sprintf("%d/2026-07-01", selectedB.ID)])
	s.Require().Equal(int64(26), tokens[fmt.Sprintf("%d/2026-07-02", selectedA.ID)])
}
```

增加小时测试，断言 `2026-07-01 00:00` 与 `2026-07-01 23:00`；保留原 `TestGetUserUsageTrend` 并改为：

```go
trend, err := s.repo.GetUserUsageTrend(s.ctx, startTime, endTime, "day", nil, 10)
```

- [x] **Step 2: 运行 Repository 测试确认 RED**

在仓库根目录运行，使用 `llm-wiki/wiki/ops.md` 的仓库内 Go cache，并为本轮创建新的 `GOTMPDIR`：

```powershell
$backend = Resolve-Path backend
$cacheRoot = Join-Path $backend ".gocache"
$env:GOCACHE = Join-Path $cacheRoot "review-cache"
$env:GOPATH = Join-Path $cacheRoot "review-gopath"
$env:GOMODCACHE = Join-Path $env:GOPATH "pkg\mod"
$env:GOTMPDIR = Join-Path $cacheRoot ("run-tmp-user-trend-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $env:GOCACHE,$env:GOMODCACHE,$env:GOTMPDIR | Out-Null
Push-Location backend
go test -tags=integration -p 1 -count=1 ./internal/repository -run 'TestUsageLogRepoSuite/TestGetUserUsageTrend' -v
Pop-Location
```

Expected: FAIL；`GetUserUsageTrend` 尚不接受 `userIDs`，或选人 SQL 尚未实现。

- [x] **Step 3: 扩展 Repository 接口和实现**

将接口签名统一为：

```go
GetUserUsageTrend(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userIDs []int64,
	limit int,
) ([]usagestats.UserUsageTrendPoint, error)
```

在 `usage_log_repo_trend.go` 引入 `github.com/lib/pq`，并让具体实现先复制用户 ID，避免调用方修改切片：

```go
func (r *usageLogRepository) GetUserUsageTrend(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userIDs []int64,
	limit int,
) ([]UserUsageTrendPoint, error) {
	if len(userIDs) > 0 {
		return r.getSelectedUserUsageTrend(ctx, startTime, endTime, granularity, append([]int64(nil), userIDs...))
	}
	return r.getTopUserUsageTrend(ctx, startTime, endTime, granularity, limit)
}
```

把当前函数体移动到 `getTopUserUsageTrend`，SQL内容保持不变。新增完整选人查询：

```go
func selectedUserUsageTrendQuery(granularity string) string {
	dateFormat := safeDateFormat(granularity)
	return fmt.Sprintf(`
		SELECT
			TO_CHAR(u.created_at AT TIME ZONE 'Asia/Shanghai', '%s') AS date,
			u.user_id,
			COALESCE(us.email, '') AS email,
			COALESCE(us.username, '') AS username,
			COUNT(*) AS requests,
			COALESCE(SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens), 0) AS tokens,
			COALESCE(SUM(u.total_cost), 0) AS cost,
			COALESCE(SUM(u.actual_cost), 0) AS actual_cost
		FROM usage_logs u
		LEFT JOIN users us ON us.id = u.user_id
		WHERE u.user_id = ANY($1)
		  AND u.created_at >= $2
		  AND u.created_at < $3
		GROUP BY date, u.user_id, us.email, us.username
		ORDER BY date ASC, u.user_id ASC
	`, dateFormat)
}

func (r *usageLogRepository) getSelectedUserUsageTrend(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userIDs []int64,
) (results []UserUsageTrendPoint, err error) {
	rows, err := r.sql.QueryContext(ctx, selectedUserUsageTrendQuery(granularity), pq.Array(userIDs), startTime, endTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results = make([]UserUsageTrendPoint, 0)
	for rows.Next() {
		var row UserUsageTrendPoint
		if err = rows.Scan(&row.Date, &row.UserID, &row.Email, &row.Username, &row.Requests, &row.Tokens, &row.Cost, &row.ActualCost); err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
```

- [ ] **Step 4: 运行 Repository 测试确认 GREEN**

重新创建新的 `GOTMPDIR`，重复 Step 2 命令。

Expected: PASS；Top N、选人日分桶和选人小时分桶全部通过。

2026-07-14 当前证据：integration 测试二进制已在 `-tags=integration` 下编译通过；本机没有 Docker/PostgreSQL/外部 DSN，尚不能把 SQL 运行态标记为 GREEN。

### Task 2: Handler 验证、Service 透传与缓存隔离

**Files:**
- Modify: `backend/internal/service/dashboard_service.go:352`
- Modify: `backend/internal/handler/admin/dashboard_handler.go:449`
- Modify: `backend/internal/handler/admin/dashboard_query_cache.go:44`
- Modify: `backend/internal/handler/admin/dashboard_handler_cache_test.go:43`

- [x] **Step 1: 写参数与缓存失败测试**

扩展 `dashboardUsageRepoCacheProbe` 记录收到的用户集合：

```go
type dashboardUsageRepoCacheProbe struct {
	service.UsageLogRepository
	trendCalls          atomic.Int32
	usersTrendCalls     atomic.Int32
	lastUsersTrendIDsMu sync.Mutex
	lastUsersTrendIDs   []int64
}
```

添加表驱动 Handler 测试：

```go
func TestDashboardHandler_GetUserUsageTrend_SelectedUsersValidation(t *testing.T) {
	tests := []struct {
		name string
		url  string
		code int
	}{
		{"valid day", "/admin/dashboard/users-trend?user_ids=8,7,8&start_date=2026-07-01&end_date=2026-07-30&granularity=day", http.StatusOK},
		{"invalid id", "/admin/dashboard/users-trend?user_ids=7,nope&start_date=2026-07-01&end_date=2026-07-01", http.StatusBadRequest},
		{"empty segment", "/admin/dashboard/users-trend?user_ids=7,,8&start_date=2026-07-01&end_date=2026-07-01", http.StatusBadRequest},
		{"too many", "/admin/dashboard/users-trend?user_ids=1,2,3,4,5,6&start_date=2026-07-01&end_date=2026-07-01", http.StatusBadRequest},
		{"over 90 days", "/admin/dashboard/users-trend?user_ids=7&start_date=2026-01-01&end_date=2026-04-01&granularity=day", http.StatusBadRequest},
		{"hour spans days", "/admin/dashboard/users-trend?user_ids=7&start_date=2026-07-01&end_date=2026-07-02&granularity=hour", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDashboardReadCachesForTest()
			repo := &dashboardUsageRepoCacheProbe{}
			dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
			handler := NewDashboardHandler(dashboardSvc, nil)
			router := gin.New()
			router.GET("/admin/dashboard/users-trend", handler.GetUserUsageTrend)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.url, nil))
			require.Equal(t, tt.code, recorder.Code)
		})
	}
}
```

新增缓存测试：`user_ids=8,7,8` 与 `user_ids=7,8` 第二次应命中同一缓存；`user_ids=7,9` 必须 miss 并让 Repository 调用次数加一。

- [x] **Step 2: 运行 Handler 测试确认 RED**

```powershell
# 沿用 Task 1 的 GOCACHE/GOPATH/GOMODCACHE，但创建新的 GOTMPDIR。
$env:GOTMPDIR = Join-Path (Resolve-Path 'backend/.gocache') ("run-tmp-user-trend-handler-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null
Push-Location backend
go test -p 1 -count=1 ./internal/handler/admin -run 'TestDashboardHandler_GetUserUsageTrend' -v
Pop-Location
```

Expected: FAIL；`user_ids` 尚未校验、缓存键尚未包含用户集合。

- [x] **Step 3: 实现规范化与范围验证**

在 `dashboard_handler.go` 新增常量和完整 helper：

```go
const (
	selectedUserTrendMaxUsers = 5
	selectedUserTrendMaxDays  = 90
)

func parseSelectedUserTrendIDs(raw string) ([]int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	seen := make(map[int64]struct{}, len(parts))
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, errors.New("user_ids contains an empty value")
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("user_ids must contain positive integers")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) > selectedUserTrendMaxUsers {
		return nil, fmt.Errorf("user_ids supports at most %d users", selectedUserTrendMaxUsers)
	}
	slices.Sort(ids)
	return ids, nil
}

func parseSelectedUserTrendRange(c *gin.Context, granularity string) (time.Time, time.Time, error) {
	startRaw := strings.TrimSpace(c.Query("start_date"))
	endRaw := strings.TrimSpace(c.Query("end_date"))
	if startRaw == "" || endRaw == "" {
		return time.Time{}, time.Time{}, errors.New("start_date and end_date are required")
	}
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	start, err := time.ParseInLocation("2006-01-02", startRaw, loc)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid start_date")
	}
	endDate, err := time.ParseInLocation("2006-01-02", endRaw, loc)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid end_date")
	}
	if endDate.Before(start) {
		return time.Time{}, time.Time{}, errors.New("end_date must not be before start_date")
	}
	if granularity != "day" && granularity != "hour" {
		return time.Time{}, time.Time{}, errors.New("granularity must be day or hour")
	}
	if granularity == "hour" && !start.Equal(endDate) {
		return time.Time{}, time.Time{}, errors.New("hour granularity requires a single day")
	}
	end := endDate.AddDate(0, 0, 1)
	if granularity == "day" && int(end.Sub(start).Hours()/24) > selectedUserTrendMaxDays {
		return time.Time{}, time.Time{}, fmt.Errorf("day granularity supports at most %d days", selectedUserTrendMaxDays)
	}
	return start, end, nil
}
```

`GetUserUsageTrend` 中先解析 `user_ids`。有 IDs 时使用严格北京时间范围；无 IDs 时继续使用 `parseTimeRange`，保持旧行为。

Handler 必须使用 `c.GetQuery("user_ids")` 区分参数是否出现：完全未传保持 Top N，显式 `user_ids=` 即使 helper 返回空切片也返回 400。

- [x] **Step 4: 更新 Service 和缓存签名**

`DashboardService` 透传 `userIDs`：

```go
func (s *DashboardService) GetUserUsageTrend(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userIDs []int64,
	limit int,
) ([]usagestats.UserUsageTrendPoint, error) {
	trend, err := s.usageRepo.GetUserUsageTrend(ctx, startTime, endTime, granularity, userIDs, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get user usage trend: %w", err)
	}
	return trend, nil
}
```

新增专用缓存键，避免改变 API Key 趋势缓存：

```go
type dashboardUserTrendCacheKey struct {
	StartTime   string  `json:"start_time"`
	EndTime     string  `json:"end_time"`
	Granularity string  `json:"granularity"`
	Limit       int     `json:"limit"`
	UserIDs     []int64 `json:"user_ids,omitempty"`
}
```

`getUserUsageTrendCached` 接收 `userIDs`；复制切片写入 key 和 loader。选人模式把 `Limit` 规范化为 0，无 IDs 时保留真实 limit。

同步修改 `dashboard_snapshot_v2_handler.go` 的旧调用：

```go
usersTrend, _, err := h.getUserUsageTrendCached(
	ctx,
	startTime,
	endTime,
	granularity,
	nil,
	usersTrendLimit,
)
```

这样 `snapshot-v2?include_users_trend=true` 继续走 Top N，不进入精确选人分支。

- [x] **Step 5: 运行 Handler 与 Service 相关测试确认 GREEN**

```powershell
$env:GOTMPDIR = Join-Path (Resolve-Path 'backend/.gocache') ("run-tmp-user-trend-service-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null
Push-Location backend
go test -p 1 -count=1 ./internal/handler/admin ./internal/service -run 'Dashboard.*UserUsageTrend|GetUserUsageTrend' -v
Pop-Location
```

Expected: PASS；旧 Top N 缓存测试、选人验证与缓存规范化测试全部通过。

### Task 3: 前端 API 序列化合同

**Files:**
- Modify: `frontend/src/api/admin/dashboard.ts:229`
- Create: `frontend/src/api/__tests__/admin.dashboard.spec.ts`

- [x] **Step 1: 写 `user_ids` 序列化失败测试**

沿用其他 admin API 测试的 `@/api/client` mock 模式：

```ts
import { beforeEach, describe, expect, it, vi } from 'vitest'
import dashboardAPI from '../admin/dashboard'

const get = vi.hoisted(() => vi.fn())

vi.mock('../client', () => ({
  apiClient: { get }
}))

describe('admin dashboard user trend API', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({ data: { trend: [] } })
  })

  it('serializes selected user IDs as a stable comma-separated query value', async () => {
    await dashboardAPI.getUserUsageTrend({
      user_ids: [8, 7],
      start_date: '2026-07-01',
      end_date: '2026-07-01',
      granularity: 'hour'
    })

    expect(get).toHaveBeenCalledWith('/admin/dashboard/users-trend', {
      params: {
        user_ids: '8,7',
        start_date: '2026-07-01',
        end_date: '2026-07-01',
        granularity: 'hour'
      }
    })
  })
})
```

- [x] **Step 2: 运行 API 测试确认 RED**

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/api/__tests__/admin.dashboard.spec.ts
```

Expected: FAIL；`UserTrendParams` 尚无 `user_ids`，请求也未序列化数组。

- [x] **Step 3: 实现 API 参数类型与序列化**

```ts
export interface UserTrendParams extends TrendParams {
  limit?: number
  user_ids?: number[]
}

export async function getUserUsageTrend(params?: UserTrendParams): Promise<UserTrendResponse> {
  const { user_ids, ...rest } = params || {}
  const { data } = await apiClient.get<UserTrendResponse>('/admin/dashboard/users-trend', {
    params: {
      ...rest,
      ...(user_ids?.length ? { user_ids: user_ids.join(',') } : {})
    }
  })
  return data
}
```

- [x] **Step 4: 运行 API 测试确认 GREEN**

重复 Step 2 命令。

Expected: PASS。

### Task 4: Token Analysis 用户选择和折线图

**Files:**
- Modify: `frontend/src/views/admin/TokenAnalysisView.vue:300`
- Modify: `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts:1`
- Modify: `frontend/src/i18n/locales/zh/admin/tokenAnalysis.ts:15`
- Modify: `frontend/src/i18n/locales/en/admin/tokenAnalysis.ts:15`

- [x] **Step 1: 扩展页面测试 mock 并写选择失败测试**

给 `adminAPI` mock 增加 Dashboard API，并把现有 `vi.mock('@/api/admin')` 替换为同时暴露两个 namespace；`Line` stub 暴露收到的图表数据：

```ts
const dashboardApi = vi.hoisted(() => ({
  getUserUsageTrend: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    tokenAnalysis: api,
    dashboard: dashboardApi
  }
}))

vi.mock('vue-chartjs', () => ({
  Line: {
    name: 'Line',
    props: ['data', 'options'],
    template: '<div data-testid="selected-user-trend-chart" />'
  }
}))
```

新增用例：

```ts
it('selects ranking users and loads authoritative daily usage trend', async () => {
  api.listUsers.mockResolvedValue({
    items: [
      { user_id: 7, user_email: 'a@example.com', total_tokens: 100, actual_cost: 1 },
      { user_id: 8, user_email: 'b@example.com', total_tokens: 200, actual_cost: 2 }
    ],
    total: 2,
    page: 1,
    page_size: 20
  })
  dashboardApi.getUserUsageTrend.mockResolvedValue({
    trend: [
      { date: '2026-07-01', user_id: 7, email: 'a@example.com', username: '', requests: 1, tokens: 100, cost: 1, actual_cost: 1 },
      { date: '2026-07-01', user_id: 8, email: 'b@example.com', username: '', requests: 1, tokens: 200, cost: 2, actual_cost: 2 }
    ],
    start_date: '2026-07-01',
    end_date: '2026-07-01',
    granularity: 'day'
  })
  const wrapper = mount(TokenAnalysisView, {
    global: { stubs: { AppLayout: AppLayoutStub } }
  })
  await flushPromises()

  const selectors = wrapper.findAll('input[data-user-trend-select]')
  await selectors[0].setValue(true)
  await selectors[1].setValue(true)
  await flushPromises()

  expect(dashboardApi.getUserUsageTrend).toHaveBeenLastCalledWith({
    user_ids: [7, 8],
    start_date: expect.any(String),
    end_date: expect.any(String),
    granularity: 'day'
  })
  expect(wrapper.find('[data-testid="selected-user-trend-chart"]').exists()).toBe(true)
})
```

增加以下四个独立用例，每个用例显式 mock `listUsers` 与 `dashboardApi.getUserUsageTrend`：

| 测试名 | fixture | 精确断言 |
| --- | --- | --- |
| `disables unselected users after five selections` | 当前页返回 6 个不同 `user_id` | 依次选择前 5 个后，第 6 个 `disabled`；前 5 个仍可取消 |
| `keeps selected users when the ranking page changes` | 第一页用户 7，第二次 `listUsers` 返回用户 8 | 选择 7 后触发下一页加载，趋势请求仍包含 `user_ids: [7]` |
| `ignores an older trend response after the selection changes` | 两个手动 deferred Promise | 先选 7，再选 8；第二个响应先完成后再完成第一个，Chart datasets 仍包含 7/8 的最新结果 |
| `fills missing user periods with zero` | 日期范围 7/1～7/3，后端只返回用户 7 的 7/2 | `Line` 的 labels 为三天，用户 7 dataset 为 `[0, value, 0]` |

- [x] **Step 2: 写粒度和范围失败测试**

覆盖以下行为：

```ts
it('enables hourly mode only for a single selected date', async () => {
  api.listUsers.mockResolvedValue({
    items: [{ user_id: 7, user_email: 'a@example.com', total_tokens: 100, actual_cost: 1 }],
    total: 1,
    page: 1,
    page_size: 20
  })
  dashboardApi.getUserUsageTrend.mockResolvedValue({ trend: [] })
  const wrapper = mount(TokenAnalysisView, { global: { stubs: { AppLayout: AppLayoutStub } } })
  await flushPromises()
  await wrapper.find('input[data-user-trend-select]').setValue(true)
  const hourButton = wrapper.findAll('button').find((button) => button.text().includes('admin.tokenAnalysis.trendHour'))
  expect(hourButton).toBeTruthy()
  expect(hourButton!.attributes('disabled')).toBeUndefined()
  await hourButton!.trigger('click')
  await flushPromises()
  expect(dashboardApi.getUserUsageTrend).toHaveBeenLastCalledWith(expect.objectContaining({ granularity: 'hour' }))
})

it('does not request hourly trend for a multi-day range', async () => {
  api.listUsers.mockResolvedValue({
    items: [{ user_id: 7, user_email: 'a@example.com', total_tokens: 100, actual_cost: 1 }],
    total: 1,
    page: 1,
    page_size: 20
  })
  dashboardApi.getUserUsageTrend.mockResolvedValue({ trend: [] })
  const wrapper = mount(TokenAnalysisView, { global: { stubs: { AppLayout: AppLayoutStub } } })
  await flushPromises()
  const dateInputs = wrapper.findAll('input[type="date"]')
  expect(dateInputs).toHaveLength(2)
  await dateInputs[0].setValue('2026-07-01')
  await dateInputs[1].setValue('2026-07-02')
  await wrapper.find('input[data-user-trend-select]').setValue(true)
  await flushPromises()
  const callsBefore = dashboardApi.getUserUsageTrend.mock.calls.length
  const hourButton = wrapper.findAll('button').find((button) => button.text().includes('admin.tokenAnalysis.trendHour'))
  expect(hourButton!.attributes('disabled')).toBeDefined()
  await hourButton!.trigger('click')
  expect(dashboardApi.getUserUsageTrend).toHaveBeenCalledTimes(callsBefore)
})

it('clears stale chart data and exposes retry after a load error', async () => {
  api.listUsers.mockResolvedValue({
    items: [{ user_id: 7, user_email: 'a@example.com', total_tokens: 100, actual_cost: 1 }],
    total: 1,
    page: 1,
    page_size: 20
  })
  dashboardApi.getUserUsageTrend.mockRejectedValueOnce(new Error('network'))
  const wrapper = mount(TokenAnalysisView, { global: { stubs: { AppLayout: AppLayoutStub } } })
  await flushPromises()
  await wrapper.find('input[data-user-trend-select]').setValue(true)
  await flushPromises()
  expect(wrapper.text()).toContain('admin.tokenAnalysis.trendLoadFailed')
  expect(wrapper.find('[data-testid="selected-user-trend-chart"]').exists()).toBe(false)

  dashboardApi.getUserUsageTrend.mockResolvedValueOnce({ trend: [] })
  const retry = wrapper.findAll('button').find((button) => button.text().includes('admin.tokenAnalysis.trendRetry'))
  await retry!.trigger('click')
  await flushPromises()
  expect(dashboardApi.getUserUsageTrend).toHaveBeenCalledTimes(2)
})
```

- [x] **Step 3: 运行页面测试确认 RED**

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/views/admin/__tests__/TokenAnalysisView.spec.ts
```

Expected: FAIL；页面尚无用户选择、趋势状态或折线图。

- [x] **Step 4: 注册 Chart.js 并增加趋势状态**

在页面脚本中增加：

```ts
import type { UserUsageTrendPoint } from '@/types'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend
} from 'chart.js'
import { Line } from 'vue-chartjs'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend)

type TrendGranularity = 'day' | 'hour'
type SelectedTrendUser = { user_id: number; email: string }

const selectedTrendUsers = ref<SelectedTrendUser[]>([])
const userTrendPoints = ref<UserUsageTrendPoint[]>([])
const userTrendGranularity = ref<TrendGranularity>('day')
const userTrendLoading = ref(false)
const userTrendError = ref(false)
let userTrendLoadSeq = 0
const MAX_SELECTED_TREND_USERS = 5
```

实现 `toggleTrendUser`、`isTrendUserSelected` 和 `trendUserSelectionDisabled`。选择顺序决定颜色顺序；跨分页不清空 `selectedTrendUsers`。

- [x] **Step 5: 实现完整横轴、Chart data 和加载竞态**

完整横轴 helper：

```ts
function buildTrendLabels(startDate: string, endDate: string, granularity: TrendGranularity): string[] {
  if (!startDate || !endDate) return []
  if (granularity === 'hour') {
    if (startDate !== endDate) return []
    return Array.from({ length: 24 }, (_, hour) => `${startDate} ${String(hour).padStart(2, '0')}:00`)
  }

  const labels: string[] = []
  const cursor = new Date(`${startDate}T00:00:00Z`)
  const end = new Date(`${endDate}T00:00:00Z`)
  while (cursor <= end) {
    labels.push(cursor.toISOString().slice(0, 10))
    cursor.setUTCDate(cursor.getUTCDate() + 1)
  }
  return labels
}
```

`selectedUserTrendChartData` 按 `(user_id, date)` 建 Map，并对每个 label 使用 `map.get(label) ?? 0`。颜色数组复用 Dashboard 的稳定色板。

加载函数必须先清空旧数据，并丢弃迟到响应：

```ts
async function loadSelectedUserTrend() {
  const seq = ++userTrendLoadSeq
  userTrendPoints.value = []
  userTrendError.value = false
  if (selectedTrendUsers.value.length === 0) {
    userTrendLoading.value = false
    return
  }

  userTrendLoading.value = true
  try {
    const response = await adminAPI.dashboard.getUserUsageTrend({
      user_ids: selectedTrendUsers.value.map((user) => user.user_id),
      start_date: filters.start_date,
      end_date: filters.end_date,
      granularity: userTrendGranularity.value
    })
    if (seq !== userTrendLoadSeq) return
    userTrendPoints.value = response.trend || []
  } catch {
    if (seq !== userTrendLoadSeq) return
    userTrendError.value = true
  } finally {
    if (seq === userTrendLoadSeq) userTrendLoading.value = false
  }
}
```

`reloadAll` 在存在选择时把 `loadSelectedUserTrend()` 放入同一轮 `Promise.all`；切换用户和粒度时只重载趋势。

- [x] **Step 6: 实现排行复选框和响应式趋势面板**

用户排行表头新增固定宽度选择列，行内复选框：

```vue
<input
  v-if="user.user_id"
  data-user-trend-select
  type="checkbox"
  :checked="isTrendUserSelected(user.user_id)"
  :disabled="trendUserSelectionDisabled(user.user_id)"
  :aria-label="t('admin.tokenAnalysis.selectUserTrend', { email: user.user_email || user.user_id })"
  @change="toggleTrendUser(user)"
/>
```

在排行/请求区域之后增加独立 `.card` 面板，仅在选择非空时渲染。顶部使用两按钮分段控制“按日 / 按小时”，小时按钮在多日范围禁用并提供 `title`。面板内容包含固定高度 `h-72 md:h-80` 的图表容器、加载、错误重试和无数据状态；不把卡片嵌套进其他卡片。

用户排行同时补 `Pagination`，只切换当前排行页，不清空 `selectedTrendUsers`；筛选条件整体刷新时页码回到 1，但已选集合继续保留并重新加载趋势。

- [x] **Step 7: 补中英文文案**

两个 locale 文件增加同构 key：

```ts
usageTrend: '计费用量趋势',
selectedUsersCount: '已选 {count}/5',
selectUserTrend: '选择 {email} 的用量趋势',
trendDay: '按日',
trendHour: '按小时',
trendHourSingleDayHint: '按小时仅支持同一天',
trendLoadFailed: '用量趋势加载失败',
trendRetry: '重试',
trendNoUsage: '所选用户在当前范围内没有计费用量'
```

英文对应 `Billing Usage Trend`、`Selected {count}/5`、`Daily`、`Hourly` 等。

- [x] **Step 8: 运行页面测试确认 GREEN**

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/views/admin/__tests__/TokenAnalysisView.spec.ts src/api/__tests__/admin.dashboard.spec.ts
```

Expected: PASS；选择、上限、日/小时、补零、竞态、错误和 API 序列化全部通过。

### Task 5: 文档、性能验证与完整回归

**Files:**
- Modify: `llm-wiki/wiki/backend.md`
- Modify: `llm-wiki/wiki/frontend.md`
- Modify: `llm-wiki/wiki/data-and-domain.md`
- Verify: all files above

- [x] **Step 1: 更新 llm-wiki 稳定知识**

`backend.md` 记录 `/admin/dashboard/users-trend` 的兼容双模式、5 人/90 天/单日小时限制与缓存键；`frontend.md` 记录 Token Analysis 的选择状态、总 Token 折线图和迟到响应保护；`data-and-domain.md` 记录该图使用 `usage_logs`、默认 90 天原始日志边界，且不使用全局预聚合表或归档摘要。

- [x] **Step 2: 运行不依赖 PostgreSQL 的 Go 回归测试**

```powershell
$env:GOTMPDIR = Join-Path (Resolve-Path 'backend/.gocache') ("run-tmp-user-trend-final-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null
Push-Location backend
go test -p 1 -count=1 ./internal/handler/admin
go test -p 1 -count=1 ./internal/service
go test -p 1 -count=1 ./internal/server
Pop-Location
```

Expected: Handler 与 Service 返回 PASS；Server 没有测试文件时返回 `[no test files]` 且 exit code 0。

2026-07-14 当前证据：Handler 使用三轮 fresh `GOTMPDIR` 均通过；Service 全包通过；Server 返回 `[no test files]` 且 exit code 0。Windows 在两轮 Handler 测试退出后曾报告清理临时 `admin.test.exe` 时 `Access is denied`，但对应测试命令均已返回 `ok`，未复用受锁目录。

- [ ] **Step 3: 运行 Repository PostgreSQL integration 测试**

```powershell
$env:GOTMPDIR = Join-Path (Resolve-Path 'backend/.gocache') ("run-tmp-user-trend-repository-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null
Push-Location backend
go test -tags=integration -p 1 -count=1 ./internal/repository -run 'TestUsageLogRepoSuite/TestGetUserUsageTrend' -v
Pop-Location
```

Expected: Top N、精确选人、北京时间日/小时分桶和四类 Token 求和用例全部 PASS。

2026-07-14 当前证据：integration 测试二进制已编译通过；本机没有 Docker、PostgreSQL 服务或外部 DSN，SQL 运行态仍未验证。

- [ ] **Step 4: 对选人 SQL 运行真实 PostgreSQL EXPLAIN ANALYZE**

新建 `backend/internal/repository/user_usage_trend_explain_integration_test.go`。测试复用同包的 `integrationDB`、`seedOrganizationUsageExplainData`、`cleanupOrganizationUsageExplainData` 与 `explainOrganizationUsageQuery`，仅在 `USER_USAGE_TREND_RUN_EXPLAIN=1` 时运行；从 `org-usage-perf-%` fixture 中按 ID 取前 5 名用户，对 1/5 人分别执行日 90 天和小时 1 天。

```go
//go:build integration

package repository

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestSelectedUserUsageTrendExplainAnalyze(t *testing.T) {
	if os.Getenv("USER_USAGE_TREND_RUN_EXPLAIN") != "1" {
		t.Skip("set USER_USAGE_TREND_RUN_EXPLAIN=1 to analyze selected-user trend")
	}
	ctx := context.Background()
	cleanupOrganizationUsageExplainData(t, ctx)
	seedOrganizationUsageExplainData(t, ctx)
	t.Cleanup(func() { cleanupOrganizationUsageExplainData(t, context.Background()) })
	_, err := integrationDB.ExecContext(ctx, `ANALYZE users`)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `ANALYZE usage_logs`)
	require.NoError(t, err)

	rows, err := integrationDB.QueryContext(ctx, `
		SELECT id FROM users WHERE email LIKE $1 ORDER BY id LIMIT 5`,
		organizationUsageExplainPrefix+"%",
	)
	require.NoError(t, err)
	defer rows.Close()
	userIDs := make([]int64, 0, 5)
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		userIDs = append(userIDs, id)
	}
	require.NoError(t, rows.Err())
	require.Len(t, userIDs, 5)

	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	dayEnd := time.Date(2026, 7, 1, 0, 0, 0, 0, loc)
	tests := []struct {
		name        string
		granularity string
		users       []int64
		start       time.Time
		end         time.Time
	}{
		{"day-1-user", "day", userIDs[:1], dayEnd.AddDate(0, 0, -90), dayEnd},
		{"day-5-users", "day", userIDs, dayEnd.AddDate(0, 0, -90), dayEnd},
		{"hour-1-user", "hour", userIDs[:1], dayEnd.AddDate(0, 0, -1), dayEnd},
		{"hour-5-users", "hour", userIDs, dayEnd.AddDate(0, 0, -1), dayEnd},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := explainOrganizationUsageQuery(
				t,
				ctx,
				selectedUserUsageTrendQuery(tt.granularity),
				pq.Array(tt.users),
				t.start.UTC(),
				t.end.UTC(),
			)
			require.True(t, selectedTrendPlanUsesUserCreatedIndex(plan.Plan))
			t.Logf("USER_TREND_EXPLAIN case=%s execution_ms=%.3f rows=%.0f", tt.name, plan.ExecutionTime, plan.Plan.ActualRows)
		})
	}
}

func selectedTrendPlanUsesUserCreatedIndex(plan organizationUsageExplainPlan) bool {
	if strings.Contains(plan.NodeType, "Index") && plan.IndexName == "idx_usage_logs_user_created" {
		return true
	}
	for _, child := range plan.Plans {
		if selectedTrendPlanUsesUserCreatedIndex(child) {
			return true
		}
	}
	return false
}
```

```sql
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
SELECT
    TO_CHAR(u.created_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD') AS date,
    u.user_id,
    SUM(u.input_tokens + u.output_tokens + u.cache_creation_tokens + u.cache_read_tokens)
FROM usage_logs u
WHERE u.user_id = ANY($1)
  AND u.created_at >= $2
  AND u.created_at < $3
GROUP BY date, u.user_id;
```

运行命令：

```powershell
$env:USER_USAGE_TREND_RUN_EXPLAIN = "1"
$env:SUB2API_POSTGRES_ONLY_INTEGRATION_DSN = "<temporary PostgreSQL DSN>"
Push-Location backend
go test -tags=integration -p 1 -count=1 ./internal/repository -run '^TestSelectedUserUsageTrendExplainAnalyze$' -v
Pop-Location
Remove-Item Env:USER_USAGE_TREND_RUN_EXPLAIN
Remove-Item Env:SUB2API_POSTGRES_ONLY_INTEGRATION_DSN
```

验收：计划使用 `idx_usage_logs_user_created` 的 Index/Bitmap Index Scan；不得出现因选 1～5 人而扫描全部历史 `usage_logs`。记录执行时间、命中行数和 buffers，但本轮不因合成数据结果新增索引。

2026-07-14 当前环境：测试文件和 Repository integration 测试二进制已编译通过；本机无 Docker、PostgreSQL 服务/CLI、监听端口和 `SUB2API_POSTGRES_ONLY_INTEGRATION_DSN`，因此本步骤保持未完成，不能用静态检查替代真实 `EXPLAIN ANALYZE`。

- [x] **Step 5: 运行前端完整验证**

```powershell
cmd.exe /c pnpm --dir frontend run test:run
cmd.exe /c pnpm --dir frontend run typecheck
cmd.exe /c pnpm --dir frontend run lint:check
```

Expected: 全部 exit code 0。

2026-07-14 合入 0.1.155 后结果：全量 Vitest `174` 个测试文件、`1143/1143` 通过；Token Analysis 页面 `24/24`，Dashboard API `2/2`；typecheck 与 lint:check 均通过。

- [ ] **Step 6: 浏览器验收**

启动前后端并以管理员身份打开：

```text
http://localhost:3000/admin/token-analysis
```

桌面和移动视口验证：勾选 1/5 人、达到上限、翻页后选择保留、按日折线、单日按小时、跨日小时禁用、快速切换不出现旧数据、错误重试、无重叠或横向溢出。检查控制台没有相关 error/warn，并保存仓库外截图证据。

- [x] **Step 7: 最终补丁检查**

```powershell
git diff --check
git status --short
```

Expected: `git diff --check` 无输出；没有 migration、路由或新组件目录；保留当前本地分支，按用户既定要求不提交、不推送。

2026-07-14 当前结果：`git diff --check` exit code 0；未新增 migration、页面路由或组件目录，工作区保留为未提交状态。

## 0.1.155 基线合并验证

2026-07-14 将本地 `feature/hy/10155_同步sub2api主线` 合入本分支，使用 `git merge --no-commit --no-ff` 停在待审核状态：

| 检查项 | 结果 |
| --- | --- |
| 文本冲突 / 未解决文件 | 无 |
| Handler、Service、Repository、Server Routes | 全部通过；Server 包无测试文件 |
| Repository integration 测试二进制 | `-tags=integration` 编译通过 |
| 前端全量 Vitest | `174` 个文件、`1143/1143` 通过 |
| TypeScript / ESLint | `typecheck`、`lint:check` 通过 |
| 生产构建 | 前端 Vite 构建通过；后端 `-tags embed` 构建 0.1.155 通过 |
| 真实 PostgreSQL integration / EXPLAIN | 仍待测试环境执行 |
| 合并提交 | 未创建，等待用户审核 |
