package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type TopUpOrder struct {
	ent.Schema
}

/**
 * Fields 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (TopUpOrder) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("user_id", uuid.UUID{}),
		field.String("out_trade_no").NotEmpty().MaxLen(64).Unique().Immutable(),
		field.Enum("provider").Values("epay").Default("epay").Immutable(),
		field.String("channel").NotEmpty().MaxLen(32).Immutable(),
		field.Int64("amount_micros").Positive().Immutable(),
		field.Int64("credited_micros").Positive().Immutable(),
		field.Enum("status").Values("pending", "paid").Default("pending"),
		field.String("provider_trade_no").MaxLen(128).Optional().Nillable().Unique(),
		field.Time("paid_at").Optional().Nillable(),
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
func (TopUpOrder) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("top_up_orders").Unique().Field("user_id").Required(),
	}
}

/**
 * Indexes 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (TopUpOrder) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("status", "created_at"),
	}
}
