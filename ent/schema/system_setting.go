package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
)

// SystemSetting stores installation markers and non-secret application settings.
type SystemSetting struct {
	ent.Schema
}

/**
 * Fields 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (SystemSetting) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").StorageKey("key").NotEmpty().MaxLen(128).Immutable(),
		field.Text("value").SchemaType(map[string]string{dialect.MySQL: "text"}).Default(""),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
