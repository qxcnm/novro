package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// UserIdentity binds an external OIDC subject to one local user. Email is
// metadata only and is never used to merge accounts.
type UserIdentity struct {
	ent.Schema
}

/**
 * Fields 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (UserIdentity) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("user_id", uuid.UUID{}),
		field.String("issuer").NotEmpty().MaxLen(512),
		field.String("subject").NotEmpty().MaxLen(255),
		field.String("email").MaxLen(320).Default(""),
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
func (UserIdentity) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("identities").Unique().Field("user_id").Required(),
	}
}

/**
 * Indexes 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (UserIdentity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("issuer", "subject").Unique(),
		index.Fields("user_id", "issuer").Unique(),
	}
}
