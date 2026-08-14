package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// ModelPriceWindow overrides a plan's default price for recurring local-time
// windows. Windows are validated as [start_minute, end_minute) intervals.
type ModelPriceWindow struct {
	ent.Schema
}

func (ModelPriceWindow) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("price_plan_id", uuid.UUID{}),
		field.String("label").NotEmpty().MaxLen(64),
		field.Int("weekday_mask").NonNegative().Max(127),
		field.Int("start_minute").NonNegative().Max(1439),
		field.Int("end_minute").Positive().Max(1440),
		field.Int64("input_price_micros").NonNegative(),
		field.Int64("output_price_micros").NonNegative(),
		field.Int64("cache_read_price_micros").NonNegative(),
		field.Int64("cache_write_price_micros").NonNegative(),
		field.Int64("cache_write_1h_price_micros").NonNegative(),
		field.Int64("request_price_micros").NonNegative(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (ModelPriceWindow) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("price_plan", ModelPricePlan.Type).Ref("windows").Unique().Field("price_plan_id").Required(),
	}
}

func (ModelPriceWindow) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("price_plan_id", "weekday_mask", "start_minute", "end_minute"),
	}
}
