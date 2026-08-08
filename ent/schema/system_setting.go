package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// SystemSetting stores installation markers and non-secret application settings.
type SystemSetting struct {
	ent.Schema
}

func (SystemSetting) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").StorageKey("key").NotEmpty().MaxLen(128).Immutable(),
		field.String("value").MaxLen(1024).Default(""),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
