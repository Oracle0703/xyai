package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrAdminAccessVersionRequired   = infraerrors.BadRequest("ADMIN_ACCESS_VERSION_REQUIRED", "refresh user details before changing role or permissions")
	ErrAdminAccessChanged           = infraerrors.Conflict("ADMIN_ACCESS_CHANGED", "role, permissions or department access changed; refresh and confirm again")
	ErrDepartmentInactive           = infraerrors.BadRequest("DEPARTMENT_INACTIVE", "target department is inactive")
	ErrDepartmentMemberOrganization = infraerrors.BadRequest("DEPARTMENT_MEMBER_ORGANIZATION_MISMATCH", "member does not belong to the target organization")
	ErrDepartmentGlobalConfirmation = infraerrors.BadRequest("DEPARTMENT_GLOBAL_SUBSCRIPTION_CONFIRMATION_REQUIRED", "confirm replacement of global subscription access")
	ErrDepartmentInvalid            = infraerrors.BadRequest("DEPARTMENT_INVALID", "invalid department request")
	ErrDepartmentNotFound           = infraerrors.NotFound("DEPARTMENT_NOT_FOUND", "department not found")
	ErrDepartmentConflict           = infraerrors.Conflict("DEPARTMENT_CONFLICT", "department or membership changed; refresh and retry")
	ErrDepartmentDuplicate          = infraerrors.Conflict("DEPARTMENT_DUPLICATE", "department name already exists in this organization")
	ErrDepartmentScopeDenied        = infraerrors.Forbidden("DEPARTMENT_SCOPE_DENIED", "department access denied")
	ErrDepartmentScopeChanged       = infraerrors.Conflict("REPORT_SCOPE_CHANGED", "department scope changed; refresh and retry")
)

type departmentActorKey struct{}
type departmentAdminAPIKeyKey struct{}

// WithDepartmentActor is only populated by authenticated server middleware.
// Repositories reload the actor before privileged operations; this is identity,
// never a client-supplied authorization grant.
func WithDepartmentActor(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, departmentActorKey{}, userID)
}

func DepartmentActorID(ctx context.Context) int64 {
	id, _ := ctx.Value(departmentActorKey{}).(int64)
	return id
}

func WithDepartmentAdminAPIKey(ctx context.Context, userID int64) context.Context {
	return context.WithValue(WithDepartmentActor(ctx, userID), departmentAdminAPIKeyKey{}, true)
}

func DepartmentActorIsAdminAPIKey(ctx context.Context) bool {
	value, _ := ctx.Value(departmentAdminAPIKeyKey{}).(bool)
	return value
}

type Department struct {
	ID                int64               `json:"id"`
	Organization      string              `json:"organization_key"`
	Name              string              `json:"name"`
	Status            string              `json:"status"`
	SortOrder         int                 `json:"sort_order"`
	Version           int64               `json:"version"`
	MemberCount       int64               `json:"member_count"`
	ActiveMemberCount int64               `json:"active_member_count"`
	Managers          []DepartmentManager `json:"managers"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

type DepartmentManager struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

type DepartmentGroupOption struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type DepartmentMember struct {
	ID                int64  `json:"id"`
	Email             string `json:"email"`
	Username          string `json:"username"`
	Status            string `json:"status"`
	Organization      string `json:"organization"`
	DepartmentID      *int64 `json:"department_id"`
	DepartmentName    string `json:"department_name"`
	DepartmentVersion int64  `json:"department_version"`
}

type DepartmentListFilter struct {
	UserIDs      []int64
	Organization string
	Status       string
	Q            string
	DepartmentID string
	Page         int
	PageSize     int
}

type DepartmentList struct {
	Items    []Department `json:"items"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
}

type DepartmentMemberList struct {
	Items    []DepartmentMember `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}

type DepartmentSaveInput struct {
	Organization    string `json:"organization_key"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	SortOrder       int    `json:"sort_order"`
	ExpectedVersion *int64 `json:"expected_version"`
}

type DepartmentMemberChange struct {
	UserID               int64  `json:"user_id"`
	ExpectedDepartmentID *int64 `json:"expected_department_id"`
	ExpectedVersion      int64  `json:"expected_department_version"`
}

type DepartmentAssignInput struct {
	DepartmentID *int64                   `json:"department_id"`
	Members      []DepartmentMemberChange `json:"members"`
}

type DepartmentAccess struct {
	Role          string   `json:"-"`
	UserID        int64    `json:"user_id"`
	DepartmentIDs []int64  `json:"department_ids"`
	Permissions   []string `json:"permissions"`
	Version       string   `json:"version"`
}

type DepartmentAccessInput struct {
	DepartmentIDs              []int64 `json:"department_ids"`
	Report                     bool    `json:"report"`
	ResetQuota                 bool    `json:"reset_quota"`
	ReplaceGlobalSubscriptions bool    `json:"replace_global_subscriptions"`
	ExpectedVersion            string  `json:"expected_version"`
}

type DepartmentScope struct {
	Unrestricted        bool               `json:"unrestricted"`
	Organizations       []string           `json:"organizations"`
	Departments         []Department       `json:"departments"`
	CatalogVersion      string             `json:"catalog_version"`
	Version             string             `json:"-"`
	Actor               *User              `json:"-"`
	DefaultOrganization string             `json:"default_organization"`
	DefaultDepartment   string             `json:"default_department_id"`
	Members             []DepartmentMember `json:"-"`
}

type DepartmentScopeQuery struct {
	Organization string
	DepartmentID string
	Q            string
	Platform     string
	UserID       *int64
	GroupID      *int64
	Status       string
	Versioned    bool // Explicitly requested snapshot for an otherwise global subscription query.
	Limit        int  // Compact search only; never used to construct a query snapshot.
}

// HashDepartmentScope detects membership changes, not usage-log snapshots.
func HashDepartmentScope(value any) string {
	encoded, _ := json.Marshal(value)
	h := sha256.Sum256(encoded)
	return hex.EncodeToString(h[:])
}

func OrganizationForEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) < 2 {
		return OrganizationOther
	}
	switch strings.ToLower(parts[1]) {
	case "xunyou.com":
		return OrganizationXunyou
	case "wsdashi.com":
		return OrganizationWsdashi
	default:
		return OrganizationOther
	}
}

func ValidDepartmentOrganization(org string) bool {
	return org == OrganizationXunyou || org == OrganizationWsdashi || org == OrganizationOther
}

// NormalizeDepartmentFilter preserves all vs unassigned vs a specific department.
func NormalizeDepartmentFilter(org, department string) (string, string, error) {
	org, department = strings.TrimSpace(org), strings.TrimSpace(department)
	if org == "" {
		org = OrganizationAll
	}
	if department == "" {
		department = "all"
	}
	if org != OrganizationAll && !ValidDepartmentOrganization(org) {
		return "", "", ErrDepartmentInvalid
	}
	if department == "all" {
		return org, department, nil
	}
	if org == OrganizationAll {
		return "", "", ErrDepartmentInvalid
	}
	if department == "unassigned" {
		return org, department, nil
	}
	id, err := strconv.ParseInt(department, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != department {
		return "", "", ErrDepartmentInvalid
	}
	return org, department, nil
}

func (s *DepartmentScope) ValidateSelection(org, department, version string) error {
	if s == nil {
		return ErrDepartmentScopeDenied
	}
	if !s.Unrestricted && len(s.Departments) == 0 {
		return ErrDepartmentScopeDenied
	}
	if version != "" && version != s.Version {
		return ErrDepartmentScopeChanged
	}
	if department == "unassigned" && !s.Unrestricted {
		return ErrDepartmentScopeDenied
	}
	if org != OrganizationAll && !s.Unrestricted {
		found := false
		for _, allowed := range s.Organizations {
			if allowed == org {
				found = true
			}
		}
		if !found {
			return ErrDepartmentScopeDenied
		}
	}
	if department == "all" || department == "unassigned" {
		return nil
	}
	for _, d := range s.Departments {
		if strconv.FormatInt(d.ID, 10) == department {
			if d.Organization != org {
				return ErrDepartmentInvalid
			}
			return nil
		}
	}
	if !s.Unrestricted {
		return ErrDepartmentScopeDenied
	}
	return ErrDepartmentNotFound
}

func (s *DepartmentScope) SelectedMembers(org, department, q string) []DepartmentMember {
	result := make([]DepartmentMember, 0)
	for _, m := range s.Members {
		if org != OrganizationAll && m.Organization != org {
			continue
		}
		if department == "unassigned" && m.DepartmentID != nil {
			continue
		}
		if department != "all" && department != "unassigned" && (m.DepartmentID == nil || strconv.FormatInt(*m.DepartmentID, 10) != department) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(m.Email), strings.ToLower(q)) {
			continue
		}
		result = append(result, m)
	}
	return result
}

type DepartmentRepository interface {
	List(context.Context, DepartmentListFilter) (*DepartmentList, error)
	Save(context.Context, int64, DepartmentSaveInput) (*Department, error)
	Members(context.Context, DepartmentListFilter) (*DepartmentMemberList, error)
	Assign(context.Context, DepartmentAssignInput) (int, error)
	GetAccess(context.Context, int64) (*DepartmentAccess, error)
	SetAccess(context.Context, int64, DepartmentAccessInput) (*DepartmentAccess, error)
	Scope(context.Context, string) (*DepartmentScope, error)
	QueryScope(context.Context, string, DepartmentScopeQuery) (*DepartmentScope, error)
	SubscriptionGroups(context.Context, string) ([]DepartmentGroupOption, error)
}

type DepartmentService struct{ repo DepartmentRepository }

func NewDepartmentService(repo DepartmentRepository) *DepartmentService {
	return &DepartmentService{repo: repo}
}

func normalizeDepartmentList(f DepartmentListFilter, resourceID ...bool) (DepartmentListFilter, error) {
	if len(f.UserIDs) > 200 {
		return f, ErrDepartmentInvalid
	}
	seen := map[int64]bool{}
	for _, id := range f.UserIDs {
		if id <= 0 || seen[id] {
			return f, ErrDepartmentInvalid
		}
		seen[id] = true
	}
	var err error
	if len(resourceID) > 0 && resourceID[0] && f.Organization == "" && f.DepartmentID != "" && f.DepartmentID != "all" && f.DepartmentID != "unassigned" {
		id, parseErr := strconv.ParseInt(f.DepartmentID, 10, 64)
		if parseErr != nil || id <= 0 || strconv.FormatInt(id, 10) != f.DepartmentID {
			return f, ErrDepartmentInvalid
		}
		f.Organization = OrganizationAll
	} else {
		f.Organization, f.DepartmentID, err = NormalizeDepartmentFilter(f.Organization, f.DepartmentID)
	}
	if err != nil {
		return f, err
	}
	if f.Status != "" && f.Status != "active" && f.Status != "inactive" && f.Status != "disabled" {
		return f, ErrDepartmentInvalid
	}
	if f.Page == 0 {
		f.Page = 1
	}
	if f.PageSize == 0 {
		f.PageSize = 20
	}
	if f.Page < 1 || f.PageSize < 1 || f.PageSize > 200 {
		return f, ErrDepartmentInvalid
	}
	f.Q = strings.TrimSpace(f.Q)
	return f, nil
}

func (s *DepartmentService) List(ctx context.Context, f DepartmentListFilter) (*DepartmentList, error) {
	f, err := normalizeDepartmentList(f)
	if err != nil {
		return nil, err
	}
	return s.repo.List(ctx, f)
}

func (s *DepartmentService) Members(ctx context.Context, f DepartmentListFilter) (*DepartmentMemberList, error) {
	f, err := normalizeDepartmentList(f, true)
	if err != nil {
		return nil, err
	}
	return s.repo.Members(ctx, f)
}

func (s *DepartmentService) Save(ctx context.Context, id int64, in DepartmentSaveInput) (*Department, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Status == "" {
		in.Status = "active"
	}
	if id < 0 || !ValidDepartmentOrganization(in.Organization) || in.Name == "" || utf8.RuneCountInString(in.Name) > 100 ||
		(in.Status != "active" && in.Status != "inactive") || (id > 0 && (in.ExpectedVersion == nil || *in.ExpectedVersion < 0)) {
		return nil, ErrDepartmentInvalid
	}
	return s.repo.Save(ctx, id, in)
}

func (s *DepartmentService) Assign(ctx context.Context, in DepartmentAssignInput) (int, error) {
	if len(in.Members) == 0 || len(in.Members) > 200 || (in.DepartmentID != nil && *in.DepartmentID <= 0) {
		return 0, ErrDepartmentInvalid
	}
	seen := make(map[int64]bool)
	for _, m := range in.Members {
		if m.UserID <= 0 || m.ExpectedVersion < 0 || seen[m.UserID] || (m.ExpectedDepartmentID != nil && *m.ExpectedDepartmentID <= 0) {
			return 0, ErrDepartmentInvalid
		}
		seen[m.UserID] = true
	}
	sort.Slice(in.Members, func(i, j int) bool { return in.Members[i].UserID < in.Members[j].UserID })
	return s.repo.Assign(ctx, in)
}

func (s *DepartmentService) GetAccess(ctx context.Context, userID int64) (*DepartmentAccess, error) {
	if userID <= 0 {
		return nil, ErrDepartmentInvalid
	}
	return s.repo.GetAccess(ctx, userID)
}

func (s *DepartmentService) SetAccess(ctx context.Context, userID int64, in DepartmentAccessInput) (*DepartmentAccess, error) {
	if in.ExpectedVersion == "" {
		return nil, ErrAdminAccessVersionRequired
	}
	if userID <= 0 || len(in.DepartmentIDs) > 200 {
		return nil, ErrDepartmentInvalid
	}
	seen := make(map[int64]bool)
	for _, id := range in.DepartmentIDs {
		if id <= 0 || seen[id] {
			return nil, ErrDepartmentInvalid
		}
		seen[id] = true
	}
	sort.Slice(in.DepartmentIDs, func(i, j int) bool { return in.DepartmentIDs[i] < in.DepartmentIDs[j] })
	return s.repo.SetAccess(ctx, userID, in)
}

func (s *DepartmentService) Scope(ctx context.Context, permission string) (*DepartmentScope, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDepartmentScopeDenied
	}
	return s.repo.Scope(ctx, permission)
}

func (s *DepartmentService) SubscriptionGroups(ctx context.Context, q string) ([]DepartmentGroupOption, error) {
	return s.repo.SubscriptionGroups(ctx, strings.TrimSpace(q))
}

func (s *DepartmentService) QueryScope(ctx context.Context, permission string, query DepartmentScopeQuery) (*DepartmentScope, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDepartmentScopeDenied
	}
	var err error
	query.Organization, query.DepartmentID, err = NormalizeDepartmentFilter(query.Organization, query.DepartmentID)
	if err != nil {
		return nil, err
	}
	query.Q = strings.TrimSpace(query.Q)
	return s.repo.QueryScope(ctx, permission, query)
}
