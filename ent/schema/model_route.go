package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type ModelRoute struct {
	ent.Schema
}

func (ModelRoute) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("provider_id", uuid.UUID{}),
		field.String("public_name").NotEmpty().MaxLen(128).Unique().Immutable(),
		field.String("display_name").NotEmpty().MaxLen(128),
		field.String("upstream_name").NotEmpty().MaxLen(256),
		field.Int64("input_price_micros").NonNegative(),
		field.Int64("output_price_micros").NonNegative(),
		field.Enum("status").Values("active", "disabled").Default("active"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (ModelRoute) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("provider", Provider.Type).Ref("model_routes").Unique().Field("provider_id").Required(),
		edge.To("api_usages", APIUsage.Type),
	}
}

func (ModelRoute) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id", "status"),
		index.Fields("status", "created_at"),
	}
}
