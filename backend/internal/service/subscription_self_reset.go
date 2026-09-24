package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	errors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const SubscriptionSelfResetPolicyKey = "subscription_self_daily_reset_policy"

const (
	SubscriptionSelfResetRolloutOff   = "off"
	SubscriptionSelfResetRolloutAdmin = "admin"
	SubscriptionSelfResetRolloutAll   = "all"
)

// SubscriptionSelfResetIdempotencyScope keeps equal keys from different users apart.
func SubscriptionSelfResetIdempotencyScope(userID int64) string {
	return "subscriptions.self_reset.user:" + strconv.FormatInt(userID, 10)
}

func selfResetStorageError(err error) error {
	if errors.Code(err) < 500 {
		return err
	}
	return errors.ServiceUnavailable("SELF_RESET_UNAVAILABLE", "Self reset storage unavailable").WithCause(err)
}

type SubscriptionSelfResetPolicy struct {
	DailyLimitByOrganization map[string]int `json:"daily_limit_by_organization"`
	Rollout                  string         `json:"rollout"`
}

func ParseSubscriptionSelfResetPolicy(data []byte) (*SubscriptionSelfResetPolicy, error) {
	// Pointer values distinguish an explicit zero from JSON null.
	var input struct {
		Limits  map[string]*int `json:"daily_limit_by_organization"`
		Rollout string          `json:"rollout"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, errors.BadRequest("INVALID_SELF_RESET_POLICY", "Invalid self reset policy")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.BadRequest("INVALID_SELF_RESET_POLICY", "Expected one JSON object")
	}
	if len(input.Limits) != 3 {
		return nil, errors.BadRequest("INVALID_SELF_RESET_POLICY", "All three organizations are required")
	}
	policy := &SubscriptionSelfResetPolicy{DailyLimitByOrganization: make(map[string]int, 3)}
	policy.Rollout = input.Rollout
	if policy.Rollout == "" {
		policy.Rollout = SubscriptionSelfResetRolloutAdmin
	}
	if policy.Rollout != SubscriptionSelfResetRolloutOff && policy.Rollout != SubscriptionSelfResetRolloutAdmin && policy.Rollout != SubscriptionSelfResetRolloutAll {
		return nil, errors.BadRequest("INVALID_SELF_RESET_POLICY", "rollout must be off, admin, or all")
	}
	for _, org := range []string{OrganizationXunyou, OrganizationWsdashi, OrganizationOther} {
		value := input.Limits[org]
		if value == nil || *value < 0 || *value > 100 {
			return nil, errors.BadRequest("INVALID_SELF_RESET_POLICY", "Daily limits must be integers between 0 and 100")
		}
		policy.DailyLimitByOrganization[org] = *value
	}
	return policy, nil
}

func ResolveSubscriptionOrganization(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) > 1 {
		switch strings.ToLower(parts[1]) {
		case "xunyou.com":
			return OrganizationXunyou
		case "wsdashi.com":
			return OrganizationWsdashi
		}
	}
	return OrganizationOther
}

type SubscriptionSelfResetUsage struct {
	QuotaDate string
	UsedCount int
}

type SubscriptionSelfResetRepository interface {
	ReadPolicy(context.Context) (string, error)
	WritePolicy(context.Context, string) error
	ListOwned(context.Context, int64) ([]UserSubscription, map[int64]SubscriptionSelfResetUsage, string, error)
	GetOwnedByIDForUpdate(context.Context, int64, int64) (*UserSubscription, error)
	GetUsage(context.Context, int64) (SubscriptionSelfResetUsage, error)
	Consume(context.Context, int64, string) error
}

type SubscriptionSelfResetItem struct {
	SubscriptionID int64   `json:"subscription_id"`
	UsedCount      int     `json:"used_count"`
	RemainingCount int     `json:"remaining_count"`
	CanReset       bool    `json:"can_reset"`
	DisabledReason *string `json:"disabled_reason"`
}

type SubscriptionSelfResetStatus struct {
	Organization  string                      `json:"organization"`
	DailyLimit    int                         `json:"daily_limit"`
	QuotaDate     string                      `json:"quota_date"`
	ServerNow     time.Time                   `json:"server_now"`
	NextResetAt   time.Time                   `json:"next_reset_at"`
	Subscriptions []SubscriptionSelfResetItem `json:"subscriptions"`
}

func selfResetRolloutAllowed(policy *SubscriptionSelfResetPolicy, role string) bool {
	if policy == nil || policy.Rollout == SubscriptionSelfResetRolloutOff {
		return false
	}
	return policy.Rollout == SubscriptionSelfResetRolloutAll || (policy.Rollout == SubscriptionSelfResetRolloutAdmin && role == RoleAdmin)
}

type SubscriptionSelfResetResult struct {
	SubscriptionID int64     `json:"subscription_id"`
	QuotaDate      string    `json:"quota_date"`
	DailyLimit     int       `json:"daily_limit"`
	UsedCount      int       `json:"used_count"`
	RemainingCount int       `json:"remaining_count"`
	NextResetAt    time.Time `json:"next_reset_at"`
}

type SubscriptionSelfResetService struct {
	repo          SubscriptionSelfResetRepository
	subscriptions *SubscriptionService
	now           func() time.Time
}

func NewSubscriptionSelfResetService(repo SubscriptionSelfResetRepository, subscriptions *SubscriptionService) *SubscriptionSelfResetService {
	return &SubscriptionSelfResetService{repo: repo, subscriptions: subscriptions, now: time.Now}
}

func (s *SubscriptionSelfResetService) Policy(ctx context.Context) (*SubscriptionSelfResetPolicy, error) {
	raw, err := s.repo.ReadPolicy(ctx)
	if err != nil {
		return nil, errors.ServiceUnavailable("SELF_RESET_UNAVAILABLE", "Self reset policy unavailable").WithCause(err)
	}
	policy, err := ParseSubscriptionSelfResetPolicy([]byte(raw))
	if err != nil {
		return nil, errors.ServiceUnavailable("SELF_RESET_UNAVAILABLE", "Self reset policy invalid").WithCause(err)
	}
	return policy, nil
}

func (s *SubscriptionSelfResetService) SetPolicy(ctx context.Context, raw []byte) (*SubscriptionSelfResetPolicy, error) {
	policy, err := ParseSubscriptionSelfResetPolicy(raw)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	if err := s.repo.WritePolicy(ctx, string(encoded)); err != nil {
		return nil, selfResetStorageError(err)
	}
	return policy, nil
}

func selfResetItem(sub *UserSubscription, usage SubscriptionSelfResetUsage, limit int, now time.Time) SubscriptionSelfResetItem {
	used := 0
	if usage.QuotaDate == timezone.StartOfDay(now).Format(time.DateOnly) {
		used = usage.UsedCount
	}
	item := SubscriptionSelfResetItem{SubscriptionID: sub.ID, UsedCount: used, RemainingCount: max(0, limit-used)}
	reason := ""
	switch {
	case sub.DeletedAt != nil || sub.Status != SubscriptionStatusActive || now.Before(sub.StartsAt) || !now.Before(sub.ExpiresAt):
		reason = "SUBSCRIPTION_INACTIVE"
	case sub.Group == nil || !sub.Group.IsActive():
		reason = "GROUP_DISABLED"
	case sub.HasOneTimeDailyQuota():
		reason = "ONE_TIME_QUOTA"
	case !sub.Group.HasDailyLimit():
		reason = "NO_DAILY_LIMIT"
	case limit == 0:
		reason = "POLICY_DISABLED"
	case used >= limit:
		reason = "DAILY_LIMIT_REACHED"
	case sub.DailyUsageUSD <= 0 || sub.canAutomaticallyResetDailyAt(now):
		reason = "NO_USAGE"
	}
	item.CanReset = reason == ""
	if reason != "" {
		item.DisabledReason = &reason
	}
	return item
}

func (s *SubscriptionSelfResetService) Status(ctx context.Context, userID int64, role string) (*SubscriptionSelfResetStatus, error) {
	policy, err := s.Policy(ctx)
	if err != nil {
		return nil, err
	}
	subs, counts, email, err := s.repo.ListOwned(ctx, userID)
	if err != nil {
		return nil, selfResetStorageError(err)
	}
	now := s.now()
	org := ResolveSubscriptionOrganization(email)
	limit := policy.DailyLimitByOrganization[org]
	result := &SubscriptionSelfResetStatus{Organization: org, DailyLimit: limit, QuotaDate: timezone.StartOfDay(now).Format(time.DateOnly), ServerNow: now, NextResetAt: timezone.StartOfDay(now).AddDate(0, 0, 1), Subscriptions: make([]SubscriptionSelfResetItem, 0, len(subs))}
	for i := range subs {
		item := selfResetItem(&subs[i], counts[subs[i].ID], limit, now)
		if !selfResetRolloutAllowed(policy, role) {
			reason := "ROLLOUT_DISABLED"
			item.CanReset = false
			item.DisabledReason = &reason
		}
		result.Subscriptions = append(result.Subscriptions, item)
	}
	return result, nil
}

func (s *SubscriptionSelfResetService) Reset(ctx context.Context, userID, subscriptionID int64, role, date string) (*SubscriptionSelfResetResult, error) {
	if dbent.TxFromContext(ctx) == nil {
		return nil, ErrIdempotencyStoreUnavail
	}
	policy, err := s.Policy(ctx) // Same transaction connection, before locking the subscription.
	if err != nil {
		return nil, err
	}
	if !selfResetRolloutAllowed(policy, role) {
		return nil, errors.Forbidden("SELF_RESET_ROLLOUT_DISABLED", "Self reset is not enabled for this account")
	}
	sub, err := s.repo.GetOwnedByIDForUpdate(ctx, userID, subscriptionID)
	if err != nil {
		return nil, selfResetStorageError(err)
	}
	if sub.User == nil || !sub.User.IsActive() || sub.User.DeletedAt != nil {
		return nil, ErrSubscriptionNotFound
	}
	usage, err := s.repo.GetUsage(ctx, subscriptionID)
	if err != nil {
		return nil, selfResetStorageError(err)
	}
	now := s.now() // Evaluate the day after acquiring the row lock.
	today := timezone.StartOfDay(now).Format(time.DateOnly)
	if date != today {
		return nil, errors.Conflict("SELF_RESET_DAY_CHANGED", "Refresh the reset status before confirming")
	}
	limit := policy.DailyLimitByOrganization[ResolveSubscriptionOrganization(sub.User.Email)]
	item := selfResetItem(sub, usage, limit, now)
	if item.DisabledReason != nil {
		reason := *item.DisabledReason
		if reason == "POLICY_DISABLED" {
			return nil, errors.Forbidden("SELF_RESET_DISABLED", "Self reset is disabled for this organization")
		}
		if reason == "DAILY_LIMIT_REACHED" {
			return nil, errors.Conflict("SELF_RESET_LIMIT_REACHED", "Daily self reset limit reached")
		}
		return nil, errors.Conflict("SELF_RESET_"+reason, "Subscription cannot be reset: "+reason)
	}
	if err := s.repo.Consume(ctx, subscriptionID, today); err != nil {
		return nil, selfResetStorageError(err)
	}
	if err := s.subscriptions.userSubRepo.ResetUsageWindows(ctx, subscriptionID, true, false, false, timezone.StartOfDay(now), now); err != nil {
		return nil, selfResetStorageError(err)
	}
	if !DeferIdempotencyPostCommit(ctx, func() {
		s.subscriptions.invalidateFilteredDailyResetCaches([]SubscriptionCacheKey{{UserID: userID, GroupID: sub.GroupID}})
	}) {
		return nil, ErrIdempotencyStoreUnavail
	}
	return &SubscriptionSelfResetResult{SubscriptionID: subscriptionID, QuotaDate: today, DailyLimit: limit, UsedCount: item.UsedCount + 1, RemainingCount: item.RemainingCount - 1, NextResetAt: timezone.StartOfDay(now).AddDate(0, 0, 1)}, nil
}
