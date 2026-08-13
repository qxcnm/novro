package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type pingerFunc func(context.Context) error

/**
 * PingContext 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (f pingerFunc) PingContext(ctx context.Context) error {
	return f(ctx)
}

/**
 * TestHealthEndpoints 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestHealthEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		pinger     pingerFunc
		wantStatus int
		wantBody   string
	}{
		{
			name:       "liveness does not depend on database",
			path:       "/healthz",
			pinger:     func(context.Context) error { return errors.New("unavailable") },
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}` + "\n",
		},
		{
			name:       "readiness succeeds with database",
			path:       "/readyz",
			pinger:     func(context.Context) error { return nil },
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}` + "\n",
		},
		{
			name:       "readiness hides database error",
			path:       "/readyz",
			pinger:     func(context.Context) error { return errors.New("secret database detail") },
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"not_ready"}` + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			response := httptest.NewRecorder()
			NewHealthHandler(tt.pinger).ServeHTTP(response, request)
			if response.Code != tt.wantStatus || response.Body.String() != tt.wantBody {
				t.Fatalf("got status %d body %q", response.Code, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("missing no-store cache policy")
			}
		})
	}
}
