package service

import (
	"context"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// OrganizationLeaderOrganizations are the organizations that may have an
// organization leader (D1: never "other", which holds every external user).
var OrganizationLeaderOrganizations = []string{OrganizationXunyou, OrganizationWsdashi}

// OrganizationAccessInput replaces a user's organization grants. Organizations
// must be an explicit array: nil (omitted or null) is rejected, [] revokes.
type OrganizationAccessInput struct {
	Organizations              *[]string `json:"organizations"`
	ExpectedVersion            string    `json:"expected_version"`
	ReplaceGlobalSubscriptions bool      `json:"replace_global_subscriptions"`
	ReplaceDepartmentGrants    bool      `json:"replace_department_grants"`
}

// OrganizationLeaderInput is the final grant set an organization leader assigns
// to a department leader. Every field is required (design §4.1).
type OrganizationLeaderInput struct {
	DepartmentIDs   *[]int64 `json:"department_ids"`
	Report          *bool    `json:"report"`
	ResetQuota      *bool    `json:"reset_quota"`
	ManageMembers   *bool    `json:"manage_members"`
	ExpectedVersion *string  `json:"expected_version"`
}

func (in OrganizationLeaderInput) Permissions() []string {
	permissions := []string{}
	if *in.Report {
		permissions = append(permissions, AdminPermissionOrganizationUsage)
	}
	if *in.ResetQuota {
		permissions = append(permissions, AdminPermissionDepartmentSubscriptions)
	}
	if *in.ManageMembers {
		permissions = append(permissions, AdminPermissionDepartmentMembers)
	}
	return permissions
}

type OrganizationLeaderDepartment struct {
	ID           int64  `json:"id"`
	Organization string `json:"organization_key"`
	Name         string `json:"name"`
	Status       string `json:"status"`
}

// OrganizationLeader is a department leader as seen by an organization leader.
// Departments only include the organization leader's organizations; permissions
// and version are only returned for leaders the caller may manage.
type OrganizationLeader struct {
	ID           int64                          `json:"id"`
	Email        string                         `json:"email"`
	Username     string                         `json:"username"`
	Status       string                         `json:"status"`
	Organization string                         `json:"organization"`
	Departments  []OrganizationLeaderDepartment `json:"departments"`
	Permissions  []string                       `json:"permissions"`
	Managed      bool                           `json:"managed"`
	Version      string                         `json:"version,omitempty"`
}

type OrganizationLeaderList struct {
	Items    []OrganizationLeader `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

type OrganizationLeaderCandidate struct {
	ID             int64   `json:"id"`
	Email          string  `json:"email"`
	Username       string  `json:"username"`
	Organization   string  `json:"organization"`
	DepartmentID   *int64  `json:"department_id"`
	DepartmentName *string `json:"department_name"`
	Version        string  `json:"version"`
}

type OrganizationLeaderRepository interface {
	SetOrganizationAccess(context.Context, int64, OrganizationAccessInput) (*DepartmentAccess, error)
	OrganizationLeaders(context.Context, DepartmentListFilter) (*OrganizationLeaderList, error)
	OrganizationLeader(context.Context, int64) (*OrganizationLeader, error)
	OrganizationLeaderCandidates(context.Context, string, string) ([]OrganizationLeaderCandidate, error)
	UpdateOrganizationLeader(context.Context, int64, OrganizationLeaderInput) (*DepartmentAccess, bool, error)
	ResetOrganizationUserPassword(context.Context, int64, string) error
	DeleteOrganizationUser(context.Context, int64, OrganizationUserDeleteInput) ([]string, error)
}

// OrganizationUserDeleteInput carries the member CAS for deleting an
// organization user; ExpectedUnassigned is a JSON null department (design §4.9).
type OrganizationUserDeleteInput struct {
	ExpectedDepartmentID      *int64 `json:"expected_department_id"`
	ExpectedUnassigned        bool   `json:"-"`
	ExpectedDepartmentVersion *int64 `json:"expected_department_version"`
}

// OrganizationUserPasswordMinLength matches the administrator password rule.
const OrganizationUserPasswordMinLength = 6

// ResetOrganizationUserPassword lets an organization leader set the password
// of a normal user of the leader's organizations (design §4.9, D12).
func (s *DepartmentService) ResetOrganizationUserPassword(ctx context.Context, userID int64, password string) error {
	if s == nil || s.repo == nil {
		return ErrDepartmentScopeDenied
	}
	if userID <= 0 || len(password) < OrganizationUserPasswordMinLength {
		return ErrDepartmentInvalid
	}
	user := &User{}
	if err := user.SetPassword(password); err != nil {
		return ErrDepartmentInvalid
	}
	return s.repo.ResetOrganizationUserPassword(ctx, userID, user.PasswordHash)
}

// DeleteOrganizationUser soft-deletes a normal user of the leader's
// organizations together with the user's API keys (design §4.9, D12).
func (s *DepartmentService) DeleteOrganizationUser(ctx context.Context, userID int64, in OrganizationUserDeleteInput) error {
	if s == nil || s.repo == nil {
		return ErrDepartmentScopeDenied
	}
	if userID <= 0 || (in.ExpectedDepartmentID == nil && !in.ExpectedUnassigned) || in.ExpectedDepartmentVersion == nil {
		return ErrDepartmentInvalid
	}
	keys, err := s.repo.DeleteOrganizationUser(ctx, userID, in)
	if err != nil {
		return err
	}
	if s.authCacheInvalidator != nil {
		for _, key := range keys {
			s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, key)
		}
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	return nil
}

func (s *DepartmentService) OrganizationScope(ctx context.Context) (*DepartmentScope, error) {
	scope, err := s.Scope(ctx, AdminPermissionOrganizationManage)
	if err != nil {
		return nil, err
	}
	if len(scope.FullOrganizations) == 0 {
		return nil, ErrDepartmentScopeDenied
	}
	scope.DelegablePermissions = append([]string(nil), OrganizationLeaderDepartmentPermissions...)
	return scope, nil
}

func (s *DepartmentService) SetOrganizationAccess(ctx context.Context, userID int64, in OrganizationAccessInput) (*DepartmentAccess, error) {
	if in.ExpectedVersion == "" {
		return nil, ErrAdminAccessVersionRequired
	}
	if userID <= 0 || in.Organizations == nil || len(*in.Organizations) > len(OrganizationLeaderOrganizations) {
		return nil, ErrDepartmentInvalid
	}
	organizations := append([]string{}, *in.Organizations...)
	sort.Strings(organizations)
	for i, organization := range organizations {
		if !slices.Contains(OrganizationLeaderOrganizations, organization) || (i > 0 && organizations[i-1] == organization) {
			return nil, ErrDepartmentInvalid
		}
	}
	in.Organizations = &organizations
	access, err := s.repo.SetOrganizationAccess(ctx, userID, in)
	if err != nil {
		return nil, err
	}
	s.invalidateAuth(ctx, userID)
	return access, nil
}

func (s *DepartmentService) OrganizationLeaders(ctx context.Context, f DepartmentListFilter) (*OrganizationLeaderList, error) {
	f.DepartmentID, f.Status, f.UserIDs = "", "", nil
	f, err := normalizeDepartmentList(f, false)
	if err != nil {
		return nil, err
	}
	return s.repo.OrganizationLeaders(ctx, f)
}

func (s *DepartmentService) OrganizationLeader(ctx context.Context, userID int64) (*OrganizationLeader, error) {
	if userID <= 0 {
		return nil, ErrDepartmentInvalid
	}
	return s.repo.OrganizationLeader(ctx, userID)
}

func (s *DepartmentService) OrganizationLeaderCandidates(ctx context.Context, organization, q string) ([]OrganizationLeaderCandidate, error) {
	organization, q = strings.TrimSpace(organization), strings.TrimSpace(q)
	if organization == "" {
		organization = OrganizationAll
	}
	if (organization != OrganizationAll && !ValidDepartmentOrganization(organization)) || utf8.RuneCountInString(q) < 2 {
		return nil, ErrDepartmentInvalid
	}
	return s.repo.OrganizationLeaderCandidates(ctx, organization, q)
}

func (s *DepartmentService) UpdateOrganizationLeader(ctx context.Context, userID int64, in OrganizationLeaderInput) (*DepartmentAccess, error) {
	if userID <= 0 || in.DepartmentIDs == nil || in.Report == nil || in.ResetQuota == nil || in.ManageMembers == nil || in.ExpectedVersion == nil {
		return nil, ErrDepartmentInvalid
	}
	if *in.ExpectedVersion == "" {
		return nil, ErrAdminAccessVersionRequired
	}
	ids := append([]int64{}, *in.DepartmentIDs...)
	if len(ids) > 200 {
		return nil, ErrDepartmentInvalid
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i, id := range ids {
		if id <= 0 || (i > 0 && ids[i-1] == id) {
			return nil, ErrDepartmentInvalid
		}
	}
	in.DepartmentIDs = &ids
	// Departments and capabilities are both present (appoint/update) or both
	// absent (revoke); a half-specified leader is rejected (design §4.1).
	if (len(ids) > 0) != (len(in.Permissions()) > 0) {
		return nil, ErrDepartmentInvalid
	}
	access, accessChanged, err := s.repo.UpdateOrganizationLeader(ctx, userID, in)
	if err != nil {
		return nil, err
	}
	if accessChanged {
		s.invalidateAuth(ctx, userID)
	}
	return access, nil
}

func (s *DepartmentService) invalidateAuth(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
}
