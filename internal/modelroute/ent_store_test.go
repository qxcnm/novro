package modelroute

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
	entmodelroute "github.com/novro-gateway/novro/ent/modelroute"
	entprovider "github.com/novro-gateway/novro/ent/provider"
	entupstreammodel "github.com/novro-gateway/novro/ent/upstreammodel"
	"github.com/novro-gateway/novro/internal/billinggroup"
)

/**
 * TestEntStoreListAppliesSearchAndStatusFilters 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestEntStoreListAppliesSearchAndStatusFilters(t *testing.T) {
	client := openModelRouteIntegrationClient(t)
	ctx := context.Background()
	groupID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	activeProvider, err := client.Provider.Create().
		SetCode("active-provider").SetDisplayName("Active Provider").
		SetProtocol(entprovider.ProtocolOpenai).SetBaseURL("https://active.example.com").
		SetEncryptedAPIKey("encrypted").SetAPIKeyHint("rypt").Save(ctx)
	if err != nil {
		t.Fatalf("create active provider: %v", err)
	}
	disabledProvider, err := client.Provider.Create().
		SetCode("disabled-provider").SetDisplayName("Disabled Provider").
		SetProtocol(entprovider.ProtocolOpenai).SetBaseURL("https://disabled.example.com").
		SetEncryptedAPIKey("encrypted").SetAPIKeyHint("rypt").
		SetStatus(entprovider.StatusDisabled).Save(ctx)
	if err != nil {
		t.Fatalf("create disabled provider: %v", err)
	}
	if _, err := client.ModelRoute.Create().SetProviderID(activeProvider.ID).
		SetBillingGroupID(groupID).
		SetPublicName("alpha-chat").SetDisplayName("Alpha Chat").SetUpstreamName("alpha-upstream").
		SetInputPriceMicros(1).SetOutputPriceMicros(1).Save(ctx); err != nil {
		t.Fatalf("create active matching route: %v", err)
	}
	if _, err := client.ModelRoute.Create().SetProviderID(activeProvider.ID).
		SetBillingGroupID(groupID).
		SetPublicName("beta-chat").SetDisplayName("Beta Chat").SetUpstreamName("beta-upstream").
		SetInputPriceMicros(1).SetOutputPriceMicros(1).SetStatus(entmodelroute.StatusDisabled).Save(ctx); err != nil {
		t.Fatalf("create disabled matching route: %v", err)
	}
	if _, err := client.ModelRoute.Create().SetProviderID(disabledProvider.ID).
		SetBillingGroupID(groupID).
		SetPublicName("gamma-chat").SetDisplayName("Gamma Chat").SetUpstreamName("gamma-upstream").
		SetInputPriceMicros(1).SetOutputPriceMicros(1).Save(ctx); err != nil {
		t.Fatalf("create route with disabled provider: %v", err)
	}

	store := NewEntStore(client)
	items, err := store.List(ctx, ListFilter{Search: "beta", Status: StatusDisabled})
	if err != nil {
		t.Fatalf("list filtered model routes: %v", err)
	}
	if len(items) != 1 || items[0].PublicName != "beta-chat" || items[0].Status != StatusDisabled {
		t.Fatalf("unexpected filtered routes: %+v", items)
	}
	items, err = store.List(ctx, ListFilter{Search: "disabled-provider", Status: StatusActive})
	if err != nil {
		t.Fatalf("list provider-filtered model routes: %v", err)
	}
	if len(items) != 1 || items[0].PublicName != "gamma-chat" {
		t.Fatalf("unexpected provider-filtered routes: %+v", items)
	}
}

/**
 * TestEntStoreDeleteKeepsHistoricalRouteAndHidesItFromLists 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestEntStoreDeleteKeepsHistoricalRouteAndHidesItFromLists(t *testing.T) {
	client := openModelRouteIntegrationClient(t)
	ctx := context.Background()
	groupID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	configured, err := client.Provider.Create().
		SetCode("soft-delete-provider").SetDisplayName("Soft Delete Provider").
		SetProtocol(entprovider.ProtocolOpenai).SetBaseURL("https://soft-delete.example.com").
		SetEncryptedAPIKey("encrypted").SetAPIKeyHint("rypt").Save(ctx)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	route, err := client.ModelRoute.Create().SetProviderID(configured.ID).
		SetBillingGroupID(groupID).
		SetPublicName("soft-delete-chat").SetDisplayName("Soft Delete Chat").SetUpstreamName("soft-delete-upstream").
		SetInputPriceMicros(1).SetOutputPriceMicros(1).Save(ctx)
	if err != nil {
		t.Fatalf("create route: %v", err)
	}
	store := NewEntStore(client)
	if err := store.Delete(ctx, route.ID); err != nil {
		t.Fatalf("soft delete route: %v", err)
	}
	preserved, err := client.ModelRoute.Get(ctx, route.ID)
	if err != nil {
		t.Fatalf("read preserved route: %v", err)
	}
	if preserved.DeletedAt == nil || preserved.Status != entmodelroute.StatusDisabled {
		t.Fatalf("route was not soft deleted: %+v", preserved)
	}
	listed, err := store.List(ctx, ListFilter{Search: "soft-delete-chat"})
	if err != nil {
		t.Fatalf("list routes: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("soft-deleted route remained visible: %+v", listed)
	}
}

/**
 * TestEntStoreResolvesOrderedCandidatesForOnePublicModel 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestEntStoreResolvesOrderedCandidatesForOnePublicModel(t *testing.T) {
	client := openModelRouteIntegrationClient(t)
	ctx := context.Background()
	groupID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	firstProvider, err := client.Provider.Create().
		SetCode("candidate-first").SetDisplayName("Candidate First").
		SetProtocol(entprovider.ProtocolOpenai).SetBaseURL("https://first.example.com").
		SetEncryptedAPIKey("first-encrypted").SetAPIKeyHint("rypt").Save(ctx)
	if err != nil {
		t.Fatalf("create first provider: %v", err)
	}
	secondProvider, err := client.Provider.Create().
		SetCode("candidate-second").SetDisplayName("Candidate Second").
		SetProtocol(entprovider.ProtocolOpenai).SetBaseURL("https://second.example.com").
		SetEncryptedAPIKey("second-encrypted").SetAPIKeyHint("rypt").Save(ctx)
	if err != nil {
		t.Fatalf("create second provider: %v", err)
	}
	sharedModel, err := client.UpstreamModel.Create().SetProviderName("Shared Catalog").SetUpstreamName("shared-chat").SetDisplayName("Shared Chat").SetStatus(entupstreammodel.StatusActive).Save(ctx)
	if err != nil {
		t.Fatalf("create shared upstream model: %v", err)
	}
	createdAt := time.Date(2026, 8, 7, 1, 0, 0, 0, time.UTC)
	if _, err := client.ModelRoute.Create().SetProviderID(secondProvider.ID).SetUpstreamModelID(sharedModel.ID).
		SetBillingGroupID(groupID).
		SetPublicName("shared-chat").SetDisplayName("Shared Chat").SetUpstreamName(sharedModel.UpstreamName).
		SetInputPriceMicros(1).SetOutputPriceMicros(1).SetCreatedAt(createdAt.Add(time.Minute)).Save(ctx); err != nil {
		t.Fatalf("create second candidate: %v", err)
	}
	if _, err := client.ModelRoute.Create().SetProviderID(firstProvider.ID).SetUpstreamModelID(sharedModel.ID).
		SetBillingGroupID(groupID).
		SetPublicName("shared-chat").SetDisplayName("Shared Chat").SetUpstreamName(sharedModel.UpstreamName).
		SetInputPriceMicros(1).SetOutputPriceMicros(1).SetCreatedAt(createdAt).Save(ctx); err != nil {
		t.Fatalf("create first candidate: %v", err)
	}

	resolved, err := NewEntStore(client).ResolveCandidates(ctx, "shared-chat", groupID)
	if err != nil {
		t.Fatalf("resolve candidates: %v", err)
	}
	if len(resolved) != 2 || resolved[0].Record.Provider.Code != "candidate-first" || resolved[0].EncryptedAPIKey != "first-encrypted" || resolved[1].Record.Provider.Code != "candidate-second" {
		t.Fatalf("unexpected candidates: %+v", resolved)
	}
	if _, err := client.ModelRoute.Create().SetProviderID(firstProvider.ID).SetUpstreamModelID(sharedModel.ID).
		SetBillingGroupID(groupID).
		SetPublicName("shared-chat").SetDisplayName("Duplicate").SetUpstreamName(sharedModel.UpstreamName).
		SetInputPriceMicros(1).SetOutputPriceMicros(1).Save(ctx); !ent.IsConstraintError(err) {
		t.Fatalf("duplicate route constraint error=%v", err)
	}
}

/**
 * TestEntStoreFiltersActiveRoutesByBillingGroup 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestEntStoreFiltersActiveRoutesByBillingGroup(t *testing.T) {
	client := openModelRouteIntegrationClient(t)
	ctx := context.Background()
	groupAID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	groupB, err := client.BillingGroup.Create().
		SetCode("group-b").SetDisplayName("Group B").SetMultiplierBps(10_000).
		Save(ctx)
	if err != nil {
		t.Fatalf("create second billing group: %v", err)
	}
	sharedProvider, err := client.Provider.Create().
		SetCode("shared-group-provider").SetDisplayName("Shared Group Provider").
		SetProtocol(entprovider.ProtocolOpenai).SetBaseURL("https://shared-groups.example.com").
		SetEncryptedAPIKey("shared-group-encrypted").SetAPIKeyHint("rypt").Save(ctx)
	if err != nil {
		t.Fatalf("create shared provider: %v", err)
	}
	sharedModel, err := client.UpstreamModel.Create().
		SetProviderName("Shared Catalog").SetUpstreamName("grouped-shared-chat").SetDisplayName("Grouped Shared Chat").
		SetPricingConfigured(true).SetStatus(entupstreammodel.StatusActive).Save(ctx)
	if err != nil {
		t.Fatalf("create shared upstream model: %v", err)
	}
	for _, groupID := range []uuid.UUID{groupAID, groupB.ID} {
		if _, err := client.ModelRoute.Create().SetProviderID(sharedProvider.ID).SetUpstreamModelID(sharedModel.ID).
			SetBillingGroupID(groupID).
			SetPublicName("grouped-shared-chat").SetDisplayName("Grouped Shared Chat").SetUpstreamName(sharedModel.UpstreamName).
			SetInputPriceMicros(1).SetOutputPriceMicros(1).Save(ctx); err != nil {
			t.Fatalf("create grouped route for %s: %v", groupID, err)
		}
	}

	store := NewEntStore(client)
	groupAResolved, err := store.ResolveCandidates(ctx, "grouped-shared-chat", groupAID)
	if err != nil {
		t.Fatalf("resolve group A candidates: %v", err)
	}
	if len(groupAResolved) != 1 || groupAResolved[0].Record.Provider.Code != "shared-group-provider" || groupAResolved[0].Record.BillingGroupID != groupAID {
		t.Fatalf("unexpected group A candidates: %+v", groupAResolved)
	}
	groupBResolved, err := store.ResolveCandidates(ctx, "grouped-shared-chat", groupB.ID)
	if err != nil {
		t.Fatalf("resolve group B candidates: %v", err)
	}
	if len(groupBResolved) != 1 || groupBResolved[0].Record.Provider.Code != "shared-group-provider" || groupBResolved[0].Record.BillingGroupID != groupB.ID {
		t.Fatalf("unexpected group B candidates: %+v", groupBResolved)
	}
	groupAActive, err := store.ListActive(ctx, groupAID)
	if err != nil {
		t.Fatalf("list group A active routes: %v", err)
	}
	for _, route := range groupAActive {
		if route.PublicName == "grouped-shared-chat" && (route.Provider.Code != "shared-group-provider" || route.BillingGroupID != groupAID) {
			t.Fatalf("group A list exposed cross-group route: %+v", route)
		}
	}
	groupBActive, err := store.ListActive(ctx, groupB.ID)
	if err != nil {
		t.Fatalf("list group B active routes: %v", err)
	}
	for _, route := range groupBActive {
		if route.PublicName == "grouped-shared-chat" && (route.Provider.Code != "shared-group-provider" || route.BillingGroupID != groupB.ID) {
			t.Fatalf("group B list exposed cross-group route: %+v", route)
		}
	}
	if err := billinggroup.NewEntStore(client).Delete(ctx, groupB.ID); !errors.Is(err, billinggroup.ErrInUse) {
		t.Fatalf("delete billing group referenced by model route error=%v", err)
	}
}

/**
 * TestEntStoreRequiresCatalogPricingForActiveRoute 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestEntStoreRequiresCatalogPricingForActiveRoute(t *testing.T) {
	client := openModelRouteIntegrationClient(t)
	ctx := context.Background()
	groupID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	configured, err := client.Provider.Create().
		SetCode("unpriced-provider").SetDisplayName("Unpriced Provider").
		SetProtocol(entprovider.ProtocolOpenai).SetBaseURL("https://unpriced.example.com").
		SetEncryptedAPIKey("encrypted").SetAPIKeyHint("rypt").Save(ctx)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	model, err := client.UpstreamModel.Create().
		SetProviderName("Unpriced Catalog").SetUpstreamName("unpriced-chat").SetDisplayName("Unpriced Chat").
		SetPricingConfigured(false).SetStatus(entupstreammodel.StatusDisabled).Save(ctx)
	if err != nil {
		t.Fatalf("create unpriced model: %v", err)
	}
	route, err := client.ModelRoute.Create().SetProviderID(configured.ID).SetUpstreamModelID(model.ID).
		SetBillingGroupID(groupID).
		SetPublicName("unpriced-chat").SetDisplayName("Unpriced Chat").SetUpstreamName(model.UpstreamName).
		SetInputPriceMicros(0).SetOutputPriceMicros(0).SetStatus(entmodelroute.StatusDisabled).Save(ctx)
	if err != nil {
		t.Fatalf("create unpriced route: %v", err)
	}
	store := NewEntStore(client)
	if _, err := store.SetStatus(ctx, route.ID, StatusActive); !errors.Is(err, ErrPricingRequired) {
		t.Fatalf("activate unpriced route error=%v", err)
	}
	if _, err := client.ModelRoute.UpdateOneID(route.ID).SetStatus(entmodelroute.StatusActive).Save(ctx); err != nil {
		t.Fatalf("prepare active unpriced route: %v", err)
	}
	active, err := store.ListActive(ctx, groupID)
	if err != nil {
		t.Fatalf("list active routes: %v", err)
	}
	for _, item := range active {
		if item.PublicName == "unpriced-chat" {
			t.Fatalf("unpriced route was exposed as active: %+v", item)
		}
	}
}

/**
 * openModelRouteIntegrationClient 封装该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func openModelRouteIntegrationClient(t *testing.T) *ent.Client {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("NOVRO_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("set NOVRO_TEST_MYSQL_DSN to run the MySQL model-route integration test")
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
		t.Fatalf("connect to isolated MySQL integration server: %v", err)
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
