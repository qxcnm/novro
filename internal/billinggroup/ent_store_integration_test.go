package billinggroup

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
	entapikey "github.com/novro-gateway/novro/ent/apikey"
	"github.com/novro-gateway/novro/ent/migrate"
)

func TestEntStoreCompositeMembershipAndDeletionInvariants(t *testing.T) {
	client := openBillingGroupIntegrationClient(t)
	ctx := context.Background()
	store := NewEntStore(client)
	memberA, err := store.Create(ctx, CreateInput{Code: "member-a", DisplayName: "Member A", Kind: KindStandard, MultiplierBPS: 3_000})
	if err != nil {
		t.Fatalf("create member A: %v", err)
	}
	memberB, err := store.Create(ctx, CreateInput{Code: "member-b", DisplayName: "Member B", Kind: KindStandard, MultiplierBPS: 5_000})
	if err != nil {
		t.Fatalf("create member B: %v", err)
	}
	compositeA, err := store.Create(ctx, CreateInput{
		Code: "composite-a", DisplayName: "Composite A", Kind: KindComposite, MultiplierBPS: DefaultMultiplierBPS,
		MemberGroupIDs: []uuid.UUID{memberA.ID, memberB.ID},
	})
	if err != nil {
		t.Fatalf("create composite A: %v", err)
	}
	compositeB, err := store.Create(ctx, CreateInput{
		Code: "composite-b", DisplayName: "Composite B", Kind: KindComposite, MultiplierBPS: DefaultMultiplierBPS,
		MemberGroupIDs: []uuid.UUID{memberA.ID},
	})
	if err != nil {
		t.Fatalf("reuse member A in composite B: %v", err)
	}
	if compositeA.MemberGroupCount != 2 || compositeB.MemberGroupCount != 1 {
		t.Fatalf("unexpected member counts: A=%+v B=%+v", compositeA, compositeB)
	}

	if _, err := store.Create(ctx, CreateInput{Code: "duplicate-members", DisplayName: "Duplicate Members", Kind: KindComposite, MultiplierBPS: DefaultMultiplierBPS, MemberGroupIDs: []uuid.UUID{memberA.ID, memberA.ID}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duplicate member error=%v", err)
	}
	if _, err := store.Create(ctx, CreateInput{Code: "missing-member", DisplayName: "Missing Member", Kind: KindComposite, MultiplierBPS: DefaultMultiplierBPS, MemberGroupIDs: []uuid.UUID{uuid.New()}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nonexistent member error=%v", err)
	}
	if _, err := store.Create(ctx, CreateInput{Code: "nested-composite", DisplayName: "Nested Composite", Kind: KindComposite, MultiplierBPS: DefaultMultiplierBPS, MemberGroupIDs: []uuid.UUID{compositeA.ID}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nested composite error=%v", err)
	}
	selfMembers := []uuid.UUID{compositeA.ID}
	if _, err := store.Update(ctx, compositeA.ID, UpdateInput{MemberGroupIDs: &selfMembers}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("self membership error=%v", err)
	}
	if err := store.Delete(ctx, memberA.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("delete referenced member error=%v", err)
	}
	targetKind := KindComposite
	memberIDs := []uuid.UUID{memberB.ID}
	if _, err := store.Update(ctx, memberA.ID, UpdateInput{Kind: &targetKind, MemberGroupIDs: &memberIDs}); !errors.Is(err, ErrInUse) {
		t.Fatalf("change referenced member to composite error=%v", err)
	}

	owner, err := client.User.Create().SetUsername("composite-key-owner").SetDisplayName("Composite Key Owner").Save(ctx)
	if err != nil {
		t.Fatalf("create key owner: %v", err)
	}
	key, err := client.APIKey.Create().SetUserID(owner.ID).SetBillingGroupID(compositeA.ID).SetName("Composite Key").SetKeyPrefix("nvr_test").SetKeyHash(strings.Repeat("a", 64)).Save(ctx)
	if err != nil {
		t.Fatalf("create active composite key: %v", err)
	}
	if err := store.Delete(ctx, compositeA.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("delete composite with active key error=%v", err)
	}
	if _, err := client.APIKey.UpdateOneID(key.ID).SetStatus(entapikey.StatusRevoked).SetRevokedAt(time.Now().UTC()).Save(ctx); err != nil {
		t.Fatalf("revoke composite key: %v", err)
	}
	if err := store.Delete(ctx, compositeA.ID); err != nil {
		t.Fatalf("delete composite after key revocation: %v", err)
	}
	if err := store.Delete(ctx, memberA.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("member reused by second composite should remain protected: %v", err)
	}
	if err := store.Delete(ctx, compositeB.ID); err != nil {
		t.Fatalf("delete second composite: %v", err)
	}
	if err := store.Delete(ctx, memberA.ID); err != nil {
		t.Fatalf("delete member after removing all references: %v", err)
	}
}

func openBillingGroupIntegrationClient(t *testing.T) *ent.Client {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("NOVRO_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("set NOVRO_TEST_MYSQL_DSN to run the MySQL billing-group integration test")
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
		t.Fatalf("create isolated billing-group database: %v", err)
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
		t.Fatalf("connect to isolated billing-group database: %v", err)
	}
	if err := migrate.Apply(ctx, database); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	driver := entsql.OpenDB(dialect.MySQL, database)
	client := ent.NewClient(ent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	return client
}
