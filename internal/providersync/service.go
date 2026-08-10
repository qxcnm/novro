package providersync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entmodelroute "github.com/novro-gateway/novro/ent/modelroute"
	entprovider "github.com/novro-gateway/novro/ent/provider"
	entupstreammodel "github.com/novro-gateway/novro/ent/upstreammodel"
	"github.com/novro-gateway/novro/internal/provider"
	"github.com/novro-gateway/novro/internal/upstreamhttp"
)

const (
	maxDiscoveredModels = 1000
	maxLinkModels       = 200
	maxDiscoveryBody    = 4 << 20
	modelSyncTimeout    = 8 * time.Second
)

var (
	ErrInvalidInput      = errors.New("invalid provider model sync input")
	ErrProviderNotFound  = errors.New("provider not found")
	ErrDiscoveryFailed   = errors.New("provider model discovery failed")
	ErrModelsUnavailable = errors.New("provider did not return models")
)

// DiscoveryError is safe to return to an administrator. It never includes
// request headers or the upstream response body.
type DiscoveryError struct {
	StatusCode int
	Reason     string
}

func (e *DiscoveryError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("上游模型接口返回 HTTP %d：%s", e.StatusCode, e.Reason)
	}
	return e.Reason
}

func (e *DiscoveryError) Unwrap() error { return ErrDiscoveryFailed }

type CatalogModel struct {
	ID                uuid.UUID `json:"id"`
	ProviderName      string    `json:"provider_name"`
	UpstreamName      string    `json:"upstream_name"`
	DisplayName       string    `json:"display_name"`
	PricingConfigured bool      `json:"pricing_configured"`
	Status            string    `json:"status"`
	Added             bool      `json:"added"`
	Restored          bool      `json:"restored"`
}

type LinkResult struct {
	Created   int `json:"created"`
	Existing  int `json:"existing"`
	Reenabled int `json:"reenabled"`
	Disabled  int `json:"disabled"`
}

type Service struct {
	client *ent.Client
	cipher *provider.Cipher
	http   *http.Client
}

func NewService(client *ent.Client, cipher *provider.Cipher, httpClient *http.Client) *Service {
	if httpClient == nil {
		httpClient = upstreamhttp.NewClient()
	}
	return &Service{client: client, cipher: cipher, http: httpClient}
}

func (s *Service) Sync(ctx context.Context, providerID uuid.UUID) ([]CatalogModel, error) {
	if providerID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	// Model discovery should fail in a bounded time. The shared upstream client
	// allows long response-header waits for model generation, which is not
	// appropriate for an administrator dialog waiting on a catalog refresh.
	syncCtx, cancel := context.WithTimeout(ctx, modelSyncTimeout)
	defer cancel()
	configured, err := s.client.Provider.Query().Where(entprovider.IDEQ(providerID), entprovider.DeletedAtIsNil()).Only(syncCtx)
	if ent.IsNotFound(err) {
		return nil, ErrProviderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read provider for model sync: %w", err)
	}
	apiKey, err := s.cipher.Decrypt(configured.EncryptedAPIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt provider credential for model sync: %w", err)
	}
	discovered, err := s.discover(syncCtx, configured, apiKey)
	if err != nil {
		return nil, err
	}
	if len(discovered) == 0 {
		return nil, ErrModelsUnavailable
	}
	discoveredNames := make(map[string]struct{}, len(discovered))
	for _, model := range discovered {
		discoveredNames[strings.ToLower(model.UpstreamName)] = struct{}{}
	}

	tx, err := s.client.Tx(syncCtx)
	if err != nil {
		return nil, fmt.Errorf("begin model catalog sync: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result := make([]CatalogModel, 0, len(discovered))
	for _, model := range discovered {
		entity, queryErr := findCatalogModel(syncCtx, tx, model)
		added := false
		restored := false
		if queryErr != nil {
			return nil, fmt.Errorf("find model %s in catalog: %w", model.UpstreamName, queryErr)
		}
		if entity != nil && entity.DeletedAt != nil {
			// A later authoritative discovery is the recovery workflow for a
			// catalog entry removed by an earlier sync or administrator action.
			// Keep the global ID and any reviewed price card, then make the
			// currently advertised model visible to the administrator again.
			pricingConfigured := entity.PricingConfigured
			status := entupstreammodel.StatusDisabled
			if pricingConfigured {
				status = entupstreammodel.StatusActive
			}
			entity, queryErr = tx.UpstreamModel.UpdateOneID(entity.ID).
				ClearDeletedAt().
				SetProviderName(model.ProviderName).
				SetDisplayName(model.DisplayName).
				SetStatus(status).
				Save(syncCtx)
			restored = queryErr == nil
		}
		if entity == nil {
			// Discovery only creates a candidate. The pricing tab is the sole
			// source of billing rates and activation decisions.
			entity, queryErr = tx.UpstreamModel.Create().
				SetProviderName(model.ProviderName).
				SetUpstreamName(model.UpstreamName).
				SetDisplayName(model.DisplayName).
				SetPricingConfigured(false).
				SetStatus(entupstreammodel.StatusDisabled).
				Save(syncCtx)
			added = queryErr == nil
		}
		if queryErr != nil {
			return nil, fmt.Errorf("sync model %s into catalog: %w", model.UpstreamName, queryErr)
		}
		result = append(result, catalogModel(entity, added, restored))
	}
	// A successful sync is authoritative for this provider's advertised model
	// capability. Preserve catalog rows and historical usage, but stop routing
	// models that disappeared upstream until an administrator explicitly links
	// them again after a later sync.
	routes, queryErr := tx.ModelRoute.Query().
		Where(entmodelroute.ProviderIDEQ(providerID), entmodelroute.DeletedAtIsNil(), entmodelroute.StatusEQ(entmodelroute.StatusActive), entmodelroute.UpstreamModelIDNotNil()).
		WithUpstreamModel().All(syncCtx)
	if queryErr != nil {
		return nil, fmt.Errorf("read provider routes after model sync: %w", queryErr)
	}
	for _, route := range routes {
		modelEntity, edgeErr := route.Edges.UpstreamModelOrErr()
		if edgeErr != nil || modelEntity == nil {
			continue
		}
		if _, ok := discoveredNames[strings.ToLower(modelEntity.UpstreamName)]; ok {
			continue
		}
		if _, updateErr := tx.ModelRoute.UpdateOneID(route.ID).SetStatus(entmodelroute.StatusDisabled).Save(syncCtx); updateErr != nil {
			return nil, fmt.Errorf("disable stale provider route %s: %w", route.ID, updateErr)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit model catalog sync: %w", err)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpstreamName < result[j].UpstreamName })
	return result, nil
}

// A model ID has one global catalog record and one shared price card. Provider
// discovery only reuses that record; it never owns or overwrites its pricing.
func findCatalogModel(ctx context.Context, tx *ent.Tx, model discoveredModel) (*ent.UpstreamModel, error) {
	entities, err := tx.UpstreamModel.Query().Where(
		entupstreammodel.UpstreamNameEqualFold(model.UpstreamName),
	).Order(ent.Desc(entupstreammodel.FieldDeletedAt), ent.Asc(entupstreammodel.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, err
	}
	if len(entities) > 0 {
		// Migration 0023 removes historical duplicates. Prefer a visible record
		// during a rolling deployment, while still honoring an explicit deletion
		// when it is the only record left.
		for _, entity := range entities {
			if entity.DeletedAt == nil {
				return entity, nil
			}
		}
		return entities[0], nil
	}
	return nil, nil
}

func (s *Service) Link(ctx context.Context, providerID uuid.UUID, modelIDs []uuid.UUID) (LinkResult, error) {
	modelIDs = uniqueIDs(modelIDs)
	if providerID == uuid.Nil || len(modelIDs) == 0 || len(modelIDs) > maxLinkModels {
		return LinkResult{}, ErrInvalidInput
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return LinkResult{}, fmt.Errorf("begin provider model linking: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	configured, err := tx.Provider.Query().Where(entprovider.IDEQ(providerID), entprovider.DeletedAtIsNil()).Only(ctx)
	if ent.IsNotFound(err) {
		return LinkResult{}, ErrProviderNotFound
	}
	if err != nil {
		return LinkResult{}, fmt.Errorf("read provider for model linking: %w", err)
	}
	models, err := tx.UpstreamModel.Query().Where(entupstreammodel.IDIn(modelIDs...), entupstreammodel.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return LinkResult{}, fmt.Errorf("read catalog models for provider linking: %w", err)
	}
	if len(models) != len(modelIDs) {
		return LinkResult{}, ErrInvalidInput
	}
	existingRoutes, err := tx.ModelRoute.Query().Where(entmodelroute.ProviderIDEQ(providerID), entmodelroute.UpstreamModelIDIn(modelIDs...)).All(ctx)
	if err != nil {
		return LinkResult{}, fmt.Errorf("read existing provider model routes: %w", err)
	}
	existing := make(map[uuid.UUID]*ent.ModelRoute, len(existingRoutes))
	for _, route := range existingRoutes {
		if route.UpstreamModelID != nil {
			existing[*route.UpstreamModelID] = route
		}
	}

	result := LinkResult{}
	for _, model := range models {
		if route, ok := existing[model.ID]; ok {
			result.Existing++
			status := entmodelroute.StatusDisabled
			if model.Status == entupstreammodel.StatusActive && model.PricingConfigured {
				status = entmodelroute.StatusActive
			}
			if route.DeletedAt != nil {
				if _, updateErr := tx.ModelRoute.UpdateOneID(route.ID).
					ClearDeletedAt().
					SetDisplayName(model.DisplayName).
					SetUpstreamName(model.UpstreamName).
					SetInputPriceMicros(model.InputPriceMicros).
					SetOutputPriceMicros(model.OutputPriceMicros).
					SetStatus(status).
					Save(ctx); updateErr != nil {
					return LinkResult{}, fmt.Errorf("restore provider model route %s: %w", model.UpstreamName, updateErr)
				}
				result.Reenabled++
			} else if route.Status != status {
				if _, updateErr := tx.ModelRoute.UpdateOneID(route.ID).SetStatus(status).Save(ctx); updateErr != nil {
					return LinkResult{}, fmt.Errorf("update provider model route %s: %w", model.UpstreamName, updateErr)
				}
				if status == entmodelroute.StatusActive {
					result.Reenabled++
				}
			}
			if status == entmodelroute.StatusDisabled {
				result.Disabled++
			}
			continue
		}
		if !validAutomaticPublicName(model.UpstreamName) {
			return LinkResult{}, ErrInvalidInput
		}
		publicName := model.UpstreamName
		status := entmodelroute.StatusActive
		if model.Status != entupstreammodel.StatusActive || !model.PricingConfigured {
			status = entmodelroute.StatusDisabled
			result.Disabled++
		}
		route, err := tx.ModelRoute.Create().
			SetProviderID(configured.ID).
			SetUpstreamModelID(model.ID).
			SetPublicName(publicName).
			SetDisplayName(model.DisplayName).
			SetUpstreamName(model.UpstreamName).
			SetInputPriceMicros(model.InputPriceMicros).
			SetOutputPriceMicros(model.OutputPriceMicros).
			SetStatus(status).
			Save(ctx)
		if err != nil {
			return LinkResult{}, fmt.Errorf("create provider model route %s: %w", publicName, err)
		}
		existing[model.ID] = route
		result.Created++
	}
	if err := tx.Commit(); err != nil {
		return LinkResult{}, fmt.Errorf("commit provider model linking: %w", err)
	}
	return result, nil
}

type discoveredModel struct {
	ProviderName string
	UpstreamName string
	DisplayName  string
}

func (s *Service) discover(ctx context.Context, configured *ent.Provider, apiKey string) ([]discoveredModel, error) {
	endpoint, err := modelListURL(configured.BaseURL, provider.Protocol(configured.Protocol), configured.ModelListPath)
	if err != nil {
		return nil, &DiscoveryError{Reason: "模型列表地址配置无效，请检查 Base URL 和模型列表路径"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &DiscoveryError{Reason: "无法创建模型列表请求，请检查 Base URL 和模型列表路径"}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Novro-Gateway/1")
	if provider.Protocol(configured.Protocol) == provider.ProtocolAnthropic {
		request.Header.Set("X-API-Key", apiKey)
		request.Header.Set("Anthropic-Version", "2023-06-01")
	} else {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := s.http.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &DiscoveryError{Reason: "同步模型超时，请检查上游连接或模型列表路径"}
		}
		if errors.Is(err, context.Canceled) {
			return nil, &DiscoveryError{Reason: "同步模型请求已取消"}
		}
		return nil, &DiscoveryError{Reason: "连接上游失败，请检查 DNS、TCP 443 端口和 TLS 配置：" + trimError(err.Error(), 320)}
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		if response.StatusCode == http.StatusUnauthorized {
			return nil, &DiscoveryError{StatusCode: response.StatusCode, Reason: "上游 API Key 无效或已撤销，请在提供商配置中重新填写上游密钥"}
		}
		return nil, &DiscoveryError{StatusCode: response.StatusCode, Reason: "请检查模型列表路径和 API 密钥"}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDiscoveryBody+1))
	if err != nil || len(body) > maxDiscoveryBody {
		return nil, &DiscoveryError{Reason: "上游响应读取失败或超过大小限制"}
	}
	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil || len(payload.Data) > maxDiscoveredModels {
		return nil, &DiscoveryError{Reason: "上游返回的模型列表格式无效"}
	}
	seen := make(map[string]struct{}, len(payload.Data))
	models := make([]discoveredModel, 0, len(payload.Data))
	for _, item := range payload.Data {
		modelID := strings.TrimSpace(item.ID)
		if modelID == "" || utf8.RuneCountInString(modelID) > 256 {
			continue
		}
		displayName := strings.TrimSpace(item.DisplayName)
		if displayName == "" || utf8.RuneCountInString(displayName) > 128 {
			displayName = modelID
		}
		key := strings.ToLower(modelID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		models = append(models, discoveredModel{ProviderName: catalogProviderNameForModel(configured, modelID), UpstreamName: modelID, DisplayName: displayName})
	}
	return models, nil
}

func trimError(value string, maximum int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= maximum {
		return value
	}
	return value[:maximum] + "..."
}

func catalogProviderName(configured *ent.Provider) string {
	parsed, err := url.Parse(configured.BaseURL)
	if err == nil {
		switch strings.ToLower(parsed.Hostname()) {
		case "api.deepseek.com":
			return "DeepSeek"
		case "open.bigmodel.cn":
			return "智谱 GLM"
		case "api.moonshot.cn":
			return "Kimi"
		}
	}
	return configured.DisplayName
}

func catalogProviderNameForModel(configured *ent.Provider, modelID string) string {
	normalized := strings.ToLower(strings.TrimSpace(modelID))
	switch {
	case strings.HasPrefix(normalized, "deepseek-"):
		return "DeepSeek"
	case strings.HasPrefix(normalized, "glm-"):
		return "智谱 GLM"
	case strings.HasPrefix(normalized, "kimi-"):
		return "Kimi"
	default:
		return catalogProviderName(configured)
	}
}

func modelListURL(base string, protocol provider.Protocol, modelPath string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", ErrInvalidInput
	}
	if modelPath = strings.TrimSpace(modelPath); modelPath != "" {
		if !strings.HasPrefix(modelPath, "/") || strings.ContainsAny(modelPath, "?#") {
			return "", ErrInvalidInput
		}
		parsed.Path = modelPath
	} else {
		basePath := strings.TrimRight(parsed.Path, "/")
		if strings.HasSuffix(basePath, "/models") {
			parsed.Path = basePath
			parsed.RawPath, parsed.RawQuery, parsed.Fragment = "", "", ""
			return parsed.String(), nil
		}
		if (protocol == provider.ProtocolOpenAI || protocol == provider.ProtocolAnthropic) && !strings.HasSuffix(basePath, "/v1") {
			basePath += "/v1"
		}
		parsed.Path = basePath + "/models"
	}
	parsed.RawPath, parsed.RawQuery, parsed.Fragment = "", "", ""
	return parsed.String(), nil
}

func catalogModel(entity *ent.UpstreamModel, added, restored bool) CatalogModel {
	return CatalogModel{ID: entity.ID, ProviderName: entity.ProviderName, UpstreamName: entity.UpstreamName, DisplayName: entity.DisplayName, PricingConfigured: entity.PricingConfigured, Status: string(entity.Status), Added: added, Restored: restored}
}

func uniqueIDs(values []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(values))
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if value == uuid.Nil {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func validAutomaticPublicName(value string) bool {
	if len(value) < 2 || len(value) > 256 {
		return false
	}
	for index, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || (index > 0 && strings.ContainsRune("._:/-", char)) {
			continue
		}
		return false
	}
	return true
}
