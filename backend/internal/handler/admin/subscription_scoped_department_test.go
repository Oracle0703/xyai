//go:build unit

package admin

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type scopedDepartmentSubscriptionStub struct{ subscriptionHandlerService }

func (*scopedDepartmentSubscriptionStub) ListAdmin(_ context.Context, page, size int, filter service.SubscriptionAdminFilter) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	*filter.ResolvedScopeVersion = "department-version"
	return []service.UserSubscription{{ID: 1, UserID: 2, GroupID: 3, DailyUsageUSD: 4,
		User:  &service.User{ID: 2, Email: "private@example.com"},
		Group: &service.Group{ID: 3, RateMultiplier: 42},
	}}, &pagination.PaginationResult{Page: page, PageSize: size, Total: 1, Pages: 1}, nil
}

func TestSubscriptionListDepartmentEnvelopeHonorsScopedProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newSubscriptionHandler(&scopedDepartmentSubscriptionStub{})
	h.departments = service.NewDepartmentService(nil)
	for _, scoped := range []bool{false, true} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/subscriptions", nil)
		ctx := service.WithDepartmentActor(c.Request.Context(), 9)
		if scoped {
			ctx = adminaccess.WithPrincipal(ctx, &adminaccess.Principal{Policy: adminaccess.Policy{GlobalPermissions: []string{"subscriptions.read"}}})
		}
		c.Request = c.Request.WithContext(ctx)
		h.List(c)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), `"scope_version":"department-version"`)
		require.Contains(t, w.Body.String(), `"daily_usage_usd":4`)
		if scoped {
			require.NotContains(t, w.Body.String(), "private@example.com")
			require.NotContains(t, w.Body.String(), "rate_multiplier")
		} else {
			require.Contains(t, w.Body.String(), "private@example.com")
			require.Contains(t, w.Body.String(), "rate_multiplier")
		}
	}
}
