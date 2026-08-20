package gateway

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestProtocolResponsePreservesSourcePartOrder(t *testing.T) {
	t.Parallel()

	t.Run("responses to messages keeps web search before final text and function call", func(t *testing.T) {
		t.Parallel()
		body := mustTestJSON(t, map[string]any{
			"id": "resp_ordered", "object": "response", "status": "completed", "model": "source",
			"output": []any{
				map[string]any{"id": "ws_first", "type": "web_search_call", "status": "completed", "action": map[string]any{
					"type": "search", "query": "Novro", "sources": []any{map[string]any{"type": "url", "url": "https://example.test/source", "title": "Source"}},
				}},
				map[string]any{"id": "msg_after", "type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "after search"}}},
				map[string]any{"id": "fc_after", "type": "function_call", "call_id": "call_after", "name": "finish", "arguments": `{}`},
			},
		})
		converted, err := adaptProtocolResponse(body, "responses", "messages")
		if err != nil {
			t.Fatalf("convert ordered Responses response: %v", err)
		}
		content := sliceValue(decodeTestObject(t, converted)["content"])
		got := responseContentTypes(content)
		want := []string{"server_tool_use", "web_search_tool_result", "text", "tool_use"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Messages content order = %v, want %v; body=%s", got, want, converted)
		}
	})

	t.Run("messages to responses keeps interleaved output item order", func(t *testing.T) {
		t.Parallel()
		body := mustTestJSON(t, map[string]any{
			"id": "msg_ordered", "type": "message", "role": "assistant", "model": "source", "stop_reason": "tool_use",
			"content": []any{
				map[string]any{"type": "thinking", "thinking": "plan", "signature": "signed-plan"},
				map[string]any{"type": "server_tool_use", "id": "ws_first", "name": "web_search", "input": map[string]any{"query": "Novro"}},
				map[string]any{"type": "web_search_tool_result", "tool_use_id": "ws_first", "content": []any{map[string]any{"type": "web_search_result", "url": "https://example.test/source", "title": "Source"}}},
				map[string]any{"type": "text", "text": "after search"},
				map[string]any{"type": "tool_use", "id": "call_after", "name": "finish", "input": map[string]any{}},
			},
		})
		converted, err := adaptProtocolResponse(body, "messages", "responses")
		if err != nil {
			t.Fatalf("convert ordered Messages response: %v", err)
		}
		output := sliceValue(decodeTestObject(t, converted)["output"])
		got := responseContentTypes(output)
		want := []string{"reasoning", "web_search_call", "message", "function_call"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Responses output order = %v, want %v; body=%s", got, want, converted)
		}
	})
}

func TestResponsesResponsePreservesMessageBoundariesAndFunctionOutputs(t *testing.T) {
	t.Parallel()

	body := mustTestJSON(t, map[string]any{
		"id": "resp_boundaries", "object": "response", "status": "completed", "model": "source",
		"output": []any{
			map[string]any{"id": "msg_first", "type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "first"}}},
			map[string]any{"id": "msg_second", "type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "second"}}},
			map[string]any{"id": "fco_result", "type": "function_call_output", "call_id": "call_lookup", "output": `{"value":42}`, "status": "completed"},
		},
	})
	decoded, err := decodeAdapterResponse(body, "responses")
	if err != nil {
		t.Fatalf("decode Responses response: %v", err)
	}
	encoded := encodeAdapterResponse(decoded, "responses")
	output := sliceValue(encoded["output"])
	if got := responseContentTypes(output); !reflect.DeepEqual(got, []string{"message", "message", "function_call_output"}) {
		t.Fatalf("Responses output types = %v; output=%#v", got, output)
	}
	if stringValue(mapValue(output[0])["id"]) != "msg_first" || stringValue(mapValue(output[1])["id"]) != "msg_second" {
		t.Fatalf("message item IDs not preserved: %#v", output)
	}
	functionOutput := mapValue(output[2])
	if stringValue(functionOutput["id"]) != "fco_result" || stringValue(functionOutput["call_id"]) != "call_lookup" || stringValue(functionOutput["output"]) != `{"value":42}` {
		t.Fatalf("function_call_output not preserved: %#v", functionOutput)
	}
}

func TestResponseOpenAIAnnotationsMergesAndDeduplicatesSources(t *testing.T) {
	t.Parallel()

	shared := map[string]any{"type": "url_citation", "url": "https://example.test/shared", "title": "Shared", "start_index": 0, "end_index": 6}
	additional := adapterCitation{Type: "url_citation", URL: "https://example.test/additional", Title: "Additional"}
	response := adapterResponse{
		Annotations: []any{shared},
		Citations:   append(decodeAdapterCitations([]any{shared}), additional),
	}
	annotations := responseOpenAIAnnotations(response)
	if len(annotations) != 2 {
		encoded, _ := json.Marshal(annotations)
		t.Fatalf("merged annotations count = %d, want 2: %s", len(annotations), encoded)
	}
	if stringValue(mapValue(annotations[0])["url"]) != "https://example.test/shared" || stringValue(mapValue(annotations[1])["url"]) != "https://example.test/additional" {
		t.Fatalf("merged annotations = %#v", annotations)
	}
}

func TestResponseConversionLossReportsUnrepresentableOutput(t *testing.T) {
	t.Parallel()

	chatBody := mustTestJSON(t, map[string]any{
		"choices": []any{
			map[string]any{"message": map[string]any{
				"content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.test/image.png"}}},
				"audio":   map[string]any{"id": "audio_1"},
			}, "logprobs": map[string]any{"content": []any{}}},
			map[string]any{"message": map[string]any{"content": "second choice"}},
		},
		"usage": map[string]any{
			"prompt_tokens_details":     map[string]any{"cached_tokens": 2, "audio_tokens": 3},
			"completion_tokens_details": map[string]any{"reasoning_tokens": 4, "audio_tokens": 5, "accepted_prediction_tokens": 6},
		},
	})
	chatLosses := responseConversionLostFields(chatBody, "chat_completions", "responses")
	for _, want := range []string{
		"choices[]", "choices[].logprobs", "choices[].message.audio", "choices[].message.content[].image_url",
		"usage.prompt_tokens_details.audio_tokens", "usage.completion_tokens_details.audio_tokens", "usage.completion_tokens_details.accepted_prediction_tokens",
	} {
		if !containsResponseLoss(chatLosses, want) {
			t.Errorf("Chat response losses %v do not contain %q", chatLosses, want)
		}
	}

	responsesBody := []byte(`{"output":[{"id":"fco_1","type":"function_call_output","call_id":"call_1","output":"result"}]}`)
	for _, target := range []string{"chat_completions", "messages"} {
		if losses := responseConversionLostFields(responsesBody, "responses", target); !containsResponseLoss(losses, "output[].function_call_output") {
			t.Errorf("Responses -> %s function output losses = %v", target, losses)
		}
	}

	messagesBody := []byte(`{"content":[{"type":"image"},{"type":"audio"},{"type":"document"},{"type":"thinking","thinking":"plan","signature":"opaque"},{"type":"redacted_thinking","data":"opaque"}]}`)
	messagesLosses := responseConversionLostFields(messagesBody, "messages", "responses")
	for _, want := range []string{"content[].image", "content[].audio", "content[].document", "content[].thinking.signature", "content[].redacted_thinking"} {
		if !containsResponseLoss(messagesLosses, want) {
			t.Errorf("Messages response losses %v do not contain %q", messagesLosses, want)
		}
	}
}

func containsResponseLoss(fields []string, want string) bool {
	for _, field := range fields {
		if field == want {
			return true
		}
	}
	return false
}

func responseContentTypes(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, stringValue(mapValue(value)["type"]))
	}
	return result
}
