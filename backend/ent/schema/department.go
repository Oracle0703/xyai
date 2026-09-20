package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
)

// Department is a personnel boundary, independent of model routing groups.
type Department struct{ ent.Schema }

func (Department) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "departments"}}
}

func (Department) Mixin() []ent.Mixin { return []ent.Mixin{mixins.TimeMixin{}} }

func (Department) Fields() []ent.Field {
	return []ent.Field{
		field.String("organization_key").MaxLen(20).Immutable(),
		field.String("name").MaxLen(100).NotEmpty(),
		field.String("status").MaxLen(20).Default("active"),
		field.Int("sort_order").Default(0),
		field.Int64("version").Default(0),
	}
}

func (Department) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("members", User.Type),
		edge.From("authorized_users", User.Type).Ref("authorized_departments").
			Through("access_grants", DepartmentAccessGrant.Type),
	}
}

func (Department) Indexes() []ent.Index {
	return []ent.Index{index.Fields("organization_key", "status", "sort_order", "id")}
}
