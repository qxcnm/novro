package apikey

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
	"github.com/novro-gateway/novro/internal/billinggroup"
)

func TestCompositeKeyAuthorizationUsesParentAndIgnoresHiddenMembers(t *testing.T) {
	client := openAPIKeyIntegrationClient(t)
	ctx := context.Background()
	owner, err := client.User.Create().SetUsername("composite-auth-owner").SetDisplayName("Composite Auth Owner").Save(ctx)
	if err != nil {
		t.Fatalf("create API key owner: %v", err)
	}
	groupStore := billinggroup.NewEntStore(client)
	hiddenMember, err := groupStore.Create(ctx, billinggroup.CreateInput{Code: "hidden-member", DisplayName: "Hidden Member", Kind: billinggroup.KindStandard, MultiplierBPS: 3_000, IsHidden: true})
	if err != nil {
		t.Fatalf("create hidden member: %v", err)
	}
	composite, err := groupStore.Create(ctx, billinggroup.CreateInput{
		Code: "hidden-composite", DisplayName: "Hidden Composite", Kind: billinggroup.KindComposite, MultiplierBPS: billinggroup.DefaultMultiplierBPS, IsHidden: true,
		AuthorizedUserIDs: []uuid.UUID{owner.ID}, MemberGroupIDs: []uuid.UUID{hiddenMember.ID},
	})
	if err != nil {
		t.Fatalf("create hidden composite: %v", err)
	}
	store := NewEntStore(client)
	if _, err := store.Create(ctx, owner.ID, hiddenMember.ID, "Direct Hidden Member", "nvr_direct", strings.Repeat("d", 64), "encrypted-direct", 10); !errors.Is(err, ErrGroupUnavailable) {
		t.Fatalf("direct hidden member key creation error=%v", err)
	}
	created, err := store.Create(ctx, owner.ID, composite.ID, "Composite Key", "nvr_parent", strings.Repeat("p", 64), "encrypted-parent", 10)
	if err != nil {
		t.Fatalf("create composite key: %v", err)
	}
	if created.BillingGroup.Kind != billinggroup.KindComposite || created.BillingGroup.MemberGroupCount != 1 || len(created.BillingGroup.MemberGroupIDs) != 1 || created.BillingGroup.MemberGroupIDs[0] != hiddenMember.ID {
		t.Fatalf("composite key summary=%+v", created.BillingGroup)
	}
	if _, err := store.AuthenticateHash(ctx, strings.Repeat("p", 64), time.Now().UTC()); err != nil {
		t.Fatalf("parent authorization did not cover hidden member: %v", err)
	}
	if _, err := client.BillingGroup.UpdateOneID(composite.ID).ClearAuthorizedUsers().Save(ctx); err != nil {
		t.Fatalf("revoke composite authorization: %v", err)
	}
	if _, err := store.AuthenticateHash(ctx, strings.Repeat("p", 64), time.Now().UTC()); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked parent authorization error=%v", err)
	}
	if _, err := client.BillingGroup.UpdateOneID(composite.ID).AddAuthorizedUserIDs(owner.ID).Save(ctx); err != nil {
		t.Fatalf("restore composite authorization: %v", err)
	}
	if _, err := groupStore.SetStatus(ctx, composite.ID, billinggroup.StatusDisabled); err != nil {
		t.Fatalf("disable composite group: %v", err)
	}
	if _, err := store.AuthenticateHash(ctx, strings.Repeat("p", 64), time.Now().UTC()); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("disabled composite key authentication error=%v", err)
	}
	if _, err := groupStore.SetStatus(ctx, composite.ID, billinggroup.StatusActive); err != nil {
		t.Fatalf("reactivate composite group: %v", err)
	}
	if _, err := groupStore.SetStatus(ctx, hiddenMember.ID, billinggroup.StatusDisabled); err != nil {
		t.Fatalf("disable hidden member: %v", err)
	}
	if _, err := store.AuthenticateHash(ctx, strings.Repeat("p", 64), time.Now().UTC()); err != nil {
		t.Fatalf("member status should affect routes, not parent key authentication: %v", err)
	}
}

func openAPIKeyIntegrationClient(t *testing.T) *ent.Client {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("NOVRO_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("set NOVRO_TEST_MYSQL_DSN to run the MySQL API-key integration test")
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
		t.Fatalf("create isolated API-key database: %v", err)
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
		t.Fatalf("connect to isolated API-key database: %v", err)
	}
	if err := migrate.Apply(ctx, database); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	driver := entsql.OpenDB(dialect.MySQL, database)
	client := ent.NewClient(ent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	return client
}
