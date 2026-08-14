package providersync

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entbillinggroup "github.com/novro-gateway/novro/ent/billinggroup"
	"github.com/novro-gateway/novro/ent/migrate"
	entmodelroute "github.com/novro-gateway/novro/ent/modelroute"
	entprovider "github.com/novro-gateway/novro/ent/provider"
	entupstreammodel "github.com/novro-gateway/novro/ent/upstreammodel"
	"github.com/novro-gateway/novro/internal/provider"
)

/**
 * TestSyncAndLinkReuseOneGlobalModelAcrossProviders 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestSyncAndLinkReuseOneGlobalModelAcrossProviders(t *testing.T) {
	client := openProviderSyncIntegrationClient(t)
	ctx := context.Background()
	cipher, err := provider.NewCipher("01234567890123456789012345678901")
	if err != nil {
		t.Fatalf("create provider cipher: %v", err)
	}
	encrypted, err := cipher.Encrypt("provider-secret")
	if err != nil {
		t.Fatalf("encrypt provider credential: %v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"shared-kimi-test","display_name":"Shared Kimi Test","pricing":{"unit":"rmb_per_million_tokens","input":1,"output":2}}]}`))
	}))
	defer server.Close()
	group, err := client.BillingGroup.Query().Where(entbillinggroup.IsDefaultEQ(true)).Only(ctx)
	if err != nil {
		t.Fatalf("read default billing group: %v", err)
	}

	createProvider := func(code string) *ent.Provider {
		t.Helper()
		created, err := client.Provider.Create().
			SetCode(code).
			SetDisplayName(strings.ToUpper(code)).
			SetProtocol(entprovider.ProtocolOpenai).
			SetBaseURL(server.URL + "/v1").
			SetEncryptedAPIKey(encrypted).
			SetAPIKeyHint("cret").
			Save(ctx)
		if err != nil {
			t.Fatalf("create provider %s: %v", code, err)
		}
		return created
	}
	firstProvider := createProvider("sync-first")
	secondProvider := createProvider("sync-second")
	service := NewService(client, cipher, server.Client())

	firstSync, err := service.Sync(ctx, firstProvider.ID)
	if err != nil || len(firstSync) != 1 || !firstSync[0].Added || firstSync[0].PricingConfigured || firstSync[0].Status != string(entupstreammodel.StatusDisabled) {
		t.Fatalf("first sync=%+v err=%v", firstSync, err)
	}
	modelID := firstSync[0].ID
	if _, err := client.UpstreamModel.UpdateOneID(modelID).
		SetInputPriceMicros(20_000_000).
		SetOutputPriceMicros(100_000_000).
		SetPricingConfigured(true).
		SetStatus(entupstreammodel.StatusActive).
		Save(ctx); err != nil {
		t.Fatalf("price shared model: %v", err)
	}

	secondSync, err := service.Sync(ctx, secondProvider.ID)
	if err != nil || len(secondSync) != 1 || secondSync[0].Added || secondSync[0].ID != modelID || !secondSync[0].PricingConfigured || secondSync[0].Status != string(entupstreammodel.StatusActive) {
		t.Fatalf("second sync=%+v err=%v", secondSync, err)
	}
	models, err := client.UpstreamModel.Query().Where(entupstreammodel.UpstreamNameEqualFold("shared-kimi-test")).All(ctx)
	if err != nil || len(models) != 1 || models[0].InputPriceMicros != 20_000_000 || models[0].OutputPriceMicros != 100_000_000 {
		t.Fatalf("global models=%+v err=%v", models, err)
	}

	defaultBinding := ModelBinding{UpstreamModelID: modelID, BillingGroupID: group.ID}
	for _, configured := range []*ent.Provider{firstProvider, secondProvider} {
		result, err := service.Link(ctx, configured.ID, []ModelBinding{defaultBinding})
		if err != nil || result.Created != 1 || result.Existing != 0 || result.Disabled != 0 {
			t.Fatalf("link provider %s result=%+v err=%v", configured.Code, result, err)
		}
	}
	routes, err := client.ModelRoute.Query().Where(
		entmodelroute.PublicNameEQ("shared-kimi-test"),
		entmodelroute.UpstreamModelIDEQ(modelID),
		entmodelroute.StatusEQ(entmodelroute.StatusActive),
		entmodelroute.DeletedAtIsNil(),
	).All(ctx)
	if err != nil || len(routes) != 2 {
		t.Fatalf("shared routes=%+v err=%v", routes, err)
	}
	var firstProviderRoute *ent.ModelRoute
	for _, route := range routes {
		if route.ProviderID == firstProvider.ID {
			firstProviderRoute = route
			break
		}
	}
	if firstProviderRoute == nil {
		t.Fatalf("missing route for first provider %s", firstProvider.ID)
	}
	if _, err := client.ModelRoute.UpdateOneID(firstProviderRoute.ID).SetStatus(entmodelroute.StatusDisabled).Save(ctx); err != nil {
		t.Fatalf("disable route for reenable check: %v", err)
	}
	reenabled, err := service.Link(ctx, firstProvider.ID, []ModelBinding{defaultBinding})
	if err != nil || reenabled.Reenabled != 1 || reenabled.Existing != 1 {
		t.Fatalf("reenable result=%+v err=%v", reenabled, err)
	}
	premiumGroup, err := client.BillingGroup.Create().
		SetCode("sync-premium").SetDisplayName("Sync Premium").SetMultiplierBps(15_000).
		Save(ctx)
	if err != nil {
		t.Fatalf("create premium billing group: %v", err)
	}
	premiumBinding := ModelBinding{UpstreamModelID: modelID, BillingGroupID: premiumGroup.ID}
	premiumLink, err := service.Link(ctx, firstProvider.ID, []ModelBinding{premiumBinding})
	if err != nil || premiumLink.Created != 1 || premiumLink.Existing != 0 {
		t.Fatalf("link same provider and model to premium group result=%+v err=%v", premiumLink, err)
	}
	firstProviderRoutes, err := client.ModelRoute.Query().Where(
		entmodelroute.ProviderIDEQ(firstProvider.ID),
		entmodelroute.UpstreamModelIDEQ(modelID),
		entmodelroute.DeletedAtIsNil(),
	).All(ctx)
	if err != nil || len(firstProviderRoutes) != 2 {
		t.Fatalf("same-provider multi-group routes=%+v err=%v", firstProviderRoutes, err)
	}
	seenGroups := map[uuid.UUID]bool{}
	for _, route := range firstProviderRoutes {
		seenGroups[route.BillingGroupID] = true
	}
	if !seenGroups[group.ID] || !seenGroups[premiumGroup.ID] {
		t.Fatalf("same-provider routes did not preserve group bindings: %+v", firstProviderRoutes)
	}

	// A model removed from the catalog can be advertised again by an
	// authoritative upstream sync. Its global price card and historical route
	// IDs must be recovered instead of creating a duplicate or disappearing
	// from the picker.
	deletedAt := time.Now().UTC()
	if _, err := client.UpstreamModel.UpdateOneID(modelID).
		SetStatus(entupstreammodel.StatusDisabled).
		SetDeletedAt(deletedAt).
		Save(ctx); err != nil {
		t.Fatalf("soft delete shared model: %v", err)
	}
	if _, err := client.ModelRoute.Update().
		Where(entmodelroute.UpstreamModelIDEQ(modelID)).
		SetStatus(entmodelroute.StatusDisabled).
		SetDeletedAt(deletedAt).
		Save(ctx); err != nil {
		t.Fatalf("soft delete shared routes: %v", err)
	}
	restoredSync, err := service.Sync(ctx, secondProvider.ID)
	if err != nil || len(restoredSync) != 1 || restoredSync[0].Added || !restoredSync[0].Restored || restoredSync[0].ID != modelID || restoredSync[0].Status != string(entupstreammodel.StatusActive) {
		t.Fatalf("restored sync=%+v err=%v", restoredSync, err)
	}
	restoredLink, err := service.Link(ctx, firstProvider.ID, []ModelBinding{defaultBinding})
	if err != nil || restoredLink.Existing != 1 || restoredLink.Reenabled != 1 || restoredLink.Created != 0 {
		t.Fatalf("restored link=%+v err=%v", restoredLink, err)
	}
	visible, err := client.UpstreamModel.Query().Where(entupstreammodel.IDEQ(modelID), entupstreammodel.DeletedAtIsNil()).Only(ctx)
	if err != nil || visible.Status != entupstreammodel.StatusActive || !visible.PricingConfigured {
		t.Fatalf("restored model=%+v err=%v", visible, err)
	}
}

/**
 * openProviderSyncIntegrationClient 封装该名称对应的业务处理逻辑。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func openProviderSyncIntegrationClient(t *testing.T) *ent.Client {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("NOVRO_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("set NOVRO_TEST_MYSQL_DSN to run the MySQL provider-sync integration test")
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
	databaseName := "novro_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci", databaseName)); err != nil {
		t.Fatalf("create isolated provider-sync database: %v", err)
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
	if err := migrate.Apply(ctx, database); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	driver := entsql.OpenDB(dialect.MySQL, database)
	client := ent.NewClient(ent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	return client
}
