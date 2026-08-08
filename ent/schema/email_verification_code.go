package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type EmailVerificationCode struct {
	ent.Schema
}

func (EmailVerificationCode) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("email").NotEmpty().MaxLen(320),
		field.String("code_hash").NotEmpty().MaxLen(64).Sensitive(),
		field.Time("expires_at"),
		field.Time("consumed_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (EmailVerificationCode) Indexes() []ent.Index {
	return []ent.Index{index.Fields("email").Unique()}
}
