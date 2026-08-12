package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type APIKey struct {
	ent.Schema
}

func (APIKey) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("user_id", uuid.UUID{}),
		field.UUID("billing_group_id", uuid.UUID{}),
		field.String("name").NotEmpty().MaxLen(64),
		field.String("key_prefix").NotEmpty().MaxLen(16),
		field.String("key_hash").NotEmpty().MaxLen(64).Unique().Sensitive(),
		field.String("key_secret_ciphertext").Default("").MaxLen(256).Sensitive(),
		field.Enum("status").Values("active", "revoked").Default("active"),
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("revoked_at").Optional().Nillable(),
	}
}

func (APIKey) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("api_keys").Unique().Field("user_id").Required(),
		edge.From("billing_group", BillingGroup.Type).Ref("api_keys").Unique().Field("billing_group_id").Required(),
		edge.To("api_usages", APIUsage.Type),
		edge.To("gateway_operations", GatewayOperation.Type),
	}
}

func (APIKey) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status"),
		index.Fields("billing_group_id", "status"),
		index.Fields("status", "created_at"),
	}
}
