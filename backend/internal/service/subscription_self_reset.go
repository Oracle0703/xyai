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
const MaxSelfResetGroupChanges = 1000

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

var ErrInvalidSelfResetGroups = errors.BadRequest("INVALID_SELF_RESET_GROUPS", "Invalid self reset group changes")

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

// SubscriptionSelfResetEvent is one successful self reset. UserEmail and GroupName are read-time joins.
type SubscriptionSelfResetEvent struct {
	ID                  int64     `json:"id"`
	SubscriptionID      int64     `json:"subscription_id"`
	UserID              int64     `json:"user_id"`
	UserEmail           string    `json:"user_email"`
	GroupID             int64     `json:"group_id"`
	GroupName           string    `json:"group_name"`
	Organization        string    `json:"organization"`
	QuotaDate           string    `json:"quota_date"`
	UsedCount           int       `json:"used_count"`
	DailyLimit          int       `json:"daily_limit"`
	DailyUsageUSDBefore float64   `json:"daily_usage_usd_before"`
	CreatedAt           time.Time `json:"created_at"`
}

type SubscriptionSelfResetEventFilter struct {
	Email        string
	Organization string
	GroupID      int64
	StartDate    string
	EndDate      string
	Page         int
	PageSize     int
}

type SubscriptionSelfResetGroup struct {
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	Platform         string   `json:"platform"`
	Status           string   `json:"status"`
	SubscriptionType string   `json:"subscription_type"`
	DailyLimitUSD    *float64 `json:"daily_limit_usd"`
	Enabled          bool     `json:"enabled"`
}

type SubscriptionSelfResetEventGroup struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type SubscriptionSelfResetGroupChanges struct {
	Enable  []int64 `json:"enable"`
	Disable []int64 `json:"disable"`
}

type SubscriptionSelfResetRepository interface {
	ConfigRevision(context.Context, bool) (int64, error)
	AdvanceConfigRevision(context.Context, int64) error
	ReadPolicy(context.Context) (string, error)
	WritePolicy(context.Context, string) error
	ListOwned(context.Context, int64) ([]UserSubscription, map[int64]SubscriptionSelfResetUsage, string, error)
	GetOwnedByIDForUpdate(context.Context, int64, int64) (*UserSubscription, error)
	GetUsage(context.Context, int64) (SubscriptionSelfResetUsage, error)
	Consume(context.Context, int64, string) error
	RecordEvent(context.Context, *SubscriptionSelfResetEvent) error
	ListEvents(context.Context, SubscriptionSelfResetEventFilter) ([]SubscriptionSelfResetEvent, int64, error)
	ListGroups(context.Context) (map[string][]SubscriptionSelfResetGroup, error)
	ListEventGroups(context.Context) ([]SubscriptionSelfResetEventGroup, error)
	ApplyGroupChanges(context.Context, map[string]SubscriptionSelfResetGroupChanges) (map[string][]int64, error)
	ReadAdmission(context.Context, []int64, string) (string, map[int64]bool, error)
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
	return parseStoredSelfResetPolicy(raw)
}

func parseStoredSelfResetPolicy(raw string) (*SubscriptionSelfResetPolicy, error) {
	policy, err := ParseSubscriptionSelfResetPolicy([]byte(raw))
	if err != nil {
		return nil, errors.ServiceUnavailable("SELF_RESET_UNAVAILABLE", "Self reset policy invalid").WithCause(err)
	}
	return policy, nil
}

func (s *SubscriptionSelfResetService) ListEvents(ctx context.Context, filter SubscriptionSelfResetEventFilter) ([]SubscriptionSelfResetEvent, int64, error) {
	events, total, err := s.repo.ListEvents(ctx, filter)
	if err != nil {
		return nil, 0, selfResetStorageError(err)
	}
	return events, total, nil
}

func (s *SubscriptionSelfResetService) Groups(ctx context.Context) (map[string][]SubscriptionSelfResetGroup, error) {
	groups, err := s.repo.ListGroups(ctx)
	if err != nil {
		return nil, selfResetStorageError(err)
	}
	return groups, nil
}

func (s *SubscriptionSelfResetService) EventGroups(ctx context.Context) ([]SubscriptionSelfResetEventGroup, error) {
	groups, err := s.repo.ListEventGroups(ctx)
	if err != nil {
		return nil, selfResetStorageError(err)
	}
	return groups, nil
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

func selfResetItemWithGroupAccess(sub *UserSubscription, usage SubscriptionSelfResetUsage, limit int, now time.Time, groupAllowed bool) SubscriptionSelfResetItem {
	if !groupAllowed {
		used := 0
		if usage.QuotaDate == timezone.StartOfDay(now).Format(time.DateOnly) {
			used = usage.UsedCount
		}
		return SubscriptionSelfResetItem{
			SubscriptionID: sub.ID,
			UsedCount:      used,
			RemainingCount: max(0, limit-used),
			CanReset:       false,
			DisabledReason: subscriptionSelfResetStringPtr("GROUP_NOT_ALLOWED"),
		}
	}
	return selfResetItem(sub, usage, limit, now)
}

func subscriptionSelfResetStringPtr(value string) *string { return &value }

func (s *SubscriptionSelfResetService) readAdmission(ctx context.Context, groupIDs []int64, org string) (*SubscriptionSelfResetPolicy, map[int64]bool, error) {
	raw, allowed, err := s.repo.ReadAdmission(ctx, groupIDs, org)
	if err != nil {
		return nil, nil, selfResetStorageError(err)
	}
	policy, err := parseStoredSelfResetPolicy(raw)
	return policy, allowed, err
}

func (s *SubscriptionSelfResetService) Status(ctx context.Context, userID int64, role string) (*SubscriptionSelfResetStatus, error) {
	subs, counts, email, err := s.repo.ListOwned(ctx, userID)
	if err != nil {
		return nil, selfResetStorageError(err)
	}
	org := ResolveSubscriptionOrganization(email)
	groupIDs := make([]int64, len(subs))
	for i := range subs {
		groupIDs[i] = subs[i].GroupID
	}
	policy, allowed, err := s.readAdmission(ctx, groupIDs, org)
	if err != nil {
		return nil, err
	}
	now := s.now()
	limit := policy.DailyLimitByOrganization[org]
	rolloutAllowed := selfResetRolloutAllowed(policy, role)
	result := &SubscriptionSelfResetStatus{Organization: org, DailyLimit: limit, QuotaDate: timezone.StartOfDay(now).Format(time.DateOnly), ServerNow: now, NextResetAt: timezone.StartOfDay(now).AddDate(0, 0, 1), Subscriptions: make([]SubscriptionSelfResetItem, 0, len(subs))}
	for i := range subs {
		item := selfResetItemWithGroupAccess(&subs[i], counts[subs[i].ID], limit, now, allowed[subs[i].GroupID])
		if !rolloutAllowed {
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
	// Reject a closed rollout without taking the subscription lock. Only the
	// admission snapshot read after the lock can authorize the reset.
	preflight, err := s.Policy(ctx)
	if err != nil {
		return nil, err
	}
	if !selfResetRolloutAllowed(preflight, role) {
		return nil, errors.Forbidden("SELF_RESET_ROLLOUT_DISABLED", "Self reset is not enabled for this account")
	}
	sub, err := s.repo.GetOwnedByIDForUpdate(ctx, userID, subscriptionID)
	if err != nil {
		return nil, selfResetStorageError(err)
	}
	if sub.User == nil || !sub.User.IsActive() || sub.User.DeletedAt != nil {
		return nil, ErrSubscriptionNotFound
	}
	org := ResolveSubscriptionOrganization(sub.User.Email)
	policy, allowed, err := s.readAdmission(ctx, []int64{sub.GroupID}, org)
	if err != nil {
		return nil, err
	}
	if !selfResetRolloutAllowed(policy, role) {
		return nil, errors.Forbidden("SELF_RESET_ROLLOUT_DISABLED", "Self reset is not enabled for this account")
	}
	if !allowed[sub.GroupID] {
		return nil, errors.Forbidden("SELF_RESET_GROUP_NOT_ALLOWED", "Self reset is not enabled for this subscription group")
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
	limit := policy.DailyLimitByOrganization[org]
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
	// Same transaction as Consume: a success response always has its history row; replays never re-run this.
	if err := s.repo.RecordEvent(ctx, &SubscriptionSelfResetEvent{SubscriptionID: subscriptionID, UserID: userID, GroupID: sub.GroupID, Organization: org, QuotaDate: today, UsedCount: item.UsedCount + 1, DailyLimit: limit, DailyUsageUSDBefore: sub.DailyUsageUSD}); err != nil {
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
