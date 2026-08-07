package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("billing_group_id", uuid.UUID{}).Optional().Nillable(),
		field.String("username").NotEmpty().MaxLen(64).Unique(),
		field.String("email").NotEmpty().MaxLen(320).Optional().Nillable().Unique(),
		field.String("display_name").MaxLen(128).Default(""),
		field.String("password_hash").Optional().Nillable().Sensitive(),
		field.Enum("role").Values("admin", "member").Default("member"),
		field.Enum("status").Values("active", "disabled").Default("active"),
		field.Time("last_login_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("sessions", UserSession.Type),
		edge.To("identities", UserIdentity.Type),
		edge.To("api_keys", APIKey.Type),
		edge.To("wallet", Wallet.Type).Unique(),
		edge.To("wallet_entries", WalletEntry.Type),
		edge.To("top_up_orders", TopUpOrder.Type),
		edge.To("api_usages", APIUsage.Type),
		edge.From("billing_group", BillingGroup.Type).Ref("users").Unique().Field("billing_group_id"),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("role", "status"),
	}
}
