package gateway

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type extendedStreamInput struct {
	event string
	data  string
}

type extendedStreamEvent struct {
	event string
	data  map[string]any
	raw   string
}

func TestExtendedChatRefusalStreamToResponses(t *testing.T) {
	adapter := newProtocolStreamAdapter("chat_completions", "responses")
	events := translateExtendedStream(t, adapter, []extendedStreamInput{
		{data: `{"id":"chat-refusal","model":"glm-5.3","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`},
		{data: `{"id":"chat-refusal","model":"glm-5.3","choices":[{"index":0,"delta":{"refusal":"I cannot "},"finish_reason":null}]}`},
		{data: `{"id":"chat-refusal","model":"glm-5.3","choices":[{"index":0,"delta":{"refusal":"help."},"finish_reason":null}]}`},
		{data: `{"id":"chat-refusal","model":"glm-5.3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`},
		{data: `[DONE]`},
	})

	types := extendedEventTypes(events)
	for _, wanted := range []string{
		"response.created", "response.in_progress", "response.output_item.added",
		"response.content_part.added", "response.refusal.delta", "response.refusal.done",
		"response.content_part.done", "response.output_item.done", "response.incomplete",
	} {
		if !containsExtendedType(types, wanted) {
			t.Fatalf("missing %s in event sequence %v", wanted, types)
		}
	}
	if countExtendedType(types, "response.refusal.delta") != 2 {
		t.Fatalf("refusal delta count = %d, want 2; events=%v", countExtendedType(types, "response.refusal.delta"), types)
	}
	partAdded := findExtendedEvent(t, events, "response.content_part.added")
	if part := mapValue(partAdded.data["part"]); stringValue(part["type"]) != "refusal" {
		t.Fatalf("content part = %#v, want refusal", part)
	}
	itemDone := findExtendedEvent(t, events, "response.output_item.done")
	content := sliceValue(mapValue(itemDone.data["item"])["content"])
	if len(content) != 1 || stringValue(mapValue(content[0])["refusal"]) != "I cannot help." {
		t.Fatalf("completed refusal content = %#v", content)
	}
}

func TestExtendedResponsesRefusalStreamToChatAndMessages(t *testing.T) {
	inputs := []extendedStreamInput{
		{event: "response.created", data: `{"type":"response.created","response":{"id":"resp-refusal","model":"k3","status":"in_progress"}}`},
		{event: "response.refusal.delta", data: `{"type":"response.refusal.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"Cannot assist."}`},
		{event: "response.completed", data: `{"type":"response.completed","response":{"id":"resp-refusal","model":"k3","status":"completed","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`},
	}

	chatEvents := translateExtendedStream(t, newProtocolStreamAdapter("responses", "chat_completions"), inputs)
	var refusal string
	for _, event := range chatEvents {
		choices := sliceValue(event.data["choices"])
		if len(choices) == 0 {
			continue
		}
		refusal += stringValue(mapValue(mapValue(choices[0])["delta"])["refusal"])
	}
	if refusal != "Cannot assist." {
		t.Fatalf("chat refusal = %q", refusal)
	}

	messageEvents := translateExtendedStream(t, newProtocolStreamAdapter("responses", "messages"), inputs)
	var text string
	for _, event := range messageEvents {
		if event.event != "content_block_delta" {
			continue
		}
		delta := mapValue(event.data["delta"])
		if stringValue(delta["type"]) == "text_delta" {
			text += stringValue(delta["text"])
		}
	}
	if text != "Cannot assist." {
		t.Fatalf("Messages weak refusal mapping = %q", text)
	}
}

func TestExtendedResponsesWebSearchStreamToMessages(t *testing.T) {
	inputs := []extendedStreamInput{
		{event: "response.created", data: `{"type":"response.created","response":{"id":"resp-web","model":"k3","status":"in_progress"}}`},
		{event: "response.output_item.added", data: `{"type":"response.output_item.added","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"in_progress"}}`},
		{event: "response.web_search_call.in_progress", data: `{"type":"response.web_search_call.in_progress","output_index":0,"item_id":"ws_1"}`},
		{event: "response.web_search_call.searching", data: `{"type":"response.web_search_call.searching","output_index":0,"item_id":"ws_1"}`},
		{event: "response.web_search_call.completed", data: `{"type":"response.web_search_call.completed","output_index":0,"item_id":"ws_1"}`},
		{event: "response.output_item.done", data: `{"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"novro gateway","sources":[{"type":"url","url":"https://example.com/novro","title":"Novro"}]}}}`},
		{event: "response.completed", data: `{"type":"response.completed","response":{"id":"resp-web","model":"k3","status":"completed","usage":{"input_tokens":4,"output_tokens":7,"total_tokens":11}}}`},
	}
	events := translateExtendedStream(t, newProtocolStreamAdapter("responses", "messages"), inputs)

	starts := make([]extendedStreamEvent, 0)
	for _, event := range events {
		if event.event == "content_block_start" {
			starts = append(starts, event)
		}
	}
	if len(starts) != 2 {
		t.Fatalf("content block starts = %d, want 2; events=%v", len(starts), extendedEventTypes(events))
	}
	server := mapValue(starts[0].data["content_block"])
	if stringValue(server["type"]) != "server_tool_use" || stringValue(server["id"]) != "ws_1" || stringValue(mapValue(server["input"])["query"]) != "novro gateway" {
		t.Fatalf("server_tool_use = %#v", server)
	}
	result := mapValue(starts[1].data["content_block"])
	hits := sliceValue(result["content"])
	if stringValue(result["type"]) != "web_search_tool_result" || stringValue(result["tool_use_id"]) != "ws_1" || len(hits) != 1 {
		t.Fatalf("web_search_tool_result = %#v", result)
	}
	if hit := mapValue(hits[0]); stringValue(hit["url"]) != "https://example.com/novro" || stringValue(hit["title"]) != "Novro" {
		t.Fatalf("web search hit = %#v", hit)
	}
	if countExtendedEventName(events, "content_block_stop") != 2 {
		t.Fatalf("content block stop count = %d, want 2", countExtendedEventName(events, "content_block_stop"))
	}
}

func TestExtendedMessagesWebSearchStreamToResponses(t *testing.T) {
	inputs := []extendedStreamInput{
		{event: "message_start", data: `{"type":"message_start","message":{"id":"msg-web","model":"glm-5.3","usage":{"input_tokens":5,"output_tokens":0}}}`},
		{event: "content_block_start", data: `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"ws_2","name":"web_search","input":{}}}`},
		{event: "content_block_delta", data: `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"protocol adapters\"}"}}`},
		{event: "content_block_stop", data: `{"type":"content_block_stop","index":0}`},
		{event: "content_block_start", data: `{"type":"content_block_start","index":1,"content_block":{"type":"web_search_tool_result","tool_use_id":"ws_2","content":[{"type":"web_search_result","url":"https://example.com/adapters","title":"Adapters"}]}}`},
		{event: "content_block_stop", data: `{"type":"content_block_stop","index":1}`},
		{event: "message_delta", data: `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":8}}`},
		{event: "message_stop", data: `{"type":"message_stop"}`},
	}
	events := translateExtendedStream(t, newProtocolStreamAdapter("messages", "responses"), inputs)
	types := extendedEventTypes(events)
	for _, wanted := range []string{
		"response.output_item.added", "response.web_search_call.in_progress",
		"response.web_search_call.searching", "response.web_search_call.completed",
		"response.output_item.done", "response.completed",
	} {
		if !containsExtendedType(types, wanted) {
			t.Fatalf("missing %s in %v", wanted, types)
		}
	}
	itemDone := findExtendedWebSearchDone(t, events)
	item := mapValue(itemDone.data["item"])
	action := mapValue(item["action"])
	if stringValue(item["id"]) != "ws_2" || stringValue(item["status"]) != "completed" || stringValue(action["query"]) != "protocol adapters" {
		t.Fatalf("Responses web_search_call = %#v", item)
	}
	sources := sliceValue(action["sources"])
	if len(sources) != 1 || stringValue(mapValue(sources[0])["url"]) != "https://example.com/adapters" {
		t.Fatalf("Responses web search sources = %#v", sources)
	}
}

func TestExtendedMessagesWebSearchErrorToResponses(t *testing.T) {
	inputs := []extendedStreamInput{
		{event: "message_start", data: `{"type":"message_start","message":{"id":"msg-web-error","model":"k3","usage":{"input_tokens":1,"output_tokens":0}}}`},
		{event: "content_block_start", data: `{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","id":"ws_error","name":"web_search","input":{"query":"blocked"}}}`},
		{event: "content_block_stop", data: `{"type":"content_block_stop","index":0}`},
		{event: "content_block_start", data: `{"type":"content_block_start","index":1,"content_block":{"type":"web_search_tool_result","tool_use_id":"ws_error","content":{"type":"web_search_tool_result_error","error_code":"unavailable"}}}`},
		{event: "content_block_stop", data: `{"type":"content_block_stop","index":1}`},
		{event: "message_stop", data: `{"type":"message_stop"}`},
	}
	events := translateExtendedStream(t, newProtocolStreamAdapter("messages", "responses"), inputs)
	item := mapValue(findExtendedWebSearchDone(t, events).data["item"])
	if stringValue(item["status"]) != "failed" {
		t.Fatalf("failed search status = %q, item=%#v", stringValue(item["status"]), item)
	}
}

func TestExtendedResponsesAnnotationStreamToMessages(t *testing.T) {
	inputs := []extendedStreamInput{
		{event: "response.created", data: `{"type":"response.created","response":{"id":"resp-cite","model":"glm-5.2","status":"in_progress"}}`},
		{event: "response.output_text.delta", data: `{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"Hello world"}`},
		{event: "response.output_text.annotation.added", data: `{"type":"response.output_text.annotation.added","item_id":"msg_1","output_index":0,"content_index":0,"annotation_index":0,"annotation":{"type":"url_citation","url":"https://example.com","title":"Example","start_index":0,"end_index":5}}`},
		{event: "response.completed", data: `{"type":"response.completed","response":{"id":"resp-cite","model":"glm-5.2","status":"completed"}}`},
	}
	events := translateExtendedStream(t, newProtocolStreamAdapter("responses", "messages"), inputs)
	var citation map[string]any
	for _, event := range events {
		if event.event != "content_block_delta" {
			continue
		}
		delta := mapValue(event.data["delta"])
		if stringValue(delta["type"]) == "citations_delta" {
			citation = mapValue(delta["citation"])
		}
	}
	if citation == nil || stringValue(citation["type"]) != "web_search_result_location" || stringValue(citation["url"]) != "https://example.com" || stringValue(citation["cited_text"]) != "Hello" {
		t.Fatalf("Anthropic citation delta = %#v", citation)
	}
}

func TestExtendedStreamErrorUsesTargetProtocolAndStops(t *testing.T) {
	adapter := newProtocolStreamAdapter("responses", "messages")
	events := translateExtendedStream(t, adapter, []extendedStreamInput{
		{event: "error", data: `{"type":"error","code":"rate_limit_exceeded","message":"try later","param":"model"}`},
		{event: "response.output_text.delta", data: `{"type":"response.output_text.delta","output_index":0,"delta":"must not leak"}`},
	})
	if len(events) != 1 || events[0].event != "error" || stringValue(events[0].data["type"]) != "error" {
		t.Fatalf("target error events = %#v", events)
	}
	streamError := mapValue(events[0].data["error"])
	if stringValue(streamError["message"]) != "try later" || stringValue(streamError["type"]) != "api_error" {
		t.Fatalf("Messages error payload = %#v", streamError)
	}
	if !adapter.finished {
		t.Fatal("adapter did not enter terminal state after upstream error")
	}
}

func translateExtendedStream(t *testing.T, adapter *protocolStreamAdapter, inputs []extendedStreamInput) []extendedStreamEvent {
	t.Helper()
	var output bytes.Buffer
	for _, input := range inputs {
		translated, err := adapter.Translate(input.event, []byte(input.data))
		if err != nil {
			t.Fatalf("Translate(%s): %v", input.event, err)
		}
		output.Write(translated)
	}
	return parseExtendedStream(t, output.String())
}

func parseExtendedStream(t *testing.T, stream string) []extendedStreamEvent {
	t.Helper()
	frames := strings.Split(strings.ReplaceAll(stream, "\r\n", "\n"), "\n\n")
	events := make([]extendedStreamEvent, 0, len(frames))
	for _, frame := range frames {
		if strings.TrimSpace(frame) == "" {
			continue
		}
		var eventName, dataLine string
		for _, line := range strings.Split(frame, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
			case strings.HasPrefix(line, "data: "):
				dataLine = strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			}
		}
		if dataLine == "[DONE]" {
			events = append(events, extendedStreamEvent{event: eventName, raw: dataLine})
			continue
		}
		var data map[string]any
		decoder := json.NewDecoder(strings.NewReader(dataLine))
		decoder.UseNumber()
		if err := decoder.Decode(&data); err != nil {
			t.Fatalf("decode SSE frame %q: %v", frame, err)
		}
		events = append(events, extendedStreamEvent{event: eventName, data: data, raw: dataLine})
	}
	return events
}

func extendedEventTypes(events []extendedStreamEvent) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		if typeName := stringValue(event.data["type"]); typeName != "" {
			types = append(types, typeName)
		}
	}
	return types
}

func containsExtendedType(types []string, wanted string) bool {
	return countExtendedType(types, wanted) > 0
}

func countExtendedType(types []string, wanted string) int {
	count := 0
	for _, typeName := range types {
		if typeName == wanted {
			count++
		}
	}
	return count
}

func countExtendedEventName(events []extendedStreamEvent, wanted string) int {
	count := 0
	for _, event := range events {
		if event.event == wanted {
			count++
		}
	}
	return count
}

func findExtendedEvent(t *testing.T, events []extendedStreamEvent, wanted string) extendedStreamEvent {
	t.Helper()
	for _, event := range events {
		if stringValue(event.data["type"]) == wanted {
			return event
		}
	}
	t.Fatalf("event %s not found in %v", wanted, extendedEventTypes(events))
	return extendedStreamEvent{}
}

func findExtendedWebSearchDone(t *testing.T, events []extendedStreamEvent) extendedStreamEvent {
	t.Helper()
	for _, event := range events {
		if stringValue(event.data["type"]) != "response.output_item.done" {
			continue
		}
		if stringValue(mapValue(event.data["item"])["type"]) == "web_search_call" {
			return event
		}
	}
	t.Fatalf("web_search_call output_item.done not found in %v", extendedEventTypes(events))
	return extendedStreamEvent{}
}
