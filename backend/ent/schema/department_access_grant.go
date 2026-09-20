package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type DepartmentAccessGrant struct{ ent.Schema }

func (DepartmentAccessGrant) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "department_access_grants"}, field.ID("user_id", "department_id")}
}

func (DepartmentAccessGrant) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"), field.Int64("department_id"), field.Int64("created_by"),
		field.Time("created_at").Immutable().Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (DepartmentAccessGrant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).Unique().Required().Field("user_id").Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.To("department", Department.Type).Unique().Required().Field("department_id").Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.To("creator", User.Type).Unique().Required().Field("created_by").Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (DepartmentAccessGrant) Indexes() []ent.Index {
	return []ent.Index{index.Fields("department_id", "user_id")}
}
