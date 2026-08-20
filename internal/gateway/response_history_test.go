package gateway

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestResponseHistoryStoreExpandsTextAndToolChains(t *testing.T) {
	t.Parallel()
	store := newResponseHistoryStore()
	apiKeyID := uuid.New()
	now := time.Date(2026, time.August, 20, 2, 0, 0, 0, time.UTC)
	firstPayload := map[string]any{"input": "write the file"}
	firstResponse := map[string]any{
		"id":     "resp_tool",
		"status": "completed",
		"output": []any{map[string]any{
			"type": "function_call", "call_id": "call_write", "name": "bash", "arguments": `{"command":"write"}`,
		}},
	}
	if err := store.remember(apiKeyID, firstPayload, firstResponse, now); err != nil {
		t.Fatalf("remember first response: %v", err)
	}

	secondPayload := map[string]any{
		"previous_response_id": "resp_tool",
		"input": []any{map[string]any{
			"type": "function_call_output", "call_id": "call_write", "output": "123456",
		}},
	}
	expandedSecond, err := store.expand(apiKeyID, secondPayload, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expand tool response: %v", err)
	}
	if _, exists := expandedSecond["previous_response_id"]; exists {
		t.Fatal("expanded request retained previous_response_id")
	}
	secondInput := sliceValue(expandedSecond["input"])
	if len(secondInput) != 3 {
		t.Fatalf("expanded tool input count = %d, want 3: %#v", len(secondInput), secondInput)
	}
	if stringValue(mapValue(secondInput[0])["type"]) != "message" || stringValue(mapValue(secondInput[1])["type"]) != "function_call" || stringValue(mapValue(secondInput[2])["type"]) != "function_call_output" {
		t.Fatalf("expanded tool input order = %#v", secondInput)
	}

	secondResponse := map[string]any{
		"id":     "resp_final",
		"status": "completed",
		"output": []any{map[string]any{
			"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "done"}},
		}},
	}
	if err := store.remember(apiKeyID, secondPayload, secondResponse, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("remember second response: %v", err)
	}
	expandedThird, err := store.expand(apiKeyID, map[string]any{"previous_response_id": "resp_final", "input": "next"}, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("expand chained response: %v", err)
	}
	thirdInput := sliceValue(expandedThird["input"])
	if len(thirdInput) != 5 {
		t.Fatalf("expanded chained input count = %d, want 5: %#v", len(thirdInput), thirdInput)
	}
	if textFromContent(mapValue(thirdInput[4])["content"]) != "next" {
		t.Fatalf("current input was not appended last: %#v", thirdInput)
	}
}

func TestResponseHistoryStoreScopesAndExpiresEntries(t *testing.T) {
	t.Parallel()
	store := newResponseHistoryStore()
	apiKeyID := uuid.New()
	now := time.Date(2026, time.August, 20, 2, 0, 0, 0, time.UTC)
	payload := map[string]any{"input": "private"}
	response := map[string]any{"id": "resp_private", "status": "completed", "output": []any{}}
	if err := store.remember(apiKeyID, payload, response, now); err != nil {
		t.Fatalf("remember response: %v", err)
	}
	request := map[string]any{"previous_response_id": "resp_private", "input": "continue"}
	if _, err := store.expand(uuid.New(), request, now); !errors.Is(err, errPreviousResponseUnavailable) {
		t.Fatalf("cross-key expand error = %v, want unavailable", err)
	}
	if _, err := store.expand(apiKeyID, request, now.Add(responseHistoryRetentionPeriod)); !errors.Is(err, errPreviousResponseUnavailable) {
		t.Fatalf("expired expand error = %v, want unavailable", err)
	}
}
