package providersync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/ent"
	entprovider "github.com/novro-gateway/novro/ent/provider"
	"github.com/novro-gateway/novro/internal/provider"
)

func TestDiscoverUsesConfiguredProviderNameAndCredentials(t *testing.T) {
	var authorization string
	var requestPath string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		requestPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-chat","owned_by":"upstream-owner"},{"id":"deepseek-reasoner","display_name":"DeepSeek Reasoner"}]}`))
	}))
	defer server.Close()

	service := NewService(nil, nil, server.Client())
	models, err := service.discover(context.Background(), &ent.Provider{
		DisplayName: "DeepSeek 主账号",
		Protocol:    entprovider.ProtocolOpenai,
		BaseURL:     server.URL + "/v1",
	}, "provider-secret")
	if err != nil {
		t.Fatalf("discover models: %v", err)
	}
	if authorization != "Bearer provider-secret" {
		t.Fatalf("authorization=%q", authorization)
	}
	if requestPath != "/v1/models" {
		t.Fatalf("path=%q want /v1/models", requestPath)
	}
	if len(models) != 2 || models[0].ProviderName != "DeepSeek 主账号" || models[0].DisplayName != "deepseek-chat" || models[1].DisplayName != "DeepSeek Reasoner" {
		t.Fatalf("unexpected discovered models: %+v", models)
	}
}

func TestModelListURLHandlesSupportedProtocols(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		protocol provider.Protocol
		want     string
	}{
		{name: "OpenAI versioned base", baseURL: "https://api.example.com/v1", protocol: provider.ProtocolOpenAI, want: "https://api.example.com/v1/models"},
		{name: "Anthropic root base", baseURL: "https://api.anthropic.com", protocol: provider.ProtocolAnthropic, want: "https://api.anthropic.com/v1/models"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := modelListURL(tt.baseURL, tt.protocol)
			if err != nil || got != tt.want {
				t.Fatalf("modelListURL()=(%q, %v), want %q", got, err, tt.want)
			}
		})
	}
	if _, err := modelListURL("http://127.0.0.1:8080", provider.ProtocolOpenAI); err == nil {
		t.Fatal("expected insecure model URL to be rejected")
	}
}

func TestCatalogProviderNameCanonicalizesOfficialChineseEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		provider *ent.Provider
		want     string
	}{
		{name: "DeepSeek", provider: &ent.Provider{DisplayName: "DS 主账号", BaseURL: "https://api.deepseek.com/v1"}, want: "DeepSeek"},
		{name: "GLM", provider: &ent.Provider{DisplayName: "智谱生产账号", BaseURL: "https://open.bigmodel.cn/api/paas/v4"}, want: "智谱 GLM"},
		{name: "Kimi", provider: &ent.Provider{DisplayName: "kimi官方key", BaseURL: "https://api.moonshot.cn/v1"}, want: "Kimi"},
		{name: "custom provider", provider: &ent.Provider{DisplayName: "内部兼容网关", BaseURL: "https://models.example.com/v1"}, want: "内部兼容网关"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := catalogProviderName(tt.provider); got != tt.want {
				t.Fatalf("catalogProviderName()=%q want %q", got, tt.want)
			}
		})
	}
}

func TestUniqueIDsAndRouteNamesAreDeterministic(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	ids := uniqueIDs([]uuid.UUID{first, uuid.Nil, first, second})
	if len(ids) != 2 || ids[0] != first || ids[1] != second {
		t.Fatalf("unexpected unique IDs: %v", ids)
	}
	used := map[string]struct{}{"deepseek-chat": {}, "deepseek-deepseek-chat": {}}
	if got := availableRouteName("deepseek chat", "deepseek", used); got != "deepseek-deepseek-chat-2" {
		t.Fatalf("route name=%q", got)
	}
}
