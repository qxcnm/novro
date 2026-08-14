package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
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
	entgatewayoperation "github.com/novro-gateway/novro/ent/gatewayoperation"
	"github.com/novro-gateway/novro/ent/migrate"
	entuser "github.com/novro-gateway/novro/ent/user"
	entwalletentry "github.com/novro-gateway/novro/ent/walletentry"
	"github.com/novro-gateway/novro/internal/payment"
	"github.com/novro-gateway/novro/internal/referral"
)

const mysqlIntegrationDSNEnv = "NOVRO_TEST_MYSQL_DSN"

/**
 * TestMySQLConcurrentReservationsPreserveBalance 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestMySQLUsageAccountingRetriesAreIdempotent 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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
	providerEntity, err := client.Provider.Create().SetCode("integration-provider").SetDisplayName("Integration Provider").SetProtocol("openai").SetBaseURL("https://api.example.com").SetEncryptedAPIKey("encrypted").SetAPIKeyHint("hint").Save(ctx)
	if err != nil {
		t.Fatalf("create integration provider: %v", err)
	}
	upstream, err := client.UpstreamModel.Create().SetProviderName("Integration Provider").SetUpstreamName("upstream-model").SetDisplayName("Upstream Model").SetRequestPriceMicros(300_000).Save(ctx)
	if err != nil {
		t.Fatalf("create integration upstream model: %v", err)
	}
	route, err := client.ModelRoute.Create().SetProviderID(providerEntity.ID).SetUpstreamModelID(upstream.ID).SetBillingGroupID(group.ID).SetPublicName("integration-model").SetDisplayName("Integration Model").SetUpstreamName("upstream-model").SetInputPriceMicros(0).SetOutputPriceMicros(0).Save(ctx)
	if err != nil {
		t.Fatalf("create integration model route: %v", err)
	}

	service := NewService(NewEntStore(client))
	requestID := uuid.New()
	operationInput := OperationStartInput{RequestID: requestID, UserID: createdUser.ID, APIKeyID: apiKey.ID, IdempotencyKeyHash: strings.Repeat("b", 64), RequestHash: strings.Repeat("c", 64), Endpoint: "chat_completions", ReservedMicros: 400_000}
	started, err := service.StartOperation(ctx, operationInput)
	if err != nil || !started.Created {
		t.Fatalf("start operation: result=%+v err=%v", started, err)
	}
	retry, err := service.StartOperation(ctx, operationInput)
	if err != nil || retry.Created || retry.Operation.RequestID != requestID {
		t.Fatalf("retry operation: result=%+v err=%v", retry, err)
	}
	conflictingStart := operationInput
	conflictingStart.RequestID = uuid.New()
	conflictingStart.RequestHash = strings.Repeat("d", 64)
	if _, err := service.StartOperation(ctx, conflictingStart); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("conflicting operation err=%v", err)
	}

	usage := UsageInput{UserID: createdUser.ID, APIKeyID: apiKey.ID, ModelRouteID: route.ID, UpstreamModelID: &upstream.ID, BillingGroupID: &group.ID, RequestID: requestID, Endpoint: "chat_completions", InputTokens: 10, Tokens: TokenBreakdown{UncachedInput: 10, Output: 20}, OutputTokens: 20, Rates: RateCard{RequestMicros: 300_000}, BaseCostMicros: 300_000, MultiplierBPS: 10_000, CostMicros: 300_000, ReservedMicros: 400_000, ModelName: "integration-model", UpstreamModelName: "upstream-model", BillingGroupCode: group.Code, BillingGroupName: group.DisplayName, CalculationVersion: CalculationVersion, CreatedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()}
	if err := service.MarkOperationPendingSettlement(ctx, requestID, usage); err != nil {
		t.Fatalf("persist pending usage: %v", err)
	}
	if err := service.Finalize(ctx, usage); err != nil {
		t.Fatalf("finalize usage: %v", err)
	}
	if err := service.CompleteOperation(ctx, requestID); err != nil {
		t.Fatalf("complete operation: %v", err)
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
	orphanedStart := OperationStartInput{RequestID: orphanedRequestID, UserID: createdUser.ID, APIKeyID: apiKey.ID, IdempotencyKeyHash: strings.Repeat("e", 64), RequestHash: strings.Repeat("f", 64), Endpoint: "chat_completions", ReservedMicros: 50_000}
	if _, err := service.StartOperation(ctx, orphanedStart); err != nil {
		t.Fatalf("start orphaned request: %v", err)
	}
	reservedSummary, err := service.Summary(ctx, createdUser.ID)
	if err != nil {
		t.Fatalf("summarize orphaned request: %v", err)
	}
	if reservedSummary.ReservedMicros != 50_000 {
		t.Fatalf("active reservation=%d want=50000", reservedSummary.ReservedMicros)
	}
	if err := service.FailOperation(ctx, orphanedRequestID, "upstream_failed"); err != nil {
		t.Fatalf("fail orphaned request: %v", err)
	}
	if err := service.FailOperation(ctx, orphanedRequestID, "upstream_failed"); err != nil {
		t.Fatalf("retry failed request: %v", err)
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

	missingReservationUsage := usage
	missingReservationUsage.RequestID = uuid.New()
	if err := service.Finalize(ctx, missingReservationUsage); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("finalize without reservation err=%v", err)
	}

	mismatchedReservationRequestID := uuid.New()
	mismatchedStart := OperationStartInput{RequestID: mismatchedReservationRequestID, UserID: createdUser.ID, APIKeyID: apiKey.ID, IdempotencyKeyHash: strings.Repeat("1", 64), RequestHash: strings.Repeat("2", 64), Endpoint: "chat_completions", ReservedMicros: 60_000}
	if _, err := service.StartOperation(ctx, mismatchedStart); err != nil {
		t.Fatalf("start mismatched request: %v", err)
	}
	mismatchedReservationUsage := usage
	mismatchedReservationUsage.RequestID = mismatchedReservationRequestID
	mismatchedReservationUsage.ReservedMicros = 50_000
	if err := service.Finalize(ctx, mismatchedReservationUsage); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("finalize with mismatched reservation err=%v", err)
	}
	if err := service.FailOperation(ctx, mismatchedReservationRequestID, "test_cleanup"); err != nil {
		t.Fatalf("cleanup mismatched operation: %v", err)
	}

	overageRequestID := uuid.New()
	overageStart := OperationStartInput{RequestID: overageRequestID, UserID: createdUser.ID, APIKeyID: apiKey.ID, IdempotencyKeyHash: strings.Repeat("3", 64), RequestHash: strings.Repeat("4", 64), Endpoint: "chat_completions", ReservedMicros: 50_000}
	if _, err := service.StartOperation(ctx, overageStart); err != nil {
		t.Fatalf("start overage: %v", err)
	}
	overageUsage := usage
	overageUsage.RequestID = overageRequestID
	overageUsage.Rates.RequestMicros = 75_000
	overageUsage.BaseCostMicros = 75_000
	overageUsage.CostMicros = 75_000
	overageUsage.ReservedMicros = 50_000
	if err := service.MarkOperationPendingSettlement(ctx, overageRequestID, overageUsage); err != nil {
		t.Fatalf("persist overage settlement: %v", err)
	}
	if err := service.Finalize(ctx, overageUsage); err != nil {
		t.Fatalf("finalize overage: %v", err)
	}
	if err := service.Finalize(ctx, overageUsage); err != nil {
		t.Fatalf("retry overage finalization: %v", err)
	}
	if err := service.CompleteOperation(ctx, overageRequestID); err != nil {
		t.Fatalf("complete overage: %v", err)
	}
	settlementEntries, err := client.WalletEntry.Query().Where(
		entwalletentry.ReferenceIDEQ(overageRequestID),
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeUsageSettlement),
	).All(ctx)
	if err != nil {
		t.Fatalf("list overage settlements: %v", err)
	}
	if len(settlementEntries) != 1 || settlementEntries[0].AmountMicros != -25_000 {
		t.Fatalf("overage settlements=%+v", settlementEntries)
	}
	overageSummary, err := service.Summary(ctx, createdUser.ID)
	if err != nil {
		t.Fatalf("summarize overage request: %v", err)
	}
	if overageSummary.Wallet.BalanceMicros != initialBalance-375_000 {
		t.Fatalf("overage balance=%d want=%d", overageSummary.Wallet.BalanceMicros, initialBalance-375_000)
	}
	completed, err := client.GatewayOperation.Query().Where(entgatewayoperation.IDEQ(overageRequestID), entgatewayoperation.StatusEQ(entgatewayoperation.StatusCompleted)).Count(ctx)
	if err != nil || completed != 1 {
		t.Fatalf("completed overage operation=%d err=%v", completed, err)
	}
}

/**
 * TestMySQLConcurrentGatewayBillingTransitionsAreIdempotent 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestMySQLConcurrentGatewayBillingTransitionsAreIdempotent(t *testing.T) {
	client := openMySQLIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	createdUser, err := client.User.Create().
		SetUsername("gateway-races-" + strings.ReplaceAll(uuid.NewString(), "-", "")).
		SetDisplayName("Gateway Race Test").SetRole(entuser.RoleMember).SetStatus(entuser.StatusActive).
		Save(ctx)
	if err != nil {
		t.Fatalf("create gateway race user: %v", err)
	}
	const initialBalance int64 = 5_000_000
	wallet, err := client.Wallet.Create().SetUserID(createdUser.ID).SetBalanceMicros(initialBalance).Save(ctx)
	if err != nil {
		t.Fatalf("create gateway race wallet: %v", err)
	}
	group, err := client.BillingGroup.Query().Where(entbillinggroup.IsDefaultEQ(true)).Only(ctx)
	if err != nil {
		t.Fatalf("read gateway race billing group: %v", err)
	}
	apiKey, err := client.APIKey.Create().SetUserID(createdUser.ID).SetBillingGroupID(group.ID).SetName("gateway-race").SetKeyPrefix("nvr_race").SetKeyHash(strings.Repeat("6", 64)).Save(ctx)
	if err != nil {
		t.Fatalf("create gateway race API key: %v", err)
	}
	providerEntity, err := client.Provider.Create().SetCode("gateway-race-provider").SetDisplayName("Gateway Race Provider").SetProtocol("openai").SetBaseURL("https://api.example.com").SetEncryptedAPIKey("encrypted").SetAPIKeyHint("hint").Save(ctx)
	if err != nil {
		t.Fatalf("create gateway race provider: %v", err)
	}
	upstream, err := client.UpstreamModel.Create().SetProviderName("Gateway Race Provider").SetUpstreamName("gateway-race-model").SetDisplayName("Gateway Race Model").SetRequestPriceMicros(50_000).Save(ctx)
	if err != nil {
		t.Fatalf("create gateway race upstream model: %v", err)
	}
	route, err := client.ModelRoute.Create().SetProviderID(providerEntity.ID).SetUpstreamModelID(upstream.ID).SetBillingGroupID(group.ID).SetPublicName("gateway-race-model").SetDisplayName("Gateway Race Model").SetUpstreamName("gateway-race-model").SetInputPriceMicros(0).SetOutputPriceMicros(0).Save(ctx)
	if err != nil {
		t.Fatalf("create gateway race route: %v", err)
	}
	service := NewService(NewEntStore(client))

	startInput := OperationStartInput{
		RequestID: uuid.New(), UserID: createdUser.ID, APIKeyID: apiKey.ID,
		IdempotencyKeyHash: strings.Repeat("7", 64), RequestHash: strings.Repeat("8", 64), Endpoint: "chat_completions", ReservedMicros: 100_000,
	}
	type startResult struct {
		result OperationStartResult
		err    error
	}
	startGate := make(chan struct{})
	startResults := make(chan startResult, 8)
	for range 8 {
		go func() {
			<-startGate
			concurrentInput := startInput
			concurrentInput.RequestID = uuid.New()
			result, startErr := service.StartOperation(context.Background(), concurrentInput)
			startResults <- startResult{result: result, err: startErr}
		}()
	}
	close(startGate)
	createdCount := 0
	returnedRequestIDs := make(map[uuid.UUID]struct{})
	for range 8 {
		result := <-startResults
		if result.err != nil {
			t.Fatalf("concurrent start operation: %v", result.err)
		}
		returnedRequestIDs[result.result.Operation.RequestID] = struct{}{}
		if result.result.Created {
			createdCount++
		}
	}
	if createdCount != 1 || len(returnedRequestIDs) != 1 {
		t.Fatalf("concurrent start created=%d request_ids=%v", createdCount, returnedRequestIDs)
	}
	var winningRequestID uuid.UUID
	for requestID := range returnedRequestIDs {
		winningRequestID = requestID
	}
	reservationCount, err := client.WalletEntry.Query().Where(
		entwalletentry.WalletIDEQ(wallet.ID),
		entwalletentry.ReferenceIDEQ(winningRequestID),
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeUsageReservation),
	).Count(ctx)
	if err != nil || reservationCount != 1 {
		t.Fatalf("concurrent start reservations=%d err=%v", reservationCount, err)
	}
	if err := service.FailOperation(ctx, winningRequestID, "test_cleanup"); err != nil {
		t.Fatalf("release concurrent start reservation: %v", err)
	}

	transitionRequestID := uuid.New()
	transitionStart := OperationStartInput{
		RequestID: transitionRequestID, UserID: createdUser.ID, APIKeyID: apiKey.ID,
		IdempotencyKeyHash: strings.Repeat("9", 64), RequestHash: strings.Repeat("a", 64), Endpoint: "chat_completions", ReservedMicros: 100_000,
	}
	if _, err := service.StartOperation(ctx, transitionStart); err != nil {
		t.Fatalf("start competing transition: %v", err)
	}
	transitionUsage := UsageInput{
		UserID: createdUser.ID, APIKeyID: apiKey.ID, ModelRouteID: route.ID, UpstreamModelID: &upstream.ID, BillingGroupID: &group.ID,
		RequestID: transitionRequestID, Endpoint: "chat_completions", InputTokens: 1, Tokens: TokenBreakdown{UncachedInput: 1}, Rates: RateCard{RequestMicros: 50_000},
		BaseCostMicros: 50_000, MultiplierBPS: 10_000, CostMicros: 50_000, ReservedMicros: 100_000, ModelName: route.PublicName,
		UpstreamModelName: upstream.UpstreamName, BillingGroupCode: group.Code, BillingGroupName: group.DisplayName, CalculationVersion: CalculationVersion,
		CreatedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(),
	}
	transitionGate := make(chan struct{})
	transitionResults := make(chan error, 2)
	go func() {
		<-transitionGate
		transitionResults <- service.MarkOperationPendingSettlement(context.Background(), transitionRequestID, transitionUsage)
	}()
	go func() {
		<-transitionGate
		transitionResults <- service.FailOperation(context.Background(), transitionRequestID, "upstream_failed")
	}()
	close(transitionGate)
	assertOneSuccessOneConflict(t, <-transitionResults, <-transitionResults)
	transitionOperation, err := client.GatewayOperation.Get(ctx, transitionRequestID)
	if err != nil {
		t.Fatalf("read competing transition: %v", err)
	}
	switch transitionOperation.Status {
	case entgatewayoperation.StatusPendingSettlement:
		if err := service.Finalize(ctx, transitionUsage); err != nil {
			t.Fatalf("finalize winning settlement: %v", err)
		}
		if err := service.CompleteOperation(ctx, transitionRequestID); err != nil {
			t.Fatalf("complete winning settlement: %v", err)
		}
	case entgatewayoperation.StatusFailed:
		if err := service.Finalize(ctx, transitionUsage); !errors.Is(err, ErrRequestConflict) {
			t.Fatalf("finalize after winning failure err=%v", err)
		}
	default:
		t.Fatalf("competing transition status=%s", transitionOperation.Status)
	}

	unknownRequestID := uuid.New()
	unknownStart := OperationStartInput{
		RequestID: unknownRequestID, UserID: createdUser.ID, APIKeyID: apiKey.ID,
		IdempotencyKeyHash: strings.Repeat("b", 64), RequestHash: strings.Repeat("c", 64), Endpoint: "chat_completions", ReservedMicros: 100_000,
	}
	if _, err := service.StartOperation(ctx, unknownStart); err != nil {
		t.Fatalf("start uncertain transition: %v", err)
	}
	unknownUsage := transitionUsage
	unknownUsage.RequestID = unknownRequestID
	unknownGate := make(chan struct{})
	unknownResults := make(chan error, 2)
	go func() {
		<-unknownGate
		unknownResults <- service.MarkOperationPendingSettlement(context.Background(), unknownRequestID, unknownUsage)
	}()
	go func() {
		<-unknownGate
		unknownResults <- service.MarkOperationPendingUnknown(context.Background(), unknownRequestID, "upstream_result_unknown")
	}()
	close(unknownGate)
	assertOneSuccessOneConflict(t, <-unknownResults, <-unknownResults)
	unknownOperation, err := client.GatewayOperation.Get(ctx, unknownRequestID)
	if err != nil {
		t.Fatalf("read uncertain transition: %v", err)
	}
	switch unknownOperation.Status {
	case entgatewayoperation.StatusPendingSettlement:
		if err := service.Finalize(ctx, unknownUsage); err != nil {
			t.Fatalf("finalize settlement after unknown race: %v", err)
		}
		if err := service.CompleteOperation(ctx, unknownRequestID); err != nil {
			t.Fatalf("complete settlement after unknown race: %v", err)
		}
	case entgatewayoperation.StatusPendingUnknown:
		if unknownOperation.FailureCode != "upstream_result_unknown" {
			t.Fatalf("pending unknown reason=%q", unknownOperation.FailureCode)
		}
	default:
		t.Fatalf("uncertain transition status=%s", unknownOperation.Status)
	}

	recoveryRequestID := uuid.New()
	recoveryStart := OperationStartInput{
		RequestID: recoveryRequestID, UserID: createdUser.ID, APIKeyID: apiKey.ID,
		IdempotencyKeyHash: strings.Repeat("d", 64), RequestHash: strings.Repeat("e", 64), Endpoint: "chat_completions", ReservedMicros: 50_000,
	}
	if _, err := service.StartOperation(ctx, recoveryStart); err != nil {
		t.Fatalf("start recovery operation: %v", err)
	}
	recoveryUsage := transitionUsage
	recoveryUsage.RequestID = recoveryRequestID
	recoveryUsage.ReservedMicros = 50_000
	recoveryUsage.Rates.RequestMicros = 75_000
	recoveryUsage.BaseCostMicros = 75_000
	recoveryUsage.CostMicros = 75_000
	if err := service.MarkOperationPendingSettlement(ctx, recoveryRequestID, recoveryUsage); err != nil {
		t.Fatalf("persist recovery settlement: %v", err)
	}
	recoveryGate := make(chan struct{})
	recoveryResults := make(chan error, 4)
	for range 4 {
		go func() {
			<-recoveryGate
			_, recoveryErr := service.RecoverPendingSettlements(context.Background(), 100)
			recoveryResults <- recoveryErr
		}()
	}
	close(recoveryGate)
	for range 4 {
		if err := <-recoveryResults; err != nil {
			t.Fatalf("concurrent settlement recovery: %v", err)
		}
	}
	recoveredOperation, err := client.GatewayOperation.Get(ctx, recoveryRequestID)
	if err != nil || recoveredOperation.Status != entgatewayoperation.StatusCompleted {
		t.Fatalf("recovered operation status=%v err=%v", recoveredOperation, err)
	}
	settlementCount, err := client.WalletEntry.Query().Where(
		entwalletentry.WalletIDEQ(wallet.ID),
		entwalletentry.ReferenceIDEQ(recoveryRequestID),
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeUsageSettlement),
	).Count(ctx)
	if err != nil || settlementCount != 1 {
		t.Fatalf("recovery settlement entries=%d err=%v", settlementCount, err)
	}
	usageCount, err := client.APIUsage.Query().Where(entapiusage.RequestIDEQ(recoveryRequestID)).Count(ctx)
	if err != nil || usageCount != 1 {
		t.Fatalf("recovered usage rows=%d err=%v", usageCount, err)
	}

	adjustmentReferenceID := uuid.New()
	adjustmentGate := make(chan struct{})
	adjustmentResults := make(chan error, 8)
	beforeAdjustment, err := client.Wallet.Get(ctx, wallet.ID)
	if err != nil {
		t.Fatalf("read wallet before concurrent adjustment: %v", err)
	}
	for range 8 {
		go func() {
			<-adjustmentGate
			_, adjustmentErr := service.Adjust(context.Background(), createdUser.ID, createdUser.ID, adjustmentReferenceID, 25_000, "并发幂等调整")
			adjustmentResults <- adjustmentErr
		}()
	}
	close(adjustmentGate)
	for range 8 {
		if err := <-adjustmentResults; err != nil {
			t.Fatalf("concurrent balance adjustment: %v", err)
		}
	}
	afterAdjustment, err := client.Wallet.Get(ctx, wallet.ID)
	if err != nil || afterAdjustment.BalanceMicros != beforeAdjustment.BalanceMicros+25_000 {
		t.Fatalf("adjusted balance=%v before=%v err=%v", afterAdjustment, beforeAdjustment, err)
	}
	adjustmentCount, err := client.WalletEntry.Query().Where(
		entwalletentry.WalletIDEQ(wallet.ID),
		entwalletentry.ReferenceIDEQ(adjustmentReferenceID),
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeManualAdjustment),
	).Count(ctx)
	if err != nil || adjustmentCount != 1 {
		t.Fatalf("manual adjustment entries=%d err=%v", adjustmentCount, err)
	}

	legacyRequestID := uuid.New()
	if _, err := client.APIUsage.Create().SetUserID(createdUser.ID).SetAPIKeyID(apiKey.ID).SetModelRouteID(route.ID).SetUpstreamModelID(upstream.ID).SetBillingGroupID(group.ID).
		SetRequestID(legacyRequestID).SetEndpoint(entapiusage.EndpointChatCompletions).SetStatusCode(http.StatusOK).SetInputTokens(1).SetUncachedInputTokens(1).
		SetCostMicros(30_000).SetReservedMicros(30_000).SetEstimated(true).SetModelName(route.PublicName).SetUpstreamModelName(upstream.UpstreamName).
		SetBillingGroupCode(group.Code).SetBillingGroupName(group.DisplayName).SetCalculationVersion("token-v2").Save(ctx); err != nil {
		t.Fatalf("create legacy usage for compensation: %v", err)
	}
	beforeCompensation, err := client.Wallet.Get(ctx, wallet.ID)
	if err != nil {
		t.Fatalf("read wallet before compensation: %v", err)
	}
	compensationGate := make(chan struct{})
	compensationResults := make(chan error, 8)
	for range 8 {
		go func() {
			<-compensationGate
			_, amount, compensationErr := service.CompensateLegacyUsage(context.Background(), legacyRequestID, createdUser.ID)
			if compensationErr == nil && amount != 30_000 {
				compensationErr = fmt.Errorf("compensation amount=%d want=30000", amount)
			}
			compensationResults <- compensationErr
		}()
	}
	close(compensationGate)
	for range 8 {
		if err := <-compensationResults; err != nil {
			t.Fatalf("concurrent legacy compensation: %v", err)
		}
	}
	afterCompensation, err := client.Wallet.Get(ctx, wallet.ID)
	if err != nil || afterCompensation.BalanceMicros != beforeCompensation.BalanceMicros+30_000 {
		t.Fatalf("compensated balance=%v before=%v err=%v", afterCompensation, beforeCompensation, err)
	}
	compensationCount, err := client.WalletEntry.Query().Where(
		entwalletentry.WalletIDEQ(wallet.ID),
		entwalletentry.ReferenceIDEQ(legacyRequestID),
		entwalletentry.EntryTypeEQ(entwalletentry.EntryTypeUsageCompensation),
	).Count(ctx)
	if err != nil || compensationCount != 1 {
		t.Fatalf("usage compensation entries=%d err=%v", compensationCount, err)
	}
}

/**
 * assertOneSuccessOneConflict 封装该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @param first 本次操作需要使用的输入参数。
 * @param second 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func assertOneSuccessOneConflict(t *testing.T, first, second error) {
	t.Helper()
	successes, conflicts := 0, 0
	for _, err := range []error{first, second} {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrRequestConflict):
			conflicts++
		default:
			t.Fatalf("unexpected competing transition error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("competing transition successes=%d conflicts=%d", successes, conflicts)
	}
}

/**
 * TestMySQLConcurrentTopUpNotificationsCreditOnce 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * TestMySQLReferralCashbackCreditsInviterOnce 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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

/**
 * openMySQLIntegrationClient 封装该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
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
