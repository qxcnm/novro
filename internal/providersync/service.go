package providersync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
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
)

var (
	ErrInvalidInput      = errors.New("invalid provider model sync input")
	ErrProviderNotFound  = errors.New("provider not found")
	ErrDiscoveryFailed   = errors.New("provider model discovery failed")
	ErrModelsUnavailable = errors.New("provider did not return models")
	invalidRouteName     = regexp.MustCompile(`[^A-Za-z0-9._:/-]+`)
)

type CatalogModel struct {
	ID                uuid.UUID `json:"id"`
	ProviderName      string    `json:"provider_name"`
	UpstreamName      string    `json:"upstream_name"`
	DisplayName       string    `json:"display_name"`
	PricingConfigured bool      `json:"pricing_configured"`
	Status            string    `json:"status"`
	Added             bool      `json:"added"`
}

type LinkResult struct {
	Created  int `json:"created"`
	Existing int `json:"existing"`
	Disabled int `json:"disabled"`
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
	configured, err := s.client.Provider.Query().Where(entprovider.IDEQ(providerID), entprovider.DeletedAtIsNil()).Only(ctx)
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
	discovered, err := s.discover(ctx, configured, apiKey)
	if err != nil {
		return nil, err
	}
	if len(discovered) == 0 {
		return nil, ErrModelsUnavailable
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin model catalog sync: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result := make([]CatalogModel, 0, len(discovered))
	for _, model := range discovered {
		entity, queryErr := tx.UpstreamModel.Query().Where(
			entupstreammodel.ProviderNameEQ(model.ProviderName),
			entupstreammodel.UpstreamNameEQ(model.UpstreamName),
		).Only(ctx)
		added := false
		if queryErr == nil && entity.DeletedAt != nil {
			// Deletion is an explicit administrator decision. Discovery must not
			// silently restore a catalog entry that has no recovery workflow.
			continue
		}
		if ent.IsNotFound(queryErr) {
			entity, queryErr = tx.UpstreamModel.Create().
				SetProviderName(model.ProviderName).
				SetUpstreamName(model.UpstreamName).
				SetDisplayName(model.DisplayName).
				SetPricingConfigured(false).
				SetStatus(entupstreammodel.StatusDisabled).
				Save(ctx)
			added = queryErr == nil
		}
		if queryErr != nil {
			return nil, fmt.Errorf("sync model %s into catalog: %w", model.UpstreamName, queryErr)
		}
		result = append(result, catalogModel(entity, added))
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit model catalog sync: %w", err)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpstreamName < result[j].UpstreamName })
	return result, nil
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
	existingRoutes, err := tx.ModelRoute.Query().Where(entmodelroute.ProviderIDEQ(providerID), entmodelroute.UpstreamModelIDIn(modelIDs...), entmodelroute.DeletedAtIsNil()).All(ctx)
	if err != nil {
		return LinkResult{}, fmt.Errorf("read existing provider model routes: %w", err)
	}
	existing := make(map[uuid.UUID]struct{}, len(existingRoutes))
	for _, route := range existingRoutes {
		if route.UpstreamModelID != nil {
			existing[*route.UpstreamModelID] = struct{}{}
		}
	}
	publicNames, err := tx.ModelRoute.Query().Select(entmodelroute.FieldPublicName).Strings(ctx)
	if err != nil {
		return LinkResult{}, fmt.Errorf("read model route names: %w", err)
	}
	usedNames := make(map[string]struct{}, len(publicNames))
	for _, name := range publicNames {
		usedNames[name] = struct{}{}
	}

	result := LinkResult{Existing: len(existingRoutes)}
	for _, model := range models {
		if _, ok := existing[model.ID]; ok {
			continue
		}
		publicName := availableRouteName(model.UpstreamName, configured.Code, usedNames)
		status := entmodelroute.StatusActive
		if model.Status != entupstreammodel.StatusActive || !model.PricingConfigured {
			status = entmodelroute.StatusDisabled
			result.Disabled++
		}
		if _, err := tx.ModelRoute.Create().
			SetProviderID(configured.ID).
			SetUpstreamModelID(model.ID).
			SetPublicName(publicName).
			SetDisplayName(model.DisplayName).
			SetUpstreamName(model.UpstreamName).
			SetInputPriceMicros(model.InputPriceMicros).
			SetOutputPriceMicros(model.OutputPriceMicros).
			SetStatus(status).
			Save(ctx); err != nil {
			return LinkResult{}, fmt.Errorf("create provider model route %s: %w", publicName, err)
		}
		usedNames[publicName] = struct{}{}
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
		return nil, ErrDiscoveryFailed
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create provider model discovery request: %w", err)
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
		return nil, fmt.Errorf("%w: request models: %v", ErrDiscoveryFailed, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return nil, fmt.Errorf("%w: upstream status %d", ErrDiscoveryFailed, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDiscoveryBody+1))
	if err != nil || len(body) > maxDiscoveryBody {
		return nil, fmt.Errorf("%w: invalid response body", ErrDiscoveryFailed)
	}
	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil || len(payload.Data) > maxDiscoveredModels {
		return nil, fmt.Errorf("%w: invalid response format", ErrDiscoveryFailed)
	}
	seen := make(map[string]struct{}, len(payload.Data))
	models := make([]discoveredModel, 0, len(payload.Data))
	providerName := catalogProviderName(configured)
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
		models = append(models, discoveredModel{ProviderName: providerName, UpstreamName: modelID, DisplayName: displayName})
	}
	return models, nil
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
		if protocol == provider.ProtocolAnthropic && !strings.HasSuffix(basePath, "/v1") {
			basePath += "/v1"
		}
		parsed.Path = basePath + "/models"
	}
	parsed.RawPath, parsed.RawQuery, parsed.Fragment = "", "", ""
	return parsed.String(), nil
}

func catalogModel(entity *ent.UpstreamModel, added bool) CatalogModel {
	return CatalogModel{ID: entity.ID, ProviderName: entity.ProviderName, UpstreamName: entity.UpstreamName, DisplayName: entity.DisplayName, PricingConfigured: entity.PricingConfigured, Status: string(entity.Status), Added: added}
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

func availableRouteName(modelName, providerCode string, used map[string]struct{}) string {
	base := strings.Trim(invalidRouteName.ReplaceAllString(strings.TrimSpace(modelName), "-"), "-._:/")
	if len(base) < 2 {
		base = providerCode + "-model"
	}
	base = truncateASCII(base, 128)
	if _, exists := used[base]; !exists {
		return base
	}
	prefixed := truncateASCII(providerCode+"-"+base, 128)
	if _, exists := used[prefixed]; !exists {
		return prefixed
	}
	for index := 2; ; index++ {
		suffix := fmt.Sprintf("-%d", index)
		candidate := truncateASCII(prefixed, 128-len(suffix)) + suffix
		if _, exists := used[candidate]; !exists {
			return candidate
		}
	}
}

func truncateASCII(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}
