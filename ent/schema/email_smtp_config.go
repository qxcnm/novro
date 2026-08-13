package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// EmailSMTPConfig stores the administrator-managed SMTP connection used for
// registration verification emails. The password is encrypted before it is
// persisted.
type EmailSMTPConfig struct {
	ent.Schema
}

/**
 * Fields 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (EmailSMTPConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(32).NotEmpty().Immutable(),
		field.Bool("enabled").Default(false),
		field.String("host").MaxLen(255).Default(""),
		field.Int("port").Default(587),
		field.String("username").MaxLen(320).Default(""),
		field.String("encrypted_password").MaxLen(2048).Sensitive().Default(""),
		field.String("from_address").MaxLen(320).Default(""),
		field.String("security").MaxLen(16).Default("starttls"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
