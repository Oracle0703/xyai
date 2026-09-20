//go:build integration

package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Count application SQL calls on real PG connections, excluding BEGIN/COMMIT.
type departmentCountingConnector struct {
	driver.Connector
	calls   atomic.Int64
	mu      sync.Mutex
	queries []departmentQuerySample
}
type departmentQuerySample struct {
	text string
	args []any
}

func (c *departmentCountingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &departmentCountingConn{Conn: conn, calls: &c.calls, owner: c}, nil
}

type departmentCountingConn struct {
	driver.Conn
	calls *atomic.Int64
	owner *departmentCountingConnector
}

func (c *departmentCountingConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	c.calls.Add(1)
	values := make([]any, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	c.owner.mu.Lock()
	c.owner.queries = append(c.owner.queries, departmentQuerySample{q, values})
	c.owner.mu.Unlock()
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return queryer.QueryContext(ctx, q, args)
}
func (c *departmentCountingConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	c.calls.Add(1)
	execer, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return execer.ExecContext(ctx, q, args)
}
func (c *departmentCountingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	beginner, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, driver.ErrSkip
	}
	return beginner.BeginTx(ctx, opts)
}

func newDepartmentCountingDB(t *testing.T) (*sql.DB, *departmentCountingConnector) {
	t.Helper()
	dsn := os.Getenv("SUB2API_POSTGRES_ONLY_INTEGRATION_DSN")
	if dsn == "" {
		t.Skip("query instrumentation requires the isolated PostgreSQL DSN")
	}
	base, err := pq.NewConnector(dsn)
	require.NoError(t, err)
	connector := &departmentCountingConnector{Connector: base}
	db := sql.OpenDB(connector)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db, connector
}

func TestDepartmentQueryCountIntegration_BoundedPagesAndGlobalFastPath(t *testing.T) {
	ctx := context.Background()
	prefix := organizationUsageIntegrationPrefix("query_count")
	cleanupOrganizationUsageIntegrationData(t, prefix)
	admin, _ := organizationUsageIntegrationUser(t, prefix+"admin@example.com", service.StatusActive)
	_, err := integrationDB.Exec(`UPDATE users SET role='admin' WHERE id=$1`, admin.ID)
	require.NoError(t, err)
	actorCtx := service.WithDepartmentActor(ctx, admin.ID)
	setup := service.NewDepartmentService(NewDepartmentRepository(integrationDB))
	ids := []int64{}
	t.Cleanup(func() {
		_, e := integrationDB.Exec(`DELETE FROM departments WHERE id=ANY($1)`, pq.Array(ids))
		require.NoError(t, e)
		_, e = integrationDB.Exec(`DELETE FROM audit_logs WHERE actor_user_id=$1`, admin.ID)
		require.NoError(t, e)
	})
	for _, org := range []string{service.OrganizationXunyou, service.OrganizationWsdashi} {
		d, e := setup.Save(actorCtx, 0, service.DepartmentSaveInput{Organization: org, Name: prefix})
		require.NoError(t, e)
		ids = append(ids, d.ID)
	}
	db, counter := newDepartmentCountingDB(t)
	svc := service.NewDepartmentService(NewDepartmentRepository(db))
	for _, size := range []int{1, 200} {
		counter.calls.Store(0)
		page, e := svc.List(actorCtx, service.DepartmentListFilter{Organization: "all", Q: prefix, Page: 1, PageSize: size})
		require.NoError(t, e)
		require.Equal(t, int64(2), page.Total)
		require.Len(t, page.Items, min(2, size))
		require.Equal(t, int64(4), counter.calls.Load(), "department page size must not add one query per row")
	}
	counter.calls.Store(0)
	_, err = svc.Members(actorCtx, service.DepartmentListFilter{Q: prefix, Page: 1, PageSize: 1})
	require.NoError(t, err)
	require.Equal(t, int64(3), counter.calls.Load())
	counter.calls.Store(0)
	scope, err := svc.QueryScope(actorCtx, service.AdminPermissionDepartmentSubscriptions, service.DepartmentScopeQuery{})
	require.NoError(t, err)
	require.True(t, scope.Unrestricted)
	require.Nil(t, scope.Members)
	require.Empty(t, scope.Version)
	require.Equal(t, int64(1), counter.calls.Load(), "a global subscription scope needs only current actor authentication")
	t.Log("department pages=4 SQL calls; member page=3; global subscription scope=1; independent of page size")
	// Explain the actual SQL and bindings captured above, outside measured calls.
	for i, sample := range counter.queries {
		var plan string
		require.NoError(t, integrationDB.QueryRowContext(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+sample.text, sample.args...).Scan(&plan))
		t.Logf("DEPARTMENT_ADMIN_PLAN query=%d sql=%s plan=%s", i, sample.text, plan)
	}
}
