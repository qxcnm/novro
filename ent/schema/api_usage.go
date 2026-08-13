package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type APIUsage struct {
	ent.Schema
}

/**
 * Fields 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (APIUsage) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("user_id", uuid.UUID{}),
		field.UUID("api_key_id", uuid.UUID{}),
		field.UUID("model_route_id", uuid.UUID{}),
		field.UUID("upstream_model_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("billing_group_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("request_id", uuid.UUID{}).Unique(),
		field.Enum("endpoint").Values("chat_completions", "responses", "messages"),
		field.Int("status_code").Positive().Default(200),
		field.String("error_code").MaxLen(64).Default(""),
		field.String("error_message").MaxLen(1024).Default(""),
		field.Int("input_tokens").NonNegative().Default(0),
		field.Int("uncached_input_tokens").NonNegative().Default(0),
		field.Int("cache_read_input_tokens").NonNegative().Default(0),
		field.Int("cache_write_input_tokens").NonNegative().Default(0),
		field.Int("cache_write_1h_input_tokens").NonNegative().Default(0),
		field.Int("output_tokens").NonNegative().Default(0),
		field.Int64("input_price_micros").NonNegative().Default(0),
		field.Int64("output_price_micros").NonNegative().Default(0),
		field.Int64("cache_read_price_micros").NonNegative().Default(0),
		field.Int64("cache_write_price_micros").NonNegative().Default(0),
		field.Int64("cache_write_1h_price_micros").NonNegative().Default(0),
		field.Int64("request_price_micros").NonNegative().Default(0),
		field.Int64("base_cost_micros").NonNegative().Default(0),
		field.Int64("multiplier_bps").Positive().Default(10_000),
		field.Int64("cost_micros").NonNegative().Default(0),
		field.Int64("reserved_micros").NonNegative().Default(0),
		field.Bool("estimated").Default(false),
		field.String("upstream_request_id").MaxLen(255).Default(""),
		field.String("model_name").MaxLen(256).Default(""),
		field.String("upstream_model_name").MaxLen(256).Default(""),
		field.String("billing_group_code").MaxLen(64).Default(""),
		field.String("billing_group_name").MaxLen(128).Default(""),
		field.String("calculation_version").MaxLen(32).Default("v1"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("finished_at").Default(time.Now),
		field.Int64("duration_ms").NonNegative().Default(0),
	}
}

/**
 * Edges 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (APIUsage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("api_usages").Unique().Field("user_id").Required(),
		edge.From("api_key", APIKey.Type).Ref("api_usages").Unique().Field("api_key_id").Required(),
		edge.From("model_route", ModelRoute.Type).Ref("api_usages").Unique().Field("model_route_id").Required(),
		edge.From("upstream_model", UpstreamModel.Type).Ref("api_usages").Unique().Field("upstream_model_id"),
		edge.From("billing_group", BillingGroup.Type).Ref("api_usages").Unique().Field("billing_group_id"),
	}
}

/**
 * Indexes 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (APIUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("user_id", "model_name", "created_at"),
		index.Fields("user_id", "status_code", "created_at"),
		index.Fields("api_key_id", "created_at"),
		index.Fields("model_route_id", "created_at"),
		index.Fields("upstream_model_id", "created_at"),
		index.Fields("billing_group_id", "created_at"),
	}
}
