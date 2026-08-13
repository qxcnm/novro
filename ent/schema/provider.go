package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type Provider struct {
	ent.Schema
}

/**
 * Edges 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (Provider) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("billing_group", BillingGroup.Type).Ref("providers").Unique().Field("billing_group_id").Required(),
		edge.To("model_routes", ModelRoute.Type),
	}
}

/**
 * Fields 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (Provider) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("billing_group_id", uuid.UUID{}),
		field.String("code").NotEmpty().MaxLen(64).Unique().Immutable(),
		field.String("display_name").NotEmpty().MaxLen(128),
		field.Enum("protocol").Values("openai", "anthropic"),
		field.String("base_url").NotEmpty().MaxLen(512),
		field.String("model_list_path").Default("").MaxLen(512),
		field.Int("weight").Positive().Max(1_000_000).Default(100),
		field.String("encrypted_api_key").NotEmpty().MaxLen(2048).Sensitive(),
		field.String("api_key_hint").NotEmpty().MaxLen(8),
		field.Enum("status").Values("active", "disabled").Default("active"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
		field.Time("deleted_at").Optional().Nillable(),
	}
}

/**
 * Indexes 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (Provider) Indexes() []ent.Index {
	return []ent.Index{index.Fields("billing_group_id", "status"), index.Fields("status", "created_at"), index.Fields("deleted_at")}
}
