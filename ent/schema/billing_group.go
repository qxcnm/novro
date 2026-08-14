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

/**
 * Fields 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (BillingGroup) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("code").NotEmpty().MaxLen(64).Unique().Immutable(),
		field.String("display_name").NotEmpty().MaxLen(128),
		field.Int64("multiplier_bps").Positive().Default(10_000),
		field.Bool("is_default").Default(false),
		field.Bool("is_hidden").Default(false),
		field.Enum("status").Values("active", "disabled").Default("active"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
		field.Time("deleted_at").Optional().Nillable(),
	}
}

/**
 * Edges 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (BillingGroup) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("api_keys", APIKey.Type),
		edge.To("model_routes", ModelRoute.Type),
		edge.To("api_usages", APIUsage.Type),
		edge.To("authorized_users", User.Type).
			StorageKey(edge.Table("billing_group_authorized_users"), edge.Columns("billing_group_id", "user_id")),
	}
}

/**
 * Indexes 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (BillingGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
		index.Fields("is_default"),
		index.Fields("deleted_at"),
	}
}
