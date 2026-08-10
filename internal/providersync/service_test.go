package providersync

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
		DisplayName:   "DeepSeek 主账号",
		Protocol:      entprovider.ProtocolOpenai,
		BaseURL:       server.URL + "/v1",
		ModelListPath: "/catalog/models",
	}, "provider-secret")
	if err != nil {
		t.Fatalf("discover models: %v", err)
	}
	if authorization != "Bearer provider-secret" {
		t.Fatalf("authorization=%q", authorization)
	}
	if requestPath != "/catalog/models" {
		t.Fatalf("path=%q want /catalog/models", requestPath)
	}
	if len(models) != 2 || models[0].ProviderName != "DeepSeek" || models[1].ProviderName != "DeepSeek" || models[0].DisplayName != "deepseek-chat" || models[1].DisplayName != "DeepSeek Reasoner" {
		t.Fatalf("unexpected discovered models: %+v", models)
	}
}

func TestDiscoverIgnoresUpstreamPricing(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"vendor-model","pricing":{"unit":"rmb_per_million_tokens","input":"1.25","output":8,"cache_read":0.1}}]}`))
	}))
	defer server.Close()

	service := NewService(nil, nil, server.Client())
	models, err := service.discover(context.Background(), &ent.Provider{DisplayName: "Vendor", Protocol: entprovider.ProtocolOpenai, BaseURL: server.URL}, "provider-secret")
	if err != nil || len(models) != 1 {
		t.Fatalf("discover models=%+v err=%v", models, err)
	}
	if models[0].UpstreamName != "vendor-model" || models[0].ProviderName != "Vendor" {
		t.Fatalf("unexpected discovered model: %+v", models[0])
	}
}

func TestDiscoverReportsContextDeadline(t *testing.T) {
	service := NewService(nil, nil, &http.Client{Transport: blockingRoundTripper{}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := service.discover(ctx, &ent.Provider{DisplayName: "slow", Protocol: entprovider.ProtocolOpenai, BaseURL: "https://models.example.com/v1"}, "provider-secret")
	var discoveryError *DiscoveryError
	if !errors.As(err, &discoveryError) || discoveryError.Reason != "同步模型超时，请检查上游连接或模型列表路径" {
		t.Fatalf("discover error=%v", err)
	}
}

func TestDiscoverReportsUnauthorizedCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_api_key"}}`))
	}))
	defer server.Close()

	service := NewService(nil, nil, server.Client())
	_, err := service.discover(context.Background(), &ent.Provider{
		DisplayName: "Test",
		Protocol:    entprovider.ProtocolOpenai,
		BaseURL:     server.URL,
	}, "provider-secret")
	var discoveryError *DiscoveryError
	if !errors.As(err, &discoveryError) {
		t.Fatalf("discover error=%v, want DiscoveryError", err)
	}
	if discoveryError.StatusCode != http.StatusUnauthorized || discoveryError.Reason != "上游 API Key 无效或已撤销，请在提供商配置中重新填写上游密钥" {
		t.Fatalf("unexpected unauthorized error: %+v", discoveryError)
	}
}

type blockingRoundTripper struct{}

func (blockingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	<-request.Context().Done()
	return nil, request.Context().Err()
}

func TestModelListURLHandlesSupportedProtocols(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		protocol provider.Protocol
		want     string
	}{
		{name: "OpenAI root base", baseURL: "https://api.example.com", protocol: provider.ProtocolOpenAI, want: "https://api.example.com/v1/models"},
		{name: "OpenAI versioned base", baseURL: "https://api.example.com/v1", protocol: provider.ProtocolOpenAI, want: "https://api.example.com/v1/models"},
		{name: "Anthropic root base", baseURL: "https://api.anthropic.com", protocol: provider.ProtocolAnthropic, want: "https://api.anthropic.com/v1/models"},
		{name: "HTTP self-hosted base", baseURL: "http://203.0.113.10:3000/v1", protocol: provider.ProtocolOpenAI, want: "http://203.0.113.10:3000/v1/models"},
		{name: "Already models path", baseURL: "https://models.example.com/v1/models", protocol: provider.ProtocolOpenAI, want: "https://models.example.com/v1/models"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := modelListURL(tt.baseURL, tt.protocol, "")
			if err != nil || got != tt.want {
				t.Fatalf("modelListURL()=(%q, %v), want %q", got, err, tt.want)
			}
		})
	}
	if _, err := modelListURL("http://127.0.0.1:8080", provider.ProtocolOpenAI, ""); err != nil {
		t.Fatalf("expected HTTP URL format to be accepted before network filtering: %v", err)
	}
	custom, err := modelListURL("https://models.example.com/v1", provider.ProtocolOpenAI, "/api/catalog")
	if err != nil || custom != "https://models.example.com/api/catalog" {
		t.Fatalf("custom model list URL=%q err=%v", custom, err)
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

func TestCatalogProviderNameFollowsAggregatedModelVendor(t *testing.T) {
	configured := &ent.Provider{DisplayName: "1024token", BaseURL: "https://1024token.net/v1"}
	for _, test := range []struct {
		model string
		want  string
	}{
		{model: "deepseek-v4-flash:cloud", want: "DeepSeek"},
		{model: "glm-5.1:cloud", want: "智谱 GLM"},
		{model: "kimi-k2.7-code", want: "Kimi"},
	} {
		t.Run(test.model, func(t *testing.T) {
			if got := catalogProviderNameForModel(configured, test.model); got != test.want {
				t.Fatalf("catalog provider=%q want %q", got, test.want)
			}
		})
	}
}

func TestUniqueIDsAndAutomaticPublicNames(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	ids := uniqueIDs([]uuid.UUID{first, uuid.Nil, first, second})
	if len(ids) != 2 || ids[0] != first || ids[1] != second {
		t.Fatalf("unexpected unique IDs: %v", ids)
	}
	for _, name := range []string{"kimi-k3", "vendor/model:latest", "A_very.long-model"} {
		if !validAutomaticPublicName(name) {
			t.Fatalf("expected valid automatic model ID %q", name)
		}
	}
	for _, name := range []string{"", "x", "bad model", "/starts-wrong", "中文模型"} {
		if validAutomaticPublicName(name) {
			t.Fatalf("expected invalid automatic model ID %q", name)
		}
	}
}
