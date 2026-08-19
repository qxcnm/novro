package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

/**
 * TestMySQLMigrationChecksumsUpgradeAndRejectDrift 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestMySQLMigrationChecksumsUpgradeAndRejectDrift(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("NOVRO_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("set NOVRO_TEST_MYSQL_DSN to run the MySQL migration integration test")
	}
	serverConfig, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse NOVRO_TEST_MYSQL_DSN: %v", err)
	}
	serverConfig.DBName = ""
	serverConfig.MultiStatements = true
	serverConfig.ParseTime = true
	serverConfig.Loc = time.UTC
	connector, err := mysql.NewConnector(serverConfig)
	if err != nil {
		t.Fatalf("create MySQL integration connector: %v", err)
	}
	adminDB := sql.OpenDB(connector)
	t.Cleanup(func() { _ = adminDB.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("connect to MySQL integration server: %v", err)
	}
	databaseName := "novro_test_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci", databaseName)); err != nil {
		t.Fatalf("create isolated migration database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = adminDB.ExecContext(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", databaseName))
	})

	databaseConfig := *serverConfig
	databaseConfig.DBName = databaseName
	databaseConnector, err := mysql.NewConnector(&databaseConfig)
	if err != nil {
		t.Fatalf("create isolated database connector: %v", err)
	}
	database := sql.OpenDB(databaseConnector)
	t.Cleanup(func() { _ = database.Close() })
	if err := database.PingContext(ctx); err != nil {
		t.Fatalf("connect to isolated migration database: %v", err)
	}

	if err := Apply(ctx, database); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := Apply(ctx, database); err != nil {
		t.Fatalf("reapply migrations: %v", err)
	}
	assertMigrationChecksums(t, ctx, database)

	if _, err := database.ExecContext(ctx, `ALTER TABLE novro_schema_migrations DROP COLUMN checksum`); err != nil {
		t.Fatalf("simulate legacy migration metadata: %v", err)
	}
	if err := Apply(ctx, database); err != nil {
		t.Fatalf("upgrade legacy migration metadata: %v", err)
	}
	assertMigrationChecksums(t, ctx, database)

	migrations, err := readMigrations(VersionedSQL)
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE novro_schema_migrations SET checksum = ? WHERE version = ?`, strings.Repeat("0", 64), migrations[0].Version); err != nil {
		t.Fatalf("alter recorded checksum: %v", err)
	}
	if err := Apply(ctx, database); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected checksum drift rejection, got %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE novro_schema_migrations SET checksum = ? WHERE version = ?`, migrations[0].Checksum, migrations[0].Version); err != nil {
		t.Fatalf("restore recorded checksum: %v", err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO novro_schema_migrations (version, checksum, applied_at) VALUES ('9999_missing', ?, CURRENT_TIMESTAMP(6))`, strings.Repeat("1", 64)); err != nil {
		t.Fatalf("insert missing migration metadata: %v", err)
	}
	if err := Apply(ctx, database); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing migration rejection, got %v", err)
	}
}

func TestMySQLProviderProtocolsMigrationPreservesExistingProtocol(t *testing.T) {
	database := openMigrationIntegrationDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	migrations, err := readMigrations(VersionedSQL)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	var protocolMigration migrationFile
	for _, migration := range migrations {
		if migration.Version == "0015_provider_protocols" {
			protocolMigration = migration
			break
		}
		if _, err := database.ExecContext(ctx, migration.SQL); err != nil {
			t.Fatalf("apply pre-protocol migration %s: %v", migration.Version, err)
		}
	}
	if protocolMigration.Version == "" {
		t.Fatal("provider protocols migration not found")
	}

	for _, seed := range []struct {
		id, code, protocol string
	}{
		{id: "f1000000-0000-0000-0000-000000000001", code: "openai-provider", protocol: "openai"},
		{id: "f1000000-0000-0000-0000-000000000002", code: "anthropic-provider", protocol: "anthropic"},
	} {
		if _, err := database.ExecContext(ctx, `INSERT INTO providers (id, code, display_name, protocol, base_url, model_list_path, encrypted_api_key, api_key_hint, status, created_at, updated_at) VALUES (?, ?, ?, ?, 'https://example.com/v1', '', 'encrypted', 'rypt', 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, seed.id, seed.code, seed.code, seed.protocol); err != nil {
			t.Fatalf("seed provider %s: %v", seed.code, err)
		}
	}
	if _, err := database.ExecContext(ctx, protocolMigration.SQL); err != nil {
		t.Fatalf("apply provider protocols migration: %v", err)
	}

	rows, err := database.QueryContext(ctx, `SELECT code, JSON_UNQUOTE(JSON_EXTRACT(protocols, '$[0]')), JSON_LENGTH(protocols) FROM providers WHERE code IN ('openai-provider', 'anthropic-provider') ORDER BY code`)
	if err != nil {
		t.Fatalf("read migrated provider protocols: %v", err)
	}
	defer func() { _ = rows.Close() }()
	want := map[string]string{"openai-provider": "openai", "anthropic-provider": "anthropic"}
	for rows.Next() {
		var code, protocol string
		var count int
		if err := rows.Scan(&code, &protocol, &count); err != nil {
			t.Fatalf("scan migrated provider protocols: %v", err)
		}
		if protocol != want[code] || count != 1 {
			t.Fatalf("provider %s protocols=%s count=%d", code, protocol, count)
		}
		delete(want, code)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate migrated provider protocols: %v", err)
	}
	if len(want) != 0 {
		t.Fatalf("providers missing after migration: %+v", want)
	}
}

func TestMySQLBillingGroupCompositionsMigrationPreservesLegacyBillingData(t *testing.T) {
	database := openMigrationIntegrationDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	migrations, err := readMigrations(VersionedSQL)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	var compositionMigration migrationFile
	for _, migration := range migrations {
		if migration.Version == "0016_billing_group_compositions" {
			compositionMigration = migration
			break
		}
		if _, err := database.ExecContext(ctx, migration.SQL); err != nil {
			t.Fatalf("apply pre-composition migration %s: %v", migration.Version, err)
		}
	}
	if compositionMigration.Version == "" {
		t.Fatal("billing group compositions migration not found")
	}

	const (
		groupID    = "b1600000-0000-0000-0000-000000000001"
		userID     = "b1600000-0000-0000-0000-000000000002"
		apiKeyID   = "b1600000-0000-0000-0000-000000000003"
		providerID = "b1600000-0000-0000-0000-000000000004"
		modelID    = "b1600000-0000-0000-0000-000000000005"
		routeID    = "b1600000-0000-0000-0000-000000000006"
		usageID    = "b1600000-0000-0000-0000-000000000007"
		requestID  = "b1600000-0000-0000-0000-000000000008"
	)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO billing_groups (id, code, display_name, multiplier_bps, is_default, status, created_at, updated_at) VALUES (?, 'legacy-standard', 'Legacy Standard', 3500, FALSE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{groupID}},
		{`INSERT INTO users (id, invite_code, username, display_name, role, status, created_at, updated_at) VALUES (?, 'BILLING0016', 'billing-0016-user', 'Billing 0016 User', 'member', 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{userID}},
		{`INSERT INTO api_keys (id, name, key_prefix, key_hash, status, created_at, billing_group_id, user_id) VALUES (?, 'Legacy Key', 'nvr_legacy', REPEAT('b', 64), 'active', UTC_TIMESTAMP(6), ?, ?)`, []any{apiKeyID, groupID, userID}},
		{`INSERT INTO providers (id, code, display_name, protocol, protocols, base_url, model_list_path, encrypted_api_key, api_key_hint, status, created_at, updated_at) VALUES (?, 'legacy-provider', 'Legacy Provider', 'openai', JSON_ARRAY('openai'), 'https://legacy.example.com/v1', '', 'encrypted', 'pted', 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{providerID}},
		{`INSERT INTO upstream_models (id, provider_name, upstream_name, display_name, input_price_micros, output_price_micros, pricing_configured, status, created_at, updated_at) VALUES (?, 'Legacy Catalog', 'legacy-upstream', 'Legacy Upstream', 2000000, 8000000, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{modelID}},
		{`INSERT INTO model_routes (id, public_name, display_name, upstream_name, input_price_micros, output_price_micros, status, created_at, updated_at, billing_group_id, provider_id, upstream_model_id) VALUES (?, 'legacy-chat', 'Legacy Chat', 'legacy-upstream', 2000000, 8000000, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6), ?, ?, ?)`, []any{routeID, groupID, providerID, modelID}},
		{`INSERT INTO api_usages (id, request_id, endpoint, multiplier_bps, billing_group_code, billing_group_name, created_at, finished_at, api_key_id, billing_group_id, model_route_id, upstream_model_id, user_id) VALUES (?, ?, 'chat_completions', 3500, 'legacy-standard', 'Legacy Standard', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6), ?, ?, ?, ?, ?)`, []any{usageID, requestID, apiKeyID, groupID, routeID, modelID, userID}},
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed legacy billing data: %v", err)
		}
	}

	if _, err := database.ExecContext(ctx, compositionMigration.SQL); err != nil {
		t.Fatalf("apply billing group compositions migration: %v", err)
	}
	var kind string
	var multiplier int64
	if err := database.QueryRowContext(ctx, `SELECT kind, multiplier_bps FROM billing_groups WHERE id = ?`, groupID).Scan(&kind, &multiplier); err != nil {
		t.Fatalf("read migrated billing group: %v", err)
	}
	if kind != "standard" || multiplier != 3_500 {
		t.Fatalf("migrated billing group kind=%s multiplier=%d", kind, multiplier)
	}
	for _, reference := range []struct {
		name  string
		query string
		id    string
	}{
		{name: "API key", query: `SELECT billing_group_id FROM api_keys WHERE id = ?`, id: apiKeyID},
		{name: "model route", query: `SELECT billing_group_id FROM model_routes WHERE id = ?`, id: routeID},
		{name: "usage", query: `SELECT billing_group_id FROM api_usages WHERE id = ?`, id: usageID},
	} {
		var storedGroupID string
		if err := database.QueryRowContext(ctx, reference.query, reference.id).Scan(&storedGroupID); err != nil {
			t.Fatalf("read migrated %s reference: %v", reference.name, err)
		}
		if storedGroupID != groupID {
			t.Fatalf("%s billing group=%s, want %s", reference.name, storedGroupID, groupID)
		}
	}
	var usageMultiplier int64
	var usageCode, usageName string
	if err := database.QueryRowContext(ctx, `SELECT multiplier_bps, billing_group_code, billing_group_name FROM api_usages WHERE id = ?`, usageID).Scan(&usageMultiplier, &usageCode, &usageName); err != nil {
		t.Fatalf("read migrated usage snapshot: %v", err)
	}
	if usageMultiplier != 3_500 || usageCode != "legacy-standard" || usageName != "Legacy Standard" {
		t.Fatalf("usage snapshot changed: multiplier=%d code=%s name=%s", usageMultiplier, usageCode, usageName)
	}
	var compositionCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM billing_group_compositions`).Scan(&compositionCount); err != nil {
		t.Fatalf("read empty composition table: %v", err)
	}
	if compositionCount != 0 {
		t.Fatalf("legacy upgrade created unexpected compositions: %d", compositionCount)
	}
}

/**
 * TestMySQLModelRouteBillingGroupMigrationMovesExistingAssignments 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-15
 */
func TestMySQLModelRouteBillingGroupMigrationMovesExistingAssignments(t *testing.T) {
	database := openMigrationIntegrationDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	migrations, err := readMigrations(VersionedSQL)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	var routeGroupMigration migrationFile
	for _, migration := range migrations {
		if migration.Version == "0013_model_route_billing_groups" {
			routeGroupMigration = migration
			break
		}
		if _, err := database.ExecContext(ctx, migration.SQL); err != nil {
			t.Fatalf("apply pre-route-group migration %s: %v", migration.Version, err)
		}
	}
	if routeGroupMigration.Version == "" {
		t.Fatal("model route billing group migration not found")
	}

	const (
		defaultGroupID = "00000000-0000-0000-0000-000000000001"
		secondGroupID  = "d1000000-0000-0000-0000-000000000001"
		providerID     = "d2000000-0000-0000-0000-000000000001"
		modelID        = "d3000000-0000-0000-0000-000000000001"
		routeID        = "d4000000-0000-0000-0000-000000000001"
	)
	seedStatements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO billing_groups (id, code, display_name, multiplier_bps, is_default, status, created_at, updated_at) VALUES (?, 'migration-second', 'Migration Second', 15000, FALSE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{secondGroupID}},
		{`INSERT INTO providers (id, code, display_name, protocol, base_url, model_list_path, encrypted_api_key, api_key_hint, status, created_at, updated_at, billing_group_id) VALUES (?, 'route-group-provider', 'Route Group Provider', 'openai', 'https://example.com/v1', '', 'encrypted', 'rypt', 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6), ?)`, []any{providerID, defaultGroupID}},
		{`INSERT INTO upstream_models (id, provider_name, upstream_name, display_name, input_price_micros, output_price_micros, cache_read_price_micros, cache_write_price_micros, cache_write_1h_price_micros, request_price_micros, pricing_configured, status, created_at, updated_at) VALUES (?, 'Migration', 'route-group-model', 'Route Group Model', 1, 2, 0, 0, 0, 0, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{modelID}},
		{`INSERT INTO model_routes (id, provider_id, upstream_model_id, public_name, display_name, upstream_name, input_price_micros, output_price_micros, status, created_at, updated_at) VALUES (?, ?, ?, 'route-group-model', 'Route Group Model', 'route-group-model', 1, 2, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{routeID, providerID, modelID}},
	}
	for _, statement := range seedStatements {
		if _, err := database.ExecContext(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seed pre-route-group state: %v", err)
		}
	}

	if _, err := database.ExecContext(ctx, routeGroupMigration.SQL); err != nil {
		t.Fatalf("apply model route billing group migration: %v", err)
	}

	var migratedGroupID string
	if err := database.QueryRowContext(ctx, `SELECT billing_group_id FROM model_routes WHERE id = ?`, routeID).Scan(&migratedGroupID); err != nil {
		t.Fatalf("read migrated route group: %v", err)
	}
	if migratedGroupID != defaultGroupID {
		t.Fatalf("migrated route group=%s want=%s", migratedGroupID, defaultGroupID)
	}

	var providerGroupColumns, routeGroupNullable, routeGroupForeignKeys, routeGroupUniqueIndexes int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'providers' AND column_name = 'billing_group_id'`).Scan(&providerGroupColumns); err != nil {
		t.Fatalf("inspect provider group column: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'model_routes' AND column_name = 'billing_group_id' AND is_nullable = 'NO'`).Scan(&routeGroupNullable); err != nil {
		t.Fatalf("inspect route group column: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.referential_constraints WHERE constraint_schema = DATABASE() AND table_name = 'model_routes' AND constraint_name = 'model_routes_billing_groups_model_routes'`).Scan(&routeGroupForeignKeys); err != nil {
		t.Fatalf("inspect route group foreign key: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(DISTINCT index_name) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'model_routes' AND index_name = 'modelroute_group_public_provider_model' AND non_unique = 0`).Scan(&routeGroupUniqueIndexes); err != nil {
		t.Fatalf("inspect route group unique index: %v", err)
	}
	if providerGroupColumns != 0 || routeGroupNullable != 1 || routeGroupForeignKeys != 1 || routeGroupUniqueIndexes != 1 {
		t.Fatalf("migrated schema provider_columns=%d route_not_null=%d route_fks=%d route_unique_indexes=%d", providerGroupColumns, routeGroupNullable, routeGroupForeignKeys, routeGroupUniqueIndexes)
	}

	if _, err := database.ExecContext(ctx, `INSERT INTO model_routes (id, provider_id, upstream_model_id, billing_group_id, public_name, display_name, upstream_name, input_price_micros, output_price_micros, status, created_at, updated_at) VALUES (UUID(), ?, ?, ?, 'route-group-model', 'Route Group Model', 'route-group-model', 1, 2, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, providerID, modelID, secondGroupID); err != nil {
		t.Fatalf("insert same provider/model route in second group: %v", err)
	}
	var routeCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_routes WHERE provider_id = ? AND upstream_model_id = ? AND public_name = 'route-group-model'`, providerID, modelID).Scan(&routeCount); err != nil || routeCount != 2 {
		t.Fatalf("multi-group route count=%d err=%v", routeCount, err)
	}
}

/**
 * TestUnifiedModelCatalogMigrationConsolidatesExistingReferences 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestUnifiedModelCatalogMigrationConsolidatesExistingReferences(t *testing.T) {
	t.Skip("legacy 0023 migration assets are not part of the current squashed migration chain")
	database := openMigrationIntegrationDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	migrations, err := readMigrations(VersionedSQL)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	for _, migration := range migrations {
		if migration.Version == "0023_unified_model_catalog" {
			break
		}
		if _, err := database.ExecContext(ctx, migration.SQL); err != nil {
			t.Fatalf("apply migration %s: %v", migration.Version, err)
		}
	}

	const (
		providerOne  = "91000000-0000-0000-0000-000000000001"
		providerTwo  = "91000000-0000-0000-0000-000000000002"
		duplicateOne = "92000000-0000-0000-0000-000000000001"
		duplicateTwo = "92000000-0000-0000-0000-000000000002"
		routeOne     = "93000000-0000-0000-0000-000000000001"
		routeTwo     = "93000000-0000-0000-0000-000000000002"
		routeThree   = "93000000-0000-0000-0000-000000000003"
		userID       = "94000000-0000-0000-0000-000000000001"
		apiKeyID     = "95000000-0000-0000-0000-000000000001"
		usageID      = "96000000-0000-0000-0000-000000000001"
		requestID    = "97000000-0000-0000-0000-000000000001"
	)
	seedStatements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO providers (id, code, display_name, protocol, base_url, model_list_path, encrypted_api_key, api_key_hint, status, created_at, updated_at) VALUES (?, 'zijian', '自建', 'openai', 'https://one.example.com/v1', '', 'encrypted', 'rypt', 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{providerOne}},
		{`INSERT INTO providers (id, code, display_name, protocol, base_url, model_list_path, encrypted_api_key, api_key_hint, status, created_at, updated_at) VALUES (?, 'reasonix', 'Reasonix', 'openai', 'https://two.example.com/v1', '', 'encrypted', 'rypt', 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{providerTwo}},
		{`INSERT INTO upstream_models (id, provider_name, upstream_name, display_name, input_price_micros, output_price_micros, cache_read_price_micros, cache_write_price_micros, cache_write_1h_price_micros, request_price_micros, pricing_configured, status, created_at, updated_at) VALUES (?, '自建', 'kimi-k3', 'kimi-k3', 0, 0, 0, 0, 0, 0, FALSE, 'disabled', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{duplicateOne}},
		{`INSERT INTO upstream_models (id, provider_name, upstream_name, display_name, input_price_micros, output_price_micros, cache_read_price_micros, cache_write_price_micros, cache_write_1h_price_micros, request_price_micros, pricing_configured, status, created_at, updated_at) VALUES (?, 'Reasonix', 'KIMI-K3', 'KIMI-K3', 1, 2, 0, 0, 0, 0, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{duplicateTwo}},
		{`INSERT INTO model_routes (id, provider_id, upstream_model_id, public_name, display_name, upstream_name, input_price_micros, output_price_micros, status, created_at, updated_at) VALUES (?, ?, ?, 'zijian-kimi-k3', 'kimi-k3', 'kimi-k3', 0, 0, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{routeOne, providerOne, duplicateOne}},
		{`INSERT INTO model_routes (id, provider_id, upstream_model_id, public_name, display_name, upstream_name, input_price_micros, output_price_micros, status, created_at, updated_at) VALUES (?, ?, ?, 'reasonix-KIMI-K3', 'KIMI-K3', 'KIMI-K3', 1, 2, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{routeTwo, providerTwo, duplicateTwo}},
		{`INSERT INTO model_routes (id, provider_id, upstream_model_id, public_name, display_name, upstream_name, input_price_micros, output_price_micros, status, created_at, updated_at) VALUES (?, ?, ?, 'zijian-kimi-k3-2', 'kimi-k3', 'kimi-k3', 0, 0, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{routeThree, providerOne, duplicateOne}},
		{`INSERT INTO users (id, billing_group_id, invite_code, username, display_name, role, status, created_at, updated_at) VALUES (?, '00000000-0000-0000-0000-000000000001', 'MIGRATION001', 'migration-user', '', 'member', 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{userID}},
		{`INSERT INTO api_keys (id, user_id, name, key_prefix, key_hash, status, created_at) VALUES (?, ?, 'migration-key', 'nv-test', REPEAT('a', 64), 'active', UTC_TIMESTAMP(6))`, []any{apiKeyID, userID}},
		{`INSERT INTO api_usages (id, user_id, api_key_id, model_route_id, upstream_model_id, billing_group_id, request_id, endpoint, model_name, upstream_model_name, billing_group_code, billing_group_name, created_at, finished_at) VALUES (?, ?, ?, ?, ?, '00000000-0000-0000-0000-000000000001', ?, 'chat_completions', 'zijian-kimi-k3-2', 'kimi-k3', 'default', '默认分组', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{usageID, userID, apiKeyID, routeThree, duplicateOne, requestID}},
	}
	for _, statement := range seedStatements {
		if _, err := database.ExecContext(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seed duplicate model state: %v", err)
		}
	}

	var unified migrationFile
	var normalized migrationFile
	for _, migration := range migrations {
		if migration.Version == "0023_unified_model_catalog" {
			unified = migration
		}
		if migration.Version == "0024_normalize_generated_model_routes" {
			normalized = migration
		}
	}
	if unified.Version == "" || normalized.Version == "" {
		t.Fatal("unified model catalog migrations not found")
	}
	if _, err := database.ExecContext(ctx, unified.SQL); err != nil {
		t.Fatalf("apply unified model catalog migration: %v", err)
	}
	if _, err := database.ExecContext(ctx, normalized.SQL); err != nil {
		t.Fatalf("normalize generated model routes: %v", err)
	}

	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_models WHERE LOWER(upstream_name) = 'kimi-k3'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("global kimi-k3 catalog count=%d err=%v", count, err)
	}
	var canonicalID string
	var inputPrice, outputPrice int64
	var pricingConfigured bool
	if err := database.QueryRowContext(ctx, `SELECT id, input_price_micros, output_price_micros, pricing_configured FROM upstream_models WHERE LOWER(upstream_name) = 'kimi-k3'`).Scan(&canonicalID, &inputPrice, &outputPrice, &pricingConfigured); err != nil {
		t.Fatalf("read canonical kimi-k3: %v", err)
	}
	if canonicalID != "30000000-0000-0000-0000-000000000001" || !pricingConfigured || inputPrice != 20_000_000 || outputPrice != 100_000_000 {
		t.Fatalf("canonical model id=%s configured=%v prices=%d/%d", canonicalID, pricingConfigured, inputPrice, outputPrice)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_routes WHERE public_name = 'kimi-k3' AND upstream_model_id = ?`, canonicalID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("unified kimi-k3 routes=%d err=%v", count, err)
	}
	var usageModelID, usageRouteID string
	if err := database.QueryRowContext(ctx, `SELECT upstream_model_id, model_route_id FROM api_usages WHERE id = ?`, usageID).Scan(&usageModelID, &usageRouteID); err != nil {
		t.Fatalf("read migrated usage: %v", err)
	}
	if usageModelID != canonicalID || usageRouteID != routeOne {
		t.Fatalf("usage model=%s route=%s", usageModelID, usageRouteID)
	}
}

/**
 * TestGeneratedRouteRepairMigrationNormalizesRoutesCreatedAfterPriorMigrations 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestGeneratedRouteRepairMigrationNormalizesRoutesCreatedAfterPriorMigrations(t *testing.T) {
	t.Skip("legacy 0025 migration assets are not part of the current squashed migration chain")
	database := openMigrationIntegrationDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	migrations, err := readMigrations(VersionedSQL)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	var repair migrationFile
	for _, migration := range migrations {
		if migration.Version == "0025_repair_generated_model_route_names" {
			repair = migration
			break
		}
		if _, err := database.ExecContext(ctx, migration.SQL); err != nil {
			t.Fatalf("apply migration %s: %v", migration.Version, err)
		}
	}
	if repair.Version == "" {
		t.Fatal("generated route repair migration not found")
	}

	const (
		providerID     = "a1000000-0000-0000-0000-000000000001"
		modelID        = "a2000000-0000-0000-0000-000000000001"
		deletedRouteID = "a3000000-0000-0000-0000-000000000001"
		activeRouteID  = "a3000000-0000-0000-0000-000000000002"
		customRouteID  = "a3000000-0000-0000-0000-000000000003"
		userID         = "a4000000-0000-0000-0000-000000000001"
		apiKeyID       = "a5000000-0000-0000-0000-000000000001"
		usageID        = "a6000000-0000-0000-0000-000000000001"
		requestID      = "a7000000-0000-0000-0000-000000000001"
	)
	seedStatements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO providers (id, code, display_name, protocol, base_url, model_list_path, encrypted_api_key, api_key_hint, status, created_at, updated_at) VALUES (?, 'reasonix', '1024token', 'openai', 'https://example.com/v1', '', 'encrypted', 'rypt', 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{providerID}},
		{`INSERT INTO upstream_models (id, provider_name, upstream_name, display_name, input_price_micros, output_price_micros, cache_read_price_micros, cache_write_price_micros, cache_write_1h_price_micros, request_price_micros, pricing_configured, status, created_at, updated_at) VALUES (?, 'Kimi', 'repair-model', 'repair-model', 6500000, 27000000, 1300000, 0, 0, 0, TRUE, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{modelID}},
		{`INSERT INTO model_routes (id, provider_id, upstream_model_id, public_name, display_name, upstream_name, input_price_micros, output_price_micros, status, created_at, updated_at, deleted_at) VALUES (?, ?, ?, 'reasonix-repair-model', 'repair-model', 'repair-model', 6500000, 27000000, 'disabled', UTC_TIMESTAMP(6) - INTERVAL 2 MINUTE, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{deletedRouteID, providerID, modelID}},
		{`INSERT INTO model_routes (id, provider_id, upstream_model_id, public_name, display_name, upstream_name, input_price_micros, output_price_micros, status, created_at, updated_at) VALUES (?, ?, ?, 'reasonix-repair-model-2', 'repair-model', 'repair-model', 6500000, 27000000, 'active', UTC_TIMESTAMP(6) - INTERVAL 1 MINUTE, UTC_TIMESTAMP(6))`, []any{activeRouteID, providerID, modelID}},
		{`INSERT INTO model_routes (id, provider_id, upstream_model_id, public_name, display_name, upstream_name, input_price_micros, output_price_micros, status, created_at, updated_at) VALUES (?, ?, ?, 'my-repair-model', 'My Repair Model', 'repair-model', 6500000, 27000000, 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{customRouteID, providerID, modelID}},
		{`INSERT INTO users (id, billing_group_id, invite_code, username, display_name, role, status, created_at, updated_at) VALUES (?, '00000000-0000-0000-0000-000000000001', 'REPAIRMIG001', 'route-repair-user', '', 'member', 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{userID}},
		{`INSERT INTO api_keys (id, user_id, name, key_prefix, key_hash, status, created_at) VALUES (?, ?, 'repair-key', 'nv-test', REPEAT('b', 64), 'active', UTC_TIMESTAMP(6))`, []any{apiKeyID, userID}},
		{`INSERT INTO api_usages (id, user_id, api_key_id, model_route_id, upstream_model_id, billing_group_id, request_id, endpoint, model_name, upstream_model_name, billing_group_code, billing_group_name, created_at, finished_at) VALUES (?, ?, ?, ?, ?, '00000000-0000-0000-0000-000000000001', ?, 'chat_completions', 'reasonix-repair-model', 'repair-model', 'default', '默认分组', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{usageID, userID, apiKeyID, deletedRouteID, modelID, requestID}},
	}
	for _, statement := range seedStatements {
		if _, err := database.ExecContext(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seed generated route state: %v", err)
		}
	}
	if _, err := database.ExecContext(ctx, repair.SQL); err != nil {
		t.Fatalf("apply generated route repair migration: %v", err)
	}

	var publicName, status string
	var deletedAt sql.NullTime
	if err := database.QueryRowContext(ctx, `SELECT public_name, status, deleted_at FROM model_routes WHERE id = ?`, activeRouteID).Scan(&publicName, &status, &deletedAt); err != nil {
		t.Fatalf("read repaired active route: %v", err)
	}
	if publicName != "repair-model" || status != "active" || deletedAt.Valid {
		t.Fatalf("repaired route name=%s status=%s deleted_at=%v", publicName, status, deletedAt)
	}
	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_routes WHERE id = ?`, deletedRouteID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("duplicate generated route count=%d err=%v", count, err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_routes WHERE id = ? AND public_name = 'my-repair-model'`, customRouteID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("custom alias count=%d err=%v", count, err)
	}
	var usageRouteID string
	if err := database.QueryRowContext(ctx, `SELECT model_route_id FROM api_usages WHERE id = ?`, usageID).Scan(&usageRouteID); err != nil {
		t.Fatalf("read repaired usage: %v", err)
	}
	if usageRouteID != activeRouteID {
		t.Fatalf("usage route=%s want=%s", usageRouteID, activeRouteID)
	}
}

/**
 * TestMySQLGatewayBillingSafetyMigrationUpgradesExistingData 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestMySQLGatewayBillingSafetyMigrationUpgradesExistingData(t *testing.T) {
	database := openMigrationIntegrationDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	migrations, err := readMigrations(VersionedSQL)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
CREATE TABLE novro_schema_migrations (
    version VARCHAR(128) NOT NULL PRIMARY KEY,
    checksum CHAR(64) NOT NULL DEFAULT '',
    applied_at DATETIME(6) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`); err != nil {
		t.Fatalf("create migration metadata: %v", err)
	}
	var safetyMigration migrationFile
	for _, migration := range migrations {
		if migration.Version == "0009_gateway_billing_safety" {
			safetyMigration = migration
			break
		}
		if _, err := database.ExecContext(ctx, migration.SQL); err != nil {
			t.Fatalf("apply pre-safety migration %s: %v", migration.Version, err)
		}
		if _, err := database.ExecContext(ctx,
			`INSERT INTO novro_schema_migrations (version, checksum, applied_at) VALUES (?, ?, UTC_TIMESTAMP(6))`,
			migration.Version, migration.Checksum,
		); err != nil {
			t.Fatalf("record pre-safety migration %s: %v", migration.Version, err)
		}
	}
	if safetyMigration.Version == "" {
		t.Fatal("gateway billing safety migration not found")
	}

	const (
		userID        = "b1000000-0000-0000-0000-000000000001"
		walletID      = "b2000000-0000-0000-0000-000000000001"
		apiKeyID      = "b3000000-0000-0000-0000-000000000001"
		reservationID = "b4000000-0000-0000-0000-000000000001"
		reservation   = int64(100_000)
		balance       = int64(900_000)
	)
	seedStatements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users (id, invite_code, username, display_name, role, status, created_at, updated_at) VALUES (?, 'BILLINGSAFE1', 'billing-safety-user', 'Billing Safety User', 'member', 'active', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6))`, []any{userID}},
		{`INSERT INTO wallets (id, balance_micros, created_at, updated_at, user_id) VALUES (?, ?, UTC_TIMESTAMP(6), UTC_TIMESTAMP(6), ?)`, []any{walletID, balance, userID}},
		{`INSERT INTO api_keys (id, name, key_prefix, key_hash, status, created_at, billing_group_id, user_id) VALUES (?, 'billing-safety-key', 'nvr_safe', REPEAT('a', 64), 'active', UTC_TIMESTAMP(6), '00000000-0000-0000-0000-000000000001', ?)`, []any{apiKeyID, userID}},
		{`INSERT INTO wallet_entries (id, reference_id, entry_type, amount_micros, balance_after_micros, description, created_at, wallet_id) VALUES (UUID(), ?, 'usage_reservation', ?, ?, 'existing reservation', UTC_TIMESTAMP(6), ?)`, []any{reservationID, -reservation, balance, walletID}},
	}
	for _, statement := range seedStatements {
		if _, err := database.ExecContext(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seed pre-safety billing state: %v", err)
		}
	}

	if err := Apply(ctx, database); err != nil {
		t.Fatalf("upgrade through gateway billing safety migration: %v", err)
	}
	if err := Apply(ctx, database); err != nil {
		t.Fatalf("reapply gateway billing safety migration: %v", err)
	}
	assertMigrationChecksums(t, ctx, database)

	var storedBalance, reservationAmount int64
	if err := database.QueryRowContext(ctx, `SELECT balance_micros FROM wallets WHERE id = ?`, walletID).Scan(&storedBalance); err != nil {
		t.Fatalf("read preserved wallet: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT amount_micros FROM wallet_entries WHERE reference_id = ? AND entry_type = 'usage_reservation'`, reservationID).Scan(&reservationAmount); err != nil {
		t.Fatalf("read preserved reservation: %v", err)
	}
	if storedBalance != balance || reservationAmount != -reservation {
		t.Fatalf("migration changed existing billing state: balance=%d reservation=%d", storedBalance, reservationAmount)
	}

	var operationStatusType, walletEntryType string
	if err := database.QueryRowContext(ctx, `SELECT COLUMN_TYPE FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'gateway_operations' AND column_name = 'status'`).Scan(&operationStatusType); err != nil {
		t.Fatalf("read gateway operation status type: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COLUMN_TYPE FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'wallet_entries' AND column_name = 'entry_type'`).Scan(&walletEntryType); err != nil {
		t.Fatalf("read wallet entry type: %v", err)
	}
	if !strings.Contains(operationStatusType, "pending_settlement") || !strings.Contains(operationStatusType, "pending_unknown") || !strings.Contains(walletEntryType, "usage_compensation") {
		t.Fatalf("billing safety enums missing: operation=%q wallet_entry=%q", operationStatusType, walletEntryType)
	}

	operationID := uuid.NewString()
	if _, err := database.ExecContext(ctx, `
INSERT INTO gateway_operations
    (id, idempotency_key_hash, request_hash, endpoint, status, reserved_micros, settlement_json, failure_code, created_at, updated_at, api_key_id, user_id)
VALUES (?, REPEAT('b', 64), REPEAT('c', 64), 'chat_completions', 'pending_unknown', ?, '', 'upstream_result_unknown', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6), ?, ?)`,
		operationID, reservation, apiKeyID, userID,
	); err != nil {
		t.Fatalf("insert pending unknown operation: %v", err)
	}
	_, duplicateErr := database.ExecContext(ctx, `
INSERT INTO gateway_operations
    (id, idempotency_key_hash, request_hash, endpoint, status, reserved_micros, settlement_json, failure_code, created_at, updated_at, api_key_id, user_id)
VALUES (?, REPEAT('b', 64), REPEAT('d', 64), 'chat_completions', 'processing', ?, '', '', UTC_TIMESTAMP(6), UTC_TIMESTAMP(6), ?, ?)`,
		uuid.NewString(), reservation, apiKeyID, userID,
	)
	var mysqlError *mysql.MySQLError
	if !errors.As(duplicateErr, &mysqlError) || mysqlError.Number != 1062 {
		t.Fatalf("expected idempotency unique-key rejection, got %v", duplicateErr)
	}
	if _, err := database.ExecContext(ctx, `
INSERT INTO wallet_entries
    (id, reference_id, entry_type, amount_micros, balance_after_micros, description, created_at, actor_user_id, wallet_id)
VALUES (UUID(), ?, 'usage_compensation', ?, ?, 'legacy usage compensation', UTC_TIMESTAMP(6), ?, ?)`,
		operationID, reservation, balance+reservation, userID, walletID,
	); err != nil {
		t.Fatalf("insert usage compensation ledger entry: %v", err)
	}
}

/**
 * openMigrationIntegrationDatabase 封装该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func openMigrationIntegrationDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("NOVRO_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("set NOVRO_TEST_MYSQL_DSN to run the MySQL migration integration test")
	}
	serverConfig, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse NOVRO_TEST_MYSQL_DSN: %v", err)
	}
	serverConfig.DBName = ""
	serverConfig.MultiStatements = true
	serverConfig.ParseTime = true
	serverConfig.Loc = time.UTC
	connector, err := mysql.NewConnector(serverConfig)
	if err != nil {
		t.Fatalf("create MySQL integration connector: %v", err)
	}
	adminDB := sql.OpenDB(connector)
	t.Cleanup(func() { _ = adminDB.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("connect to MySQL integration server: %v", err)
	}
	databaseName := "novro_test_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci", databaseName)); err != nil {
		t.Fatalf("create isolated migration database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = adminDB.ExecContext(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", databaseName))
	})
	databaseConfig := *serverConfig
	databaseConfig.DBName = databaseName
	databaseConnector, err := mysql.NewConnector(&databaseConfig)
	if err != nil {
		t.Fatalf("create isolated database connector: %v", err)
	}
	database := sql.OpenDB(databaseConnector)
	t.Cleanup(func() { _ = database.Close() })
	if err := database.PingContext(ctx); err != nil {
		t.Fatalf("connect to isolated integration database: %v", err)
	}
	return database
}

/**
 * assertMigrationChecksums 封装该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param database 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func assertMigrationChecksums(t *testing.T, ctx context.Context, database *sql.DB) {
	t.Helper()
	var total, valid int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*), SUM(CHAR_LENGTH(checksum) = 64) FROM novro_schema_migrations`).Scan(&total, &valid); err != nil {
		t.Fatalf("read migration checksums: %v", err)
	}
	if total == 0 || valid != total {
		t.Fatalf("migration checksums are incomplete: total=%d valid=%d", total, valid)
	}
}
