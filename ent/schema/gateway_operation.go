package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// GatewayOperation persists the money-sensitive lifecycle of one model call.
// It intentionally stores hashes and settlement metadata, never prompts,
// credentials, authorization headers, or full model responses.
type GatewayOperation struct {
	ent.Schema
}

/**
 * Fields 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (GatewayOperation) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("user_id", uuid.UUID{}),
		field.UUID("api_key_id", uuid.UUID{}),
		field.String("idempotency_key_hash").NotEmpty().MaxLen(64),
		field.String("request_hash").NotEmpty().MaxLen(64),
		field.Enum("endpoint").Values("chat_completions", "responses", "messages"),
		field.Enum("status").Values("processing", "pending_settlement", "pending_unknown", "completed", "failed").Default("processing"),
		field.Int64("reserved_micros").NonNegative().Default(0),
		field.Text("settlement_json").Default("").Sensitive(),
		field.String("failure_code").MaxLen(64).Default(""),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

/**
 * Edges 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (GatewayOperation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("gateway_operations").Unique().Field("user_id").Required(),
		edge.From("api_key", APIKey.Type).Ref("gateway_operations").Unique().Field("api_key_id").Required(),
	}
}

/**
 * Indexes 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (GatewayOperation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("api_key_id", "idempotency_key_hash").Unique(),
		index.Fields("status", "updated_at"),
		index.Fields("user_id", "created_at"),
	}
}
