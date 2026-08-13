package requestid

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

/**
 * TestMiddlewareGeneratesAndReusesRequestID 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestMiddlewareGeneratesAndReusesRequestID(t *testing.T) {
	wanted := uuid.New()
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := FromContext(r.Context()); got != wanted {
			t.Fatalf("context request id=%s want=%s", got, wanted)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(WithContext(request.Context(), wanted))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Header().Get(Header); got != wanted.String() {
		t.Fatalf("request id header=%q want=%q", got, wanted)
	}
}

/**
 * TestMiddlewareIgnoresClientRequestID 验证对应功能在指定场景下的行为。
 * @param t 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func TestMiddlewareIgnoresClientRequestID(t *testing.T) {
	clientID := uuid.New().String()
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := FromContext(r.Context()); got == uuid.Nil || got.String() == clientID {
			t.Fatalf("unexpected generated request id %s", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(Header, clientID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Header().Get(Header); got == "" || got == clientID {
		t.Fatalf("request id header=%q", got)
	}
}
