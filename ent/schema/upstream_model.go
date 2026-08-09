package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type UpstreamModel struct {
	ent.Schema
}

func (UpstreamModel) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("provider_name").NotEmpty().MaxLen(128),
		field.String("upstream_name").NotEmpty().MaxLen(256),
		field.String("display_name").NotEmpty().MaxLen(128),
		field.Int64("input_price_micros").NonNegative().Default(0),
		field.Int64("output_price_micros").NonNegative().Default(0),
		field.Int64("cache_read_price_micros").NonNegative().Default(0),
		field.Int64("cache_write_price_micros").NonNegative().Default(0),
		field.Int64("cache_write_1h_price_micros").NonNegative().Default(0),
		field.Int64("request_price_micros").NonNegative().Default(0),
		field.Bool("pricing_configured").Default(true),
		field.Enum("status").Values("active", "disabled").Default("active"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
		field.Time("deleted_at").Optional().Nillable(),
	}
}

func (UpstreamModel) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("model_routes", ModelRoute.Type),
		edge.To("api_usages", APIUsage.Type),
	}
}

func (UpstreamModel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("upstream_name").Unique(),
		index.Fields("provider_name", "status"),
		index.Fields("status", "created_at"),
		index.Fields("deleted_at"),
	}
}
