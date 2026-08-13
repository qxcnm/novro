package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// PaymentConfig stores the administrator-managed configuration for a payment
// provider. Provider credentials are encrypted before they reach this table.
type PaymentConfig struct {
	ent.Schema
}

/**
 * Fields 封装该名称对应的业务处理逻辑。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (PaymentConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").StorageKey("provider").NotEmpty().MaxLen(32).Immutable(),
		field.Bool("enabled").Default(false),
		field.String("api_url").MaxLen(512).Default(""),
		field.String("merchant_id").MaxLen(128).Default(""),
		field.String("encrypted_merchant_key").MaxLen(2048).Sensitive().Default(""),
		field.String("site_name").MaxLen(64).Default("Novro"),
		field.String("channels").MaxLen(512).Default(""),
		field.Text("methods_json").Default("[]"),
		field.Int64("min_top_up_micros").Default(10_000),
		field.Int64("max_top_up_micros").Default(50_000_000_000),
		field.Text("preset_amounts_json").Default("[10000000,50000000,100000000,500000000]"),
		field.Text("bonus_tiers_json").Default("[]"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
