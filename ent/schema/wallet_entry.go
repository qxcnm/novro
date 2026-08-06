package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type WalletEntry struct {
	ent.Schema
}

func (WalletEntry) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("wallet_id", uuid.UUID{}),
		field.UUID("actor_user_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("reference_id", uuid.UUID{}),
		field.Enum("entry_type").Values("manual_adjustment", "usage_reservation", "usage_refund"),
		field.Int64("amount_micros"),
		field.Int64("balance_after_micros").NonNegative(),
		field.String("description").MaxLen(255).Default(""),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (WalletEntry) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("wallet", Wallet.Type).Ref("entries").Unique().Field("wallet_id").Required(),
		edge.From("actor", User.Type).Ref("wallet_entries").Unique().Field("actor_user_id"),
	}
}

func (WalletEntry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("wallet_id", "created_at"),
		index.Fields("reference_id"),
	}
}
