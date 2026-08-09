package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type BillingGroup struct {
	ent.Schema
}

func (BillingGroup) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("code").NotEmpty().MaxLen(64).Unique().Immutable(),
		field.String("display_name").NotEmpty().MaxLen(128),
		field.Int64("multiplier_bps").Positive().Default(10_000),
		field.Bool("is_default").Default(false),
		field.Enum("status").Values("active", "disabled").Default("active"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
		field.Time("deleted_at").Optional().Nillable(),
	}
}

func (BillingGroup) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("api_keys", APIKey.Type),
		edge.To("providers", Provider.Type),
		edge.To("api_usages", APIUsage.Type),
	}
}

func (BillingGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
		index.Fields("is_default"),
		index.Fields("deleted_at"),
	}
}
