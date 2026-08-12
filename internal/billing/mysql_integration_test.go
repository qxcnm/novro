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
	entapiusage "github.com/novro-gateway/novro/ent/apiusage"
	entbillinggroup "github.com/novro-gateway/novro/ent/billinggroup"
	"github.com/novro-gateway/novro/ent/migrate"
	entuser "github.com/novro-gateway/novro/ent/user"
	entwalletentry "github.com/novro-gateway/novro/ent/walletentry"
	"github.com/novro-gateway/novro/internal/payment"
	"github.com/novro-gateway/novro/internal/referral"
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

func TestMySQLUsageAccountingRetriesAreIdempotent(t *testing.T) {
	client := openMySQLIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	createdUser, err := client.User.Create().
		SetUsername("idempotency-" + strings.ReplaceAll(uuid.New().String(), "-", "")).
		SetDisplayName("Idempotency Test").SetRole(entuser.RoleMember).SetStatus(entuser.StatusActive).
		Save(ctx)
	if err != nil {
		t.Fatalf("create integration user: %v", err)
	}
	const initialBalance int64 = 1_000_000
	if _, err := client.Wallet.Create().SetUserID(createdUser.ID).SetBalanceMicros(initialBalance).Save(ctx); err != nil {
		t.Fatalf("create integration wallet: %v", err)
	}
	group, err := client.BillingGroup.Query().Where(entbillinggroup.IsDefaultEQ(true)).Only(ctx)
	if err != nil {
		t.Fatalf("read default billing group: %v", err)
	}
	apiKey, err := client.APIKey.Create().SetUserID(createdUser.ID).SetBillingGroupID(group.ID).SetName("integration").SetKeyPrefix("nvr_test").SetKeyHash(strings.Repeat("a", 64)).Save(ctx)
	if err != nil {
		t.Fatalf("create integration API key: %v", err)
	}
	providerEntity, err := client.Provider.Create().SetBillingGroupID(group.ID).SetCode("integration-provider").SetDisplayName("Integration Provider").SetProtocol("openai").SetBaseURL("https://api.example.com").SetEncryptedAPIKey("encrypted").SetAPIKeyHint("hint").Save(ctx)
	if err != nil {
		t.Fatalf("create integration provider: %v", err)
	}
	upstream, err := client.UpstreamModel.Create().SetProviderName("Integration Provider").SetUpstreamName("upstream-model").SetDisplayName("Upstream Model").SetRequestPriceMicros(300_000).Save(ctx)
	if err != nil {
		t.Fatalf("create integration upstream model: %v", err)
	}
	route, err := client.ModelRoute.Create().SetProviderID(providerEntity.ID).SetUpstreamModelID(upstream.ID).SetPublicName("integration-model").SetDisplayName("Integration Model").SetUpstreamName("upstream-model").SetInputPriceMicros(0).SetOutputPriceMicros(0).Save(ctx)
	if err != nil {
		t.Fatalf("create integration model route: %v", err)
	}

	service := NewService(NewEntStore(client))
	requestID := uuid.New()
	if err := service.Reserve(ctx, createdUser.ID, requestID, 400_000, "idempotent request"); err != nil {
		t.Fatalf("reserve balance: %v", err)
	}
	if err := service.Reserve(ctx, createdUser.ID, requestID, 400_000, "idempotent retry"); err != nil {
		t.Fatalf("retry reservation: %v", err)
	}
	if err := service.Reserve(ctx, createdUser.ID, requestID, 300_000, "conflicting retry"); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("conflicting reservation err=%v", err)
	}
	if err := service.Refund(ctx, createdUser.ID, requestID, 100_000, "partial refund"); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("partial refund err=%v", err)
	}

	usage := UsageInput{UserID: createdUser.ID, APIKeyID: apiKey.ID, ModelRouteID: route.ID, UpstreamModelID: &upstream.ID, BillingGroupID: &group.ID, RequestID: requestID, Endpoint: "chat_completions", InputTokens: 10, Tokens: TokenBreakdown{UncachedInput: 10, Output: 20}, OutputTokens: 20, Rates: RateCard{RequestMicros: 300_000}, BaseCostMicros: 300_000, MultiplierBPS: 10_000, CostMicros: 300_000, ReservedMicros: 400_000, ModelName: "integration-model", UpstreamModelName: "upstream-model", BillingGroupCode: group.Code, BillingGroupName: group.DisplayName, CalculationVersion: CalculationVersion, CreatedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()}
	if err := service.Finalize(ctx, usage); err != nil {
		t.Fatalf("finalize usage: %v", err)
	}
	if err := service.Finalize(ctx, usage); err != nil {
		t.Fatalf("retry finalization: %v", err)
	}
	if err := service.ReleaseReservation(ctx, createdUser.ID, requestID, 400_000, "unknown commit retry"); err != nil {
		t.Fatalf("release after committed usage: %v", err)
	}
	conflictingUsage := usage
	conflictingUsage.OutputTokens++
	conflictingUsage.Tokens.Output++
	if err := service.Finalize(ctx, conflictingUsage); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("conflicting finalization err=%v", err)
	}

	summary, err := service.Summary(ctx, createdUser.ID)
	if err != nil {
		t.Fatalf("read idempotent balance: %v", err)
	}
	if summary.Wallet.BalanceMicros != initialBalance-300_000 {
		t.Fatalf("balance=%d want=%d", summary.Wallet.BalanceMicros, initialBalance-300_000)
	}
	usagePage, err := service.Usage(ctx, createdUser.ID, UsageFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list usage page: %v", err)
	}
	if usagePage.Total != 1 || len(usagePage.Usage) != 1 || usagePage.TotalTokens != 30 || usagePage.TotalCostMicros != 300_000 || len(usagePage.Models) != 1 || usagePage.Models[0] != "integration-model" {
		t.Fatalf("usage page=%+v", usagePage)
	}
	searchPage, err := service.Usage(ctx, createdUser.ID, UsageFilter{Search: requestID.String(), Status: UsageStatusSuccess, Offset: 1, Limit: 1})
	if err != nil {
		t.Fatalf("filter usage page: %v", err)
	}
	if searchPage.Total != 1 || len(searchPage.Usage) != 0 || searchPage.TotalTokens != 30 || searchPage.TotalCostMicros != 300_000 {
		t.Fatalf("filtered usage page=%+v", searchPage)
	}
	entryPage, err := service.SummaryPage(ctx, createdUser.ID, EntryFilter{Offset: 1, Limit: 1})
	if err != nil {
		t.Fatalf("list wallet entry page: %v", err)
	}
	if entryPage.EntriesTotal != 2 || len(entryPage.Entries) != 1 || entryPage.EntriesOffset != 1 || entryPage.EntriesLimit != 1 {
		t.Fatalf("wallet entry page=%+v", entryPage)
	}
	entries, err := client.WalletEntry.Query().Where(entwalletentry.ReferenceIDEQ(requestID)).All(ctx)
	if err != nil {
		t.Fatalf("list idempotent ledger entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ledger entries=%d want=2", len(entries))
	}

	orphanedRequestID := uuid.New()
	if err := service.Reserve(ctx, createdUser.ID, orphanedRequestID, 50_000, "orphaned request"); err != nil {
		t.Fatalf("reserve orphaned request: %v", err)
	}
	reservedSummary, err := service.Summary(ctx, createdUser.ID)
	if err != nil {
		t.Fatalf("summarize orphaned request: %v", err)
	}
	if reservedSummary.ReservedMicros != 50_000 {
		t.Fatalf("active reservation=%d want=50000", reservedSummary.ReservedMicros)
	}
	if err := service.ReleaseReservation(ctx, createdUser.ID, orphanedRequestID, 50_000, "release orphaned request"); err != nil {
		t.Fatalf("release orphaned request: %v", err)
	}
	if err := service.ReleaseReservation(ctx, createdUser.ID, orphanedRequestID, 50_000, "retry orphaned release"); err != nil {
		t.Fatalf("retry orphaned release: %v", err)
	}
	releasedSummary, err := service.Summary(ctx, createdUser.ID)
	if err != nil {
		t.Fatalf("summarize released request: %v", err)
	}
	if releasedSummary.ReservedMicros != 0 || releasedSummary.Wallet.BalanceMicros != initialBalance-300_000 {
		t.Fatalf("released summary=%+v", releasedSummary)
	}
	fullyChargedUsage := usage
	fullyChargedUsage.RequestID = orphanedRequestID
	fullyChargedUsage.Rates.RequestMicros = 50_000
	fullyChargedUsage.BaseCostMicros = 50_000
	fullyChargedUsage.CostMicros = 50_000
	fullyChargedUsage.ReservedMicros = 50_000
	if err := service.Finalize(ctx, fullyChargedUsage); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("finalize after incompatible refund err=%v", err)
	}
	usageCount, err := client.APIUsage.Query().Where(entapiusage.RequestIDEQ(orphanedRequestID)).Count(ctx)
	if err != nil {
		t.Fatalf("count usage after incompatible refund: %v", err)
	}
	if usageCount != 0 {
		t.Fatalf("usage after incompatible refund=%d want=0", usageCount)
	}
}

func TestMySQLConcurrentTopUpNotificationsCreditOnce(t *testing.T) {
	client := openMySQLIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	createdUser, err := client.User.Create().
		SetUsername("top-up-" + strings.ReplaceAll(uuid.New().String(), "-", "")).
		SetDisplayName("Top-up Test").SetRole(entuser.RoleMember).SetStatus(entuser.StatusActive).
		Save(ctx)
	if err != nil {
		t.Fatalf("create top-up integration user: %v", err)
	}
	wallet, err := client.Wallet.Create().SetUserID(createdUser.ID).Save(ctx)
	if err != nil {
		t.Fatalf("create top-up integration wallet: %v", err)
	}
	store := payment.NewEntStore(client)
	orderID := uuid.New()
	created, err := store.Create(ctx, payment.CreateParams{
		ID: orderID, UserID: createdUser.ID, OutTradeNo: "NVR" + strings.ReplaceAll(orderID.String(), "-", ""),
		Channel: "alipay", AmountMicros: 10_000_000, CreditedMicros: 10_000_000,
	})
	if err != nil {
		t.Fatalf("create top-up integration order: %v", err)
	}
	completion := payment.CompleteParams{
		OutTradeNo: created.OutTradeNo, ProviderTradeNo: "EPAY-CONCURRENT-1", Channel: "alipay",
		AmountMicros: created.AmountMicros, PaidAt: time.Now().UTC(),
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, completeErr := store.Complete(context.Background(), completion)
			results <- completeErr
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("complete concurrent top-up notification: %v", err)
		}
	}

	updatedWallet, err := client.Wallet.Get(ctx, wallet.ID)
	if err != nil {
		t.Fatalf("read top-up wallet: %v", err)
	}
	if updatedWallet.BalanceMicros != created.AmountMicros {
		t.Fatalf("top-up balance=%d want=%d", updatedWallet.BalanceMicros, created.AmountMicros)
	}
	entries, err := client.WalletEntry.Query().Where(
		entwalletentry.WalletIDEQ(wallet.ID),
		entwalletentry.ReferenceIDEQ(created.ID),
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeTopUp),
	).All(ctx)
	if err != nil {
		t.Fatalf("list top-up ledger entries: %v", err)
	}
	if len(entries) != 1 || entries[0].AmountMicros != created.AmountMicros {
		t.Fatalf("top-up ledger=%+v", entries)
	}
}

func TestMySQLReferralCashbackCreditsInviterOnce(t *testing.T) {
	client := openMySQLIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	referrer, err := client.User.Create().
		SetUsername("referrer-" + strings.ReplaceAll(uuid.NewString(), "-", "")).
		SetDisplayName("Referrer").SetRole(entuser.RoleMember).SetStatus(entuser.StatusActive).
		Save(ctx)
	if err != nil {
		t.Fatalf("create referrer: %v", err)
	}
	referrerWallet, err := client.Wallet.Create().SetUserID(referrer.ID).Save(ctx)
	if err != nil {
		t.Fatalf("create referrer wallet: %v", err)
	}
	referred, err := client.User.Create().
		SetUsername("referred-" + strings.ReplaceAll(uuid.NewString(), "-", "")).
		SetDisplayName("Referred").SetRole(entuser.RoleMember).SetStatus(entuser.StatusActive).
		SetReferredByUserID(referrer.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create referred user: %v", err)
	}
	if _, err := client.Wallet.Create().SetUserID(referred.ID).Save(ctx); err != nil {
		t.Fatalf("create referred wallet: %v", err)
	}
	if _, err := client.SystemSetting.UpdateOneID(referral.RewardBPSSettingKey).SetValue("500").Save(ctx); err != nil {
		t.Fatalf("set referral reward rate: %v", err)
	}

	store := payment.NewEntStore(client, 1_000)
	orderID := uuid.New()
	created, err := store.Create(ctx, payment.CreateParams{
		ID: orderID, UserID: referred.ID, OutTradeNo: "NVR" + strings.ReplaceAll(orderID.String(), "-", ""),
		Channel: "alipay", AmountMicros: 10_000_000, CreditedMicros: 10_000_000,
	})
	if err != nil {
		t.Fatalf("create referred top-up: %v", err)
	}
	completion := payment.CompleteParams{
		OutTradeNo: created.OutTradeNo, ProviderTradeNo: "EPAY-REFERRAL-1", Channel: "alipay",
		AmountMicros: created.AmountMicros, PaidAt: time.Now().UTC(),
	}
	for range 2 {
		if _, err := store.Complete(ctx, completion); err != nil {
			t.Fatalf("complete referred top-up: %v", err)
		}
	}

	updatedWallet, err := client.Wallet.Get(ctx, referrerWallet.ID)
	if err != nil {
		t.Fatalf("read referrer wallet: %v", err)
	}
	if updatedWallet.BalanceMicros != 500_000 {
		t.Fatalf("referrer balance=%d want=500000", updatedWallet.BalanceMicros)
	}
	entries, err := client.WalletEntry.Query().Where(
		entwalletentry.WalletIDEQ(referrerWallet.ID),
		entwalletentry.ReferenceIDEQ(created.ID),
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeReferralReward),
	).All(ctx)
	if err != nil {
		t.Fatalf("list referral cashback entries: %v", err)
	}
	if len(entries) != 1 || entries[0].AmountMicros != 500_000 {
		t.Fatalf("referral cashback entries=%+v", entries)
	}
	stats, err := referral.NewEntStore(client).Stats(ctx, referrer.ID, 500)
	if err != nil {
		t.Fatalf("read referral details: %v", err)
	}
	if len(stats.Invitations) != 1 || stats.Invitations[0].Username != referred.Username ||
		len(stats.Rewards) != 1 || stats.Rewards[0].RewardMicros != 500_000 || stats.Rewards[0].PaidAmountMicros != created.AmountMicros {
		t.Fatalf("referral details=%+v", stats)
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
