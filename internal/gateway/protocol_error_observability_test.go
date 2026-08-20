package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/apikey"
	"github.com/novro-gateway/novro/internal/requestid"
)

func TestServeHTTPSelectsNativeErrorEnvelopeBeforeAuthentication(t *testing.T) {
	handler := New(Dependencies{APIKeys: fakeKeys{err: apikey.ErrUnauthenticated}})
	tests := []struct {
		path          string
		wantRootType  string
		wantErrorType string
	}{
		{path: "/v1/chat/completions", wantErrorType: "authentication_error"},
		{path: "/v1/responses", wantErrorType: "authentication_error"},
		{path: "/v1/messages", wantRootType: "error", wantErrorType: "authentication_error"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{}`)))
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if response.Code != http.StatusUnauthorized || stringValue(body["type"]) != test.wantRootType || stringValue(mapValue(body["error"])["type"]) != test.wantErrorType {
				t.Fatalf("status=%d body=%+v", response.Code, body)
			}
			if response.Header().Get(requestid.Header) == "" || response.Header().Get(novroErrorCodeHeader) != "invalid_api_key" {
				t.Fatalf("headers=%+v", response.Header())
			}
		})
	}
}

func TestWriteErrorUsesOpenAIEnvelopeForChatAndResponses(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/responses"} {
		path := path
		t.Run(path, func(t *testing.T) {
			requestID := uuid.New()
			response := httptest.NewRecorder()
			response.Header().Set(requestid.Header, requestID.String())
			writer := withProtocolResponseWriter(response, path)

			writeError(writer, http.StatusBadRequest, "invalid_request", "safe message")

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
			if got := response.Header().Get(novroErrorCodeHeader); got != "invalid_request" {
				t.Fatalf("%s = %q", novroErrorCodeHeader, got)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if _, exists := body["type"]; exists {
				t.Fatalf("OpenAI error has Anthropic top-level type: %+v", body)
			}
			errorObject := mapValue(body["error"])
			if stringValue(errorObject["type"]) != "invalid_request_error" || stringValue(errorObject["code"]) != "invalid_request" || stringValue(errorObject["message"]) != "safe message" {
				t.Fatalf("unexpected OpenAI error: %+v", body)
			}
			if value, exists := errorObject["param"]; !exists || value != nil {
				t.Fatalf("OpenAI error param = %#v, exists=%v", value, exists)
			}
			if stringValue(body["request_id"]) != requestID.String() {
				t.Fatalf("request_id = %q", stringValue(body["request_id"]))
			}
		})
	}
}

func TestWriteErrorUsesAnthropicEnvelopeForMessages(t *testing.T) {
	requestID := uuid.New()
	response := httptest.NewRecorder()
	response.Header().Set(requestid.Header, requestID.String())
	writer := withProtocolResponseWriter(response, "/v1/messages")

	writeError(writer, http.StatusUnauthorized, "invalid_api_key", "safe message")

	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if stringValue(body["type"]) != "error" || stringValue(body["request_id"]) != requestID.String() {
		t.Fatalf("unexpected Anthropic envelope: %+v", body)
	}
	errorObject := mapValue(body["error"])
	if stringValue(errorObject["type"]) != "authentication_error" || stringValue(errorObject["code"]) != "invalid_api_key" || stringValue(errorObject["message"]) != "safe message" {
		t.Fatalf("unexpected Anthropic error: %+v", body)
	}
	if _, exists := errorObject["param"]; exists {
		t.Fatalf("Anthropic error contains OpenAI param: %+v", body)
	}
	if got := response.Header().Get(novroErrorCodeHeader); got != "invalid_api_key" {
		t.Fatalf("%s = %q", novroErrorCodeHeader, got)
	}
}

func TestConversionLostFieldsReportsPathsWithoutValues(t *testing.T) {
	secret := "top-secret-prompt-and-token"
	payload := map[string]any{
		"model": "test",
		"input": []any{
			map[string]any{"type": "item_reference", "id": secret},
			map[string]any{"type": "message", "role": "user", "content": []any{
				map[string]any{"type": "input_file", "file_id": secret, "filename": "secret.txt"},
			}},
		},
		"tools":            []any{map[string]any{"type": "web_search", "filters": map[string]any{"allowed_domains": []any{secret}}}},
		"prompt_cache_key": secret,
	}

	fields := requestConversionLostFields(payload, "responses", "chat_completions")
	joinedFields := strings.Join(fields, ",")
	for _, want := range []string{"input[].content[].input_file", "input[].item_reference", "prompt_cache_key"} {
		if !strings.Contains(joinedFields, want) {
			t.Errorf("fields %q do not contain %q", joinedFields, want)
		}
	}
	if !strings.Contains(joinedFields, "tools[].filters") {
		t.Fatalf("web search filter loss was not reported: %q", joinedFields)
	}
	if strings.Contains(joinedFields, secret) || strings.Contains(joinedFields, "secret.txt") {
		t.Fatalf("lost-fields report exposed request content: %q", joinedFields)
	}

	response := httptest.NewRecorder()
	setConversionLostFieldsHeader(response, fields)
	if got := response.Header().Get(ConversionLostFieldsHeader); got != "4" {
		t.Fatalf("observable header = %q, want 4", got)
	}
}

func TestResponseConversionLostFieldsMergesAndIsBounded(t *testing.T) {
	body := []byte(`{"id":"resp_1","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","content":[{"type":"refusal","refusal":"sensitive refusal","annotations":[{"url":"https://secret.example"}]}]},{"type":"web_search_call","query":"secret query"}]}`)
	fields := responseConversionLostFields(body, "responses", "chat_completions")
	response := httptest.NewRecorder()
	response.Header().Set(ConversionLostFieldsHeader, "1")
	mergeConversionLostFieldsHeader(response, fields)
	header := response.Header().Get(ConversionLostFieldsHeader)
	joinedFields := strings.Join(fields, ",")
	for _, want := range []string{"output[].web_search_call"} {
		if !strings.Contains(joinedFields, want) {
			t.Errorf("fields %q do not contain %q", joinedFields, want)
		}
	}
	if header != "2" {
		t.Fatalf("merged lost-fields count = %q, want 2", header)
	}
	for _, sensitive := range []string{"sensitive refusal", "secret.example", "secret query"} {
		if strings.Contains(joinedFields, sensitive) || strings.Contains(header, sensitive) {
			t.Fatalf("lost-fields observability exposed response content: fields=%q header=%q", joinedFields, header)
		}
	}
}

func TestConversionLostFieldsOmitsPortableRichFeatures(t *testing.T) {
	responsesRequest := map[string]any{
		"model": "source",
		"input": []any{map[string]any{"type": "message", "role": "user", "content": []any{
			map[string]any{"type": "input_file", "file_url": "https://example.test/file.pdf"},
			map[string]any{"type": "input_text", "text": "summarize"},
		}}, map[string]any{"id": "ws_1", "type": "web_search_call", "status": "completed", "action": map[string]any{"type": "search", "query": "novro"}}},
		"tools": []any{map[string]any{"type": "web_search_preview"}},
	}
	if fields := requestConversionLostFields(responsesRequest, "responses", "messages"); len(fields) != 0 {
		t.Fatalf("Responses -> Messages portable document/Web Search reported as lost: %v", fields)
	}

	responsesResponse := []byte(`{"id":"resp_1","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"answer","annotations":[{"type":"url_citation","url":"https://example.test"}]},{"type":"refusal","refusal":"no"}]},{"type":"web_search_call","status":"completed","action":{"type":"search","query":"q"}}]}`)
	if fields := responseConversionLostFields(responsesResponse, "responses", "messages"); len(fields) != 0 {
		t.Fatalf("Responses -> Messages refusal/citation/Web Search reported as lost: %v", fields)
	}

	chatResponse := []byte(`{"id":"chat_1","choices":[{"message":{"role":"assistant","content":"answer","refusal":"no","annotations":[{"type":"url_citation","url":"https://example.test"}]},"finish_reason":"content_filter"}]}`)
	if fields := responseConversionLostFields(chatResponse, "chat_completions", "responses"); len(fields) != 0 {
		t.Fatalf("Chat -> Responses refusal/annotations reported as lost: %v", fields)
	}
}
