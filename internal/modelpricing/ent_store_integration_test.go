package modelpricing

import (
	"context"
	"database/sql"
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
	entbillinggroup "github.com/novro-gateway/novro/ent/billinggroup"
	"github.com/novro-gateway/novro/ent/migrate"
	entmodelpriceplan "github.com/novro-gateway/novro/ent/modelpriceplan"
	entmodelroute "github.com/novro-gateway/novro/ent/modelroute"
	entprovider "github.com/novro-gateway/novro/ent/provider"
	entupstreammodel "github.com/novro-gateway/novro/ent/upstreammodel"
	"github.com/novro-gateway/novro/internal/billing"
)

func TestPublishActivatesModelAndEligibleRoutes(t *testing.T) {
	client := openModelPricingIntegrationClient(t)
	ctx := context.Background()
	activeGroup, err := client.BillingGroup.Create().SetCode("pricing-active").SetDisplayName("Pricing Active").Save(ctx)
	if err != nil {
		t.Fatalf("create active billing group: %v", err)
	}
	disabledGroup, err := client.BillingGroup.Create().SetCode("pricing-disabled").SetDisplayName("Pricing Disabled").SetStatus(entbillinggroup.StatusDisabled).Save(ctx)
	if err != nil {
		t.Fatalf("create disabled billing group: %v", err)
	}
	activeProvider, err := client.Provider.Create().SetCode("pricing-active-provider").SetDisplayName("Pricing Active Provider").SetProtocol(entprovider.ProtocolOpenai).SetBaseURL("https://active.example.com/v1").SetEncryptedAPIKey("encrypted-active").SetAPIKeyHint("tive").Save(ctx)
	if err != nil {
		t.Fatalf("create active provider: %v", err)
	}
	disabledProvider, err := client.Provider.Create().SetCode("pricing-disabled-provider").SetDisplayName("Pricing Disabled Provider").SetProtocol(entprovider.ProtocolOpenai).SetBaseURL("https://disabled.example.com/v1").SetEncryptedAPIKey("encrypted-disabled").SetAPIKeyHint("bled").SetStatus(entprovider.StatusDisabled).Save(ctx)
	if err != nil {
		t.Fatalf("create disabled provider: %v", err)
	}
	model, err := client.UpstreamModel.Create().SetProviderName("Pricing Test").SetUpstreamName("pricing-test-model").SetDisplayName("Pricing Test Model").SetPricingConfigured(false).SetStatus(entupstreammodel.StatusDisabled).Save(ctx)
	if err != nil {
		t.Fatalf("create unpriced model: %v", err)
	}
	createRoute := func(providerID, groupID uuid.UUID, name string) *ent.ModelRoute {
		t.Helper()
		route, createErr := client.ModelRoute.Create().SetProviderID(providerID).SetUpstreamModelID(model.ID).SetBillingGroupID(groupID).SetPublicName(name).SetDisplayName(name).SetUpstreamName(model.UpstreamName).SetInputPriceMicros(0).SetOutputPriceMicros(0).SetStatus(entmodelroute.StatusDisabled).Save(ctx)
		if createErr != nil {
			t.Fatalf("create route %s: %v", name, createErr)
		}
		return route
	}
	eligibleRoute := createRoute(activeProvider.ID, activeGroup.ID, "pricing-eligible")
	disabledProviderRoute := createRoute(disabledProvider.ID, activeGroup.ID, "pricing-provider-disabled")
	disabledGroupRoute := createRoute(activeProvider.ID, disabledGroup.ID, "pricing-group-disabled")

	store := NewEntStore(client)
	draft, err := store.CreateDraft(ctx, model.ID, PlanInput{
		Mode: ModeFixed, Timezone: "UTC", EffectiveFrom: time.Now().UTC().Add(30 * 24 * time.Hour),
		DefaultRates: billing.RateCard{InputMicros: 1_250_000, OutputMicros: 3_500_000},
	})
	if err != nil {
		t.Fatalf("create pricing draft: %v", err)
	}
	if _, err := store.Publish(ctx, draft.ID); err != nil {
		t.Fatalf("publish pricing draft: %v", err)
	}
	resolved, err := store.Resolve(ctx, model.ID, time.Now().UTC())
	if err != nil || resolved.Rates.InputMicros != 1_250_000 || resolved.Rates.OutputMicros != 3_500_000 {
		t.Fatalf("fixed price did not publish immediately: resolution=%+v err=%v", resolved, err)
	}

	updatedModel, err := client.UpstreamModel.Get(ctx, model.ID)
	if err != nil || updatedModel.Status != entupstreammodel.StatusActive || !updatedModel.PricingConfigured {
		t.Fatalf("updated model=%+v err=%v", updatedModel, err)
	}
	updatedEligible, err := client.ModelRoute.Get(ctx, eligibleRoute.ID)
	if err != nil || updatedEligible.Status != entmodelroute.StatusActive || updatedEligible.InputPriceMicros != 1_250_000 || updatedEligible.OutputPriceMicros != 3_500_000 {
		t.Fatalf("eligible route=%+v err=%v", updatedEligible, err)
	}
	for _, routeID := range []uuid.UUID{disabledProviderRoute.ID, disabledGroupRoute.ID} {
		route, getErr := client.ModelRoute.Get(ctx, routeID)
		if getErr != nil || route.Status != entmodelroute.StatusDisabled {
			t.Fatalf("ineligible route=%+v err=%v", route, getErr)
		}
	}

	futureDraft, err := store.CreateDraft(ctx, model.ID, PlanInput{
		Mode: ModeFixed, Timezone: "UTC",
		DefaultRates: billing.RateCard{InputMicros: 9_000_000, OutputMicros: 12_000_000},
	})
	if err != nil {
		t.Fatalf("create legacy future pricing draft: %v", err)
	}
	futureStart := time.Now().UTC().Add(24 * time.Hour)
	if _, err := client.ModelPricePlan.UpdateOneID(draft.ID).SetEffectiveTo(futureStart).Save(ctx); err != nil {
		t.Fatalf("close current plan at legacy future boundary: %v", err)
	}
	if _, err := client.ModelPricePlan.UpdateOneID(futureDraft.ID).SetStatus(entmodelpriceplan.StatusPublished).SetEffectiveFrom(futureStart).ClearEffectiveTo().Save(ctx); err != nil {
		t.Fatalf("prepare legacy future price plan: %v", err)
	}
	if _, err := store.Republish(ctx, draft.ID, time.Now().UTC()); err != nil {
		t.Fatalf("republish current plan over legacy future version: %v", err)
	}
	retiredFuture, err := client.ModelPricePlan.Get(ctx, futureDraft.ID)
	if err != nil || retiredFuture.Status != entmodelpriceplan.StatusRetired {
		t.Fatalf("legacy future plan was not retired: plan=%+v err=%v", retiredFuture, err)
	}
	continuousCurrent, err := client.ModelPricePlan.Get(ctx, draft.ID)
	if err != nil || continuousCurrent.EffectiveTo != nil {
		t.Fatalf("current plan did not remain continuous: plan=%+v err=%v", continuousCurrent, err)
	}

	scheduledModel, err := client.UpstreamModel.Create().SetProviderName("Pricing Test").SetUpstreamName("scheduled-model").SetDisplayName("Scheduled Model").SetPricingConfigured(false).SetStatus(entupstreammodel.StatusDisabled).Save(ctx)
	if err != nil {
		t.Fatalf("create scheduled model: %v", err)
	}
	scheduledDraft, err := store.CreateDraft(ctx, scheduledModel.ID, PlanInput{
		Mode: ModeScheduled, Timezone: "UTC", EffectiveFrom: time.Now().UTC().Add(time.Hour),
		DefaultRates: billing.RateCard{InputMicros: 2_000_000, OutputMicros: 4_000_000},
		Windows:      []WindowInput{{Label: "all day", WeekdayMask: 127, StartMinute: 0, EndMinute: 1440, Rates: billing.RateCard{InputMicros: 2_000_000, OutputMicros: 4_000_000}}},
	})
	if err != nil {
		t.Fatalf("create scheduled pricing draft: %v", err)
	}
	if _, err := store.Publish(ctx, scheduledDraft.ID); err != nil {
		t.Fatalf("publish scheduled pricing immediately: %v", err)
	}
	activatedModel, err := client.UpstreamModel.Get(ctx, scheduledModel.ID)
	if err != nil || !activatedModel.PricingConfigured || activatedModel.Status != entupstreammodel.StatusActive {
		t.Fatalf("scheduled publication did not activate model: model=%+v err=%v", activatedModel, err)
	}
	scheduledResolution, err := store.Resolve(ctx, scheduledModel.ID, time.Now().UTC())
	if err != nil || scheduledResolution.Rates.InputMicros != 2_000_000 {
		t.Fatalf("scheduled publication was not immediately resolvable: resolution=%+v err=%v", scheduledResolution, err)
	}
}

func openModelPricingIntegrationClient(t *testing.T) *ent.Client {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("NOVRO_TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("set NOVRO_TEST_MYSQL_DSN to run the MySQL model-pricing integration test")
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
