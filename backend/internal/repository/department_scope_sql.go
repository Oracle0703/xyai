package repository

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// Every department-scoped query builds its scope here. An actor is either an
// organization leader (organization grants) or a department manager
// (department grants), never both, so each predicate takes exactly one branch
// and never a union of the two grant kinds.

func departmentActorIsOrganizationLeader(actor *service.User) bool {
	return actor.Role != service.RoleAdmin && len(actor.OrganizationGrants) > 0
}

// departmentRowScope limits the departments alias to the actor's departments.
func departmentRowScope(actor *service.User, department string, args *[]any) string {
	if actor.Role == service.RoleAdmin {
		return "TRUE"
	}
	if departmentActorIsOrganizationLeader(actor) {
		*args = append(*args, pq.Array(actor.OrganizationGrants))
		return fmt.Sprintf("%s.organization_key=ANY($%d::text[])", department, len(*args))
	}
	*args = append(*args, actor.ID)
	return fmt.Sprintf("EXISTS(SELECT 1 FROM department_access_grants dg WHERE dg.user_id=$%d AND dg.department_id=%s.id)", len(*args), department)
}

// departmentMemberScope limits users joined by departmentMemberFrom to the
// actor's members. Organization leaders reach unassigned members through the
// email-derived organization, so it must not depend on the department join.
func departmentMemberScope(actor *service.User, args *[]any) string {
	if departmentActorIsOrganizationLeader(actor) {
		*args = append(*args, pq.Array(actor.OrganizationGrants))
		return fmt.Sprintf("%s=ANY($%d::text[])", organizationUsageOrganizationExpression("u"), len(*args))
	}
	return departmentRowScope(actor, "d", args)
}

// subscriptionResetTargets limits a scoped actor's subscription reads and
// resets (design §4.5, D9/D10). It replaces departmentMemberScope in those
// queries because the actor's own subscriptions are included even when the
// actor is not a member of the scope. Department leaders never reach admins;
// organization leaders reach every account of their organizations.
func subscriptionResetTargets(actor *service.User, args *[]any) string {
	*args = append(*args, actor.ID)
	self := len(*args)
	members := departmentMemberScope(actor, args)
	if departmentActorIsOrganizationLeader(actor) {
		return fmt.Sprintf("(u.id=$%d OR %s)", self, members)
	}
	return fmt.Sprintf("(u.id=$%d OR (%s AND u.role IN ('user','sub_admin')))", self, members)
}

// subscriptionManageTargets limits an organization leader's subscription
// writes (design §4.8, D11): the leader, active normal users of the leader's
// organizations and active managed leaders. It is a subset of
// subscriptionResetTargets; can_manage and assignment candidates use it too.
func subscriptionManageTargets(actor *service.User, args *[]any) string {
	*args = append(*args, actor.ID)
	self := len(*args)
	*args = append(*args, pq.Array(actor.OrganizationGrants))
	organizations := len(*args)
	return fmt.Sprintf("u.deleted_at IS NULL AND u.status='active' AND (u.id=$%d OR (%s=ANY($%d::text[]) AND (u.role='user' OR (u.role='sub_admin' AND %s))))",
		self, organizationUsageOrganizationExpression("u"), organizations, managedLeaderPredicate("u", organizations))
}

// subscriptionWriteGroupScope limits an organization leader's subscription
// writes to groups scoped to one of the leader's organizations (design D13).
// The picker, can_manage and every write check use it.
func subscriptionWriteGroupScope(actor *service.User, group string, args *[]any) string {
	*args = append(*args, pq.Array(actor.OrganizationGrants))
	return fmt.Sprintf("EXISTS(SELECT 1 FROM group_organization_scopes gos WHERE gos.group_id=%s.id AND gos.organization_key=ANY($%d::text[]))", group, len(*args))
}

// managedLeaderPredicate matches sub_admins an organization leader may manage:
// own organization, department capabilities only, no organization grant and no
// department grant outside the leader's organizations (design §3.3).
func managedLeaderPredicate(user string, organizationsArg int) string {
	return fmt.Sprintf(`%[2]s=ANY($%[3]d::text[])
 AND NOT EXISTS(SELECT 1 FROM organization_access_grants mog WHERE mog.user_id=%[1]s.id)
 AND NOT EXISTS(SELECT 1 FROM department_access_grants mdg JOIN departments md ON md.id=mdg.department_id WHERE mdg.user_id=%[1]s.id AND md.organization_key<>ALL($%[3]d::text[]))
 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements_text(%[1]s.admin_permissions) mp(code) WHERE mp.code NOT IN ('admin.organization_usage','admin.department_subscriptions','admin.department_members'))`,
		user, organizationUsageOrganizationExpression(user), organizationsArg)
}
