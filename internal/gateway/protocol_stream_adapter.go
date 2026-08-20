package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type adapterStreamDelta struct {
	Text        string
	Refusal     string
	Reasoning   string
	Signature   string
	Annotations []any
	Tools       []adapterStreamToolDelta
	WebSearches []adapterStreamWebSearchEvent
	StopReason  string
	Done        bool
	ID          string
	Model       string
	Usage       map[string]any
	Error       map[string]any
}

type adapterStreamWebSearchEvent struct {
	Key       string
	Index     int
	ID        string
	Name      string
	Arguments string
	Action    map[string]any
	Result    any
	Status    string
	Stage     string
}

type adapterStreamWebSearchState struct {
	Key             string
	SourceIndex     int
	OutputIndex     int
	ID              string
	Name            string
	Arguments       string
	Action          map[string]any
	Result          any
	Status          string
	Added           bool
	Searching       bool
	Completed       bool
	SpecialRecorded bool
}

type adapterStreamToolDelta struct {
	Key       string
	Index     int
	ID        string
	ItemID    string
	Name      string
	Arguments string
	Started   bool
}

type adapterSourceTool struct {
	Key    string
	ID     string
	ItemID string
	Name   string
}

type protocolStreamAdapter struct {
	source                       string
	target                       string
	response                     adapterResponse
	started                      bool
	targetBlock                  string
	targetBlockIndex             int
	responsesTextStarted         bool
	responsesTextDone            bool
	responsesMessageStarted      bool
	responsesMessageDone         bool
	responsesRefusalStarted      bool
	responsesRefusalDone         bool
	responsesTextContentIndex    int
	responsesRefusalContentIndex int
	responsesNextContentIndex    int
	responsesReasoningStarted    bool
	responsesReasoningDone       bool
	toolIndex                    map[string]int
	toolArgs                     map[string]string
	sourceTools                  map[int]adapterSourceTool
	sourceItems                  map[string]int
	responsesToolIndex           map[string]int
	responsesToolItemID          map[string]string
	responsesToolKeys            []string
	responsesSequence            int
	responsesNextOutputIndex     int
	responsesReasoningIndex      int
	responsesTextOutputIndex     int
	webSearches                  map[string]*adapterStreamWebSearchState
	sourceWebSearch              map[int]string
	webSearchKeyByID             map[string]string
	responsesWebSearchKeys       []string
	annotations                  []any
	annotationsEmitted           int
	messagesTextCommitted        bool
	messagesTextSent             int
	messagesToolBlockIndex       map[string]int
	messagesToolBlockOpen        map[string]bool
	messagesToolKeys             []string
	toolIdentityCommitted        map[string]bool
	committedToolIDs             map[string]struct{}
	finished                     bool
	anthropicBlockOpened         bool
}

func newProtocolStreamAdapter(source, target string) *protocolStreamAdapter {
	return &protocolStreamAdapter{
		source: source, target: target,
		response:                     adapterResponse{Created: time.Now().Unix(), Usage: map[string]any{}},
		toolIndex:                    map[string]int{},
		toolArgs:                     map[string]string{},
		sourceTools:                  map[int]adapterSourceTool{},
		sourceItems:                  map[string]int{},
		responsesToolIndex:           map[string]int{},
		responsesToolItemID:          map[string]string{},
		webSearches:                  map[string]*adapterStreamWebSearchState{},
		sourceWebSearch:              map[int]string{},
		webSearchKeyByID:             map[string]string{},
		messagesToolBlockIndex:       map[string]int{},
		messagesToolBlockOpen:        map[string]bool{},
		toolIdentityCommitted:        map[string]bool{},
		committedToolIDs:             map[string]struct{}{},
		responsesReasoningIndex:      -1,
		responsesTextOutputIndex:     -1,
		responsesTextContentIndex:    -1,
		responsesRefusalContentIndex: -1,
	}
}

func (a *protocolStreamAdapter) Translate(eventName string, data []byte) ([]byte, error) {
	if a.source == a.target {
		return rawSSEFrame(eventName, data), nil
	}
	if a.finished {
		return nil, nil
	}
	delta, err := a.decodeProtocolStreamDelta(data)
	if err != nil {
		return nil, err
	}
	if delta.Error != nil {
		var output bytes.Buffer
		a.writeStreamError(&output, delta.Error)
		a.finished = true
		return output.Bytes(), nil
	}
	if a.target == "responses" && a.responsesMessageStarted && delta.Reasoning != "" {
		// The reasoning item is already closed once the message item starts.
		// Do not expose a late vendor-specific reasoning delta as output text:
		// that can leak hidden reasoning or duplicate answer text. Keeping the
		// message lifecycle valid is more important than preserving an invalidly
		// ordered extension field.
		delta.Reasoning = ""
	}
	if !a.started && delta.empty() {
		return nil, nil
	}
	a.merge(delta)
	var output bytes.Buffer
	if !a.started {
		a.writeStart(&output)
		a.started = true
	}
	if delta.Reasoning != "" {
		a.writeContentDelta(&output, "reasoning", delta.Reasoning)
	}
	if delta.Text != "" {
		if a.target == "messages" {
			a.writeMessagesTextDelta(&output, delta.Text)
		} else {
			a.writeContentDelta(&output, "text", delta.Text)
		}
	}
	if delta.Refusal != "" {
		a.writeContentDelta(&output, "refusal", delta.Refusal)
	}
	for _, annotation := range delta.Annotations {
		a.writeAnnotationDelta(&output, annotation)
	}
	for _, tool := range delta.Tools {
		a.mergeTool(tool)
		a.writeToolDelta(&output, tool)
	}
	for _, webSearch := range delta.WebSearches {
		a.writeWebSearchEvent(&output, webSearch)
	}
	if delta.Done {
		if a.target == "messages" {
			a.flushMessagesText(&output)
		}
		a.writeDone(&output)
		a.finished = true
	}
	return output.Bytes(), nil
}

func (a *protocolStreamAdapter) FromBufferedResponse(body []byte) ([]byte, error) {
	response, err := decodeAdapterResponse(body, a.source)
	if err != nil {
		return nil, err
	}
	if response.ID == "" {
		response.ID = a.response.ID
	}
	a.response = response
	a.annotations = append(a.annotations[:0], response.Annotations...)
	if a.target == "messages" && duplicatesUnsignedReasoning(response.Text, response.Reasoning, response.ReasoningSignature) {
		response.Text = ""
		a.response.Text = ""
	}
	var output bytes.Buffer
	a.writeStart(&output)
	a.started = true
	if response.Reasoning != "" {
		a.writeContentDelta(&output, "reasoning", response.Reasoning)
	}
	if response.Text != "" {
		a.writeContentDelta(&output, "text", response.Text)
	}
	if response.Refusal != "" {
		a.writeContentDelta(&output, "refusal", response.Refusal)
	}
	for _, annotation := range response.Annotations {
		a.writeAnnotationDelta(&output, annotation)
	}
	for _, citation := range response.Citations {
		a.writeAnnotationDelta(&output, citation)
	}
	for _, call := range response.ToolCalls {
		arguments, _ := json.Marshal(nonNilObject(call.Arguments))
		tool := adapterStreamToolDelta{Key: call.ID, ID: call.ID, Name: call.Name, Arguments: string(arguments), Started: true}
		a.mergeTool(tool)
		a.writeToolDelta(&output, tool)
	}
	a.writeDone(&output)
	a.finished = true
	return output.Bytes(), nil
}

func (a *protocolStreamAdapter) setFallbackResponseID(responseID string) {
	if !a.started && a.response.ID == "" {
		a.response.ID = strings.TrimSpace(responseID)
	}
}

func (a *protocolStreamAdapter) responsesResult() map[string]any {
	if a.target != "responses" || !a.finished {
		return nil
	}
	return encodeAdapterResponse(a.response, "responses")
}

// Finalize closes a stream whose upstream supplied a billable terminal state
// but omitted its protocol's final sentinel (for example, a Chat stream that
// ends after finish_reason without sending [DONE]). Callers must only use it
// after the complete upstream body has been read.
func (a *protocolStreamAdapter) Finalize() []byte {
	if a.finished {
		return nil
	}
	var output bytes.Buffer
	if !a.started {
		a.writeStart(&output)
		a.started = true
	}
	if a.target == "messages" {
		a.flushMessagesText(&output)
	}
	a.writeDone(&output)
	a.finished = true
	return output.Bytes()
}

func (a *protocolStreamAdapter) merge(delta adapterStreamDelta) {
	// The response and output-item identities must not change after the target
	// lifecycle has started. Some compatible upstreams omit an id on their
	// first data event and add one later; retain the synthetic id in that case
	// rather than emitting mismatched added/done item ids.
	if delta.ID != "" && !a.started {
		a.response.ID = delta.ID
	}
	if delta.Model != "" {
		a.response.Model = delta.Model
	}
	a.response.Text += delta.Text
	a.response.Refusal += delta.Refusal
	a.response.Reasoning += delta.Reasoning
	a.response.ReasoningSignature += delta.Signature
	if len(delta.Annotations) > 0 {
		for _, annotation := range delta.Annotations {
			a.appendAnnotation(annotation)
		}
	}
	if delta.StopReason != "" {
		a.response.StopReason = delta.StopReason
	}
	if len(delta.Usage) > 0 {
		for key, value := range delta.Usage {
			a.response.Usage[key] = value
		}
	}
}

// writeMessagesTextDelta keeps the Messages target incremental like the
// Anthropic stream contract: each safe source text delta is emitted as a
// content_block_delta and flushed by the HTTP relay. Some compatible Chat
// providers duplicate unsigned reasoning into content. While the visible text
// is still a prefix of that reasoning, hold only that ambiguous prefix; as soon
// as it differs, release the accumulated text and continue incrementally.
func (a *protocolStreamAdapter) writeMessagesTextDelta(output *bytes.Buffer, delta string) {
	if delta == "" {
		return
	}
	if a.messagesTextCommitted {
		a.writeContentDelta(output, "text", delta)
		a.messagesTextSent += len(delta)
		return
	}
	if couldDuplicateUnsignedReasoning(a.response.Text, a.response.Reasoning, a.response.ReasoningSignature) {
		return
	}
	pending := a.response.Text[a.messagesTextSent:]
	if pending == "" {
		return
	}
	a.writeContentDelta(output, "text", pending)
	a.messagesTextSent = len(a.response.Text)
	a.messagesTextCommitted = true
}

func (a *protocolStreamAdapter) flushMessagesText(output *bytes.Buffer) {
	if a.messagesTextSent >= len(a.response.Text) {
		return
	}
	if !a.messagesTextCommitted && duplicatesUnsignedReasoning(a.response.Text, a.response.Reasoning, a.response.ReasoningSignature) {
		return
	}
	a.writeContentDelta(output, "text", a.response.Text[a.messagesTextSent:])
	a.messagesTextSent = len(a.response.Text)
	a.messagesTextCommitted = true
}

func couldDuplicateUnsignedReasoning(text, reasoning, signature string) bool {
	if signature != "" {
		return false
	}
	normalizedReasoning := strings.TrimSpace(reasoning)
	if normalizedReasoning == "" {
		return false
	}
	normalizedText := strings.TrimSpace(text)
	return normalizedText == "" || strings.HasPrefix(normalizedReasoning, normalizedText)
}

func (a *protocolStreamAdapter) mergeTool(delta adapterStreamToolDelta) {
	key := delta.Key
	if key == "" {
		key = delta.ID
	}
	if key == "" {
		key = delta.ItemID
	}
	if key == "" {
		key = fmt.Sprintf("tool-%d", delta.Index)
	}
	index, exists := a.toolIndex[key]
	if !exists {
		index = len(a.response.ToolCalls)
		a.toolIndex[key] = index
		a.response.ToolCalls = append(a.response.ToolCalls, adapterPart{Type: "tool_call"})
	}
	call := &a.response.ToolCalls[index]
	if delta.ID != "" && !a.toolIdentityCommitted[key] {
		call.ID = delta.ID
	}
	if delta.Name != "" {
		call.Name = delta.Name
	}
	a.toolArgs[key] += delta.Arguments
	call.Arguments = decodeJSONValue(a.toolArgs[key])
}

func (a *protocolStreamAdapter) writeStart(output *bytes.Buffer) {
	if a.response.ID == "" {
		a.response.ID = "novro-stream"
	}
	switch a.target {
	case "chat_completions":
		writeAdapterSSE(output, "", map[string]any{"id": a.response.ID, "object": "chat.completion.chunk", "created": a.response.Created, "model": a.response.Model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}}})
	case "messages":
		usage := usageForTarget(a.response.Usage, "messages")
		if usage == nil {
			usage = map[string]any{"input_tokens": 0, "output_tokens": 0}
		}
		writeAdapterSSE(output, "message_start", map[string]any{"type": "message_start", "message": map[string]any{"id": a.response.ID, "type": "message", "role": "assistant", "content": []any{}, "model": a.response.Model, "stop_reason": nil, "stop_sequence": nil, "usage": usage}})
	case "responses":
		response := map[string]any{"id": a.response.ID, "object": "response", "created_at": a.response.Created, "status": "in_progress", "model": a.response.Model, "output": []any{}, "usage": nil}
		a.writeResponsesSSE(output, "response.created", map[string]any{"type": "response.created", "response": response})
		a.writeResponsesSSE(output, "response.in_progress", map[string]any{"type": "response.in_progress", "response": response})
	}
}

func (a *protocolStreamAdapter) writeStreamError(output *bytes.Buffer, streamError map[string]any) {
	message := firstNonEmptyString(streamError["message"], "upstream stream failed")
	errorType := firstNonEmptyString(streamError["type"], "api_error")
	code := stringValue(streamError["code"])
	param := stringValue(streamError["param"])
	switch a.target {
	case "chat_completions":
		value := map[string]any{"message": message, "type": errorType}
		if code != "" {
			value["code"] = code
		}
		if param != "" {
			value["param"] = param
		}
		writeAdapterSSE(output, "", map[string]any{"error": value})
	case "messages":
		writeAdapterSSE(output, "error", map[string]any{"type": "error", "error": map[string]any{"type": errorType, "message": message}})
	case "responses":
		value := map[string]any{"type": "error", "message": message}
		if code != "" {
			value["code"] = code
		}
		if param != "" {
			value["param"] = param
		}
		a.writeResponsesSSE(output, "error", value)
	}
}

func (a *protocolStreamAdapter) writeContentDelta(output *bytes.Buffer, kind, text string) {
	switch a.target {
	case "chat_completions":
		delta := map[string]any{}
		if kind == "reasoning" {
			delta["reasoning_content"] = text
		} else if kind == "refusal" {
			delta["refusal"] = text
		} else {
			delta["content"] = text
		}
		writeAdapterSSE(output, "", map[string]any{"id": a.response.ID, "object": "chat.completion.chunk", "created": a.response.Created, "model": a.response.Model, "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
	case "messages":
		// Cross-protocol reasoning has no Anthropic-generated signature. It is
		// private model state, so do not turn it into visible text or forge a
		// thinking block. A buffered native Messages response carries the real
		// signature and can retain the thinking lifecycle below.
		if kind == "reasoning" && a.response.ReasoningSignature == "" {
			return
		}
		blockType := "text"
		if kind == "reasoning" && a.response.ReasoningSignature != "" {
			blockType = "thinking"
		}
		a.ensureAnthropicBlock(output, blockType)
		if blockType == "thinking" {
			writeAdapterSSE(output, "content_block_delta", map[string]any{"type": "content_block_delta", "index": a.targetBlockIndex, "delta": map[string]any{"type": "thinking_delta", "thinking": text}})
		} else {
			writeAdapterSSE(output, "content_block_delta", map[string]any{"type": "content_block_delta", "index": a.targetBlockIndex, "delta": map[string]any{"type": "text_delta", "text": text}})
		}
	case "responses":
		if kind == "reasoning" {
			a.ensureResponsesReasoning(output)
			a.writeResponsesSSE(output, "response.reasoning_summary_text.delta", map[string]any{"type": "response.reasoning_summary_text.delta", "item_id": "rs_" + safeID(a.response.ID), "output_index": a.responsesReasoningIndex, "summary_index": 0, "delta": text})
		} else if kind == "refusal" {
			a.ensureResponsesRefusal(output)
			a.writeResponsesSSE(output, "response.refusal.delta", map[string]any{"type": "response.refusal.delta", "item_id": "msg_" + safeID(a.response.ID), "output_index": a.responsesTextIndex(), "content_index": a.responsesRefusalContentIndex, "delta": text})
		} else {
			a.ensureResponsesText(output)
			a.writeResponsesSSE(output, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "item_id": "msg_" + safeID(a.response.ID), "output_index": a.responsesTextIndex(), "content_index": a.responsesTextContentIndex, "delta": text})
		}
	}
}

func (a *protocolStreamAdapter) writeAnnotationDelta(output *bytes.Buffer, annotation any) {
	if annotation == nil {
		return
	}
	annotationIndex := a.annotationsEmitted
	a.annotationsEmitted++
	switch a.target {
	case "chat_completions":
		writeAdapterSSE(output, "", map[string]any{"id": a.response.ID, "object": "chat.completion.chunk", "created": a.response.Created, "model": a.response.Model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"annotations": []any{openAIStreamAnnotation(annotation)}}, "finish_reason": nil}}})
	case "messages":
		a.ensureAnthropicBlock(output, "text")
		writeAdapterSSE(output, "content_block_delta", map[string]any{"type": "content_block_delta", "index": a.targetBlockIndex, "delta": map[string]any{"type": "citations_delta", "citation": anthropicStreamCitation(annotation, a.response.Text)}})
	case "responses":
		a.ensureResponsesText(output)
		a.writeResponsesSSE(output, "response.output_text.annotation.added", map[string]any{"type": "response.output_text.annotation.added", "item_id": "msg_" + safeID(a.response.ID), "output_index": a.responsesTextIndex(), "content_index": a.responsesTextContentIndex, "annotation_index": annotationIndex, "annotation": openAIStreamAnnotation(annotation)})
	}
}

func (a *protocolStreamAdapter) writeToolDelta(output *bytes.Buffer, delta adapterStreamToolDelta) {
	key := delta.Key
	if key == "" {
		key = delta.ID
	}
	if key == "" {
		key = delta.ItemID
	}
	if key == "" {
		key = fmt.Sprintf("tool-%d", delta.Index)
	}
	index := a.toolIndex[key]
	call := a.ensureStreamingToolIdentity(key, index, delta)
	switch a.target {
	case "chat_completions":
		call := map[string]any{"index": index}
		if delta.Started {
			call["id"] = a.response.ToolCalls[index].ID
			call["type"] = "function"
		}
		function := map[string]any{}
		if delta.Started && a.response.ToolCalls[index].Name != "" {
			function["name"] = a.response.ToolCalls[index].Name
		}
		if delta.Arguments != "" {
			function["arguments"] = delta.Arguments
		}
		call["function"] = function
		writeAdapterSSE(output, "", map[string]any{"id": a.response.ID, "object": "chat.completion.chunk", "created": a.response.Created, "model": a.response.Model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{call}}, "finish_reason": nil}}})
	case "messages":
		blockIndex, exists := a.messagesToolBlockIndex[key]
		if !exists {
			// Finish any preceding text/thinking block, then give every tool call
			// its own stable Anthropic block. Parallel calls may remain open at
			// the same time; their argument deltas are routed by block index.
			a.closeAnthropicBlock(output)
			a.startNextAnthropicBlock()
			blockIndex = a.targetBlockIndex
			a.messagesToolBlockIndex[key] = blockIndex
			a.messagesToolBlockOpen[key] = true
			a.messagesToolKeys = append(a.messagesToolKeys, key)
			writeAdapterSSE(output, "content_block_start", map[string]any{"type": "content_block_start", "index": blockIndex, "content_block": map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": map[string]any{}}})
		}
		if delta.Arguments != "" {
			writeAdapterSSE(output, "content_block_delta", map[string]any{"type": "content_block_delta", "index": blockIndex, "delta": map[string]any{"type": "input_json_delta", "partial_json": delta.Arguments}})
		}
	case "responses":
		outputIndex, exists := a.responsesToolIndex[key]
		if !exists {
			// Responses identifies every tool delta by output_index/item_id, so
			// parallel calls can be forwarded immediately without serializing or
			// buffering their arguments until the terminal event.
			a.closeResponsesReasoning(output)
			a.closeResponsesMessage(output)
			outputIndex = a.nextResponsesOutputIndex()
			a.responsesToolIndex[key] = outputIndex
			a.responsesToolKeys = append(a.responsesToolKeys, key)
			itemID := strings.TrimSpace(delta.ItemID)
			if itemID == "" {
				itemID = responsesFunctionItemID(call.ID)
			}
			a.responsesToolItemID[key] = itemID
			a.writeResponsesSSE(output, "response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": outputIndex, "item": map[string]any{"id": itemID, "type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": "", "status": "in_progress"}})
		}
		if delta.Arguments != "" {
			a.writeResponsesSSE(output, "response.function_call_arguments.delta", map[string]any{"type": "response.function_call_arguments.delta", "item_id": a.responsesToolItemID[key], "output_index": outputIndex, "delta": delta.Arguments})
		}
	}
}

func (a *protocolStreamAdapter) writeWebSearchEvent(output *bytes.Buffer, event adapterStreamWebSearchEvent) {
	state := a.ensureWebSearchState(event)
	if event.Arguments != "" {
		state.Arguments += event.Arguments
	}
	if event.Action != nil {
		state.Action = cloneMap(event.Action)
	}
	if event.Result != nil {
		state.Result = event.Result
	}
	if event.Status != "" {
		state.Status = event.Status
	}
	switch a.target {
	case "messages":
		// A Responses web_search_call does not expose its query and sources until
		// output_item.done. Emitting an empty server_tool_use at item.added would
		// leave Anthropic clients waiting for JSON deltas that never arrive.
		if event.Stage == "done" {
			a.writeMessagesWebSearch(output, state)
		}
	case "responses":
		a.writeResponsesWebSearch(output, state, event.Stage)
	}
}

func (a *protocolStreamAdapter) ensureWebSearchState(event adapterStreamWebSearchEvent) *adapterStreamWebSearchState {
	key := strings.TrimSpace(event.Key)
	if key == "" && event.ID != "" {
		key = "web_search:" + event.ID
	}
	if key == "" {
		key = fmt.Sprintf("web_search:%d", event.Index)
	}
	state := a.webSearches[key]
	if state == nil {
		state = &adapterStreamWebSearchState{Key: key, SourceIndex: event.Index, OutputIndex: -1, Status: "in_progress"}
		a.webSearches[key] = state
	}
	if event.ID != "" {
		state.ID = event.ID
		a.webSearchKeyByID[event.ID] = key
	}
	if event.Name != "" {
		state.Name = event.Name
	}
	if state.Name == "" {
		state.Name = "web_search"
	}
	return state
}

func (a *protocolStreamAdapter) writeMessagesWebSearch(output *bytes.Buffer, state *adapterStreamWebSearchState) {
	if state.Completed {
		return
	}
	a.closeAnthropicBlock(output)
	a.closeMessagesToolBlocks(output)
	a.startNextAnthropicBlock()
	serverIndex := a.targetBlockIndex
	args := webSearchArguments(state)
	writeAdapterSSE(output, "content_block_start", map[string]any{"type": "content_block_start", "index": serverIndex, "content_block": map[string]any{"type": "server_tool_use", "id": state.ID, "name": state.Name, "input": args}})
	writeAdapterSSE(output, "content_block_stop", map[string]any{"type": "content_block_stop", "index": serverIndex})

	a.startNextAnthropicBlock()
	resultIndex := a.targetBlockIndex
	result := anthropicWebSearchResult(state)
	writeAdapterSSE(output, "content_block_start", map[string]any{"type": "content_block_start", "index": resultIndex, "content_block": map[string]any{"type": "web_search_tool_result", "tool_use_id": state.ID, "content": result}})
	writeAdapterSSE(output, "content_block_stop", map[string]any{"type": "content_block_stop", "index": resultIndex})
	state.Completed = true
	a.recordWebSearchSpecialParts(state)
}

func (a *protocolStreamAdapter) writeResponsesWebSearch(output *bytes.Buffer, state *adapterStreamWebSearchState, stage string) {
	if !state.Added {
		a.closeResponsesReasoning(output)
		a.closeResponsesMessage(output)
		if state.ID == "" {
			state.ID = fmt.Sprintf("ws_novro_%s_%d", safeID(a.response.ID), len(a.responsesWebSearchKeys))
		}
		state.OutputIndex = a.nextResponsesOutputIndex()
		state.Added = true
		a.responsesWebSearchKeys = append(a.responsesWebSearchKeys, state.Key)
		a.writeResponsesSSE(output, "response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": state.OutputIndex, "item": map[string]any{"id": state.ID, "type": "web_search_call", "status": "in_progress"}})
		a.writeResponsesSSE(output, "response.web_search_call.in_progress", map[string]any{"type": "response.web_search_call.in_progress", "output_index": state.OutputIndex, "item_id": state.ID})
	}
	if (stage == "server_args" || stage == "server_stop" || stage == "searching") && !state.Searching {
		state.Searching = true
		a.writeResponsesSSE(output, "response.web_search_call.searching", map[string]any{"type": "response.web_search_call.searching", "output_index": state.OutputIndex, "item_id": state.ID})
	}
	if stage == "result_start" || stage == "result_stop" || stage == "done" {
		a.completeResponsesWebSearch(output, state)
	}
}

func (a *protocolStreamAdapter) completeResponsesWebSearch(output *bytes.Buffer, state *adapterStreamWebSearchState) {
	if state.Completed {
		return
	}
	if !state.Searching {
		state.Searching = true
		a.writeResponsesSSE(output, "response.web_search_call.searching", map[string]any{"type": "response.web_search_call.searching", "output_index": state.OutputIndex, "item_id": state.ID})
	}
	status := state.Status
	if status == "" || status == "in_progress" {
		status = "completed"
	}
	action := webSearchAction(state)
	a.writeResponsesSSE(output, "response.web_search_call.completed", map[string]any{"type": "response.web_search_call.completed", "output_index": state.OutputIndex, "item_id": state.ID})
	a.writeResponsesSSE(output, "response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": state.OutputIndex, "item": map[string]any{"id": state.ID, "type": "web_search_call", "status": status, "action": action}})
	state.Status = status
	state.Action = action
	state.Completed = true
	a.recordWebSearchSpecialParts(state)
}

func (a *protocolStreamAdapter) recordWebSearchSpecialParts(state *adapterStreamWebSearchState) {
	if state.SpecialRecorded {
		return
	}
	args := webSearchArguments(state)
	a.response.SpecialParts = append(a.response.SpecialParts,
		adapterPart{Type: "server_tool_use", ID: state.ID, Name: state.Name, Arguments: args},
		adapterPart{Type: "web_search_tool_result", ID: state.ID, Content: webSearchResultParts(state), IsError: state.Status == "failed", ErrorCode: webSearchErrorCode(state)},
	)
	state.SpecialRecorded = true
}

func webSearchArguments(state *adapterStreamWebSearchState) map[string]any {
	if raw := strings.TrimSpace(state.Arguments); raw != "" {
		if value := mapValue(decodeJSONValue(raw)); value != nil {
			return value
		}
	}
	if state.Action != nil {
		query := firstNonEmptyString(state.Action["query"], firstStringValue(state.Action["queries"]), state.Action["url"], state.Action["pattern"])
		if query != "" {
			return map[string]any{"query": query}
		}
	}
	return map[string]any{}
}

func webSearchAction(state *adapterStreamWebSearchState) map[string]any {
	action := cloneMap(state.Action)
	if action == nil {
		action = map[string]any{}
	}
	if action["type"] == nil {
		action["type"] = "search"
	}
	args := webSearchArguments(state)
	if action["query"] == nil && stringValue(args["query"]) != "" {
		action["query"] = stringValue(args["query"])
	}
	if action["sources"] == nil {
		sources := make([]any, 0)
		for _, part := range webSearchResultParts(state) {
			if part.ImageURL == "" {
				continue
			}
			source := map[string]any{"type": "url", "url": part.ImageURL}
			if part.Text != "" {
				source["title"] = part.Text
			}
			sources = append(sources, source)
		}
		action["sources"] = sources
	}
	return action
}

func anthropicWebSearchResult(state *adapterStreamWebSearchState) any {
	if state.Status == "failed" || webSearchErrorCode(state) != "" {
		return map[string]any{"type": "web_search_tool_result_error", "error_code": firstNonEmptyString(webSearchErrorCode(state), "failed")}
	}
	results := make([]any, 0)
	for _, part := range webSearchResultParts(state) {
		results = append(results, map[string]any{"type": "web_search_result", "url": part.ImageURL, "title": firstNonEmptyString(part.Text, part.ImageURL)})
	}
	return results
}

func webSearchResultParts(state *adapterStreamWebSearchState) []adapterPart {
	var rawResults []any
	if state.Result != nil {
		rawResults = sliceValue(state.Result)
	}
	if len(rawResults) == 0 && state.Action != nil {
		rawResults = sliceValue(state.Action["sources"])
	}
	parts := make([]adapterPart, 0, len(rawResults))
	for _, raw := range rawResults {
		result := mapValue(raw)
		url := stringValue(result["url"])
		if url == "" {
			continue
		}
		parts = append(parts, adapterPart{Type: "web_search_result", Text: firstNonEmptyString(result["title"], url), ImageURL: url, Extra: cloneMap(result)})
	}
	return parts
}

func webSearchErrorCode(state *adapterStreamWebSearchState) string {
	result := mapValue(state.Result)
	if stringValue(result["type"]) == "web_search_tool_result_error" {
		return stringValue(result["error_code"])
	}
	return ""
}

func firstStringValue(value any) string {
	values := sliceValue(value)
	if len(values) == 0 {
		return ""
	}
	return stringValue(values[0])
}

func (a *protocolStreamAdapter) ensureResponsesMessage(output *bytes.Buffer) {
	if a.responsesMessageStarted {
		return
	}
	a.closeResponsesReasoning(output)
	a.responsesMessageStarted = true
	a.responsesTextOutputIndex = a.nextResponsesOutputIndex()
	a.responsesNextContentIndex = 0
	itemID := "msg_" + safeID(a.response.ID)
	a.writeResponsesSSE(output, "response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": a.responsesTextOutputIndex, "item": map[string]any{"id": itemID, "type": "message", "status": "in_progress", "role": "assistant", "content": []any{}}})
}

func (a *protocolStreamAdapter) ensureStreamingToolIdentity(key string, index int, delta adapterStreamToolDelta) adapterPart {
	call := &a.response.ToolCalls[index]
	if a.toolIdentityCommitted[key] {
		return *call
	}
	callID := strings.TrimSpace(call.ID)
	if callID == "" {
		callID = firstNonEmptyString(delta.ID, delta.ItemID)
	}
	if _, duplicate := a.committedToolIDs[callID]; callID == "" || duplicate {
		base := fmt.Sprintf("call_novro_%s_%d", safeID(a.response.ID), index)
		callID = base
		for suffix := 1; ; suffix++ {
			if _, exists := a.committedToolIDs[callID]; !exists {
				break
			}
			callID = fmt.Sprintf("%s_%d", base, suffix)
		}
	}
	call.ID = callID
	if call.Name == "" {
		call.Name = delta.Name
	}
	a.toolIdentityCommitted[key] = true
	a.committedToolIDs[callID] = struct{}{}
	return *call
}

func (a *protocolStreamAdapter) writeDone(output *bytes.Buffer) {
	a.response.ToolCalls = normalizeAdapterToolCalls(a.response.ID, a.response.ToolCalls)
	// Refusal is a terminal semantic state across all three protocols. Some
	// upstream streams emit only refusal deltas and omit a finish/stop reason;
	// infer the protocol-neutral reason before choosing the target terminal
	// event so Chat does not report `stop` and Anthropic does not report
	// `end_turn` for a filtered response.
	if strings.TrimSpace(a.response.Refusal) != "" {
		a.response.StopReason = "refusal"
	}
	if a.response.StopReason == "" {
		a.response.StopReason = "stop"
	}
	if len(a.response.ToolCalls) > 0 && (a.response.StopReason == "stop" || a.response.StopReason == "end_turn") {
		a.response.StopReason = "tool_use"
	}
	switch a.target {
	case "chat_completions":
		writeAdapterSSE(output, "", map[string]any{"id": a.response.ID, "object": "chat.completion.chunk", "created": a.response.Created, "model": a.response.Model, "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": chatStopReason(a.response.StopReason)}}})
		writeAdapterSSE(output, "", map[string]any{"id": a.response.ID, "object": "chat.completion.chunk", "created": a.response.Created, "model": a.response.Model, "choices": []any{}, "usage": usageForTarget(a.response.Usage, "chat_completions")})
		output.WriteString("data: [DONE]\n\n")
	case "messages":
		a.closeAnthropicBlock(output)
		a.closeMessagesToolBlocks(output)
		usage := usageForTarget(a.response.Usage, "messages")
		if usage == nil {
			usage = map[string]any{"output_tokens": 0}
		}
		writeAdapterSSE(output, "message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": anthropicStopReason(a.response.StopReason), "stop_sequence": nil}, "usage": usage})
		writeAdapterSSE(output, "message_stop", map[string]any{"type": "message_stop"})
	case "responses":
		a.closeResponsesReasoning(output)
		a.closeResponsesMessage(output)
		for _, key := range a.responsesToolKeys {
			callIndex, exists := a.toolIndex[key]
			if !exists || callIndex < 0 || callIndex >= len(a.response.ToolCalls) {
				continue
			}
			outputIndex := a.responsesToolIndex[key]
			call := a.response.ToolCalls[callIndex]
			itemID := a.responsesToolItemID[key]
			if itemID == "" {
				itemID = responsesFunctionItemID(call.ID)
				a.responsesToolItemID[key] = itemID
			}
			arguments, _ := json.Marshal(nonNilObject(call.Arguments))
			if a.toolArgs[key] == "" {
				a.writeResponsesSSE(output, "response.function_call_arguments.delta", map[string]any{"type": "response.function_call_arguments.delta", "item_id": itemID, "output_index": outputIndex, "delta": string(arguments)})
			}
			a.writeResponsesSSE(output, "response.function_call_arguments.done", map[string]any{"type": "response.function_call_arguments.done", "item_id": itemID, "output_index": outputIndex, "name": call.Name, "arguments": string(arguments)})
			a.writeResponsesSSE(output, "response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": outputIndex, "item": map[string]any{"id": itemID, "type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": string(arguments), "status": "completed"}})
		}
		for _, key := range a.responsesWebSearchKeys {
			state := a.webSearches[key]
			if state != nil && !state.Completed {
				a.completeResponsesWebSearch(output, state)
			}
		}
		// Responses distinguishes a successful terminal response from one that
		// stopped incomplete (for example, because max_output_tokens was hit).
		// Keep the event name/type aligned with the status in the synthesized
		// response; clients use this event to decide whether the generation was
		// completed or interrupted.
		terminal := encodeAdapterResponse(a.response, "responses")
		eventName := "response.completed"
		if stringValue(terminal["status"]) == "incomplete" {
			eventName = "response.incomplete"
		}
		a.writeResponsesSSE(output, eventName, map[string]any{"type": eventName, "response": terminal})
	}
}

func (a *protocolStreamAdapter) ensureAnthropicBlock(output *bytes.Buffer, blockType string) {
	if a.targetBlock == blockType {
		return
	}
	a.closeAnthropicBlock(output)
	a.closeMessagesToolBlocks(output)
	a.startNextAnthropicBlock()
	a.targetBlock = blockType
	block := map[string]any{"type": "text", "text": ""}
	if blockType == "thinking" {
		block = map[string]any{"type": "thinking", "thinking": "", "signature": ""}
	}
	writeAdapterSSE(output, "content_block_start", map[string]any{"type": "content_block_start", "index": a.targetBlockIndex, "content_block": block})
}

func (a *protocolStreamAdapter) closeMessagesToolBlocks(output *bytes.Buffer) {
	for _, key := range a.messagesToolKeys {
		if !a.messagesToolBlockOpen[key] {
			continue
		}
		index := a.messagesToolBlockIndex[key]
		if a.toolArgs[key] == "" {
			writeAdapterSSE(output, "content_block_delta", map[string]any{"type": "content_block_delta", "index": index, "delta": map[string]any{"type": "input_json_delta", "partial_json": "{}"}})
		}
		writeAdapterSSE(output, "content_block_stop", map[string]any{"type": "content_block_stop", "index": index})
		a.messagesToolBlockOpen[key] = false
	}
}

func (a *protocolStreamAdapter) startNextAnthropicBlock() {
	if a.anthropicBlockOpened {
		a.targetBlockIndex++
	}
	a.anthropicBlockOpened = true
}

func (a *protocolStreamAdapter) closeAnthropicBlock(output *bytes.Buffer) {
	if a.targetBlock == "" {
		return
	}
	if a.targetBlock == "thinking" && a.response.ReasoningSignature != "" {
		writeAdapterSSE(output, "content_block_delta", map[string]any{"type": "content_block_delta", "index": a.targetBlockIndex, "delta": map[string]any{"type": "signature_delta", "signature": a.response.ReasoningSignature}})
	}
	writeAdapterSSE(output, "content_block_stop", map[string]any{"type": "content_block_stop", "index": a.targetBlockIndex})
	a.targetBlock = ""
}

func (a *protocolStreamAdapter) ensureResponsesReasoning(output *bytes.Buffer) {
	if a.responsesReasoningStarted {
		return
	}
	a.responsesReasoningStarted = true
	a.responsesReasoningIndex = a.nextResponsesOutputIndex()
	a.writeResponsesSSE(output, "response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": a.responsesReasoningIndex, "item": map[string]any{"id": "rs_" + safeID(a.response.ID), "type": "reasoning", "summary": []any{}}})
	a.writeResponsesSSE(output, "response.reasoning_summary_part.added", map[string]any{"type": "response.reasoning_summary_part.added", "item_id": "rs_" + safeID(a.response.ID), "output_index": a.responsesReasoningIndex, "summary_index": 0, "part": map[string]any{"type": "summary_text", "text": ""}})
}

func (a *protocolStreamAdapter) ensureResponsesText(output *bytes.Buffer) {
	if a.responsesTextStarted {
		return
	}
	a.ensureResponsesMessage(output)
	a.responsesTextStarted = true
	a.responsesTextContentIndex = a.responsesNextContentIndex
	a.responsesNextContentIndex++
	index := a.responsesTextIndex()
	itemID := "msg_" + safeID(a.response.ID)
	a.writeResponsesSSE(output, "response.content_part.added", map[string]any{"type": "response.content_part.added", "item_id": itemID, "output_index": index, "content_index": a.responsesTextContentIndex, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
}

func (a *protocolStreamAdapter) ensureResponsesRefusal(output *bytes.Buffer) {
	if a.responsesRefusalStarted {
		return
	}
	a.ensureResponsesMessage(output)
	a.responsesRefusalStarted = true
	a.responsesRefusalContentIndex = a.responsesNextContentIndex
	a.responsesNextContentIndex++
	itemID := "msg_" + safeID(a.response.ID)
	a.writeResponsesSSE(output, "response.content_part.added", map[string]any{"type": "response.content_part.added", "item_id": itemID, "output_index": a.responsesTextIndex(), "content_index": a.responsesRefusalContentIndex, "part": map[string]any{"type": "refusal", "refusal": ""}})
}

func (a *protocolStreamAdapter) closeResponsesReasoning(output *bytes.Buffer) {
	if !a.responsesReasoningStarted || a.responsesReasoningDone {
		return
	}
	itemID := "rs_" + safeID(a.response.ID)
	a.writeResponsesSSE(output, "response.reasoning_summary_text.done", map[string]any{"type": "response.reasoning_summary_text.done", "item_id": itemID, "output_index": a.responsesReasoningIndex, "summary_index": 0, "text": a.response.Reasoning})
	a.writeResponsesSSE(output, "response.reasoning_summary_part.done", map[string]any{"type": "response.reasoning_summary_part.done", "item_id": itemID, "output_index": a.responsesReasoningIndex, "summary_index": 0, "part": map[string]any{"type": "summary_text", "text": a.response.Reasoning}})
	a.writeResponsesSSE(output, "response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": a.responsesReasoningIndex, "item": map[string]any{"id": itemID, "type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": a.response.Reasoning}}}})
	a.responsesReasoningDone = true
}

func (a *protocolStreamAdapter) closeResponsesText(output *bytes.Buffer) {
	if !a.responsesTextStarted || a.responsesTextDone {
		return
	}
	itemID := "msg_" + safeID(a.response.ID)
	a.writeResponsesSSE(output, "response.output_text.done", map[string]any{"type": "response.output_text.done", "item_id": itemID, "output_index": a.responsesTextIndex(), "content_index": a.responsesTextContentIndex, "text": a.response.Text})
	a.writeResponsesSSE(output, "response.content_part.done", map[string]any{"type": "response.content_part.done", "item_id": itemID, "output_index": a.responsesTextIndex(), "content_index": a.responsesTextContentIndex, "part": map[string]any{"type": "output_text", "text": a.response.Text, "annotations": openAIStreamAnnotations(a.annotations)}})
	a.responsesTextDone = true
}

func (a *protocolStreamAdapter) closeResponsesRefusal(output *bytes.Buffer) {
	if !a.responsesRefusalStarted || a.responsesRefusalDone {
		return
	}
	itemID := "msg_" + safeID(a.response.ID)
	a.writeResponsesSSE(output, "response.refusal.done", map[string]any{"type": "response.refusal.done", "item_id": itemID, "output_index": a.responsesTextIndex(), "content_index": a.responsesRefusalContentIndex, "refusal": a.response.Refusal})
	a.writeResponsesSSE(output, "response.content_part.done", map[string]any{"type": "response.content_part.done", "item_id": itemID, "output_index": a.responsesTextIndex(), "content_index": a.responsesRefusalContentIndex, "part": map[string]any{"type": "refusal", "refusal": a.response.Refusal}})
	a.responsesRefusalDone = true
}

func (a *protocolStreamAdapter) closeResponsesMessage(output *bytes.Buffer) {
	if !a.responsesMessageStarted || a.responsesMessageDone {
		return
	}
	a.closeResponsesText(output)
	a.closeResponsesRefusal(output)
	itemID := "msg_" + safeID(a.response.ID)
	content := make([]any, 0, 2)
	if a.responsesTextStarted {
		content = append(content, map[string]any{"type": "output_text", "text": a.response.Text, "annotations": openAIStreamAnnotations(a.annotations)})
	}
	if a.responsesRefusalStarted {
		content = append(content, map[string]any{"type": "refusal", "refusal": a.response.Refusal})
	}
	item := map[string]any{"id": itemID, "type": "message", "status": "completed", "role": "assistant", "content": content}
	a.writeResponsesSSE(output, "response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": a.responsesTextIndex(), "item": item})
	a.responsesMessageDone = true
}

func (a *protocolStreamAdapter) responsesTextIndex() int {
	return a.responsesTextOutputIndex
}

func (a *protocolStreamAdapter) nextResponsesOutputIndex() int {
	index := a.responsesNextOutputIndex
	a.responsesNextOutputIndex++
	return index
}

func (a *protocolStreamAdapter) writeResponsesSSE(output *bytes.Buffer, eventName string, value map[string]any) {
	value["sequence_number"] = a.responsesSequence
	a.responsesSequence++
	writeAdapterSSE(output, eventName, value)
}

func (a *protocolStreamAdapter) decodeProtocolStreamDelta(data []byte) (adapterStreamDelta, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return adapterStreamDelta{Done: true}, nil
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return adapterStreamDelta{}, fmt.Errorf("decode %s stream event: %w", a.source, err)
	}
	delta := adapterStreamDelta{ID: stringValue(root["id"]), Model: stringValue(root["model"])}
	switch a.source {
	case "chat_completions":
		if root["error"] != nil {
			delta.Error = normalizeStreamError(mapValue(root["error"]), "chat_error")
			return delta, nil
		}
		if usage := mapValue(root["usage"]); usage != nil {
			delta.Usage = normalizeUsageMap(usage, a.source)
		}
		choices := sliceValue(root["choices"])
		if len(choices) > 0 {
			choice := mapValue(choices[0])
			content := mapValue(choice["delta"])
			delta.Text = stringValue(content["content"])
			delta.Refusal = stringValue(content["refusal"])
			delta.Annotations = sliceValue(content["annotations"])
			delta.Reasoning = firstNonEmptyString(content["reasoning_content"], content["reasoning"], content["reasoning_text"])
			delta.StopReason = stringValue(choice["finish_reason"])
			for _, rawCall := range sliceValue(content["tool_calls"]) {
				call := mapValue(rawCall)
				index := intValue(call["index"])
				function := mapValue(call["function"])
				state, exists := a.sourceTools[index]
				if !exists {
					state.Key = fmt.Sprintf("chat:%d", index)
				}
				if value := stringValue(call["id"]); value != "" {
					state.ID = value
				}
				if value := stringValue(function["name"]); value != "" {
					state.Name = value
				}
				a.sourceTools[index] = state
				delta.Tools = append(delta.Tools, adapterStreamToolDelta{Key: state.Key, Index: index, ID: state.ID, Name: state.Name, Arguments: stringValue(function["arguments"]), Started: !exists})
			}
		}
	case "responses":
		typeName := stringValue(root["type"])
		if typeName == "error" {
			delta.Error = normalizeStreamError(root, "api_error")
			return delta, nil
		}
		if typeName == "response.failed" {
			response := mapValue(root["response"])
			streamError := mapValue(response["error"])
			if streamError == nil {
				streamError = mapValue(root["error"])
			}
			delta.Error = normalizeStreamError(streamError, "responses_failed")
			return delta, nil
		}
		if response := mapValue(root["response"]); response != nil {
			delta.ID, delta.Model = stringValue(response["id"]), stringValue(response["model"])
			if usage := mapValue(response["usage"]); usage != nil {
				delta.Usage = normalizeUsageMap(usage, a.source)
			}
		}
		switch typeName {
		case "response.output_text.delta":
			delta.Text = stringValue(root["delta"])
		case "response.refusal.delta":
			delta.Refusal = stringValue(root["delta"])
		case "response.output_text.annotation.added":
			if annotation := root["annotation"]; annotation != nil {
				delta.Annotations = append(delta.Annotations, annotation)
			}
		case "response.content_part.done":
			part := mapValue(root["part"])
			if stringValue(part["type"]) == "output_text" && len(a.annotations) == 0 {
				delta.Annotations = append(delta.Annotations, sliceValue(part["annotations"])...)
			} else if stringValue(part["type"]) == "refusal" && a.response.Refusal == "" {
				delta.Refusal = stringValue(part["refusal"])
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			delta.Reasoning = stringValue(root["delta"])
		case "response.output_item.added":
			item := mapValue(root["item"])
			switch stringValue(item["type"]) {
			case "function_call":
				index := intValue(root["output_index"])
				itemID := stringValue(item["id"])
				state := adapterSourceTool{Key: "responses:" + itemID, ID: stringValue(item["call_id"]), ItemID: itemID, Name: stringValue(item["name"])}
				if itemID == "" {
					state.Key = fmt.Sprintf("responses:%d", index)
				}
				a.sourceTools[index] = state
				if itemID != "" {
					a.sourceItems[itemID] = index
				}
				delta.Tools = append(delta.Tools, adapterStreamToolDelta{Key: state.Key, Index: index, ID: state.ID, ItemID: state.ItemID, Name: state.Name, Started: true})
			case "web_search_call":
				index := intValue(root["output_index"])
				itemID := stringValue(item["id"])
				key := "responses-web-search:" + itemID
				if itemID == "" {
					key = fmt.Sprintf("responses-web-search:%d", index)
				}
				a.sourceWebSearch[index] = key
				if itemID != "" {
					a.webSearchKeyByID[itemID] = key
				}
				delta.WebSearches = append(delta.WebSearches, adapterStreamWebSearchEvent{Key: key, Index: index, ID: itemID, Name: "web_search", Status: stringValue(item["status"]), Stage: "added"})
			}
		case "response.web_search_call.in_progress", "response.web_search_call.searching", "response.web_search_call.completed":
			itemID := stringValue(root["item_id"])
			index := intValue(root["output_index"])
			key := a.webSearchKeyByID[itemID]
			if key == "" {
				key = a.sourceWebSearch[index]
			}
			stage := strings.TrimPrefix(typeName, "response.web_search_call.")
			delta.WebSearches = append(delta.WebSearches, adapterStreamWebSearchEvent{Key: key, Index: index, ID: itemID, Name: "web_search", Stage: stage})
		case "response.function_call_arguments.delta":
			itemID := stringValue(root["item_id"])
			index, exists := a.sourceItems[itemID]
			if !exists {
				index = intValue(root["output_index"])
			}
			state, known := a.sourceTools[index]
			if !known {
				state = adapterSourceTool{Key: "responses:" + itemID, ID: firstNonEmptyString(root["call_id"], itemID), ItemID: itemID, Name: stringValue(root["name"])}
				a.sourceTools[index] = state
			}
			delta.Tools = append(delta.Tools, adapterStreamToolDelta{Key: state.Key, Index: index, ID: state.ID, ItemID: state.ItemID, Name: state.Name, Arguments: stringValue(root["delta"]), Started: !known})
		case "response.function_call_arguments.done":
			itemID := stringValue(root["item_id"])
			index, exists := a.sourceItems[itemID]
			if !exists {
				index = intValue(root["output_index"])
			}
			state, known := a.sourceTools[index]
			if !known {
				state = adapterSourceTool{Key: "responses:" + itemID, ID: firstNonEmptyString(root["call_id"], itemID), ItemID: itemID, Name: stringValue(root["name"])}
				a.sourceTools[index] = state
			}
			if a.toolArgs[state.Key] == "" {
				delta.Tools = append(delta.Tools, adapterStreamToolDelta{Key: state.Key, Index: index, ID: state.ID, ItemID: state.ItemID, Name: state.Name, Arguments: stringValue(root["arguments"]), Started: !known})
			}
		case "response.output_item.done":
			item := mapValue(root["item"])
			if stringValue(item["type"]) == "web_search_call" {
				index := intValue(root["output_index"])
				itemID := stringValue(item["id"])
				key := a.webSearchKeyByID[itemID]
				if key == "" {
					key = a.sourceWebSearch[index]
				}
				delta.WebSearches = append(delta.WebSearches, adapterStreamWebSearchEvent{Key: key, Index: index, ID: itemID, Name: "web_search", Action: mapValue(item["action"]), Status: stringValue(item["status"]), Stage: "done"})
			} else if stringValue(item["type"]) == "message" {
				for _, rawPart := range sliceValue(item["content"]) {
					part := mapValue(rawPart)
					if stringValue(part["type"]) == "output_text" && len(a.annotations) == 0 {
						delta.Annotations = append(delta.Annotations, sliceValue(part["annotations"])...)
					} else if stringValue(part["type"]) == "refusal" && a.response.Refusal == "" {
						delta.Refusal = stringValue(part["refusal"])
					}
				}
			}
		case "response.completed", "response.incomplete":
			response := mapValue(root["response"])
			for index, rawItem := range sliceValue(response["output"]) {
				item := mapValue(rawItem)
				switch stringValue(item["type"]) {
				case "message":
					for _, rawPart := range sliceValue(item["content"]) {
						part := mapValue(rawPart)
						switch stringValue(part["type"]) {
						case "output_text":
							if a.response.Text == "" {
								delta.Text += stringValue(part["text"])
							}
							if len(a.annotations) == 0 {
								delta.Annotations = append(delta.Annotations, sliceValue(part["annotations"])...)
							}
						case "refusal":
							if a.response.Refusal == "" {
								delta.Refusal += stringValue(part["refusal"])
							}
						}
					}
				case "web_search_call":
					id := stringValue(item["id"])
					key := a.webSearchKeyByID[id]
					if key == "" {
						key = fmt.Sprintf("responses-web-search:%d", index)
					}
					if state := a.webSearches[key]; state == nil || !state.Completed {
						delta.WebSearches = append(delta.WebSearches, adapterStreamWebSearchEvent{Key: key, Index: index, ID: id, Name: "web_search", Action: mapValue(item["action"]), Status: stringValue(item["status"]), Stage: "done"})
					}
				}
			}
			delta.Done = true
			if typeName == "response.incomplete" {
				delta.StopReason = responseStopReason(response)
			} else if stringValue(response["status"]) == "incomplete" {
				// Some compatible Responses servers use response.completed as the
				// transport sentinel even when the embedded response is incomplete.
				delta.StopReason = responseStopReason(response)
			}
		}
	case "messages":
		typeName := stringValue(root["type"])
		if typeName == "error" {
			delta.Error = normalizeStreamError(mapValue(root["error"]), "messages_error")
			return delta, nil
		}
		switch typeName {
		case "message_start":
			message := mapValue(root["message"])
			delta.ID, delta.Model = stringValue(message["id"]), stringValue(message["model"])
			if usage := mapValue(message["usage"]); usage != nil {
				delta.Usage = normalizeUsageMap(usage, a.source)
			}
		case "content_block_start":
			block := mapValue(root["content_block"])
			switch stringValue(block["type"]) {
			case "tool_use":
				index := intValue(root["index"])
				state := adapterSourceTool{Key: fmt.Sprintf("messages:%d", index), ID: stringValue(block["id"]), Name: stringValue(block["name"])}
				a.sourceTools[index] = state
				delta.Tools = append(delta.Tools, adapterStreamToolDelta{Key: state.Key, Index: index, ID: state.ID, Name: state.Name, Started: true})
			case "server_tool_use":
				index := intValue(root["index"])
				id := stringValue(block["id"])
				key := "messages-web-search:" + id
				if id == "" {
					key = fmt.Sprintf("messages-web-search:%d", index)
				}
				a.sourceWebSearch[index] = key
				if id != "" {
					a.webSearchKeyByID[id] = key
				}
				arguments := ""
				if input := mapValue(block["input"]); len(input) > 0 {
					encoded, _ := json.Marshal(block["input"])
					arguments = string(encoded)
				}
				delta.WebSearches = append(delta.WebSearches, adapterStreamWebSearchEvent{Key: key, Index: index, ID: id, Name: firstNonEmptyString(block["name"], "web_search"), Arguments: arguments, Stage: "server_start"})
			case "web_search_tool_result":
				index := intValue(root["index"])
				id := stringValue(block["tool_use_id"])
				key := a.webSearchKeyByID[id]
				if key == "" {
					key = "messages-web-search:" + id
				}
				a.sourceWebSearch[index] = key
				status := "completed"
				if result := mapValue(block["content"]); stringValue(result["type"]) == "web_search_tool_result_error" {
					status = "failed"
				}
				delta.WebSearches = append(delta.WebSearches, adapterStreamWebSearchEvent{Key: key, Index: index, ID: id, Name: "web_search", Result: block["content"], Status: status, Stage: "result_start"})
			}
		case "content_block_delta":
			content := mapValue(root["delta"])
			switch stringValue(content["type"]) {
			case "text_delta":
				delta.Text = stringValue(content["text"])
			case "citations_delta", "citation_delta":
				if citation := content["citation"]; citation != nil {
					delta.Annotations = append(delta.Annotations, citation)
				}
			case "thinking_delta":
				delta.Reasoning = stringValue(content["thinking"])
			case "signature_delta":
				delta.Signature = stringValue(content["signature"])
			case "input_json_delta":
				index := intValue(root["index"])
				if key := a.sourceWebSearch[index]; key != "" {
					delta.WebSearches = append(delta.WebSearches, adapterStreamWebSearchEvent{Key: key, Index: index, Arguments: stringValue(content["partial_json"]), Stage: "server_args"})
					break
				}
				state, exists := a.sourceTools[index]
				if !exists {
					state = adapterSourceTool{Key: fmt.Sprintf("messages:%d", index)}
					a.sourceTools[index] = state
				}
				delta.Tools = append(delta.Tools, adapterStreamToolDelta{Key: state.Key, Index: index, ID: state.ID, Name: state.Name, Arguments: stringValue(content["partial_json"]), Started: !exists})
			}
		case "content_block_stop":
			index := intValue(root["index"])
			if key := a.sourceWebSearch[index]; key != "" {
				state := a.webSearches[key]
				stage := "server_stop"
				if state != nil && state.Result != nil {
					stage = "result_stop"
				}
				delta.WebSearches = append(delta.WebSearches, adapterStreamWebSearchEvent{Key: key, Index: index, Stage: stage})
			}
		case "message_delta":
			change := mapValue(root["delta"])
			delta.StopReason = stringValue(change["stop_reason"])
			if usage := mapValue(root["usage"]); usage != nil {
				delta.Usage = normalizeUsageMap(usage, a.source)
			}
		case "message_stop":
			delta.Done = true
		}
	default:
		return adapterStreamDelta{}, fmt.Errorf("%w: unknown stream source %q", errUnsupportedProtocolConversion, a.source)
	}
	return delta, nil
}

func (delta adapterStreamDelta) empty() bool {
	return delta.Text == "" && delta.Refusal == "" && delta.Reasoning == "" && len(delta.Annotations) == 0 && len(delta.Tools) == 0 && len(delta.WebSearches) == 0 && delta.StopReason == "" && !delta.Done && delta.ID == "" && delta.Model == "" && len(delta.Usage) == 0 && len(delta.Error) == 0
}

func normalizeStreamError(value map[string]any, fallbackType string) map[string]any {
	if value == nil {
		value = map[string]any{}
	}
	errorType := stringValue(value["type"])
	if errorType == "" || errorType == "error" {
		errorType = fallbackType
	}
	return map[string]any{
		"type":    errorType,
		"code":    value["code"],
		"message": firstNonEmptyString(value["message"], "upstream stream failed"),
		"param":   value["param"],
	}
}

func (a *protocolStreamAdapter) appendAnnotation(annotation any) {
	wanted, _ := json.Marshal(openAIStreamAnnotation(annotation))
	for _, existing := range a.annotations {
		encoded, _ := json.Marshal(openAIStreamAnnotation(existing))
		if bytes.Equal(wanted, encoded) {
			return
		}
	}
	a.annotations = append(a.annotations, annotation)
	a.response.Annotations = append(a.response.Annotations, annotation)
}

func writeAdapterSSE(output *bytes.Buffer, eventName string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	if eventName != "" {
		fmt.Fprintf(output, "event: %s\n", eventName)
	}
	fmt.Fprintf(output, "data: %s\n\n", data)
}

func rawSSEFrame(eventName string, data []byte) []byte {
	var output bytes.Buffer
	if strings.TrimSpace(eventName) != "" {
		fmt.Fprintf(&output, "event: %s\n", strings.TrimSpace(eventName))
	}
	fmt.Fprintf(&output, "data: %s\n\n", bytes.TrimSpace(data))
	return output.Bytes()
}

func openAIStreamAnnotations(values []any) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		if value != nil {
			result = append(result, openAIStreamAnnotation(value))
		}
	}
	return result
}

func openAIStreamAnnotation(value any) map[string]any {
	citation := canonicalStreamCitation(value)
	result := map[string]any{"type": firstNonEmptyString(citation["type"], "url_citation")}
	for _, key := range []string{"url", "title", "start_index", "end_index"} {
		if citation[key] != nil && citation[key] != "" {
			result[key] = citation[key]
		}
	}
	return result
}

func anthropicStreamCitation(value any, text string) map[string]any {
	citation := canonicalStreamCitation(value)
	result := map[string]any{}
	if stringValue(citation["url"]) != "" {
		result["type"] = "web_search_result_location"
		result["url"] = citation["url"]
		if citation["title"] != nil {
			result["title"] = citation["title"]
		}
	} else {
		result["type"] = firstNonEmptyString(citation["type"], "char_location")
		if citation["title"] != nil {
			result["document_title"] = citation["title"]
		}
	}
	start, hasStart := anyInt(citation["start_index"])
	end, hasEnd := anyInt(citation["end_index"])
	if hasStart {
		result["start_char_index"] = start
	}
	if hasEnd {
		result["end_char_index"] = end
	}
	if hasStart && hasEnd && start >= 0 && end >= start && end <= len(text) {
		result["cited_text"] = text[start:end]
	}
	return result
}

func canonicalStreamCitation(value any) map[string]any {
	if citation, ok := value.(adapterCitation); ok {
		result := cloneMap(citation.Raw)
		if result == nil {
			result = map[string]any{}
		}
		if citation.Type != "" {
			result["type"] = citation.Type
		}
		if citation.URL != "" {
			result["url"] = citation.URL
		}
		if citation.Title != "" {
			result["title"] = citation.Title
		}
		if citation.Start != nil {
			result["start_index"] = *citation.Start
		}
		if citation.End != nil {
			result["end_index"] = *citation.End
		}
		return result
	}
	result := cloneMap(mapValue(value))
	if result == nil {
		return map[string]any{}
	}
	if nested := mapValue(result["url_citation"]); nested != nil {
		for key, nestedValue := range nested {
			if result[key] == nil {
				result[key] = nestedValue
			}
		}
	}
	if result["start_index"] == nil {
		result["start_index"] = result["start_char_index"]
	}
	if result["end_index"] == nil {
		result["end_index"] = result["end_char_index"]
	}
	if result["title"] == nil {
		result["title"] = result["document_title"]
	}
	return result
}

func anyInt(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		return int(number), true
	case json.Number:
		parsed, err := number.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}
