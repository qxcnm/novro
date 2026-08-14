package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// ModelPricePlan is an immutable, versioned customer pricing policy for one
// globally catalogued upstream model.
type ModelPricePlan struct {
	ent.Schema
}

func (ModelPricePlan) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("upstream_model_id", uuid.UUID{}),
		field.Int("version").Positive(),
		field.Enum("mode").Values("fixed", "scheduled").Default("fixed"),
		field.String("timezone").NotEmpty().MaxLen(64).Default("Asia/Shanghai"),
		field.Time("effective_from"),
		field.Time("effective_to").Optional().Nillable(),
		field.Enum("status").Values("draft", "published", "retired").Default("draft"),
		field.Int64("default_input_price_micros").NonNegative(),
		field.Int64("default_output_price_micros").NonNegative(),
		field.Int64("default_cache_read_price_micros").NonNegative(),
		field.Int64("default_cache_write_price_micros").NonNegative(),
		field.Int64("default_cache_write_1h_price_micros").NonNegative(),
		field.Int64("default_request_price_micros").NonNegative(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (ModelPricePlan) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("upstream_model", UpstreamModel.Type).Ref("price_plans").Unique().Field("upstream_model_id").Required(),
		edge.To("windows", ModelPriceWindow.Type),
	}
}

func (ModelPricePlan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("upstream_model_id", "version").Unique(),
		index.Fields("upstream_model_id", "status", "effective_from"),
		index.Fields("status", "effective_from"),
	}
}
