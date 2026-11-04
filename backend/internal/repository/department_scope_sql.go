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

// departmentSubscriptionTargets keeps scoped subscription access away from the
// actor, administrators and leaders the actor may not manage (design §4.5).
func departmentSubscriptionTargets(actor *service.User, args *[]any) string {
	*args = append(*args, actor.ID)
	self := len(*args)
	if !departmentActorIsOrganizationLeader(actor) {
		return fmt.Sprintf("u.id<>$%d AND u.role='user'", self)
	}
	*args = append(*args, pq.Array(actor.OrganizationGrants))
	return fmt.Sprintf("u.id<>$%d AND (u.role='user' OR (u.role='sub_admin' AND %s))", self, managedLeaderPredicate("u", len(*args)))
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
