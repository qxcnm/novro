package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	"github.com/novro-gateway/novro/ent/migrate"
	entuser "github.com/novro-gateway/novro/ent/user"
	entwalletentry "github.com/novro-gateway/novro/ent/walletentry"
)

const mysqlIntegrationDSNEnv = "NOVRO_TEST_MYSQL_DSN"

func TestMySQLConcurrentReservationsPreserveBalance(t *testing.T) {
	client := openMySQLIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userID := uuid.New()
	createdUser, err := client.User.Create().
		SetID(userID).
		SetUsername("concurrency-" + strings.ReplaceAll(userID.String(), "-", "")).
		SetDisplayName("Concurrency Test").
		SetRole(entuser.RoleMember).
		SetStatus(entuser.StatusActive).
		Save(ctx)
	if err != nil {
		t.Fatalf("create integration user: %v", err)
	}
	const initialBalance int64 = 1_000_000
	wallet, err := client.Wallet.Create().SetUserID(createdUser.ID).SetBalanceMicros(initialBalance).Save(ctx)
	if err != nil {
		t.Fatalf("create integration wallet: %v", err)
	}

	service := NewService(NewEntStore(client))
	const (
		attempts          = 20
		reservationAmount = int64(100_000)
		expectedSuccesses = int(initialBalance / reservationAmount)
	)

	start := make(chan struct{})
	results := make(chan error, attempts)
	var workers sync.WaitGroup
	for range attempts {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- service.Reserve(context.Background(), createdUser.ID, uuid.New(), reservationAmount, "并发一致性测试")
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	var succeeded, rejected int
	for result := range results {
		switch {
		case result == nil:
			succeeded++
		case errors.Is(result, ErrInsufficientBalance):
			rejected++
		default:
			t.Fatalf("unexpected reservation error: %v", result)
		}
	}
	if succeeded != expectedSuccesses || rejected != attempts-expectedSuccesses {
		t.Fatalf("unexpected reservation results: succeeded=%d rejected=%d", succeeded, rejected)
	}

	summary, err := service.Summary(ctx, createdUser.ID)
	if err != nil {
		t.Fatalf("read final balance: %v", err)
	}
	if summary.Wallet.BalanceMicros != 0 {
		t.Fatalf("final balance=%d, want 0", summary.Wallet.BalanceMicros)
	}
	if len(summary.Entries) != expectedSuccesses {
		t.Fatalf("ledger entries=%d, want %d", len(summary.Entries), expectedSuccesses)
	}
	var ledgerTotal int64
	for _, entry := range summary.Entries {
		if entry.Type != EntryUsageReservation || entry.AmountMicros != -reservationAmount {
			t.Fatalf("unexpected ledger entry: %+v", entry)
		}
		ledgerTotal += entry.AmountMicros
	}
	if ledgerTotal != -initialBalance {
		t.Fatalf("ledger total=%d, want %d", ledgerTotal, -initialBalance)
	}
	entryCount, err := client.WalletEntry.Query().Where(entwalletentry.WalletIDEQ(wallet.ID)).Count(ctx)
	if err != nil {
		t.Fatalf("count reservation ledger: %v", err)
	}
	if entryCount != expectedSuccesses {
		t.Fatalf("persisted ledger entries=%d, want %d", entryCount, expectedSuccesses)
	}
}

func openMySQLIntegrationClient(t *testing.T) *ent.Client {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(mysqlIntegrationDSNEnv))
	if dsn == "" {
		t.Skipf("set %s to run the isolated MySQL integration test", mysqlIntegrationDSNEnv)
	}

	serverConfig, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse %s: %v", mysqlIntegrationDSNEnv, err)
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
	t.Cleanup(func() {
		if err := adminDB.Close(); err != nil {
			t.Errorf("close MySQL integration connection: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("connect to isolated MySQL integration server: %v", err)
	}
	databaseName := "novro_test_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf(
		"CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci",
		databaseName,
	)); err != nil {
		t.Fatalf("create isolated MySQL integration database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := adminDB.ExecContext(cleanupCtx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", databaseName)); err != nil {
			t.Errorf("drop isolated MySQL integration database: %v", err)
		}
	})

	databaseConfig := *serverConfig
	databaseConfig.DBName = databaseName
	databaseConnector, err := mysql.NewConnector(&databaseConfig)
	if err != nil {
		t.Fatalf("create isolated database connector: %v", err)
	}
	database := sql.OpenDB(databaseConnector)
	database.SetMaxOpenConns(25)
	database.SetMaxIdleConns(25)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		t.Fatalf("connect to isolated integration database: %v", err)
	}
	if err := migrate.Apply(ctx, database); err != nil {
		_ = database.Close()
		t.Fatalf("apply migrations to isolated integration database: %v", err)
	}
	if err := migrate.Apply(ctx, database); err != nil {
		_ = database.Close()
		t.Fatalf("reapply migrations to verify idempotence: %v", err)
	}

	driver := entsql.OpenDB(dialect.MySQL, database)
	client := ent.NewClient(ent.Driver(driver))
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close integration Ent client: %v", err)
		}
	})
	return client
}
