package gateway

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/novro-gateway/novro/internal/requestid"
)

const novroErrorCodeHeader = "X-Novro-Error-Code"

// protocolResponseWriter remembers the public protocol selected by the request
// path. Keeping this on the writer lets all existing error exits use one safe
// writer while still returning the envelope expected by the calling SDK.
type protocolResponseWriter struct {
	http.ResponseWriter
	protocol string
}

type protocolFlushingResponseWriter struct {
	*protocolResponseWriter
	flusher http.Flusher
}

func (w *protocolFlushingResponseWriter) Flush() {
	w.flusher.Flush()
}

func (w *protocolResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func withProtocolResponseWriter(w http.ResponseWriter, path string) http.ResponseWriter {
	protocol := protocolForPublicPath(path)
	if protocol == "" {
		return w
	}
	wrapped := &protocolResponseWriter{ResponseWriter: w, protocol: protocol}
	if flusher, ok := w.(http.Flusher); ok {
		return &protocolFlushingResponseWriter{protocolResponseWriter: wrapped, flusher: flusher}
	}
	return wrapped
}

func protocolForPublicPath(path string) string {
	switch path {
	case "/v1/chat/completions":
		return "chat_completions"
	case "/v1/responses":
		return "responses"
	case "/v1/messages":
		return "messages"
	default:
		return ""
	}
}

func responseProtocol(w http.ResponseWriter) string {
	if value, ok := w.(interface{ responseProtocol() string }); ok {
		return value.responseProtocol()
	}
	if value, ok := w.(*protocolResponseWriter); ok {
		return value.protocol
	}
	return ""
}

func (w *protocolResponseWriter) responseProtocol() string {
	return w.protocol
}

// writeProtocolError writes only Novro-authored, user-safe messages. Upstream
// response bodies and internal errors never reach this function.
func writeProtocolError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set(novroErrorCodeHeader, code)
	requestID := requestid.ResponseID(w)
	if responseProtocol(w) == "messages" {
		value := map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    anthropicErrorType(status),
				"message": message,
				// Anthropic SDKs ignore additional object members. Retaining the
				// stable Novro code keeps existing clients machine-readable while
				// the envelope and required fields remain Anthropic-native.
				"code": code,
			},
		}
		if requestID != uuid.Nil {
			value["request_id"] = requestID.String()
		}
		writeJSON(w, status, value)
		return
	}

	// Chat Completions and Responses share OpenAI's error envelope. The code is
	// Novro's stable machine-readable code; param is present for SDKs which
	// decode the full OpenAI error object.
	value := map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    openAIErrorType(status),
			"param":   nil,
			"code":    code,
		},
	}
	if requestID != uuid.Nil {
		value["request_id"] = requestID.String()
	}
	writeJSON(w, status, value)
}

func openAIErrorType(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		if status >= http.StatusInternalServerError {
			return "server_error"
		}
		return "invalid_request_error"
	}
}

func anthropicErrorType(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusServiceUnavailable:
		return "overloaded_error"
	default:
		if status >= http.StatusInternalServerError {
			return "api_error"
		}
		return "invalid_request_error"
	}
}
