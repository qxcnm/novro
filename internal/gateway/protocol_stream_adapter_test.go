package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const (
	streamReasoning = "Check the forecast."
	streamText      = "It is sunny."
	streamCallID0   = "call_weather"
	streamCallID1   = "call_units"
	streamCallName0 = "get_weather"
	streamCallName1 = "get_units"
	streamCallArgs0 = `{"city":"Shanghai"}`
	streamCallArgs1 = `{"unit":"celsius"}`
)

type streamAdapterInputEvent struct {
	name string
	data []byte
}

type streamAdapterOutputEvent struct {
	name string
	data string
	root map[string]any
}

func TestProtocolStreamAdapterCrossProtocol(t *testing.T) {
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

			adapter := newProtocolStreamAdapter(test.source, test.target)
			var output bytes.Buffer
			for index, event := range streamAdapterFixture(t, test.source) {
				converted, err := adapter.Translate(event.name, event.data)
				if err != nil {
					t.Fatalf("Translate event %d (%s -> %s): %v\ndata: %s", index, test.source, test.target, err, event.data)
				}
				output.Write(converted)
			}

			events := parseStreamAdapterOutput(t, output.Bytes())
			if len(events) == 0 {
				t.Fatalf("%s -> %s emitted no SSE events", test.source, test.target)
			}
			switch test.target {
			case "chat_completions":
				assertStreamChatOutput(t, test.source, events)
			case "responses":
				assertStreamResponsesOutput(t, test.source, events)
			case "messages":
				assertStreamMessagesOutput(t, events)
			default:
				t.Fatalf("unknown target %q", test.target)
			}
		})
	}
}

func streamAdapterFixture(t *testing.T, source string) []streamAdapterInputEvent {
	t.Helper()
	switch source {
	case "chat_completions":
		return []streamAdapterInputEvent{
			streamJSONEvent(t, "", map[string]any{
				"id": "chatcmpl-stream", "object": "chat.completion.chunk", "created": 1_700_000_000, "model": "stream-model",
				"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
			}),
			streamJSONEvent(t, "", chatStreamChunk(map[string]any{"reasoning_content": streamReasoning}, nil)),
			streamJSONEvent(t, "", chatStreamChunk(map[string]any{"content": "It is "}, nil)),
			streamJSONEvent(t, "", chatStreamChunk(map[string]any{"content": "sunny."}, nil)),
			streamJSONEvent(t, "", chatStreamChunk(map[string]any{"tool_calls": []any{
				map[string]any{"index": 0, "id": streamCallID0, "type": "function", "function": map[string]any{"name": streamCallName0, "arguments": `{"city":"`}},
				map[string]any{"index": 1, "id": streamCallID1, "type": "function", "function": map[string]any{"name": streamCallName1, "arguments": `{"unit":"`}},
			}}, nil)),
			streamJSONEvent(t, "", chatStreamChunk(map[string]any{"tool_calls": []any{
				map[string]any{"index": 0, "function": map[string]any{"arguments": `Shanghai"}`}},
				map[string]any{"index": 1, "function": map[string]any{"arguments": `celsius"}`}},
			}}, nil)),
			streamJSONEvent(t, "", chatStreamChunk(map[string]any{}, "tool_calls")),
			streamJSONEvent(t, "", map[string]any{
				"id": "chatcmpl-stream", "object": "chat.completion.chunk", "created": 1_700_000_000, "model": "stream-model", "choices": []any{},
				"usage": streamOpenAIUsage("chat_completions"),
			}),
			{name: "", data: []byte("[DONE]")},
		}

	case "responses":
		return []streamAdapterInputEvent{
			streamJSONEvent(t, "response.created", map[string]any{
				"type": "response.created", "sequence_number": 0,
				"response": map[string]any{"id": "resp_stream", "object": "response", "created_at": 1_700_000_000, "status": "in_progress", "model": "stream-model"},
			}),
			streamJSONEvent(t, "response.reasoning_summary_text.delta", map[string]any{"type": "response.reasoning_summary_text.delta", "sequence_number": 1, "item_id": "rs_stream", "output_index": 0, "summary_index": 0, "delta": streamReasoning}),
			streamJSONEvent(t, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "sequence_number": 2, "item_id": "msg_stream", "output_index": 1, "content_index": 0, "delta": "It is "}),
			streamJSONEvent(t, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "sequence_number": 3, "item_id": "msg_stream", "output_index": 1, "content_index": 0, "delta": "sunny."}),
			streamJSONEvent(t, "response.output_item.added", responsesToolAdded(4, 2, "fc_item_weather", streamCallID0, streamCallName0)),
			streamJSONEvent(t, "response.output_item.added", responsesToolAdded(5, 3, "fc_item_units", streamCallID1, streamCallName1)),
			streamJSONEvent(t, "response.function_call_arguments.delta", responsesToolArguments(6, 2, "fc_item_weather", `{"city":"`)),
			streamJSONEvent(t, "response.function_call_arguments.delta", responsesToolArguments(7, 3, "fc_item_units", `{"unit":"`)),
			streamJSONEvent(t, "response.function_call_arguments.delta", responsesToolArguments(8, 2, "fc_item_weather", `Shanghai"}`)),
			streamJSONEvent(t, "response.function_call_arguments.delta", responsesToolArguments(9, 3, "fc_item_units", `celsius"}`)),
			streamJSONEvent(t, "response.completed", map[string]any{
				"type": "response.completed", "sequence_number": 10,
				"response": map[string]any{"id": "resp_stream", "object": "response", "created_at": 1_700_000_000, "status": "completed", "model": "stream-model", "usage": streamOpenAIUsage("responses")},
			}),
		}

	case "messages":
		return []streamAdapterInputEvent{
			streamJSONEvent(t, "message_start", map[string]any{
				"type": "message_start",
				"message": map[string]any{
					"id": "msg_stream", "type": "message", "role": "assistant", "model": "stream-model", "content": []any{}, "stop_reason": nil,
					"usage": map[string]any{"input_tokens": 17, "output_tokens": 0, "cache_read_input_tokens": 2, "cache_creation_input_tokens": 1},
				},
			}),
			streamJSONEvent(t, "content_block_start", messagesBlockStart(0, map[string]any{"type": "thinking", "thinking": "", "signature": ""})),
			streamJSONEvent(t, "content_block_delta", messagesBlockDelta(0, map[string]any{"type": "thinking_delta", "thinking": streamReasoning})),
			streamJSONEvent(t, "content_block_delta", messagesBlockDelta(0, map[string]any{"type": "signature_delta", "signature": "sig-stream"})),
			streamJSONEvent(t, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0}),
			streamJSONEvent(t, "content_block_start", messagesBlockStart(1, map[string]any{"type": "text", "text": ""})),
			streamJSONEvent(t, "content_block_delta", messagesBlockDelta(1, map[string]any{"type": "text_delta", "text": "It is "})),
			streamJSONEvent(t, "content_block_delta", messagesBlockDelta(1, map[string]any{"type": "text_delta", "text": "sunny."})),
			streamJSONEvent(t, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 1}),
			streamJSONEvent(t, "content_block_start", messagesBlockStart(2, map[string]any{"type": "tool_use", "id": streamCallID0, "name": streamCallName0, "input": map[string]any{}})),
			streamJSONEvent(t, "content_block_delta", messagesBlockDelta(2, map[string]any{"type": "input_json_delta", "partial_json": `{"city":"`})),
			streamJSONEvent(t, "content_block_delta", messagesBlockDelta(2, map[string]any{"type": "input_json_delta", "partial_json": `Shanghai"}`})),
			streamJSONEvent(t, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 2}),
			streamJSONEvent(t, "content_block_start", messagesBlockStart(3, map[string]any{"type": "tool_use", "id": streamCallID1, "name": streamCallName1, "input": map[string]any{}})),
			streamJSONEvent(t, "content_block_delta", messagesBlockDelta(3, map[string]any{"type": "input_json_delta", "partial_json": `{"unit":"`})),
			streamJSONEvent(t, "content_block_delta", messagesBlockDelta(3, map[string]any{"type": "input_json_delta", "partial_json": `celsius"}`})),
			streamJSONEvent(t, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 3}),
			streamJSONEvent(t, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "tool_use", "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 5, "output_tokens_details": map[string]any{"thinking_tokens": 3}}}),
			streamJSONEvent(t, "message_stop", map[string]any{"type": "message_stop"}),
		}
	default:
		t.Fatalf("unknown stream source %q", source)
		return nil
	}
}

func chatStreamChunk(delta map[string]any, finishReason any) map[string]any {
	return map[string]any{
		"id": "chatcmpl-stream", "object": "chat.completion.chunk", "created": 1_700_000_000, "model": "stream-model",
		"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finishReason}},
	}
}

func responsesToolAdded(sequence, outputIndex int, itemID, callID, name string) map[string]any {
	return map[string]any{
		"type": "response.output_item.added", "sequence_number": sequence, "output_index": outputIndex,
		"item": map[string]any{"id": itemID, "type": "function_call", "call_id": callID, "name": name, "arguments": "", "status": "in_progress"},
	}
}

func responsesToolArguments(sequence, outputIndex int, itemID, delta string) map[string]any {
	return map[string]any{
		"type": "response.function_call_arguments.delta", "sequence_number": sequence, "item_id": itemID, "output_index": outputIndex, "delta": delta,
	}
}

func messagesBlockStart(index int, block map[string]any) map[string]any {
	return map[string]any{"type": "content_block_start", "index": index, "content_block": block}
}

func messagesBlockDelta(index int, delta map[string]any) map[string]any {
	return map[string]any{"type": "content_block_delta", "index": index, "delta": delta}
}

func streamOpenAIUsage(protocol string) map[string]any {
	if protocol == "chat_completions" {
		return map[string]any{
			"prompt_tokens": 20, "completion_tokens": 5, "total_tokens": 25,
			"prompt_tokens_details": map[string]any{"cached_tokens": 2}, "completion_tokens_details": map[string]any{"reasoning_tokens": 3},
		}
	}
	return map[string]any{
		"input_tokens": 20, "output_tokens": 5, "total_tokens": 25,
		"input_tokens_details": map[string]any{"cached_tokens": 2}, "output_tokens_details": map[string]any{"reasoning_tokens": 3},
	}
}

func streamJSONEvent(t *testing.T, name string, value any) streamAdapterInputEvent {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s fixture: %v", name, err)
	}
	return streamAdapterInputEvent{name: name, data: data}
}

func parseStreamAdapterOutput(t *testing.T, raw []byte) []streamAdapterOutputEvent {
	t.Helper()
	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	frames := strings.Split(normalized, "\n\n")
	result := make([]streamAdapterOutputEvent, 0, len(frames))
	for _, frame := range frames {
		if strings.TrimSpace(frame) == "" {
			continue
		}
		event := streamAdapterOutputEvent{}
		dataLines := make([]string, 0, 1)
		for _, line := range strings.Split(frame, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				event.name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			case strings.HasPrefix(line, ":"):
				continue
			default:
				t.Fatalf("unexpected SSE line %q in frame %q", line, frame)
			}
		}
		event.data = strings.Join(dataLines, "\n")
		if event.data == "" {
			t.Fatalf("SSE frame has no data: %q", frame)
		}
		if event.data != "[DONE]" {
			decoder := json.NewDecoder(strings.NewReader(event.data))
			decoder.UseNumber()
			if err := decoder.Decode(&event.root); err != nil {
				t.Fatalf("decode SSE JSON %q: %v", event.data, err)
			}
		}
		result = append(result, event)
	}
	return result
}

func assertStreamChatOutput(t *testing.T, source string, events []streamAdapterOutputEvent) {
	t.Helper()
	if events[len(events)-1].data != "[DONE]" {
		t.Fatalf("last Chat SSE event = %q, want [DONE]", events[len(events)-1].data)
	}

	text, reasoning, finishReason := "", "", ""
	toolIDs := map[int]string{}
	toolNames := map[int]string{}
	toolArguments := map[int]string{}
	var usage map[string]any
	usageIndex := -1
	for index, event := range events[:len(events)-1] {
		root := event.root
		if candidate := mapValue(root["usage"]); candidate != nil {
			usage = candidate
			usageIndex = index
		}
		choices := sliceValue(root["choices"])
		if len(choices) == 0 {
			continue
		}
		choice := mapValue(choices[0])
		if reason := stringValue(choice["finish_reason"]); reason != "" {
			finishReason = reason
		}
		delta := mapValue(choice["delta"])
		text += stringValue(delta["content"])
		reasoning += stringValue(delta["reasoning_content"])
		for _, rawCall := range sliceValue(delta["tool_calls"]) {
			call := mapValue(rawCall)
			callIndex := intValue(call["index"])
			function := mapValue(call["function"])
			if id := stringValue(call["id"]); id != "" {
				toolIDs[callIndex] = id
			}
			if name := stringValue(function["name"]); name != "" {
				toolNames[callIndex] = name
			}
			toolArguments[callIndex] += stringValue(function["arguments"])
		}
	}

	if text != streamText || reasoning != streamReasoning {
		t.Errorf("Chat text/reasoning = %q / %q, want %q / %q", text, reasoning, streamText, streamReasoning)
	}
	assertStreamTools(t, toolIDs, toolNames, toolArguments)
	if finishReason != "tool_calls" {
		t.Errorf("Chat finish_reason = %q, want tool_calls", finishReason)
	}
	if usage == nil {
		t.Fatal("Chat stream has no usage event")
	}
	if usageIndex != len(events)-2 {
		t.Errorf("Chat usage event index = %d, want immediately before [DONE] at %d", usageIndex, len(events)-2)
	}
	assertStreamOpenAIUsage(t, usage, true, true)
}

func assertStreamResponsesOutput(t *testing.T, source string, events []streamAdapterOutputEvent) {
	t.Helper()
	text, reasoning := "", ""
	added := map[int]string{}
	referencedIndexes := make([]int, 0)
	toolIDs := map[int]string{}
	toolNames := map[int]string{}
	toolArguments := map[int]string{}
	counts := map[string]int{}
	var completed map[string]any
	activeItems := map[int]struct {
		id       string
		itemType string
	}{}

	for index, event := range events {
		root := event.root
		typeName := stringValue(root["type"])
		counts[typeName]++
		if event.name != typeName {
			t.Errorf("Responses event %d name = %q, data.type = %q", index, event.name, typeName)
		}
		if sequence := intValue(root["sequence_number"]); sequence != index {
			t.Errorf("Responses event %d sequence_number = %d, want %d", index, sequence, index)
		}

		switch typeName {
		case "response.reasoning_summary_text.delta":
			reasoning += stringValue(root["delta"])
		case "response.output_text.delta":
			text += stringValue(root["delta"])
		case "response.output_item.added":
			outputIndex := intValue(root["output_index"])
			item := mapValue(root["item"])
			itemID := stringValue(item["id"])
			itemType := stringValue(item["type"])
			if active, exists := activeItems[outputIndex]; exists {
				t.Errorf("Responses output item %d (%s/%s) added while %s/%s was active at the same index", outputIndex, itemID, itemType, active.id, active.itemType)
			}
			activeItems[outputIndex] = struct {
				id       string
				itemType string
			}{id: itemID, itemType: itemType}
			if previous, exists := added[outputIndex]; exists {
				t.Errorf("Responses output_index %d reused by %s and another item", outputIndex, previous)
			}
			added[outputIndex] = itemType
			if itemType == "function_call" {
				toolIDs[outputIndex] = stringValue(item["call_id"])
				toolNames[outputIndex] = stringValue(item["name"])
			}
		case "response.function_call_arguments.delta":
			outputIndex := intValue(root["output_index"])
			active, exists := activeItems[outputIndex]
			if !exists {
				t.Errorf("Responses tool delta references inactive output_index %d", outputIndex)
			}
			if itemID := stringValue(root["item_id"]); itemID != active.id {
				t.Errorf("Responses tool delta item_id = %q, active item is %q", itemID, active.id)
			}
			referencedIndexes = append(referencedIndexes, outputIndex)
			toolArguments[outputIndex] += stringValue(root["delta"])
		case "response.function_call_arguments.done":
			outputIndex := intValue(root["output_index"])
			active, exists := activeItems[outputIndex]
			if !exists {
				t.Errorf("Responses tool done references inactive output_index %d", outputIndex)
			}
			if itemID := stringValue(root["item_id"]); itemID != active.id {
				t.Errorf("Responses tool done item_id = %q, active item is %q", itemID, active.id)
			}
			referencedIndexes = append(referencedIndexes, outputIndex)
			if got := stringValue(root["arguments"]); got != toolArguments[outputIndex] {
				t.Errorf("Responses tool %d done arguments = %q, deltas assembled %q", outputIndex, got, toolArguments[outputIndex])
			}
			if got := stringValue(root["name"]); got != toolNames[outputIndex] {
				t.Errorf("Responses tool %d done name = %q, want %q", outputIndex, got, toolNames[outputIndex])
			}
		case "response.output_item.done":
			outputIndex := intValue(root["output_index"])
			item := mapValue(root["item"])
			itemID := stringValue(item["id"])
			itemType := stringValue(item["type"])
			active, exists := activeItems[outputIndex]
			if !exists {
				t.Errorf("Responses output item %d (%s/%s) completed with no active item", outputIndex, itemID, itemType)
			} else if itemID != active.id || itemType != active.itemType {
				t.Errorf("Responses completed item %d (%s/%s), active item is %s/%s", outputIndex, itemID, itemType, active.id, active.itemType)
			}
			delete(activeItems, outputIndex)
		case "response.completed":
			if len(activeItems) != 0 {
				t.Errorf("Responses completed while output items were still active: %#v", activeItems)
			}
			completed = mapValue(root["response"])
		}
	}
	if len(activeItems) != 0 {
		t.Fatalf("Responses stream ended with active output items: %#v", activeItems)
	}

	if events[len(events)-1].name != "response.completed" || completed == nil {
		t.Fatalf("last Responses event = %q, want response.completed", events[len(events)-1].name)
	}
	if text != streamText || reasoning != streamReasoning {
		t.Errorf("Responses text/reasoning = %q / %q, want %q / %q", text, reasoning, streamText, streamReasoning)
	}
	if len(added) != 4 {
		t.Fatalf("Responses added output items = %#v, want reasoning, message, and two tools", added)
	}
	for index, wantType := range []string{"reasoning", "message", "function_call", "function_call"} {
		if got := added[index]; got != wantType {
			t.Errorf("Responses output_index %d type = %q, want %q", index, got, wantType)
		}
	}
	for _, index := range referencedIndexes {
		if _, exists := added[index]; !exists {
			t.Errorf("Responses event references unknown output_index %d", index)
		}
	}
	assertStreamTools(t, toolIDs, toolNames, toolArguments)

	for eventType, minimum := range map[string]int{
		"response.reasoning_summary_text.done":  1,
		"response.reasoning_summary_part.done":  1,
		"response.output_text.done":             1,
		"response.content_part.done":            1,
		"response.function_call_arguments.done": 2,
		"response.output_item.done":             4,
	} {
		if counts[eventType] < minimum {
			t.Errorf("Responses %s count = %d, want at least %d", eventType, counts[eventType], minimum)
		}
	}
	if stringValue(completed["status"]) != "completed" {
		t.Errorf("completed response status = %#v, want completed", completed["status"])
	}
	assertStreamOpenAIUsage(t, requireStreamMap(t, completed["usage"], "response.completed.response.usage"), false, true)
}

func assertStreamMessagesOutput(t *testing.T, events []streamAdapterOutputEvent) {
	t.Helper()
	if events[0].name != "message_start" || events[len(events)-1].name != "message_stop" {
		t.Fatalf("Messages event lifecycle starts/ends with %q/%q, want message_start/message_stop", events[0].name, events[len(events)-1].name)
	}

	openBlocks := map[int]bool{}
	nextIndex := 0
	blockTypes := map[int]string{}
	text, stopReason := "", ""
	toolIDs := map[int]string{}
	toolNames := map[int]string{}
	toolArguments := map[int]string{}
	outputTokens := -1

	for _, event := range events {
		root := event.root
		typeName := stringValue(root["type"])
		if event.name != typeName {
			t.Errorf("Messages event name = %q, data.type = %q", event.name, typeName)
		}
		switch typeName {
		case "message_start":
			usage := mapValue(mapValue(root["message"])["usage"])
			if usage == nil {
				t.Error("Messages message_start usage is null")
			}
		case "content_block_start":
			index := intValue(root["index"])
			if openBlocks[index] {
				t.Errorf("Messages block %d started twice without stopping", index)
			}
			if index != nextIndex {
				t.Errorf("Messages block index = %d, want %d", index, nextIndex)
			}
			openBlocks[index] = true
			nextIndex++
			block := mapValue(root["content_block"])
			blockType := stringValue(block["type"])
			blockTypes[index] = blockType
			if blockType == "tool_use" {
				toolIDs[index] = stringValue(block["id"])
				toolNames[index] = stringValue(block["name"])
			}
		case "content_block_delta":
			index := intValue(root["index"])
			if !openBlocks[index] {
				t.Errorf("Messages delta index = %d while that block is not open", index)
			}
			delta := mapValue(root["delta"])
			switch stringValue(delta["type"]) {
			case "thinking_delta":
				t.Errorf("converted Messages stream contains unverifiable thinking_delta: %#v", delta)
			case "text_delta":
				text += stringValue(delta["text"])
			case "input_json_delta":
				toolArguments[index] += stringValue(delta["partial_json"])
			}
		case "content_block_stop":
			index := intValue(root["index"])
			if !openBlocks[index] {
				t.Errorf("Messages stopped block %d while it was not open", index)
			}
			delete(openBlocks, index)
		case "message_delta":
			if len(openBlocks) != 0 {
				t.Errorf("Messages message_delta arrived while blocks were open: %#v", openBlocks)
			}
			stopReason = stringValue(mapValue(root["delta"])["stop_reason"])
			outputTokens = intValue(mapValue(root["usage"])["output_tokens"])
		case "message_stop":
			if len(openBlocks) != 0 {
				t.Errorf("Messages stream stopped while blocks were open: %#v", openBlocks)
			}
		}
	}

	if text != streamText {
		t.Errorf("Messages text = %q, want only visible answer %q", text, streamText)
	}
	if nextIndex != 3 {
		t.Fatalf("Messages emitted %d content blocks, want 3", nextIndex)
	}
	for index, wantType := range []string{"text", "tool_use", "tool_use"} {
		if got := blockTypes[index]; got != wantType {
			t.Errorf("Messages block %d type = %q, want %q", index, got, wantType)
		}
	}
	assertStreamTools(t, toolIDs, toolNames, toolArguments)
	if stopReason != "tool_use" {
		t.Errorf("Messages stop_reason = %q, want tool_use", stopReason)
	}
	if outputTokens != 5 {
		t.Errorf("Messages final output_tokens = %d, want 5", outputTokens)
	}
}

func assertStreamTools(t *testing.T, ids, names, arguments map[int]string) {
	t.Helper()
	indexes := make([]int, 0, len(ids))
	for index := range ids {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	if len(indexes) != 2 {
		t.Fatalf("tool indexes = %v, want two tools", indexes)
	}
	wants := []struct {
		id, name, arguments string
	}{
		{id: streamCallID0, name: streamCallName0, arguments: streamCallArgs0},
		{id: streamCallID1, name: streamCallName1, arguments: streamCallArgs1},
	}
	for position, index := range indexes {
		want := wants[position]
		if ids[index] != want.id || names[index] != want.name || arguments[index] != want.arguments {
			t.Errorf("tool %d = id %q name %q args %q, want id %q name %q args %q", index, ids[index], names[index], arguments[index], want.id, want.name, want.arguments)
		}
	}
}

func assertStreamOpenAIUsage(t *testing.T, usage map[string]any, chat, wantReasoning bool) {
	t.Helper()
	inputKey, outputKey := "input_tokens", "output_tokens"
	inputDetailsKey, outputDetailsKey := "input_tokens_details", "output_tokens_details"
	if chat {
		inputKey, outputKey = "prompt_tokens", "completion_tokens"
		inputDetailsKey, outputDetailsKey = "prompt_tokens_details", "completion_tokens_details"
	}
	if intValue(usage[inputKey]) != 20 || intValue(usage[outputKey]) != 5 || intValue(usage["total_tokens"]) != 25 {
		t.Errorf("stream usage = %#v, want input=20 output=5 total=25", usage)
	}
	if cached := intValue(mapValue(usage[inputDetailsKey])["cached_tokens"]); cached != 2 {
		t.Errorf("%s.cached_tokens = %d, want 2", inputDetailsKey, cached)
	}
	outputDetails := mapValue(usage[outputDetailsKey])
	if wantReasoning {
		if reasoning := intValue(outputDetails["reasoning_tokens"]); reasoning != 3 {
			t.Errorf("%s.reasoning_tokens = %d, want 3", outputDetailsKey, reasoning)
		}
	} else if outputDetails != nil {
		t.Errorf("%s = %#v, want omitted when source did not report reasoning tokens", outputDetailsKey, outputDetails)
	}
}

func requireStreamMap(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	result := mapValue(value)
	if result == nil {
		t.Fatalf("%s = %#v, want JSON object", path, value)
	}
	return result
}

func TestProtocolStreamAdapterIgnoresEventsAfterTerminalFrame(t *testing.T) {
	t.Parallel()
	adapter := newProtocolStreamAdapter("chat_completions", "responses")
	for _, event := range streamAdapterFixture(t, "chat_completions") {
		if _, err := adapter.Translate(event.name, event.data); err != nil {
			t.Fatalf("translate fixture: %v", err)
		}
	}
	got, err := adapter.Translate("", []byte(`{"id":"late","choices":[]}`))
	if err != nil {
		t.Fatalf("translate after terminal event: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("adapter emitted %q after terminal event", got)
	}
}

func TestProtocolStreamAdapterSameProtocolPreservesEventFrame(t *testing.T) {
	t.Parallel()
	adapter := newProtocolStreamAdapter("messages", "messages")
	data := []byte(`{"type":"ping"}`)
	got, err := adapter.Translate("ping", data)
	if err != nil {
		t.Fatalf("same-protocol Translate: %v", err)
	}
	want := fmt.Sprintf("event: ping\ndata: %s\n\n", data)
	if string(got) != want {
		t.Fatalf("same-protocol SSE frame = %q, want %q", got, want)
	}
}

func TestProtocolStreamAdapterFinalizesChatStreamWithoutDoneSentinel(t *testing.T) {
	t.Parallel()
	adapter := newProtocolStreamAdapter("chat_completions", "responses")
	inputs := [][]byte{
		[]byte(`{"id":"chat-no-done","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"hello"},"finish_reason":null}]}`),
		[]byte(`{"id":"chat-no-done","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`),
		[]byte(`{"id":"chat-no-done","model":"test-model","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`),
	}
	var output bytes.Buffer
	for _, input := range inputs {
		translated, err := adapter.Translate("", input)
		if err != nil {
			t.Fatalf("translate Chat event: %v", err)
		}
		output.Write(translated)
	}
	output.Write(adapter.Finalize())
	events := parseStreamAdapterOutput(t, output.Bytes())
	completed := 0
	for _, event := range events {
		if event.name != "response.completed" {
			continue
		}
		completed++
		usage := mapValue(mapValue(event.root["response"])["usage"])
		if intValue(usage["input_tokens"]) != 7 || intValue(usage["output_tokens"]) != 2 || intValue(usage["total_tokens"]) != 9 {
			t.Fatalf("completed usage = %#v, want 7/2/9", usage)
		}
	}
	if completed != 1 {
		t.Fatalf("response.completed count = %d, want 1", completed)
	}
	if extra := adapter.Finalize(); len(extra) != 0 {
		t.Fatalf("second Finalize emitted %q", extra)
	}
}

func TestProtocolStreamAdapterFinalizesMessagesTextWithoutDoneSentinel(t *testing.T) {
	t.Parallel()

	adapter := newProtocolStreamAdapter("chat_completions", "messages")
	inputs := [][]byte{
		[]byte(`{"id":"chat-no-done","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"hello"},"finish_reason":null}]}`),
		[]byte(`{"id":"chat-no-done","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`),
		[]byte(`{"id":"chat-no-done","model":"test-model","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`),
	}
	var output bytes.Buffer
	for _, input := range inputs {
		translated, err := adapter.Translate("", input)
		if err != nil {
			t.Fatalf("translate Chat event: %v", err)
		}
		output.Write(translated)
	}
	output.Write(adapter.Finalize())

	text := ""
	for _, event := range parseStreamAdapterOutput(t, output.Bytes()) {
		if event.name == "content_block_delta" && stringValue(mapValue(event.root["delta"])["type"]) == "text_delta" {
			text += stringValue(mapValue(event.root["delta"])["text"])
		}
	}
	if text != "hello" {
		t.Fatalf("finalized Messages text = %q, want hello", text)
	}
}

func TestProtocolStreamAdapterPreservesChatLengthStopAtDoneSentinel(t *testing.T) {
	t.Parallel()

	adapter := newProtocolStreamAdapter("chat_completions", "responses")
	inputs := [][]byte{
		[]byte(`{"id":"chat-length","model":"test-model","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`),
		[]byte(`{"id":"chat-length","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"length"}]}`),
		[]byte(`[DONE]`),
	}
	var output bytes.Buffer
	for _, input := range inputs {
		translated, err := adapter.Translate("", input)
		if err != nil {
			t.Fatalf("translate Chat event: %v", err)
		}
		output.Write(translated)
	}
	for _, event := range parseStreamAdapterOutput(t, output.Bytes()) {
		if event.name != "response.incomplete" {
			continue
		}
		response := mapValue(event.root["response"])
		if stringValue(response["status"]) != "incomplete" || stringValue(mapValue(response["incomplete_details"])["reason"]) != "max_output_tokens" {
			t.Fatalf("incomplete response = %#v, want max_output_tokens incomplete", response)
		}
		if stringValue(event.root["type"]) != "response.incomplete" {
			t.Fatalf("incomplete event type = %#v", event.root["type"])
		}
		return
	}
	t.Fatal("converted stream has no response.incomplete event")
}

func TestProtocolStreamAdapterAcceptsResponsesDoneOnlyToolArguments(t *testing.T) {
	t.Parallel()

	adapter := newProtocolStreamAdapter("responses", "chat_completions")
	inputs := []streamAdapterInputEvent{
		streamJSONEvent(t, "response.created", map[string]any{"type": "response.created", "response": map[string]any{"id": "resp-done-only", "model": "test-model"}}),
		streamJSONEvent(t, "response.function_call_arguments.done", map[string]any{"type": "response.function_call_arguments.done", "item_id": "fc-done-only", "output_index": 0, "name": "lookup", "arguments": `{"city":"Shanghai"}`}),
		streamJSONEvent(t, "response.completed", map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp-done-only", "model": "test-model", "status": "completed", "usage": map[string]any{"input_tokens": 4, "output_tokens": 2, "total_tokens": 6}}}),
	}
	var output bytes.Buffer
	for _, input := range inputs {
		translated, err := adapter.Translate(input.name, input.data)
		if err != nil {
			t.Fatalf("translate Responses event: %v", err)
		}
		output.Write(translated)
	}
	for _, event := range parseStreamAdapterOutput(t, output.Bytes()) {
		choices := sliceValue(event.root["choices"])
		if len(choices) == 0 {
			continue
		}
		delta := mapValue(mapValue(choices[0])["delta"])
		calls := sliceValue(delta["tool_calls"])
		if len(calls) == 0 {
			continue
		}
		call := mapValue(calls[0])
		function := mapValue(call["function"])
		if stringValue(call["id"]) != "fc-done-only" || stringValue(function["name"]) != "lookup" || stringValue(function["arguments"]) != `{"city":"Shanghai"}` {
			t.Fatalf("done-only tool call = %#v", call)
		}
		return
	}
	t.Fatal("done-only Responses event produced no Chat tool call")
}

func TestProtocolStreamAdapterIgnoresAnthropicPingBeforeMessageStart(t *testing.T) {
	t.Parallel()

	adapter := newProtocolStreamAdapter("messages", "chat_completions")
	got, err := adapter.Translate("ping", []byte(`{"type":"ping"}`))
	if err != nil {
		t.Fatalf("translate ping: %v", err)
	}
	if len(got) != 0 || adapter.started {
		t.Fatalf("ping emitted %q or started the target stream", got)
	}
}

func TestProtocolStreamAdapterPreservesSignedThinkingFromBufferedMessages(t *testing.T) {
	t.Parallel()

	body := []byte(`{"id":"msg-buffered","type":"message","role":"assistant","model":"claude-test","content":[{"type":"thinking","thinking":"check it","signature":"opaque-signature"},{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":4,"output_tokens":2}}`)
	adapter := newProtocolStreamAdapter("messages", "messages")
	stream, err := adapter.FromBufferedResponse(body)
	if err != nil {
		t.Fatalf("convert buffered Messages response: %v", err)
	}
	events := parseStreamAdapterOutput(t, stream)
	thinking, signature, text := "", "", ""
	for _, event := range events {
		if event.name != "content_block_delta" {
			continue
		}
		delta := mapValue(event.root["delta"])
		switch stringValue(delta["type"]) {
		case "thinking_delta":
			thinking += stringValue(delta["thinking"])
		case "signature_delta":
			signature += stringValue(delta["signature"])
		case "text_delta":
			text += stringValue(delta["text"])
		}
	}
	if thinking != "check it" || signature != "opaque-signature" || text != "done" || events[len(events)-1].name != "message_stop" {
		t.Fatalf("buffered Messages stream thinking=%q signature=%q text=%q last=%q", thinking, signature, text, events[len(events)-1].name)
	}
}

func TestBufferedResponsesWebSearchBeforeMessagePreservesMessagesSSEOrder(t *testing.T) {
	t.Parallel()

	body := mustTestJSON(t, map[string]any{
		"id": "resp-buffered-order", "object": "response", "status": "completed", "model": "source",
		"output": []any{
			map[string]any{"id": "ws_first", "type": "web_search_call", "status": "completed", "action": map[string]any{
				"type": "search", "query": "Novro", "sources": []any{map[string]any{"type": "url", "url": "https://example.test/source", "title": "Source"}},
			}},
			map[string]any{"id": "msg_after", "type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "after search"}}},
		},
	})
	adapter := newProtocolStreamAdapter("responses", "messages")
	stream, err := adapter.FromBufferedResponse(body)
	if err != nil {
		t.Fatalf("convert buffered Responses response: %v", err)
	}
	starts := make([]string, 0, 3)
	for _, event := range parseStreamAdapterOutput(t, stream) {
		if event.name == "content_block_start" {
			starts = append(starts, stringValue(mapValue(event.root["content_block"])["type"]))
		}
	}
	if want := []string{"server_tool_use", "web_search_tool_result", "text"}; !reflect.DeepEqual(starts, want) {
		t.Fatalf("Messages buffered block order = %v, want %v\n%s", starts, want, stream)
	}
}

func TestBufferedMessagesOutputOrderMatchesResponsesLifecycleAndTerminal(t *testing.T) {
	t.Parallel()

	body := mustTestJSON(t, map[string]any{
		"id": "msg-buffered-order", "type": "message", "role": "assistant", "model": "source", "stop_reason": "tool_use",
		"content": []any{
			map[string]any{"type": "thinking", "thinking": "plan", "signature": "signed-plan"},
			map[string]any{"type": "server_tool_use", "id": "ws_first", "name": "web_search", "input": map[string]any{"query": "Novro"}},
			map[string]any{"type": "web_search_tool_result", "tool_use_id": "ws_first", "content": []any{map[string]any{"type": "web_search_result", "url": "https://example.test/source", "title": "Source"}}},
			map[string]any{"type": "text", "text": "after search"},
			map[string]any{"type": "tool_use", "id": "call_after", "name": "finish", "input": map[string]any{}},
		},
	})
	adapter := newProtocolStreamAdapter("messages", "responses")
	stream, err := adapter.FromBufferedResponse(body)
	if err != nil {
		t.Fatalf("convert buffered Messages response: %v", err)
	}
	events := parseStreamAdapterOutput(t, stream)
	addedTypes := make([]string, 0, 4)
	doneByIndex := map[int]map[string]any{}
	var terminal map[string]any
	for _, event := range events {
		switch event.name {
		case "response.output_item.added":
			index := intValue(event.root["output_index"])
			if index != len(addedTypes) {
				t.Fatalf("added output_index = %d, want %d", index, len(addedTypes))
			}
			addedTypes = append(addedTypes, stringValue(mapValue(event.root["item"])["type"]))
		case "response.output_item.done":
			doneByIndex[intValue(event.root["output_index"])] = mapValue(event.root["item"])
		case "response.completed":
			terminal = mapValue(event.root["response"])
		}
	}
	wantTypes := []string{"reasoning", "web_search_call", "message", "function_call"}
	if !reflect.DeepEqual(addedTypes, wantTypes) {
		t.Fatalf("Responses added order = %v, want %v\n%s", addedTypes, wantTypes, stream)
	}
	if terminal == nil {
		t.Fatal("buffered Responses stream has no response.completed event")
	}
	terminalOutput := sliceValue(terminal["output"])
	if got := responseContentTypes(terminalOutput); !reflect.DeepEqual(got, wantTypes) {
		t.Fatalf("terminal output order = %v, want %v", got, wantTypes)
	}
	for index, rawItem := range terminalOutput {
		item := mapValue(rawItem)
		done := doneByIndex[index]
		if done == nil || stringValue(done["id"]) != stringValue(item["id"]) || stringValue(done["type"]) != stringValue(item["type"]) {
			t.Fatalf("output %d lifecycle/terminal mismatch: done=%#v terminal=%#v", index, done, item)
		}
	}
}

func TestProtocolStreamAdapterTextToolTextCreatesValidResponsesItems(t *testing.T) {
	t.Parallel()

	adapter := newProtocolStreamAdapter("chat_completions", "responses")
	inputs := [][]byte{
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"content": "before"}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"tool_calls": []any{map[string]any{
			"index": 0, "id": "call_middle", "type": "function", "function": map[string]any{"name": "lookup", "arguments": `{}`},
		}}}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"content": "after"}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{}, "tool_calls")).data,
		[]byte("[DONE]"),
	}
	var raw bytes.Buffer
	for _, input := range inputs {
		converted, err := adapter.Translate("", input)
		if err != nil {
			t.Fatalf("translate Chat stream: %v", err)
		}
		raw.Write(converted)
	}

	events := parseStreamAdapterOutput(t, raw.Bytes())
	added := make([]map[string]any, 0, 3)
	doneAt := map[string]int{}
	var terminal map[string]any
	for eventIndex, event := range events {
		itemID := stringValue(event.root["item_id"])
		if itemID != "" {
			if doneIndex, closed := doneAt[itemID]; closed {
				t.Fatalf("event %q at %d targets completed item %q closed at %d", event.name, eventIndex, itemID, doneIndex)
			}
		}
		switch event.name {
		case "response.output_item.added":
			added = append(added, mapValue(event.root["item"]))
		case "response.output_item.done":
			doneAt[stringValue(mapValue(event.root["item"])["id"])] = eventIndex
		case "response.completed":
			terminal = mapValue(event.root["response"])
		}
	}
	if got := responseContentTypes(anyMapsToSlice(added)); !reflect.DeepEqual(got, []string{"message", "function_call", "message"}) {
		t.Fatalf("Responses added item order = %v\n%s", got, raw.Bytes())
	}
	if terminal == nil || !reflect.DeepEqual(responseContentTypes(sliceValue(terminal["output"])), []string{"message", "function_call", "message"}) {
		t.Fatalf("terminal output = %#v", terminal)
	}
	output := sliceValue(terminal["output"])
	if text := stringValue(mapValue(sliceValue(mapValue(output[0])["content"])[0])["text"]); text != "before" {
		t.Fatalf("first terminal message text = %q", text)
	}
	if text := stringValue(mapValue(sliceValue(mapValue(output[2])["content"])[0])["text"]); text != "after" {
		t.Fatalf("second terminal message text = %q", text)
	}
}

func TestProtocolStreamAdapterRepeatedAnnotationEmitsOnce(t *testing.T) {
	t.Parallel()

	annotation := map[string]any{"type": "url_citation", "url": "https://example.test/source", "title": "Source", "start_index": 0, "end_index": 4}
	adapter := newProtocolStreamAdapter("responses", "chat_completions")
	inputs := []streamAdapterInputEvent{
		streamJSONEvent(t, "response.created", map[string]any{"type": "response.created", "response": map[string]any{"id": "resp_annotation", "model": "source"}}),
		streamJSONEvent(t, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "item_id": "msg_annotation", "output_index": 0, "content_index": 0, "delta": "text"}),
		streamJSONEvent(t, "response.output_text.annotation.added", map[string]any{"type": "response.output_text.annotation.added", "item_id": "msg_annotation", "output_index": 0, "content_index": 0, "annotation_index": 0, "annotation": annotation}),
		streamJSONEvent(t, "response.content_part.done", map[string]any{"type": "response.content_part.done", "item_id": "msg_annotation", "output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "text", "annotations": []any{annotation}}}),
		streamJSONEvent(t, "response.completed", map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp_annotation", "status": "completed", "model": "source"}}),
	}
	var raw bytes.Buffer
	for _, input := range inputs {
		converted, err := adapter.Translate(input.name, input.data)
		if err != nil {
			t.Fatalf("translate Responses annotation: %v", err)
		}
		raw.Write(converted)
	}
	annotationChunks := 0
	for _, event := range parseStreamAdapterOutput(t, raw.Bytes()) {
		choices := sliceValue(event.root["choices"])
		if len(choices) == 0 {
			continue
		}
		annotationChunks += len(sliceValue(mapValue(mapValue(choices[0])["delta"])["annotations"]))
	}
	if annotationChunks != 1 {
		t.Fatalf("Chat annotation chunks = %d, want 1\n%s", annotationChunks, raw.Bytes())
	}
}

func anyMapsToSlice(values []map[string]any) []any {
	result := make([]any, len(values))
	for index := range values {
		result[index] = values[index]
	}
	return result
}

func TestProtocolStreamAdapterDoesNotExposeLateReasoningAsResponseText(t *testing.T) {
	t.Parallel()

	adapter := newProtocolStreamAdapter("chat_completions", "responses")
	inputs := [][]byte{
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"reasoning_content": "initial reasoning"}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"content": "answer"}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"reasoning_content": "late private reasoning"}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"content": " complete"}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{}, "stop")).data,
		[]byte("[DONE]"),
	}

	var output bytes.Buffer
	for _, input := range inputs {
		translated, err := adapter.Translate("", input)
		if err != nil {
			t.Fatalf("translate Chat event: %v", err)
		}
		output.Write(translated)
	}

	text, reasoning := "", ""
	var completed map[string]any
	for _, event := range parseStreamAdapterOutput(t, output.Bytes()) {
		switch event.name {
		case "response.output_text.delta":
			text += stringValue(event.root["delta"])
		case "response.reasoning_summary_text.delta":
			reasoning += stringValue(event.root["delta"])
		case "response.completed":
			completed = mapValue(event.root["response"])
		}
	}
	if text != "answer complete" || reasoning != "initial reasoning" {
		t.Fatalf("converted text/reasoning = %q / %q", text, reasoning)
	}
	encoded, err := json.Marshal(completed)
	if err != nil {
		t.Fatalf("marshal completed response: %v", err)
	}
	if strings.Contains(string(encoded), "late private reasoning") {
		t.Fatalf("late reasoning leaked into completed response: %s", encoded)
	}
}

func TestProtocolStreamAdapterSuppressesDuplicatedUnsignedReasoningTextInMessages(t *testing.T) {
	t.Parallel()

	adapter := newProtocolStreamAdapter("chat_completions", "messages")
	inputs := [][]byte{
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"reasoning_content": "private "}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"reasoning_content": "plan"}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"content": "private"}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"content": " plan"}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call_one", "type": "function", "function": map[string]any{"name": "lookup", "arguments": `{}`}}}}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{}, "tool_calls")).data,
		[]byte(`[DONE]`),
	}
	var output bytes.Buffer
	for _, input := range inputs {
		translated, err := adapter.Translate("", input)
		if err != nil {
			t.Fatalf("translate Chat event: %v", err)
		}
		output.Write(translated)
	}

	starts := make([]map[string]any, 0)
	for _, event := range parseStreamAdapterOutput(t, output.Bytes()) {
		if event.name == "content_block_delta" && stringValue(mapValue(event.root["delta"])["type"]) == "text_delta" {
			t.Fatalf("Messages stream exposes duplicated reasoning as text: %#v", event.root)
		}
		if event.name == "content_block_start" {
			starts = append(starts, event.root)
		}
	}
	if len(starts) != 1 || intValue(starts[0]["index"]) != 0 || stringValue(mapValue(starts[0]["content_block"])["type"]) != "tool_use" {
		t.Fatalf("Messages block starts = %#v, want one tool_use at index 0", starts)
	}
}

func TestProtocolStreamAdapterStreamsChatTextToMessagesBeforeTerminal(t *testing.T) {
	t.Parallel()

	adapter := newProtocolStreamAdapter("chat_completions", "messages")
	translated, err := adapter.Translate("", streamJSONEvent(t, "", chatStreamChunk(map[string]any{"content": "hello now"}, nil)).data)
	if err != nil {
		t.Fatalf("translate Chat text delta: %v", err)
	}
	events := parseStreamAdapterOutput(t, translated)
	if len(events) != 3 {
		t.Fatalf("Messages events = %d, want message_start, content_block_start, and content_block_delta: %s", len(events), translated)
	}
	if events[0].name != "message_start" || events[1].name != "content_block_start" || events[2].name != "content_block_delta" {
		t.Fatalf("Messages event sequence = %q/%q/%q", events[0].name, events[1].name, events[2].name)
	}
	delta := mapValue(events[2].root["delta"])
	if stringValue(delta["type"]) != "text_delta" || stringValue(delta["text"]) != "hello now" {
		t.Fatalf("Messages first text delta = %#v", delta)
	}
	if adapter.finished {
		t.Fatal("Messages adapter finished before the source terminal event")
	}
}

func TestProtocolStreamAdapterReleasesMessagesTextWhenItDiffersFromUnsignedReasoning(t *testing.T) {
	t.Parallel()

	adapter := newProtocolStreamAdapter("chat_completions", "messages")
	inputs := [][]byte{
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"reasoning_content": "private plan"}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"content": "private"}, nil)).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{"content": " answer"}, nil)).data,
	}
	var output bytes.Buffer
	for index, input := range inputs {
		translated, err := adapter.Translate("", input)
		if err != nil {
			t.Fatalf("translate input %d: %v", index, err)
		}
		if index == 1 && strings.Contains(string(translated), "text_delta") {
			t.Fatalf("ambiguous reasoning prefix was emitted early: %s", translated)
		}
		output.Write(translated)
	}

	text := ""
	for _, event := range parseStreamAdapterOutput(t, output.Bytes()) {
		if event.name == "content_block_delta" && stringValue(mapValue(event.root["delta"])["type"]) == "text_delta" {
			text += stringValue(mapValue(event.root["delta"])["text"])
		}
	}
	if text != "private answer" {
		t.Fatalf("released Messages text = %q, want %q", text, "private answer")
	}
	if adapter.finished {
		t.Fatal("Messages adapter finished before the source terminal event")
	}
}

func TestProtocolStreamAdapterStreamsToolArgumentsToMessagesBeforeTerminal(t *testing.T) {
	adapter := newProtocolStreamAdapter("chat_completions", "messages")
	first := chatStreamChunk(map[string]any{"tool_calls": []any{map[string]any{
		"index": 0, "id": "call_lookup", "type": "function",
		"function": map[string]any{"name": "lookup", "arguments": `{"city":"`},
	}}}, nil)
	firstData, _ := json.Marshal(first)
	firstOutput, err := adapter.Translate("", firstData)
	if err != nil {
		t.Fatalf("first tool delta: %v", err)
	}
	firstEvents := parseStreamAdapterOutput(t, firstOutput)
	if !containsStreamEvent(firstEvents, "content_block_start") || !containsStreamEvent(firstEvents, "content_block_delta") {
		t.Fatalf("first Messages tool delta was not emitted incrementally: %s", firstOutput)
	}
	if containsStreamEvent(firstEvents, "content_block_stop") || adapter.finished {
		t.Fatal("Messages tool adapter finished before terminal event")
	}

	second := chatStreamChunk(map[string]any{"tool_calls": []any{map[string]any{
		"index": 0, "function": map[string]any{"arguments": `Shanghai"}`},
	}}}, nil)
	secondData, _ := json.Marshal(second)
	secondOutput, err := adapter.Translate("", secondData)
	if err != nil {
		t.Fatalf("second tool delta: %v", err)
	}
	if !containsStreamEvent(parseStreamAdapterOutput(t, secondOutput), "content_block_delta") {
		t.Fatalf("second Messages tool delta was not emitted incrementally: %s", secondOutput)
	}

	terminal := chatStreamChunk(map[string]any{}, "tool_calls")
	terminalData, _ := json.Marshal(terminal)
	if _, err := adapter.Translate("", terminalData); err != nil {
		t.Fatalf("tool finish_reason: %v", err)
	}
	terminalOutput, err := adapter.Translate("", []byte("[DONE]"))
	if err != nil {
		t.Fatalf("tool terminal: %v", err)
	}
	terminalEvents := parseStreamAdapterOutput(t, terminalOutput)
	if !containsStreamEvent(terminalEvents, "content_block_stop") || !containsStreamEvent(terminalEvents, "message_stop") {
		t.Fatalf("Messages tool terminal lifecycle incomplete: %s", terminalOutput)
	}
}

func TestProtocolStreamAdapterStreamsToolArgumentsToResponsesBeforeTerminal(t *testing.T) {
	adapter := newProtocolStreamAdapter("chat_completions", "responses")
	first := chatStreamChunk(map[string]any{"tool_calls": []any{map[string]any{
		"index": 0, "id": "call_lookup", "type": "function",
		"function": map[string]any{"name": "lookup", "arguments": `{"city":"`},
	}}}, nil)
	firstData, _ := json.Marshal(first)
	firstOutput, err := adapter.Translate("", firstData)
	if err != nil {
		t.Fatalf("first Responses tool delta: %v", err)
	}
	firstEvents := parseStreamAdapterOutput(t, firstOutput)
	if !containsStreamEvent(firstEvents, "response.output_item.added") || !containsStreamEvent(firstEvents, "response.function_call_arguments.delta") {
		t.Fatalf("first Responses tool delta was not emitted incrementally: %s", firstOutput)
	}
	if containsStreamEvent(firstEvents, "response.function_call_arguments.done") || adapter.finished {
		t.Fatal("Responses tool adapter finished before terminal event")
	}

	second := chatStreamChunk(map[string]any{"tool_calls": []any{map[string]any{
		"index": 0, "function": map[string]any{"arguments": `Shanghai"}`},
	}}}, nil)
	secondData, _ := json.Marshal(second)
	secondOutput, err := adapter.Translate("", secondData)
	if err != nil {
		t.Fatalf("second Responses tool delta: %v", err)
	}
	if !containsStreamEvent(parseStreamAdapterOutput(t, secondOutput), "response.function_call_arguments.delta") {
		t.Fatalf("second Responses tool delta was not emitted incrementally: %s", secondOutput)
	}

	terminal := chatStreamChunk(map[string]any{}, "tool_calls")
	terminalData, _ := json.Marshal(terminal)
	if _, err := adapter.Translate("", terminalData); err != nil {
		t.Fatalf("Responses tool finish_reason: %v", err)
	}
	terminalOutput, err := adapter.Translate("", []byte("[DONE]"))
	if err != nil {
		t.Fatalf("Responses tool terminal: %v", err)
	}
	terminalEvents := parseStreamAdapterOutput(t, terminalOutput)
	if !containsStreamEvent(terminalEvents, "response.function_call_arguments.done") || !containsStreamEvent(terminalEvents, "response.completed") {
		t.Fatalf("Responses tool terminal lifecycle incomplete: %s", terminalOutput)
	}
}

func containsStreamEvent(events []streamAdapterOutputEvent, name string) bool {
	for _, event := range events {
		if event.name == name {
			return true
		}
	}
	return false
}

func TestProtocolStreamAdapterNormalizesMissingAndDuplicateToolIDs(t *testing.T) {
	t.Parallel()

	adapter := newProtocolStreamAdapter("chat_completions", "responses")
	toolChunk := chatStreamChunk(map[string]any{"tool_calls": []any{
		map[string]any{"index": 0, "id": "call_duplicate", "type": "function", "function": map[string]any{"name": "first", "arguments": `{}`}},
		map[string]any{"index": 1, "id": "call_duplicate", "type": "function", "function": map[string]any{"name": "second", "arguments": `{}`}},
		map[string]any{"index": 2, "type": "function", "function": map[string]any{"name": "third", "arguments": `{}`}},
	}}, nil)
	inputs := [][]byte{
		streamJSONEvent(t, "", toolChunk).data,
		streamJSONEvent(t, "", chatStreamChunk(map[string]any{}, "tool_calls")).data,
		[]byte("[DONE]"),
	}

	var output bytes.Buffer
	for _, input := range inputs {
		translated, err := adapter.Translate("", input)
		if err != nil {
			t.Fatalf("translate Chat event: %v", err)
		}
		output.Write(translated)
	}

	addedIDs := make([]string, 0, 3)
	addedCallIDs := make([]string, 0, 3)
	doneIDs := make([]string, 0, 3)
	var completed map[string]any
	for _, event := range parseStreamAdapterOutput(t, output.Bytes()) {
		switch event.name {
		case "response.output_item.added":
			item := mapValue(event.root["item"])
			if stringValue(item["type"]) == "function_call" {
				addedIDs = append(addedIDs, stringValue(item["id"]))
				addedCallIDs = append(addedCallIDs, stringValue(item["call_id"]))
			}
		case "response.output_item.done":
			item := mapValue(event.root["item"])
			if stringValue(item["type"]) == "function_call" {
				doneIDs = append(doneIDs, stringValue(item["id"]))
			}
		case "response.completed":
			completed = mapValue(event.root["response"])
		}
	}
	if len(addedIDs) != 3 || len(doneIDs) != 3 || completed == nil {
		t.Fatalf("tool lifecycle counts added=%d done=%d completed=%v", len(addedIDs), len(doneIDs), completed != nil)
	}
	seenIDs := map[string]struct{}{}
	seenCallIDs := map[string]struct{}{}
	terminalTools := sliceValue(completed["output"])
	if len(terminalTools) != 3 {
		t.Fatalf("terminal tool count = %d, want 3", len(terminalTools))
	}
	for index := range addedIDs {
		terminal := mapValue(terminalTools[index])
		if addedIDs[index] == "" || addedCallIDs[index] == "" {
			t.Fatalf("tool %d has empty item/call ID: %q / %q", index, addedIDs[index], addedCallIDs[index])
		}
		if _, exists := seenIDs[addedIDs[index]]; exists {
			t.Fatalf("duplicate tool item ID %q", addedIDs[index])
		}
		if _, exists := seenCallIDs[addedCallIDs[index]]; exists {
			t.Fatalf("duplicate tool call ID %q", addedCallIDs[index])
		}
		seenIDs[addedIDs[index]] = struct{}{}
		seenCallIDs[addedCallIDs[index]] = struct{}{}
		if doneIDs[index] != addedIDs[index] || stringValue(terminal["id"]) != addedIDs[index] || stringValue(terminal["call_id"]) != addedCallIDs[index] {
			t.Fatalf("tool %d identities added=%q/%q done=%q terminal=%q/%q", index, addedIDs[index], addedCallIDs[index], doneIDs[index], terminal["id"], terminal["call_id"])
		}
	}
}
