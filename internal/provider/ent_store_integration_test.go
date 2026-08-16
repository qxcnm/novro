package provider

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	"github.com/novro-gateway/novro/ent/migrate"
	entprovider "github.com/novro-gateway/novro/ent/provider"
)

func TestEntStoreCreateReusesSoftDeletedProviderCode(t *testing.T) {
	client := openProviderIntegrationClient(t)
	ctx := context.Background()
	store := NewEntStore(client)
	original, err := client.Provider.Create().
		SetCode("reusable-provider").
		SetDisplayName("Original Provider").
		SetProtocol(entprovider.ProtocolOpenai).
		SetProtocols([]string{"openai"}).
		SetBaseURL("https://old.example.com/v1").
		SetEncryptedAPIKey("old-encrypted-key").
		SetAPIKeyHint("old1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create original provider: %v", err)
	}
	if err := store.Delete(ctx, original.ID); err != nil {
		t.Fatalf("delete original provider: %v", err)
	}

	restored, err := store.Create(ctx, CreateParams{
		Code: "reusable-provider", DisplayName: "Replacement Provider", Protocols: []Protocol{ProtocolOpenAI, ProtocolAnthropic}, PrimaryProtocol: ProtocolOpenAI,
		BaseURL: "https://new.example.com", ModelListPath: "/models", Weight: 250,
		EncryptedAPIKey: "new-encrypted-key", APIKeyHint: "new1",
	})
	if err != nil {
		t.Fatalf("reuse deleted provider code: %v", err)
	}
	if restored.ID != original.ID || restored.DisplayName != "Replacement Provider" || len(restored.Protocols) != 2 || !SupportsProtocol(restored.Protocols, ProtocolOpenAI) || !SupportsProtocol(restored.Protocols, ProtocolAnthropic) || restored.Status != StatusActive {
		t.Fatalf("unexpected restored provider: %+v", restored)
	}
	count, err := client.Provider.Query().Where(entprovider.CodeEQ("reusable-provider")).Count(ctx)
	if err != nil || count != 1 {
		t.Fatalf("provider code count=%d err=%v", count, err)
	}
	if _, err := store.Create(ctx, CreateParams{
		Code: "reusable-provider", DisplayName: "Duplicate", Protocols: []Protocol{ProtocolOpenAI}, PrimaryProtocol: ProtocolOpenAI,
		BaseURL: "https://duplicate.example.com", Weight: 100, EncryptedAPIKey: "duplicate-key", APIKeyHint: "dupe",
	}); !errors.Is(err, ErrCodeTaken) {
		t.Fatalf("active duplicate error=%v", err)
	}
}

func openProviderIntegrationClient(t *testing.T) *ent.Client {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("NOVRO_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("set NOVRO_TEST_MYSQL_DSN to run the MySQL provider integration test")
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
	adminDB.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = adminDB.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("connect to MySQL integration server: %v", err)
	}
	databaseName := "novro_test_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci", databaseName)); err != nil {
		t.Fatalf("create isolated MySQL integration database: %v", err)
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
	database.SetMaxOpenConns(10)
	database.SetMaxIdleConns(10)
	t.Cleanup(func() { _ = database.Close() })
	if err := database.PingContext(ctx); err != nil {
		t.Fatalf("connect to isolated integration database: %v", err)
	}
	if err := migrate.Apply(ctx, database); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	driver := entsql.OpenDB(dialect.MySQL, database)
	client := ent.NewClient(ent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	return client
}
