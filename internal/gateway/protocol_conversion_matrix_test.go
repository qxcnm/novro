package gateway

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// These tests intentionally exercise every directed edge of the protocol
// graph.  The fixtures contain portable content plus protocol-specific
// features, so a successful conversion must preserve the portable subset while
// allowing a target to degrade fields it cannot express.
func TestProtocolConversionMatrixRequestsPreservesPortableConversation(t *testing.T) {
	protocols := []string{"chat_completions", "responses", "messages"}
	for _, source := range protocols {
		source := source
		t.Run(source, func(t *testing.T) {
			payload := matrixRequestFixture(source)
			for _, target := range protocols {
				target := target
				if target == source {
					continue
				}
				t.Run(source+"_to_"+target, func(t *testing.T) {
					body, err := adaptProtocolRequest(payload, source, target, "matrix-target", false)
					if err != nil {
						t.Fatalf("request conversion: %v", err)
					}
					converted := matrixDecodeObject(t, body)
					encoded := string(body)
					for _, want := range []string{"matrix prompt", "call_left", "call_right", "get_left", "get_right", "left-result", "right-result"} {
						if !strings.Contains(encoded, want) {
							t.Errorf("%s -> %s lost portable value %q: %s", source, target, want, body)
						}
					}
					if source == "messages" {
						if !strings.Contains(encoded, "data:image/png;base64,aW1hZ2U=") {
							t.Errorf("%s -> %s lost portable image data: %s", source, target, body)
						}
					} else if !strings.Contains(encoded, "https://example.test/matrix.png") {
						t.Errorf("%s -> %s lost portable image URL: %s", source, target, body)
					}
					if source == "messages" && target == "responses" && !strings.Contains(encoded, "ZG9jdW1lbnQ=") {
						t.Errorf("Messages document did not become Responses input_file: %s", body)
					}
					if source == "responses" && target == "messages" && !strings.Contains(encoded, "https://example.test/matrix.pdf") {
						t.Errorf("Responses input_file did not become Messages document: %s", body)
					}
					if tools := sliceValue(converted["tools"]); len(tools) > 0 && target != "messages" {
						if !strings.Contains(string(body), "strict") {
							t.Errorf("strict tool contract was dropped by %s -> %s: %s", source, target, body)
						}
					}

					// A second edge must remain usable after the first conversion. This
					// catches adapters which emit a shape accepted by JSON but not by
					// their own decoder (especially tool-result coalescing).
					back, err := adaptProtocolRequest(converted, target, source, "roundtrip-model", false)
					if err != nil {
						t.Fatalf("roundtrip %s -> %s -> %s: %v", source, target, source, err)
					}
					backText := string(back)
					for _, want := range []string{"matrix prompt", "call_left", "call_right"} {
						if !strings.Contains(backText, want) {
							t.Errorf("roundtrip %s -> %s -> %s lost %q: %s", source, target, source, want, back)
						}
					}
				})
			}
		})
	}
}

func TestProtocolConversionMatrixResponsesPreservesRefusalToolsUsageAndStop(t *testing.T) {
	protocols := []string{"chat_completions", "responses", "messages"}
	for _, source := range protocols {
		source := source
		t.Run(source, func(t *testing.T) {
			wantInputTokens := 12
			if source == "messages" {
				// Anthropic reports uncached input separately. The adapter's
				// portable input total includes cache-read tokens before mapping
				// to OpenAI usage fields.
				wantInputTokens = 14
			}
			for _, target := range protocols {
				target := target
				if target == source {
					continue
				}
				t.Run(source+"_to_"+target, func(t *testing.T) {
					body, err := json.Marshal(matrixResponseFixture(source))
					if err != nil {
						t.Fatal(err)
					}
					convertedBody, err := adaptProtocolResponse(body, source, target)
					if err != nil {
						t.Fatalf("response conversion: %v", err)
					}
					converted := matrixDecodeObject(t, convertedBody)
					encoded := string(convertedBody)
					for _, want := range []string{"matrix answer", "cannot disclose", "call_left", "call_right", "get_left", "get_right"} {
						if !strings.Contains(encoded, want) {
							t.Errorf("%s -> %s lost response value %q: %s", source, target, want, convertedBody)
						}
					}
					if source != "messages" && !strings.Contains(encoded, "https://example.test/citation") {
						t.Errorf("%s -> %s lost citation URL: %s", source, target, convertedBody)
					}
					if target == "chat_completions" {
						choice := mapValue(sliceValue(converted["choices"])[0])
						if stringValue(choice["finish_reason"]) != "content_filter" {
							t.Errorf("Chat finish_reason = %#v, want content_filter", choice["finish_reason"])
						}
						usage := mapValue(converted["usage"])
						if intValue(usage["prompt_tokens"]) != wantInputTokens || intValue(usage["completion_tokens"]) != 6 || intValue(usage["total_tokens"]) != wantInputTokens+6 {
							t.Errorf("Chat usage = %#v, want %d/6/%d", usage, wantInputTokens, wantInputTokens+6)
						}
					}
					if target == "responses" {
						if stringValue(converted["status"]) != "incomplete" || stringValue(mapValue(converted["incomplete_details"])["reason"]) != "content_filter" {
							t.Errorf("Responses incomplete state = %#v/%#v", converted["status"], converted["incomplete_details"])
						}
						usage := mapValue(converted["usage"])
						if intValue(usage["input_tokens"]) != wantInputTokens || intValue(usage["output_tokens"]) != 6 || intValue(usage["total_tokens"]) != wantInputTokens+6 {
							t.Errorf("Responses usage = %#v, want %d/6/%d", usage, wantInputTokens, wantInputTokens+6)
						}
					}
					if target == "messages" {
						if stringValue(converted["stop_reason"]) != "refusal" {
							t.Errorf("Messages stop_reason = %#v, want refusal", converted["stop_reason"])
						}
						usage := mapValue(converted["usage"])
						if intValue(usage["input_tokens"]) != 12 || intValue(usage["output_tokens"]) != 6 {
							t.Errorf("Messages usage = %#v, want 12/6", usage)
						}
					}
				})
			}
		})
	}
}

func TestProtocolConversionMatrixCitationsPreserveURL(t *testing.T) {
	protocols := []string{"chat_completions", "responses", "messages"}
	for _, source := range protocols {
		for _, target := range protocols {
			if source == target {
				continue
			}
			t.Run(source+"_to_"+target, func(t *testing.T) {
				body, err := json.Marshal(matrixCitationResponseFixture(source))
				if err != nil {
					t.Fatal(err)
				}
				converted, err := adaptProtocolResponse(body, source, target)
				if err != nil {
					t.Fatalf("citation conversion: %v", err)
				}
				for _, want := range []string{"cited answer", "https://example.test/citation", "Citation"} {
					if !strings.Contains(string(converted), want) {
						t.Errorf("%s -> %s lost citation value %q: %s", source, target, want, converted)
					}
				}
			})
		}
	}
}

func TestProtocolConversionMatrixWebSearchResponseRoundTrip(t *testing.T) {
	responsesBody := map[string]any{
		"id": "web-matrix", "model": "matrix", "status": "completed",
		"output": []any{map[string]any{
			"id": "ws_matrix", "type": "web_search_call", "status": "completed",
			"action": map[string]any{"type": "search", "query": "protocol matrix", "sources": []any{map[string]any{"type": "url", "url": "https://example.test/web", "title": "Web"}}},
		}},
	}
	messagesBody, err := adaptProtocolResponse(contractMustMarshal(t, responsesBody), "responses", "messages")
	if err != nil {
		t.Fatalf("Responses Web Search -> Messages: %v", err)
	}
	for _, want := range []string{"server_tool_use", "web_search_tool_result", "ws_matrix", "protocol matrix", "https://example.test/web"} {
		if !strings.Contains(string(messagesBody), want) {
			t.Errorf("Messages Web Search response lost %q: %s", want, messagesBody)
		}
	}

	roundTrip, err := adaptProtocolResponse(messagesBody, "messages", "responses")
	if err != nil {
		t.Fatalf("Messages Web Search -> Responses: %v", err)
	}
	root := matrixDecodeObject(t, roundTrip)
	output := sliceValue(root["output"])
	if len(output) != 1 {
		t.Fatalf("round-trip Web Search output = %#v, want one combined item", output)
	}
	item := mapValue(output[0])
	action := mapValue(item["action"])
	if stringValue(item["type"]) != "web_search_call" || stringValue(item["id"]) != "ws_matrix" || stringValue(action["query"]) != "protocol matrix" {
		t.Errorf("round-trip Web Search item = %#v", item)
	}
	if !strings.Contains(string(roundTrip), "https://example.test/web") {
		t.Errorf("round-trip Web Search lost source URL: %s", roundTrip)
	}
}

func TestProtocolConversionMatrixRefusalSSEHasTargetTerminalLifecycle(t *testing.T) {
	protocols := []string{"chat_completions", "responses", "messages"}
	for _, source := range protocols {
		source := source
		t.Run(source, func(t *testing.T) {
			for _, target := range protocols {
				target := target
				if source == target {
					continue
				}
				t.Run(source+"_to_"+target, func(t *testing.T) {
					adapter := newProtocolStreamAdapter(source, target)
					var output bytes.Buffer
					for _, event := range matrixRefusalStreamFixture(source) {
						translated, err := adapter.Translate(event.name, []byte(event.data))
						if err != nil {
							t.Fatalf("Translate(%s): %v", event.name, err)
						}
						output.Write(translated)
					}
					output.Write(adapter.Finalize())
					stream := output.String()
					if got := matrixStreamText(t, stream); !strings.Contains(got, "cannot disclose") {
						t.Fatalf("%s -> %s lost refusal text: %s", source, target, stream)
					}
					switch target {
					case "chat_completions":
						if !strings.Contains(stream, "[DONE]") || !strings.Contains(stream, `"finish_reason":"content_filter"`) {
							t.Errorf("Chat terminal refusal lifecycle missing: %s", stream)
						}
					case "responses":
						if !strings.Contains(stream, "response.incomplete") || !strings.Contains(stream, `"status":"incomplete"`) || !strings.Contains(stream, `"reason":"content_filter"`) {
							t.Errorf("Responses terminal refusal lifecycle missing: %s", stream)
						}
					case "messages":
						if !strings.Contains(stream, "message_stop") || !strings.Contains(stream, `"stop_reason":"refusal"`) {
							t.Errorf("Messages terminal refusal lifecycle missing: %s", stream)
						}
					}
				})
			}
		})
	}
}

func matrixStreamText(t *testing.T, stream string) string {
	t.Helper()
	var text strings.Builder
	for _, event := range parseExtendedStream(t, stream) {
		if event.raw == "[DONE]" {
			continue
		}
		if deltaText, ok := event.data["delta"].(string); ok {
			if strings.Contains(event.event, "output_text.delta") || strings.Contains(event.event, "refusal.delta") {
				text.WriteString(deltaText)
			}
		} else if delta := mapValue(event.data["delta"]); delta != nil {
			text.WriteString(firstNonEmptyString(delta["refusal"], delta["content"], delta["text"]))
		} else if choices := sliceValue(event.data["choices"]); len(choices) > 0 {
			delta := mapValue(mapValue(choices[0])["delta"])
			if refusal := stringValue(delta["refusal"]); refusal != "" {
				text.WriteString(refusal)
			} else if !strings.Contains(stream, `"delta":{"refusal"`) {
				text.WriteString(stringValue(delta["content"]))
			}
		}
	}
	result := text.String()
	if strings.Contains(stream, "response.refusal.delta") && strings.Contains(stream, "response.output_text.delta") {
		var refusal strings.Builder
		for _, event := range parseExtendedStream(t, stream) {
			if event.event == "response.refusal.delta" {
				refusal.WriteString(stringValue(event.data["delta"]))
			}
		}
		return refusal.String()
	}
	return result
}

type matrixSSEEvent struct {
	name string
	data string
}

func matrixRefusalStreamFixture(source string) []matrixSSEEvent {
	switch source {
	case "chat_completions":
		return []matrixSSEEvent{
			{data: `{"id":"matrix-refusal","model":"k3","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`},
			{data: `{"id":"matrix-refusal","model":"k3","choices":[{"index":0,"delta":{"refusal":"cannot "},"finish_reason":null}]}`},
			{data: `{"id":"matrix-refusal","model":"k3","choices":[{"index":0,"delta":{"refusal":"disclose"},"finish_reason":"content_filter"}],"usage":{"prompt_tokens":12,"completion_tokens":6,"total_tokens":18}}`},
			{data: `[DONE]`},
		}
	case "responses":
		return []matrixSSEEvent{
			{name: "response.created", data: `{"type":"response.created","response":{"id":"matrix-refusal","model":"k3","status":"in_progress"}}`},
			{name: "response.refusal.delta", data: `{"type":"response.refusal.delta","item_id":"msg_refusal","output_index":0,"content_index":0,"delta":"cannot disclose"}`},
			{name: "response.completed", data: `{"type":"response.completed","response":{"id":"matrix-refusal","model":"k3","status":"incomplete","incomplete_details":{"reason":"content_filter"},"usage":{"input_tokens":12,"output_tokens":6,"total_tokens":18}}}`},
		}
	case "messages":
		return []matrixSSEEvent{
			{name: "message_start", data: `{"type":"message_start","message":{"id":"matrix-refusal","model":"k3","role":"assistant","content":[],"usage":{"input_tokens":12,"output_tokens":0}}}`},
			{name: "content_block_start", data: `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
			{name: "content_block_delta", data: `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"cannot disclose"}}`},
			{name: "content_block_stop", data: `{"type":"content_block_stop","index":0}`},
			{name: "message_delta", data: `{"type":"message_delta","delta":{"stop_reason":"refusal","stop_sequence":null},"usage":{"output_tokens":6}}`},
			{name: "message_stop", data: `{"type":"message_stop"}`},
		}
	default:
		return nil
	}
}

func matrixRequestFixture(protocol string) map[string]any {
	parameters := map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}, "required": []any{"q"}}
	switch protocol {
	case "chat_completions":
		return map[string]any{
			"model": "matrix-source", "max_tokens": 64, "stop": []any{"END"}, "parallel_tool_calls": false,
			"messages": []any{
				map[string]any{"role": "system", "content": "matrix system"},
				map[string]any{"role": "user", "content": []any{
					map[string]any{"type": "text", "text": "matrix prompt"},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.test/matrix.png"}},
				}},
				map[string]any{"role": "assistant", "tool_calls": []any{
					map[string]any{"id": "call_left", "type": "function", "function": map[string]any{"name": "get_left", "arguments": `{"q":"left"}`}},
					map[string]any{"id": "call_right", "type": "function", "function": map[string]any{"name": "get_right", "arguments": `{"q":"right"}`}},
				}},
				map[string]any{"role": "tool", "tool_call_id": "call_left", "content": "left-result"},
				map[string]any{"role": "tool", "tool_call_id": "call_right", "content": "right-result"},
			},
			"tools":       []any{map[string]any{"type": "function", "function": map[string]any{"name": "lookup", "parameters": parameters, "strict": true}}},
			"tool_choice": "auto",
		}
	case "responses":
		return map[string]any{
			"model": "matrix-source", "max_output_tokens": 64, "instructions": "matrix system", "stop": []any{"END"}, "parallel_tool_calls": false,
			"input": []any{
				map[string]any{"type": "message", "role": "user", "content": []any{
					map[string]any{"type": "input_text", "text": "matrix prompt"},
					map[string]any{"type": "input_image", "image_url": "https://example.test/matrix.png"},
					map[string]any{"type": "input_file", "file_url": "https://example.test/matrix.pdf", "filename": "matrix.pdf"},
				}},
				map[string]any{"type": "function_call", "call_id": "call_left", "name": "get_left", "arguments": `{"q":"left"}`},
				map[string]any{"type": "function_call", "call_id": "call_right", "name": "get_right", "arguments": `{"q":"right"}`},
				map[string]any{"type": "function_call_output", "call_id": "call_left", "output": "left-result"},
				map[string]any{"type": "function_call_output", "call_id": "call_right", "output": "right-result"},
			},
			"tools":       []any{map[string]any{"type": "function", "name": "lookup", "parameters": parameters, "strict": true}},
			"tool_choice": "auto",
		}
	case "messages":
		return map[string]any{
			"model": "matrix-source", "max_tokens": 64, "system": []any{map[string]any{"type": "text", "text": "matrix system"}}, "stop_sequences": []any{"END"},
			"messages": []any{
				map[string]any{"role": "user", "content": []any{
					map[string]any{"type": "text", "text": "matrix prompt"},
					map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "aW1hZ2U="}},
					map[string]any{"type": "document", "source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": "ZG9jdW1lbnQ="}, "title": "matrix.pdf"},
				}},
				map[string]any{"role": "assistant", "content": []any{
					map[string]any{"type": "tool_use", "id": "call_left", "name": "get_left", "input": map[string]any{"q": "left"}},
					map[string]any{"type": "tool_use", "id": "call_right", "name": "get_right", "input": map[string]any{"q": "right"}},
				}},
				map[string]any{"role": "user", "content": []any{
					map[string]any{"type": "tool_result", "tool_use_id": "call_left", "content": "left-result"},
					map[string]any{"type": "tool_result", "tool_use_id": "call_right", "content": "right-result"},
				}},
			},
			"tools":       []any{map[string]any{"name": "lookup", "input_schema": parameters, "strict": true}},
			"tool_choice": map[string]any{"type": "auto", "disable_parallel_tool_use": true},
		}
	default:
		panic("unknown matrix request protocol: " + protocol)
	}
}

func matrixResponseFixture(protocol string) map[string]any {
	usage := map[string]any{"input_tokens": 12, "output_tokens": 6, "total_tokens": 18, "cached_tokens": 2}
	switch protocol {
	case "chat_completions":
		message := map[string]any{
			"role": "assistant", "content": "matrix answer", "refusal": "cannot disclose",
			"annotations": []any{map[string]any{"type": "url_citation", "url": "https://example.test/citation", "title": "Citation", "start_index": 0, "end_index": 6}},
			"tool_calls": []any{
				map[string]any{"id": "call_left", "type": "function", "function": map[string]any{"name": "get_left", "arguments": `{"q":"left"}`}},
				map[string]any{"id": "call_right", "type": "function", "function": map[string]any{"name": "get_right", "arguments": `{"q":"right"}`}},
			},
		}
		return map[string]any{"id": "matrix-response", "object": "chat.completion", "created": 1700000000, "model": "matrix", "choices": []any{map[string]any{"index": 0, "finish_reason": "content_filter", "message": message}}, "usage": map[string]any{"prompt_tokens": 12, "completion_tokens": 6, "total_tokens": 18}}
	case "responses":
		content := []any{
			map[string]any{"type": "output_text", "text": "matrix answer", "annotations": []any{map[string]any{"type": "url_citation", "url": "https://example.test/citation", "title": "Citation", "start_index": 0, "end_index": 6}}},
			map[string]any{"type": "refusal", "refusal": "cannot disclose"},
		}
		output := []any{
			map[string]any{"type": "message", "role": "assistant", "content": content},
			map[string]any{"type": "function_call", "call_id": "call_left", "name": "get_left", "arguments": `{"q":"left"}`},
			map[string]any{"type": "function_call", "call_id": "call_right", "name": "get_right", "arguments": `{"q":"right"}`},
		}
		return map[string]any{"id": "matrix-response", "object": "response", "created_at": 1700000000, "status": "incomplete", "incomplete_details": map[string]any{"reason": "content_filter"}, "model": "matrix", "output": output, "usage": usage}
	case "messages":
		content := []any{
			map[string]any{"type": "text", "text": "matrix answer", "citations": []any{map[string]any{"type": "web_search_result", "url": "https://example.test/citation", "title": "Citation"}}},
			map[string]any{"type": "tool_use", "id": "call_left", "name": "get_left", "input": map[string]any{"q": "left"}},
			map[string]any{"type": "tool_use", "id": "call_right", "name": "get_right", "input": map[string]any{"q": "right"}},
			map[string]any{"type": "text", "text": "cannot disclose"},
		}
		return map[string]any{"id": "matrix-response", "type": "message", "role": "assistant", "model": "matrix", "content": content, "stop_reason": "refusal", "stop_sequence": "END", "usage": map[string]any{"input_tokens": 12, "output_tokens": 6, "cache_read_input_tokens": 2}}
	default:
		panic("unknown matrix response protocol: " + protocol)
	}
}

func matrixCitationResponseFixture(protocol string) map[string]any {
	citation := map[string]any{"type": "url_citation", "url": "https://example.test/citation", "title": "Citation", "start_index": 0, "end_index": 5}
	switch protocol {
	case "chat_completions":
		message := map[string]any{"role": "assistant", "content": "cited answer", "annotations": []any{citation}}
		return map[string]any{"id": "citation", "model": "matrix", "choices": []any{map[string]any{"finish_reason": "stop", "message": message}}}
	case "responses":
		messageContent := []any{map[string]any{"type": "output_text", "text": "cited answer", "annotations": []any{citation}}}
		message := map[string]any{"type": "message", "role": "assistant", "content": messageContent}
		return map[string]any{"id": "citation", "model": "matrix", "status": "completed", "output": []any{message}}
	case "messages":
		citationBlock := map[string]any{"type": "web_search_result", "url": "https://example.test/citation", "title": "Citation", "cited_text": "cited"}
		content := []any{map[string]any{"type": "text", "text": "cited answer", "citations": []any{citationBlock}}}
		return map[string]any{"id": "citation", "model": "matrix", "stop_reason": "end_turn", "content": content}
	default:
		panic("unknown citation protocol: " + protocol)
	}
}

func matrixDecodeObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode converted payload %s: %v", body, err)
	}
	return value
}
