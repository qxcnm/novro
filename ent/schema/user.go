package schema

import (
	"strings"
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
		field.String("invite_code").NotEmpty().MaxLen(16).Unique().Immutable().DefaultFunc(func() string {
			return strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:12])
		}),
		field.UUID("referred_by_user_id", uuid.UUID{}).Optional().Nillable().Immutable(),
		field.String("username").NotEmpty().MaxLen(64).Unique(),
		field.String("email").NotEmpty().MaxLen(320).Optional().Nillable().Unique(),
		field.String("display_name").MaxLen(128).Default(""),
		field.String("password_hash").Optional().Nillable().Sensitive(),
		field.Bool("is_system_admin").Default(false),
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
		edge.To("gateway_operations", GatewayOperation.Type),
		edge.From("authorized_billing_groups", BillingGroup.Type).Ref("authorized_users"),
		edge.To("referrals", User.Type).From("referrer").Unique().Field("referred_by_user_id").Immutable(),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("role", "status"),
		index.Fields("referred_by_user_id", "created_at"),
	}
}
