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

func (APIUsage) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("user_id", uuid.UUID{}),
		field.UUID("api_key_id", uuid.UUID{}),
		field.UUID("model_route_id", uuid.UUID{}),
		field.UUID("request_id", uuid.UUID{}).Unique(),
		field.Enum("endpoint").Values("chat_completions", "responses", "messages"),
		field.Int("input_tokens").NonNegative().Default(0),
		field.Int("output_tokens").NonNegative().Default(0),
		field.Int64("cost_micros").NonNegative().Default(0),
		field.Int64("reserved_micros").NonNegative().Default(0),
		field.Bool("estimated").Default(false),
		field.String("upstream_request_id").MaxLen(255).Default(""),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("finished_at").Default(time.Now),
	}
}

func (APIUsage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("api_usages").Unique().Field("user_id").Required(),
		edge.From("api_key", APIKey.Type).Ref("api_usages").Unique().Field("api_key_id").Required(),
		edge.From("model_route", ModelRoute.Type).Ref("api_usages").Unique().Field("model_route_id").Required(),
	}
}

func (APIUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("api_key_id", "created_at"),
		index.Fields("model_route_id", "created_at"),
	}
}
