package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// BillingGroupComposition links a composite group to one standard member.
type BillingGroupComposition struct {
	ent.Schema
}

func (BillingGroupComposition) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("composite_group_id", uuid.UUID{}),
		field.UUID("member_group_id", uuid.UUID{}),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (BillingGroupComposition) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("composite_group", BillingGroup.Type).Ref("compositions").Unique().Field("composite_group_id").Required(),
		edge.From("member_group", BillingGroup.Type).Ref("member_compositions").Unique().Field("member_group_id").Required(),
	}
}

func (BillingGroupComposition) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("composite_group_id", "member_group_id").Unique(),
		index.Fields("member_group_id"),
	}
}
