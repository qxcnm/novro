package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const (
	testSystemPrompt = "Keep the answer short."
	testUserText     = "What is the weather?"
	testReasoning    = "Check the forecast before answering."
	testImageDataURI = "data:image/png;base64,aGVsbG8="
	testImageBase64  = "aGVsbG8="
	testToolName     = "get_weather"
	testToolCallID   = "call_weather_1"
	testToolResult   = "sunny"
	testStop         = "END"
	testMaxTokens    = 321
)

func TestAdaptProtocolRequestCrossProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		target string
	}{
		{name: "chat_to_responses", source: "chat_completions", target: "responses"},
		{name: "chat_to_messages", source: "chat_completions", target: "messages"},
		{name: "responses_to_chat", source: "responses", target: "chat_completions"},
		{name: "responses_to_messages", source: "responses", target: "messages"},
		{name: "messages_to_chat", source: "messages", target: "chat_completions"},
		{name: "messages_to_responses", source: "messages", target: "responses"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			body, err := adaptProtocolRequest(jsonRequestFixture(t, test.source), test.source, test.target, "target-model", true)
			if err != nil {
				t.Fatalf("adaptProtocolRequest(%s -> %s): %v", test.source, test.target, err)
			}
			got := decodeTestObject(t, body)
			if gotModel := stringValue(got["model"]); gotModel != "target-model" {
				t.Fatalf("model = %q, want target-model", gotModel)
			}
			if !boolValue(got["stream"]) {
				t.Fatal("stream = false, want true")
			}

			assertConvertedRequest(t, test.target, got)
		})
	}
}

func TestAdaptProtocolResponseCrossProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		target string
	}{
		{name: "chat_to_responses", source: "chat_completions", target: "responses"},
		{name: "chat_to_messages", source: "chat_completions", target: "messages"},
		{name: "responses_to_chat", source: "responses", target: "chat_completions"},
		{name: "responses_to_messages", source: "responses", target: "messages"},
		{name: "messages_to_chat", source: "messages", target: "chat_completions"},
		{name: "messages_to_responses", source: "messages", target: "responses"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			body, err := json.Marshal(responseFixture(test.source))
			if err != nil {
				t.Fatalf("marshal %s response fixture: %v", test.source, err)
			}
			converted, err := adaptProtocolResponse(body, test.source, test.target)
			if err != nil {
				t.Fatalf("adaptProtocolResponse(%s -> %s): %v", test.source, test.target, err)
			}
			assertConvertedResponse(t, test.source, test.target, decodeTestObject(t, converted))
		})
	}
}

func TestAdaptProtocolRequestRejectsPreviousResponseIDAcrossProtocols(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"model":                "source-model",
		"input":                "continue",
		"previous_response_id": "resp_previous",
	}
	for _, target := range []string{"chat_completions", "messages"} {
		target := target
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			if _, err := adaptProtocolRequest(payload, "responses", target, "target-model", false); !errors.Is(err, errUnsupportedProtocolConversion) {
				t.Fatalf("adaptProtocolRequest with previous_response_id error = %v, want unsupported conversion", err)
			}
		})
	}
}

func TestAdaptProtocolRequestSkipsResponsesItemReferencesForCrossProtocolTargets(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"model": "source-model",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "write the file"}}},
			map[string]any{"type": "function_call", "call_id": "call_write", "name": "write_file", "arguments": `{"path":"1.txt"}`},
			map[string]any{"type": "item_reference", "id": "fc_reference"},
			map[string]any{"type": "function_call_output", "call_id": "call_write", "output": "written"},
			map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "now summarize it"}}},
		},
	}

	for _, target := range []string{"chat_completions", "messages"} {
		target := target
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			body, err := adaptProtocolRequest(payload, "responses", target, "target-model", false)
			if err != nil {
				t.Fatalf("adapt Responses -> %s: %v", target, err)
			}
			converted := decodeTestObject(t, body)
			if target == "chat_completions" {
				messages := sliceValue(converted["messages"])
				if len(messages) != 4 {
					t.Fatalf("Chat messages = %d, want 4 after skipping item_reference: %#v", len(messages), messages)
				}
				if stringValue(mapValue(messages[2])["role"]) != "tool" || textFromContent(mapValue(messages[2])["content"]) != "written" {
					t.Fatalf("Chat tool result was lost: %#v", messages[2])
				}
			} else {
				messages := sliceValue(converted["messages"])
				if len(messages) != 3 {
					t.Fatalf("Messages items = %d, want 3 after skipping item_reference and coalescing user tool result: %#v", len(messages), messages)
				}
				content := sliceValue(mapValue(messages[2])["content"])
				firstPart := mapValue(content[0])
				if stringValue(mapValue(messages[2])["role"]) != "user" || stringValue(firstPart["type"]) != "tool_result" || stringValue(firstPart["content"]) != "written" {
					t.Fatalf("Messages tool result was lost: %#v", messages[2])
				}
			}
		})
	}
}

func TestAdaptProtocolRequestSameProtocolPreservesUnknownFields(t *testing.T) {
	t.Parallel()

	for _, protocol := range []string{"chat_completions", "responses", "messages"} {
		protocol := protocol
		t.Run(protocol, func(t *testing.T) {
			t.Parallel()

			payload := requestFixture(protocol)
			payload["vendor_extension"] = map[string]any{
				"enabled": true,
				"nested":  []any{"alpha", json.Number("2")},
			}
			if protocol == "responses" {
				payload["previous_response_id"] = "resp_previous"
			}
			if protocol == "chat_completions" {
				payload["stream_options"] = map[string]any{"include_obfuscation": false}
			}

			body, err := adaptProtocolRequest(payload, protocol, protocol, "same-model", true)
			if err != nil {
				t.Fatalf("adaptProtocolRequest(%s passthrough): %v", protocol, err)
			}
			got := decodeTestObject(t, body)
			vendor := requireTestMap(t, got["vendor_extension"], "vendor_extension")
			if enabled, _ := vendor["enabled"].(bool); !enabled {
				t.Fatalf("vendor_extension.enabled = %#v, want true", vendor["enabled"])
			}
			if stringValue(got["model"]) != "same-model" || !boolValue(got["stream"]) {
				t.Fatalf("passthrough overrides = model %#v stream %#v", got["model"], got["stream"])
			}
			if protocol == "responses" && stringValue(got["previous_response_id"]) != "resp_previous" {
				t.Fatalf("previous_response_id = %#v, want preserved", got["previous_response_id"])
			}

			if protocol == "chat_completions" {
				options := requireTestMap(t, got["stream_options"], "stream_options")
				if include, _ := options["include_usage"].(bool); !include {
					t.Fatalf("stream_options.include_usage = %#v, want true", options["include_usage"])
				}
				if obfuscation, ok := options["include_obfuscation"].(bool); !ok || obfuscation {
					t.Fatalf("stream_options.include_obfuscation = %#v, want preserved false", options["include_obfuscation"])
				}
			}
		})
	}
}

func TestAdaptProtocolResponseSameProtocolIsBytePassthrough(t *testing.T) {
	t.Parallel()

	body := []byte("{\n  \"id\": \"same\",\n  \"vendor_extension\": {\"keep\": true}\n}\n")
	for _, protocol := range []string{"chat_completions", "responses", "messages"} {
		protocol := protocol
		t.Run(protocol, func(t *testing.T) {
			t.Parallel()
			got, err := adaptProtocolResponse(body, protocol, protocol)
			if err != nil {
				t.Fatalf("adaptProtocolResponse(%s passthrough): %v", protocol, err)
			}
			if !bytes.Equal(got, body) {
				t.Fatalf("same-protocol response changed:\n got: %q\nwant: %q", got, body)
			}
		})
	}
}

func TestAdaptProtocolResponseAcceptsChatUsageAliases(t *testing.T) {
	t.Parallel()

	body := []byte(`{"id":"chat-alias","model":"test-model","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10}}`)
	converted, err := adaptProtocolResponse(body, "chat_completions", "responses")
	if err != nil {
		t.Fatalf("adapt Chat response with usage aliases: %v", err)
	}
	usage := requireTestMap(t, decodeTestObject(t, converted)["usage"], "usage")
	if intValue(usage["input_tokens"]) != 7 || intValue(usage["output_tokens"]) != 3 || intValue(usage["total_tokens"]) != 10 {
		t.Fatalf("converted usage = %#v, want 7/3/10", usage)
	}
}

func TestMessagesTargetSuppressesDuplicatedUnsignedReasoningText(t *testing.T) {
	t.Parallel()

	response := adapterResponse{
		ID: "duplicate-reasoning", Text: "private plan", Reasoning: "private plan",
		ToolCalls: []adapterPart{{Type: "tool_call", ID: "call_one", Name: "lookup", Arguments: map[string]any{"key": "value"}}},
		Usage:     map[string]any{},
	}
	encodedResponse := encodeAdapterResponse(response, "messages")
	responseJSON := string(mustTestJSON(t, encodedResponse))
	if strings.Contains(responseJSON, "private plan") {
		t.Fatalf("Messages response exposes reasoning duplicated into visible text: %s", responseJSON)
	}
	content := sliceValue(encodedResponse["content"])
	if len(content) != 1 || stringValue(mapValue(content[0])["type"]) != "tool_use" {
		t.Fatalf("Messages response content = %#v, want one tool_use block", content)
	}

	requestMessages := []adapterMessage{
		{Role: "assistant", Parts: []adapterPart{{Type: "reasoning", Text: "private plan"}}},
		{Role: "assistant", Parts: []adapterPart{{Type: "text", Text: "private plan"}, {Type: "tool_call", ID: "call_one", Name: "lookup", Arguments: map[string]any{"key": "value"}}}},
	}
	encodedMessages := encodeAnthropicMessages(requestMessages)
	requestJSON := string(mustTestJSON(t, encodedMessages))
	if strings.Contains(requestJSON, "private plan") {
		t.Fatalf("Messages request exposes reasoning duplicated into visible text: %s", requestJSON)
	}
	if len(encodedMessages) != 1 || !testAnthropicHasBlock(encodedMessages, "tool_use", func(map[string]any) bool { return true }) {
		t.Fatalf("Messages request = %#v, want only the tool-calling assistant message", encodedMessages)
	}
}

func TestResponsesMultiToolHistoryCoalescesForChatAndMessages(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"model": "source-model",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "use both tools"}}},
			map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "private plan"}}},
			map[string]any{"type": "function_call", "call_id": "call_left", "name": "get_left", "arguments": `{"label":"left"}`},
			map[string]any{"type": "function_call", "call_id": "call_right", "name": "get_right", "arguments": `{"label":"right"}`},
			map[string]any{"type": "function_call_output", "call_id": "call_left", "output": `{"value":19}`},
			map[string]any{"type": "function_call_output", "call_id": "call_right", "output": `{"value":23}`},
		},
	}

	chatBody, err := adaptProtocolRequest(payload, "responses", "chat_completions", "target-model", false)
	if err != nil {
		t.Fatalf("adapt multi-tool Responses request to Chat: %v", err)
	}
	chatMessages := sliceValue(decodeTestObject(t, chatBody)["messages"])
	if len(chatMessages) != 4 {
		t.Fatalf("Chat messages count = %d, want user, assistant, and two tool results: %s", len(chatMessages), chatBody)
	}
	assistant := mapValue(chatMessages[1])
	if stringValue(assistant["role"]) != "assistant" || len(sliceValue(assistant["tool_calls"])) != 2 {
		t.Fatalf("Chat assistant message = %#v, want two tool_calls", assistant)
	}
	for index, rawMessage := range chatMessages[2:] {
		if stringValue(mapValue(rawMessage)["role"]) != "tool" {
			t.Fatalf("Chat result message %d = %#v, want tool role", index, rawMessage)
		}
	}

	messagesBody, err := adaptProtocolRequest(payload, "responses", "messages", "target-model", false)
	if err != nil {
		t.Fatalf("adapt multi-tool Responses request to Messages: %v", err)
	}
	messages := sliceValue(decodeTestObject(t, messagesBody)["messages"])
	if len(messages) != 3 {
		t.Fatalf("Messages count = %d, want user/assistant/user: %s", len(messages), messagesBody)
	}
	assistantContent := sliceValue(mapValue(messages[1])["content"])
	resultContent := sliceValue(mapValue(messages[2])["content"])
	if countBlocksOfType(assistantContent, "tool_use") != 2 || countBlocksOfType(resultContent, "tool_result") != 2 {
		t.Fatalf("Messages tool blocks = assistant %#v results %#v", assistantContent, resultContent)
	}
	if strings.Contains(string(messagesBody), "private plan") {
		t.Fatalf("Messages multi-tool history exposes unsigned reasoning: %s", messagesBody)
	}
}

func TestAdaptProtocolRequestConvertsStructuredOutputFormats(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"answer": map[string]any{"type": "string"}},
		"required":             []any{"answer"},
		"additionalProperties": false,
	}
	sources := map[string]map[string]any{
		"chat_completions": {
			"model": "source", "messages": []any{map[string]any{"role": "user", "content": "hello"}},
			"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "answer", "strict": true, "schema": schema}},
		},
		"responses": {
			"model": "source", "input": "hello",
			"text": map[string]any{"format": map[string]any{"type": "json_schema", "name": "answer", "strict": true, "schema": schema}},
		},
		"messages": {
			"model": "source", "max_tokens": 64, "messages": []any{map[string]any{"role": "user", "content": "hello"}},
			"output_config": map[string]any{"format": map[string]any{"type": "json_schema", "schema": schema}},
		},
	}
	for source, payload := range sources {
		source, payload := source, payload
		for _, target := range []string{"chat_completions", "responses", "messages"} {
			target := target
			if source == target {
				continue
			}
			t.Run(source+"_to_"+target, func(t *testing.T) {
				t.Parallel()
				body, err := adaptProtocolRequest(payload, source, target, "target", false)
				if err != nil {
					t.Fatalf("adapt structured output request: %v", err)
				}
				converted := decodeTestObject(t, body)
				var got map[string]any
				switch target {
				case "chat_completions":
					got = mapValue(mapValue(converted["response_format"])["json_schema"])["schema"].(map[string]any)
				case "responses":
					got = mapValue(mapValue(mapValue(converted["text"])["format"])["schema"])
				case "messages":
					got = mapValue(mapValue(mapValue(converted["output_config"])["format"])["schema"])
				}
				if !reflect.DeepEqual(got, schema) {
					t.Fatalf("converted schema = %#v, want %#v; body=%s", got, schema, body)
				}
			})
		}
	}
}

func TestAdapterStopReasonMappings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sourceReason        string
		wantChat            string
		wantResponsesStatus string
		wantResponsesReason string
		wantAnthropic       string
	}{
		{sourceReason: "content_filter", wantChat: "content_filter", wantResponsesStatus: "incomplete", wantResponsesReason: "content_filter", wantAnthropic: "refusal"},
		{sourceReason: "model_context_window_exceeded", wantChat: "length", wantResponsesStatus: "incomplete", wantResponsesReason: "max_output_tokens", wantAnthropic: "model_context_window_exceeded"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.sourceReason, func(t *testing.T) {
			t.Parallel()
			response := adapterResponse{ID: "test", StopReason: test.sourceReason, Usage: map[string]any{}}
			chat := encodeAdapterResponse(response, "chat_completions")
			choice := mapValue(sliceValue(chat["choices"])[0])
			if stringValue(choice["finish_reason"]) != test.wantChat {
				t.Errorf("Chat finish_reason = %#v, want %q", choice["finish_reason"], test.wantChat)
			}
			responses := encodeAdapterResponse(response, "responses")
			if stringValue(responses["status"]) != test.wantResponsesStatus || stringValue(mapValue(responses["incomplete_details"])["reason"]) != test.wantResponsesReason {
				t.Errorf("Responses status/details = %#v/%#v, want %q/%q", responses["status"], responses["incomplete_details"], test.wantResponsesStatus, test.wantResponsesReason)
			}
			messages := encodeAdapterResponse(response, "messages")
			if stringValue(messages["stop_reason"]) != test.wantAnthropic {
				t.Errorf("Messages stop_reason = %#v, want %q", messages["stop_reason"], test.wantAnthropic)
			}
		})
	}
}

func requestFixture(protocol string) map[string]any {
	parameters := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"city": map[string]any{"type": "string"},
		},
		"required": []any{"city"},
	}
	arguments := `{"city":"Shanghai"}`

	switch protocol {
	case "chat_completions":
		return map[string]any{
			"model": "source-model", "stream": false,
			"max_completion_tokens": testMaxTokens,
			"stop":                  []any{testStop},
			"reasoning_effort":      "high",
			"messages": []any{
				map[string]any{"role": "system", "content": testSystemPrompt},
				map[string]any{"role": "user", "content": []any{
					map[string]any{"type": "text", "text": testUserText},
					map[string]any{"type": "image_url", "image_url": map[string]any{"url": testImageDataURI}},
				}},
				map[string]any{
					"role": "assistant", "content": "", "reasoning_content": testReasoning,
					"tool_calls": []any{map[string]any{
						"id": testToolCallID, "type": "function",
						"function": map[string]any{"name": testToolName, "arguments": arguments},
					}},
				},
				map[string]any{"role": "tool", "tool_call_id": testToolCallID, "content": testToolResult},
			},
			"tools": []any{map[string]any{
				"type":     "function",
				"function": map[string]any{"name": testToolName, "description": "Weather lookup", "parameters": parameters},
			}},
			"tool_choice": map[string]any{"type": "function", "function": map[string]any{"name": testToolName}},
		}

	case "responses":
		return map[string]any{
			"model": "source-model", "stream": false,
			"instructions":      testSystemPrompt,
			"max_output_tokens": testMaxTokens,
			"stop":              []any{testStop},
			"reasoning":         map[string]any{"effort": "high"},
			"input": []any{
				map[string]any{"type": "message", "role": "user", "content": []any{
					map[string]any{"type": "input_text", "text": testUserText},
					map[string]any{"type": "input_image", "image_url": testImageDataURI},
				}},
				map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": testReasoning}}},
				map[string]any{"type": "function_call", "call_id": testToolCallID, "name": testToolName, "arguments": arguments},
				map[string]any{"type": "function_call_output", "call_id": testToolCallID, "output": testToolResult},
			},
			"tools": []any{map[string]any{
				"type": "function", "name": testToolName, "description": "Weather lookup", "parameters": parameters,
			}},
			"tool_choice": map[string]any{"type": "function", "name": testToolName},
		}

	case "messages":
		return map[string]any{
			"model": "source-model", "stream": false,
			"system":         testSystemPrompt,
			"max_tokens":     testMaxTokens,
			"stop_sequences": []any{testStop},
			"thinking":       map[string]any{"type": "enabled", "budget_tokens": 128},
			"output_config":  map[string]any{"effort": "high"},
			"messages": []any{
				map[string]any{"role": "user", "content": []any{
					map[string]any{"type": "text", "text": testUserText},
					map[string]any{"type": "image", "source": map[string]any{
						"type": "base64", "media_type": "image/png", "data": testImageBase64,
					}},
				}},
				map[string]any{"role": "assistant", "content": []any{
					map[string]any{"type": "thinking", "thinking": testReasoning, "signature": "sig-1"},
					map[string]any{"type": "tool_use", "id": testToolCallID, "name": testToolName, "input": map[string]any{"city": "Shanghai"}},
				}},
				map[string]any{"role": "user", "content": []any{
					map[string]any{"type": "tool_result", "tool_use_id": testToolCallID, "content": testToolResult},
				}},
			},
			"tools": []any{map[string]any{
				"name": testToolName, "description": "Weather lookup", "input_schema": parameters,
			}},
			"tool_choice": map[string]any{"type": "tool", "name": testToolName},
		}
	default:
		panic("unknown protocol fixture: " + protocol)
	}
}

func responseFixture(protocol string) map[string]any {
	arguments := `{"city":"Shanghai"}`
	switch protocol {
	case "chat_completions":
		return map[string]any{
			"id": "chatcmpl-test", "object": "chat.completion", "created": 1700000000, "model": "test-model",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "length",
				"message": map[string]any{
					"role": "assistant", "content": "It is sunny.", "reasoning_content": testReasoning,
					"tool_calls": []any{map[string]any{
						"id": testToolCallID, "type": "function",
						"function": map[string]any{"name": testToolName, "arguments": arguments},
					}},
				},
			}},
			"usage": map[string]any{
				"prompt_tokens": 20, "completion_tokens": 5, "total_tokens": 25,
				"prompt_tokens_details":     map[string]any{"cached_tokens": 2},
				"completion_tokens_details": map[string]any{"reasoning_tokens": 3},
			},
		}

	case "responses":
		return map[string]any{
			"id": "resp-test", "object": "response", "created_at": 1700000000, "status": "incomplete", "model": "test-model",
			"incomplete_details": map[string]any{"reason": "max_output_tokens"},
			"output": []any{
				map[string]any{"id": "rs-test", "type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": testReasoning}}},
				map[string]any{"id": "msg-test", "type": "message", "status": "completed", "role": "assistant", "content": []any{
					map[string]any{"type": "output_text", "text": "It is sunny.", "annotations": []any{}},
				}},
				map[string]any{"id": "fc-test", "type": "function_call", "status": "completed", "call_id": testToolCallID, "name": testToolName, "arguments": arguments},
			},
			"usage": map[string]any{
				"input_tokens": 20, "output_tokens": 5, "total_tokens": 25,
				"input_tokens_details":  map[string]any{"cached_tokens": 2},
				"output_tokens_details": map[string]any{"reasoning_tokens": 3},
			},
		}

	case "messages":
		return map[string]any{
			"id": "msg-test", "type": "message", "role": "assistant", "model": "test-model",
			"content": []any{
				map[string]any{"type": "thinking", "thinking": testReasoning, "signature": "sig-1"},
				map[string]any{"type": "text", "text": "It is sunny."},
				map[string]any{"type": "tool_use", "id": testToolCallID, "name": testToolName, "input": map[string]any{"city": "Shanghai"}},
			},
			"stop_reason": "max_tokens", "stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens": 17, "output_tokens": 5,
				"cache_read_input_tokens": 2, "cache_creation_input_tokens": 1,
				"output_tokens_details": map[string]any{"thinking_tokens": 3},
			},
		}
	default:
		panic("unknown protocol response fixture: " + protocol)
	}
}

func assertConvertedRequest(t *testing.T, target string, got map[string]any) {
	t.Helper()

	encoded := string(mustTestJSON(t, got))
	for _, want := range []string{testSystemPrompt, testUserText, testToolName, testToolCallID, testToolResult} {
		if !strings.Contains(encoded, want) {
			t.Errorf("converted %s request does not contain %q:\n%s", target, want, encoded)
		}
	}
	if target == "messages" {
		if strings.Contains(encoded, testReasoning) {
			t.Errorf("converted Messages request exposes unsigned reasoning %q:\n%s", testReasoning, encoded)
		}
	} else if !strings.Contains(encoded, testReasoning) {
		t.Errorf("converted %s request does not contain reasoning %q:\n%s", target, testReasoning, encoded)
	}

	switch target {
	case "chat_completions":
		if intValue(got["max_tokens"]) != testMaxTokens {
			t.Errorf("max_tokens = %#v, want %d", got["max_tokens"], testMaxTokens)
		}
		assertStopValue(t, got["stop"])
		options := requireTestMap(t, got["stream_options"], "stream_options")
		if include, _ := options["include_usage"].(bool); !include {
			t.Errorf("stream_options.include_usage = %#v, want true", options["include_usage"])
		}
		assertChatRequestWire(t, got)
		if stringValue(got["reasoning_effort"]) != "high" {
			t.Errorf("reasoning_effort = %#v, want high", got["reasoning_effort"])
		}

	case "responses":
		if intValue(got["max_output_tokens"]) != testMaxTokens {
			t.Errorf("max_output_tokens = %#v, want %d", got["max_output_tokens"], testMaxTokens)
		}
		if _, exists := got["stop"]; exists {
			t.Errorf("Responses request contains unsupported stop field: %#v", got["stop"])
		}
		assertResponsesRequestWire(t, got)
		if stringValue(mapValue(got["reasoning"])["effort"]) != "high" {
			t.Errorf("reasoning.effort = %#v, want high", got["reasoning"])
		}

	case "messages":
		if intValue(got["max_tokens"]) != testMaxTokens {
			t.Errorf("max_tokens = %#v, want %d", got["max_tokens"], testMaxTokens)
		}
		assertStopValue(t, got["stop_sequences"])
		assertMessagesRequestWire(t, got)
		if stringValue(mapValue(got["output_config"])["effort"]) != "high" {
			t.Errorf("output_config.effort = %#v, want high", got["output_config"])
		}

	default:
		t.Fatalf("unknown target %q", target)
	}
}

func assertChatRequestWire(t *testing.T, got map[string]any) {
	t.Helper()
	messages := sliceValue(got["messages"])
	if len(messages) < 4 {
		t.Fatalf("chat messages has %d entries, want at least 4: %#v", len(messages), messages)
	}
	if role := stringValue(requireTestMap(t, messages[0], "messages[0]")["role"]); role != "system" {
		t.Errorf("first Chat message role = %q, want system", role)
	}
	if !testChatHasImage(messages, testImageDataURI) {
		t.Errorf("Chat messages do not contain image_url %q", testImageDataURI)
	}
	if !testChatHasToolCall(messages, testToolCallID, testToolName) {
		t.Errorf("Chat messages do not contain tool call %s/%s", testToolCallID, testToolName)
	}
	if !testChatHasToolResult(messages, testToolCallID, testToolResult) {
		t.Errorf("Chat messages do not contain tool result %s/%s", testToolCallID, testToolResult)
	}
	tools := sliceValue(got["tools"])
	if len(tools) != 1 {
		t.Fatalf("Chat tools count = %d, want 1", len(tools))
	}
	tool := requireTestMap(t, tools[0], "tools[0]")
	function := requireTestMap(t, tool["function"], "tools[0].function")
	if stringValue(tool["type"]) != "function" || stringValue(function["name"]) != testToolName {
		t.Errorf("invalid Chat function tool: %#v", tool)
	}
	choice := requireTestMap(t, got["tool_choice"], "tool_choice")
	choiceFunction := requireTestMap(t, choice["function"], "tool_choice.function")
	if stringValue(choice["type"]) != "function" || stringValue(choiceFunction["name"]) != testToolName {
		t.Errorf("invalid Chat tool_choice: %#v", choice)
	}
}

func assertResponsesRequestWire(t *testing.T, got map[string]any) {
	t.Helper()
	if instructions := stringValue(got["instructions"]); !strings.Contains(instructions, testSystemPrompt) {
		t.Errorf("Responses instructions = %q, want %q", instructions, testSystemPrompt)
	}
	input := sliceValue(got["input"])
	if !testResponsesHasInputContent(input, "input_text", "text", testUserText) {
		t.Errorf("Responses input does not contain input_text %q", testUserText)
	}
	if !testResponsesHasInputContent(input, "input_image", "image_url", testImageDataURI) {
		t.Errorf("Responses input does not contain input_image %q", testImageDataURI)
	}
	if !testResponsesHasItem(input, "function_call", "call_id", testToolCallID) {
		t.Errorf("Responses input does not contain function_call %q", testToolCallID)
	}
	if !testResponsesHasItem(input, "function_call_output", "output", testToolResult) {
		t.Errorf("Responses input does not contain function_call_output %q", testToolResult)
	}
	tools := sliceValue(got["tools"])
	if len(tools) != 1 {
		t.Fatalf("Responses tools count = %d, want 1", len(tools))
	}
	tool := requireTestMap(t, tools[0], "tools[0]")
	if stringValue(tool["type"]) != "function" || stringValue(tool["name"]) != testToolName {
		t.Errorf("invalid Responses function tool: %#v", tool)
	}
	choice := requireTestMap(t, got["tool_choice"], "tool_choice")
	if stringValue(choice["type"]) != "function" || stringValue(choice["name"]) != testToolName {
		t.Errorf("invalid Responses tool_choice: %#v", choice)
	}
}

func assertMessagesRequestWire(t *testing.T, got map[string]any) {
	t.Helper()
	if system := textFromContent(got["system"]); !strings.Contains(system, testSystemPrompt) {
		t.Errorf("Messages system = %q, want %q", system, testSystemPrompt)
	}
	messages := sliceValue(got["messages"])
	for index, rawMessage := range messages {
		message := requireTestMap(t, rawMessage, "messages")
		content := sliceValue(message["content"])
		if len(content) == 0 {
			t.Errorf("Messages request contains empty message at index %d", index)
		}
		for _, rawBlock := range content {
			block := mapValue(rawBlock)
			if stringValue(block["type"]) == "text" && stringValue(block["text"]) == "" {
				t.Errorf("Messages request contains empty text block at index %d", index)
			}
		}
	}
	if !testAnthropicHasBlock(messages, "image", func(block map[string]any) bool {
		source := mapValue(block["source"])
		return stringValue(source["type"]) == "base64" && stringValue(source["media_type"]) == "image/png" && stringValue(source["data"]) == testImageBase64
	}) {
		t.Errorf("Messages request does not contain expected base64 image")
	}
	if testAnthropicHasBlock(messages, "text", func(block map[string]any) bool {
		return stringValue(block["text"]) == testReasoning
	}) {
		t.Errorf("Messages request exposes unsigned reasoning as text %q", testReasoning)
	}
	if testAnthropicHasBlock(messages, "thinking", func(block map[string]any) bool {
		return stringValue(block["signature"]) == ""
	}) {
		t.Error("Messages request contains a thinking block with a forged empty signature")
	}
	if !testAnthropicHasBlock(messages, "tool_use", func(block map[string]any) bool {
		return stringValue(block["id"]) == testToolCallID && stringValue(block["name"]) == testToolName
	}) {
		t.Errorf("Messages request does not contain tool_use %s/%s", testToolCallID, testToolName)
	}
	if !testAnthropicHasBlock(messages, "tool_result", func(block map[string]any) bool {
		return stringValue(block["tool_use_id"]) == testToolCallID && textFromContent(block["content"]) == testToolResult
	}) {
		t.Errorf("Messages request does not contain tool_result %s/%s", testToolCallID, testToolResult)
	}
	tools := sliceValue(got["tools"])
	if len(tools) != 1 {
		t.Fatalf("Messages tools count = %d, want 1", len(tools))
	}
	tool := requireTestMap(t, tools[0], "tools[0]")
	if stringValue(tool["name"]) != testToolName || mapValue(tool["input_schema"]) == nil {
		t.Errorf("invalid Messages function tool: %#v", tool)
	}
	choice := requireTestMap(t, got["tool_choice"], "tool_choice")
	if stringValue(choice["type"]) != "tool" || stringValue(choice["name"]) != testToolName {
		t.Errorf("invalid Messages tool_choice: %#v", choice)
	}
}

func assertConvertedResponse(t *testing.T, source, target string, got map[string]any) {
	t.Helper()
	encoded := string(mustTestJSON(t, got))
	for _, want := range []string{"It is sunny.", testToolName, testToolCallID} {
		if !strings.Contains(encoded, want) {
			t.Errorf("converted %s -> %s response does not contain %q:\n%s", source, target, want, encoded)
		}
	}
	if target == "messages" {
		if strings.Contains(encoded, testReasoning) {
			t.Errorf("converted %s -> Messages response exposes unsigned reasoning %q:\n%s", source, testReasoning, encoded)
		}
	} else if !strings.Contains(encoded, testReasoning) {
		t.Errorf("converted %s -> %s response does not contain reasoning %q:\n%s", source, target, testReasoning, encoded)
	}

	hasReasoningUsage := true
	cacheCreation := 0
	if source == "messages" {
		cacheCreation = 1
	}

	switch target {
	case "chat_completions":
		choices := sliceValue(got["choices"])
		if len(choices) != 1 {
			t.Fatalf("Chat choices count = %d, want 1", len(choices))
		}
		choice := requireTestMap(t, choices[0], "choices[0]")
		if stringValue(choice["finish_reason"]) != "length" {
			t.Errorf("Chat finish_reason = %#v, want length", choice["finish_reason"])
		}
		assertOpenAIUsage(t, requireTestMap(t, got["usage"], "usage"), true, hasReasoningUsage)

	case "responses":
		if stringValue(got["status"]) != "incomplete" {
			t.Errorf("Responses status = %#v, want incomplete for max-token stop", got["status"])
		}
		assertOpenAIUsage(t, requireTestMap(t, got["usage"], "usage"), false, hasReasoningUsage)

	case "messages":
		if stringValue(got["stop_reason"]) != "max_tokens" {
			t.Errorf("Messages stop_reason = %#v, want max_tokens", got["stop_reason"])
		}
		usage := requireTestMap(t, got["usage"], "usage")
		wantUncached := 20 - 2 - cacheCreation
		if intValue(usage["input_tokens"]) != wantUncached || intValue(usage["output_tokens"]) != 5 {
			t.Errorf("Messages usage = %#v, want input=%d output=5", usage, wantUncached)
		}
		if intValue(usage["cache_read_input_tokens"]) != 2 {
			t.Errorf("Messages cache_read_input_tokens = %#v, want 2", usage["cache_read_input_tokens"])
		}
		if cacheCreation > 0 && intValue(usage["cache_creation_input_tokens"]) != cacheCreation {
			t.Errorf("Messages cache_creation_input_tokens = %#v, want %d", usage["cache_creation_input_tokens"], cacheCreation)
		}
		content := sliceValue(got["content"])
		for _, rawBlock := range content {
			block := mapValue(rawBlock)
			if stringValue(block["type"]) == "thinking" && stringValue(block["signature"]) == "" {
				t.Errorf("Messages response contains a thinking block with a forged empty signature: %#v", block)
			}
		}

	default:
		t.Fatalf("unknown target %q", target)
	}
}

func assertOpenAIUsage(t *testing.T, usage map[string]any, chat, wantReasoning bool) {
	t.Helper()
	inputKey, outputKey := "input_tokens", "output_tokens"
	inputDetailsKey, outputDetailsKey := "input_tokens_details", "output_tokens_details"
	if chat {
		inputKey, outputKey = "prompt_tokens", "completion_tokens"
		inputDetailsKey, outputDetailsKey = "prompt_tokens_details", "completion_tokens_details"
	}
	if intValue(usage[inputKey]) != 20 || intValue(usage[outputKey]) != 5 || intValue(usage["total_tokens"]) != 25 {
		t.Errorf("OpenAI usage = %#v, want input=20 output=5 total=25", usage)
	}
	inputDetails := requireTestMap(t, usage[inputDetailsKey], inputDetailsKey)
	if intValue(inputDetails["cached_tokens"]) != 2 {
		t.Errorf("%s.cached_tokens = %#v, want 2", inputDetailsKey, inputDetails["cached_tokens"])
	}
	if wantReasoning {
		outputDetails := requireTestMap(t, usage[outputDetailsKey], outputDetailsKey)
		if intValue(outputDetails["reasoning_tokens"]) != 3 {
			t.Errorf("%s.reasoning_tokens = %#v, want 3", outputDetailsKey, outputDetails["reasoning_tokens"])
		}
	}
}

func assertStopValue(t *testing.T, value any) {
	t.Helper()
	if text, ok := value.(string); ok {
		if text != testStop {
			t.Errorf("stop = %q, want %q", text, testStop)
		}
		return
	}
	values := sliceValue(value)
	if len(values) != 1 || stringValue(values[0]) != testStop {
		t.Errorf("stop = %#v, want [%q]", value, testStop)
	}
}

func testChatHasImage(messages []any, want string) bool {
	for _, rawMessage := range messages {
		message := mapValue(rawMessage)
		for _, rawPart := range sliceValue(message["content"]) {
			part := mapValue(rawPart)
			if stringValue(part["type"]) != "image_url" {
				continue
			}
			image := mapValue(part["image_url"])
			if stringValue(image["url"]) == want || stringValue(part["image_url"]) == want {
				return true
			}
		}
	}
	return false
}

func testChatHasToolCall(messages []any, id, name string) bool {
	for _, rawMessage := range messages {
		message := mapValue(rawMessage)
		for _, rawCall := range sliceValue(message["tool_calls"]) {
			call := mapValue(rawCall)
			function := mapValue(call["function"])
			if stringValue(call["id"]) == id && stringValue(function["name"]) == name {
				return true
			}
		}
	}
	return false
}

func testChatHasToolResult(messages []any, id, result string) bool {
	for _, rawMessage := range messages {
		message := mapValue(rawMessage)
		if stringValue(message["role"]) == "tool" && stringValue(message["tool_call_id"]) == id && textFromContent(message["content"]) == result {
			return true
		}
	}
	return false
}

func testResponsesHasInputContent(input []any, contentType, field, want string) bool {
	for _, rawItem := range input {
		item := mapValue(rawItem)
		if itemType := stringValue(item["type"]); itemType != "message" && itemType != "" {
			continue
		}
		for _, rawPart := range sliceValue(item["content"]) {
			part := mapValue(rawPart)
			if stringValue(part["type"]) == contentType && stringValue(part[field]) == want {
				return true
			}
		}
	}
	return false
}

func testResponsesHasItem(input []any, itemType, field, want string) bool {
	for _, rawItem := range input {
		item := mapValue(rawItem)
		if stringValue(item["type"]) == itemType && stringValue(item[field]) == want {
			return true
		}
	}
	return false
}

func testAnthropicHasBlock(messages []any, blockType string, match func(map[string]any) bool) bool {
	for _, rawMessage := range messages {
		message := mapValue(rawMessage)
		for _, rawBlock := range sliceValue(message["content"]) {
			block := mapValue(rawBlock)
			if stringValue(block["type"]) == blockType && match(block) {
				return true
			}
		}
	}
	return false
}

func countBlocksOfType(blocks []any, blockType string) int {
	count := 0
	for _, rawBlock := range blocks {
		if stringValue(mapValue(rawBlock)["type"]) == blockType {
			count++
		}
	}
	return count
}

func decodeTestObject(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode JSON object %q: %v", body, err)
	}
	return result
}

func jsonRequestFixture(t *testing.T, protocol string) map[string]any {
	t.Helper()
	return decodeTestObject(t, mustTestJSON(t, requestFixture(protocol)))
}

func requireTestMap(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	result := mapValue(value)
	if result == nil {
		t.Fatalf("%s = %#v, want JSON object", path, value)
	}
	return result
}

func mustTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	result, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal test JSON: %v", err)
	}
	return result
}

func TestProtocolAdapterFixturesAreDistinct(t *testing.T) {
	// Guard against accidentally reducing the six-direction table to identical
	// source JSON while editing fixtures.
	t.Parallel()
	seen := map[string]string{}
	for _, protocol := range []string{"chat_completions", "responses", "messages"} {
		body := string(mustTestJSON(t, requestFixture(protocol)))
		if previous, exists := seen[body]; exists {
			t.Fatalf("%s and %s fixtures are identical", previous, protocol)
		}
		seen[body] = protocol
	}
	if len(seen) != 3 {
		t.Fatalf("fixture count = %d, want 3", len(seen))
	}
}
