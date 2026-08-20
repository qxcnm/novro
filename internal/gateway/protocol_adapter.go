package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var errUnsupportedProtocolConversion = errors.New("unsupported protocol conversion")

type adapterPart struct {
	Type      string
	Text      string
	ID        string
	Name      string
	Arguments any
	ImageURL  string
	MediaType string
	Data      string
	Signature string
}

type adapterMessage struct {
	Role  string
	Parts []adapterPart
}

type adapterRequest struct {
	Model       string
	System      string
	Messages    []adapterMessage
	MaxTokens   int
	Stream      bool
	Temperature any
	TopP        any
	Stop        any
	Tools       []any
	ToolChoice  any
	Extra       map[string]any
}

type adapterResponse struct {
	ID                 string
	Model              string
	Created            int64
	Text               string
	Reasoning          string
	ReasoningSignature string
	ToolCalls          []adapterPart
	StopReason         string
	Usage              map[string]any
}

func adaptProtocolRequest(payload map[string]any, source, target, model string, stream bool) ([]byte, error) {
	if source == target {
		copyPayload := cloneMap(payload)
		copyPayload["model"] = model
		if _, exists := payload["stream"]; exists {
			copyPayload["stream"] = stream
		}
		if target == "chat_completions" && stream {
			options := mapValue(copyPayload["stream_options"])
			if options == nil {
				options = map[string]any{}
			} else {
				options = cloneMap(options)
			}
			options["include_usage"] = true
			copyPayload["stream_options"] = options
		}
		return json.Marshal(copyPayload)
	}
	if source == "responses" {
		if value, exists := payload["previous_response_id"]; exists && value != nil {
			previousID, isString := value.(string)
			if !isString || strings.TrimSpace(previousID) != "" {
				return nil, fmt.Errorf("%w: previous_response_id requires a Responses upstream; resend the prior output items in input for cross-protocol conversion", errUnsupportedProtocolConversion)
			}
		}
	}
	request, err := decodeAdapterRequest(payload, source)
	if err != nil {
		return nil, err
	}
	request.Model, request.Stream = model, stream
	converted, err := encodeAdapterRequest(request, target)
	if err != nil {
		return nil, err
	}
	return json.Marshal(converted)
}

func decodeAdapterRequest(payload map[string]any, source string) (adapterRequest, error) {
	request := adapterRequest{
		Model: stringValue(payload["model"]), Stream: boolValue(payload["stream"]),
		Temperature: payload["temperature"], TopP: payload["top_p"], Stop: payload["stop"],
		Tools: sliceValue(payload["tools"]), ToolChoice: payload["tool_choice"], Extra: map[string]any{},
	}
	for _, key := range []string{"metadata", "parallel_tool_calls", "reasoning_effort", "reasoning", "thinking", "output_config", "service_tier", "response_format", "text"} {
		if value, exists := payload[key]; exists {
			request.Extra[key] = value
		}
	}
	var err error
	switch source {
	case "chat_completions":
		request.MaxTokens = firstPositiveInt(payload["max_completion_tokens"], payload["max_tokens"])
		request.Messages, request.System, err = decodeChatMessages(sliceValue(payload["messages"]))
	case "responses":
		request.MaxTokens = intValue(payload["max_output_tokens"])
		request.System = stringValue(payload["instructions"])
		var system string
		request.Messages, system, err = decodeResponsesInput(payload["input"])
		request.System = strings.Join(nonEmptyStrings([]string{request.System, system}), "\n\n")
	case "messages":
		request.MaxTokens = intValue(payload["max_tokens"])
		request.Stop = payload["stop_sequences"]
		request.System = textFromContent(payload["system"])
		request.Messages, _, err = decodeAnthropicMessages(sliceValue(payload["messages"]))
	default:
		return adapterRequest{}, fmt.Errorf("%w: unknown source %q", errUnsupportedProtocolConversion, source)
	}
	if err != nil {
		return adapterRequest{}, err
	}
	return request, nil
}

func encodeAdapterRequest(request adapterRequest, target string) (map[string]any, error) {
	payload := map[string]any{"model": request.Model, "stream": request.Stream}
	copyOptionalSampling(payload, request, target)
	switch target {
	case "chat_completions":
		payload["messages"] = encodeChatMessages(request)
		payload["max_tokens"] = positiveOrDefault(request.MaxTokens)
		if len(request.Tools) > 0 {
			tools, err := convertTools(request.Tools, target)
			if err != nil {
				return nil, err
			}
			if len(tools) > 0 {
				payload["tools"] = tools
			}
		}
		if request.ToolChoice != nil && len(sliceValue(payload["tools"])) > 0 {
			if choice := convertToolChoice(request.ToolChoice, target); choice != nil {
				payload["tool_choice"] = choice
			}
		}
		if request.Stream {
			payload["stream_options"] = map[string]any{"include_usage": true}
		}
		copyRequestExtras(payload, request.Extra, "parallel_tool_calls", "service_tier", "response_format")
		copyReasoningConfig(payload, request.Extra, target)
		if _, exists := payload["response_format"]; !exists {
			if format := responsesTextToChat(request.Extra["text"]); format != nil {
				payload["response_format"] = format
			} else if format := anthropicOutputToChat(request.Extra["output_config"]); format != nil {
				payload["response_format"] = format
			}
		}
	case "responses":
		payload["input"] = encodeResponsesInput(request.Messages)
		payload["max_output_tokens"] = positiveOrDefault(request.MaxTokens)
		if request.System != "" {
			payload["instructions"] = request.System
		}
		if len(request.Tools) > 0 {
			tools, err := convertTools(request.Tools, target)
			if err != nil {
				return nil, err
			}
			if len(tools) > 0 {
				payload["tools"] = tools
			}
		}
		if request.ToolChoice != nil && len(sliceValue(payload["tools"])) > 0 {
			if choice := convertToolChoice(request.ToolChoice, target); choice != nil {
				payload["tool_choice"] = choice
			}
		}
		copyRequestExtras(payload, request.Extra, "metadata", "parallel_tool_calls", "service_tier", "text")
		copyReasoningConfig(payload, request.Extra, target)
		if _, exists := payload["text"]; !exists {
			if format := chatResponseFormatToResponses(request.Extra["response_format"]); format != nil {
				payload["text"] = map[string]any{"format": format}
			} else if format := anthropicOutputToResponses(request.Extra["output_config"]); format != nil {
				payload["text"] = map[string]any{"format": format}
			}
		}
	case "messages":
		payload["messages"] = encodeAnthropicMessages(request.Messages)
		payload["max_tokens"] = positiveOrDefault(request.MaxTokens)
		if request.System != "" {
			payload["system"] = request.System
		}
		if len(request.Tools) > 0 {
			tools, err := convertTools(request.Tools, target)
			if err != nil {
				return nil, err
			}
			if len(tools) > 0 {
				payload["tools"] = tools
			}
		}
		if request.ToolChoice != nil && len(sliceValue(payload["tools"])) > 0 {
			if choice := convertToolChoice(request.ToolChoice, target); choice != nil {
				payload["tool_choice"] = choice
			}
		}
		copyRequestExtras(payload, request.Extra, "metadata", "service_tier")
		copyReasoningConfig(payload, request.Extra, target)
		if format := openAIOutputToAnthropic(request.Extra); format != nil {
			outputConfig := mapValue(payload["output_config"])
			if outputConfig == nil {
				outputConfig = map[string]any{}
			} else {
				outputConfig = cloneMap(outputConfig)
			}
			outputConfig["format"] = format
			payload["output_config"] = outputConfig
		}
	default:
		return nil, fmt.Errorf("%w: unknown target %q", errUnsupportedProtocolConversion, target)
	}
	if _, hasTools := payload["tools"]; !hasTools {
		delete(payload, "parallel_tool_calls")
	}
	return payload, nil
}

func decodeChatMessages(values []any) ([]adapterMessage, string, error) {
	messages := make([]adapterMessage, 0, len(values))
	systems := make([]string, 0, 2)
	for _, raw := range values {
		message := mapValue(raw)
		if message == nil {
			return nil, "", fmt.Errorf("%w: invalid chat message", errUnsupportedProtocolConversion)
		}
		role := stringValue(message["role"])
		if role == "system" || role == "developer" {
			systems = append(systems, textFromContent(message["content"]))
			continue
		}
		parts := decodeCommonContent(message["content"])
		if reasoning := stringValue(message["reasoning_content"]); reasoning != "" {
			parts = append([]adapterPart{{Type: "reasoning", Text: reasoning}}, parts...)
		}
		for _, rawCall := range sliceValue(message["tool_calls"]) {
			call := mapValue(rawCall)
			function := mapValue(call["function"])
			parts = append(parts, adapterPart{Type: "tool_call", ID: stringValue(call["id"]), Name: stringValue(function["name"]), Arguments: decodeJSONValue(function["arguments"])})
		}
		if role == "tool" {
			parts = []adapterPart{{Type: "tool_result", ID: stringValue(message["tool_call_id"]), Text: textFromContent(message["content"])}}
		}
		messages = append(messages, adapterMessage{Role: role, Parts: parts})
	}
	return messages, strings.Join(nonEmptyStrings(systems), "\n\n"), nil
}

func decodeAnthropicMessages(values []any) ([]adapterMessage, string, error) {
	messages := make([]adapterMessage, 0, len(values))
	for _, raw := range values {
		message := mapValue(raw)
		if message == nil {
			return nil, "", fmt.Errorf("%w: invalid Anthropic message", errUnsupportedProtocolConversion)
		}
		parts := make([]adapterPart, 0)
		if text, ok := message["content"].(string); ok {
			parts = append(parts, adapterPart{Type: "text", Text: text})
		} else {
			for _, rawPart := range sliceValue(message["content"]) {
				part := mapValue(rawPart)
				switch stringValue(part["type"]) {
				case "text":
					parts = append(parts, adapterPart{Type: "text", Text: stringValue(part["text"])})
				case "thinking":
					parts = append(parts, adapterPart{Type: "reasoning", Text: stringValue(part["thinking"]), Signature: stringValue(part["signature"])})
				case "tool_use":
					parts = append(parts, adapterPart{Type: "tool_call", ID: stringValue(part["id"]), Name: stringValue(part["name"]), Arguments: part["input"]})
				case "tool_result":
					parts = append(parts, adapterPart{Type: "tool_result", ID: stringValue(part["tool_use_id"]), Text: textFromContent(part["content"])})
				case "image":
					source := mapValue(part["source"])
					switch stringValue(source["type"]) {
					case "url":
						parts = append(parts, adapterPart{Type: "image", ImageURL: stringValue(source["url"])})
					case "base64":
						parts = append(parts, adapterPart{Type: "image", MediaType: stringValue(source["media_type"]), Data: stringValue(source["data"])})
					default:
						return nil, "", fmt.Errorf("%w: unsupported Anthropic image source %q", errUnsupportedProtocolConversion, stringValue(source["type"]))
					}
				default:
					return nil, "", fmt.Errorf("%w: Anthropic content block %q", errUnsupportedProtocolConversion, stringValue(part["type"]))
				}
			}
		}
		messages = append(messages, adapterMessage{Role: stringValue(message["role"]), Parts: parts})
	}
	return messages, "", nil
}

func decodeResponsesInput(raw any) ([]adapterMessage, string, error) {
	if text, ok := raw.(string); ok {
		return []adapterMessage{{Role: "user", Parts: []adapterPart{{Type: "text", Text: text}}}}, "", nil
	}
	messages := make([]adapterMessage, 0)
	systems := make([]string, 0, 2)
	for _, rawItem := range sliceValue(raw) {
		item := mapValue(rawItem)
		if item == nil {
			return nil, "", fmt.Errorf("%w: invalid Responses input item", errUnsupportedProtocolConversion)
		}
		switch stringValue(item["type"]) {
		case "", "message":
			role := stringValue(item["role"])
			parts := decodeCommonContent(item["content"])
			if role == "system" || role == "developer" {
				systems = append(systems, textFromParts(parts))
				continue
			}
			messages = append(messages, adapterMessage{Role: role, Parts: parts})
		case "function_call":
			messages = append(messages, adapterMessage{Role: "assistant", Parts: []adapterPart{{Type: "tool_call", ID: stringValue(item["call_id"]), Name: stringValue(item["name"]), Arguments: decodeJSONValue(item["arguments"])}}})
		case "function_call_output":
			messages = append(messages, adapterMessage{Role: "tool", Parts: []adapterPart{{Type: "tool_result", ID: stringValue(item["call_id"]), Text: textFromContent(item["output"])}}})
		case "reasoning":
			var summary strings.Builder
			for _, rawPart := range sliceValue(item["summary"]) {
				summary.WriteString(stringValue(mapValue(rawPart)["text"]))
			}
			if summary.Len() > 0 {
				messages = append(messages, adapterMessage{Role: "assistant", Parts: []adapterPart{{Type: "reasoning", Text: summary.String()}}})
			}
		default:
			// Responses clients may send provider-side history references such as
			// item_reference when continuing a prior tool turn. There is no
			// equivalent field in Chat or Messages. The concrete historical
			// message/function/tool items are already present when
			// previous_response_id was expanded, so drop only this non-portable
			// reference instead of rejecting the whole request.
			continue
		}
	}
	return messages, strings.Join(nonEmptyStrings(systems), "\n\n"), nil
}

func decodeCommonContent(raw any) []adapterPart {
	if text, ok := raw.(string); ok {
		return []adapterPart{{Type: "text", Text: text}}
	}
	parts := make([]adapterPart, 0)
	for _, rawPart := range sliceValue(raw) {
		part := mapValue(rawPart)
		typeName := stringValue(part["type"])
		switch typeName {
		case "text", "input_text", "output_text":
			parts = append(parts, adapterPart{Type: "text", Text: stringValue(part["text"])})
		case "image_url":
			imageURL := stringValue(part["image_url"])
			if image := mapValue(part["image_url"]); image != nil {
				imageURL = stringValue(image["url"])
			}
			parts = append(parts, decodeAdapterImage(imageURL))
		case "input_image":
			parts = append(parts, decodeAdapterImage(stringValue(part["image_url"])))
		case "refusal":
			parts = append(parts, adapterPart{Type: "text", Text: stringValue(part["refusal"])})
		}
	}
	return parts
}

func encodeChatMessages(request adapterRequest) []any {
	result := make([]any, 0, len(request.Messages)+1)
	if request.System != "" {
		result = append(result, map[string]any{"role": "system", "content": request.System})
	}
	for _, message := range coalesceAssistantMessages(request.Messages) {
		for _, part := range message.Parts {
			if part.Type == "tool_result" {
				result = append(result, map[string]any{"role": "tool", "tool_call_id": part.ID, "content": part.Text})
				continue
			}
		}
		entry := map[string]any{"role": normalizeRole(message.Role)}
		content := encodeChatContent(message.Parts)
		entry["content"] = content
		calls := make([]any, 0)
		for _, part := range message.Parts {
			if part.Type == "tool_call" {
				arguments, _ := json.Marshal(nonNilObject(part.Arguments))
				calls = append(calls, map[string]any{"id": part.ID, "type": "function", "function": map[string]any{"name": part.Name, "arguments": string(arguments)}})
			}
			if part.Type == "reasoning" {
				entry["reasoning_content"] = part.Text
			}
		}
		if len(calls) > 0 {
			entry["tool_calls"] = calls
		}
		if !onlyToolResults(message.Parts) {
			result = append(result, entry)
		}
	}
	return result
}

func encodeResponsesInput(messages []adapterMessage) []any {
	result := make([]any, 0, len(messages))
	for _, message := range messages {
		content := make([]any, 0)
		for _, part := range message.Parts {
			switch part.Type {
			case "text", "reasoning":
				content = append(content, map[string]any{"type": "input_text", "text": part.Text})
			case "image":
				content = append(content, map[string]any{"type": "input_image", "image_url": adapterImageURL(part)})
			case "tool_call":
				arguments, _ := json.Marshal(nonNilObject(part.Arguments))
				result = append(result, map[string]any{"type": "function_call", "call_id": part.ID, "name": part.Name, "arguments": string(arguments)})
			case "tool_result":
				result = append(result, map[string]any{"type": "function_call_output", "call_id": part.ID, "output": part.Text})
			}
		}
		if len(content) > 0 {
			result = append(result, map[string]any{"type": "message", "role": normalizeRole(message.Role), "content": content})
		}
	}
	return result
}

func encodeAnthropicMessages(messages []adapterMessage) []any {
	messages = coalesceAnthropicMessages(messages)
	result := make([]any, 0, len(messages))
	unsignedReasoning := make(map[string]struct{})
	for _, message := range messages {
		for _, part := range message.Parts {
			if part.Type == "reasoning" && part.Signature == "" {
				if normalized := strings.TrimSpace(part.Text); normalized != "" {
					unsignedReasoning[normalized] = struct{}{}
				}
			}
		}
	}
	for _, message := range messages {
		role := normalizeRole(message.Role)
		if role != "assistant" {
			role = "user"
		}
		content := make([]any, 0, len(message.Parts))
		for _, part := range message.Parts {
			switch part.Type {
			case "text":
				if part.Text != "" {
					if _, duplicatedReasoning := unsignedReasoning[strings.TrimSpace(part.Text)]; duplicatedReasoning {
						continue
					}
					content = append(content, map[string]any{"type": "text", "text": part.Text})
				}
			case "reasoning":
				if part.Signature != "" {
					content = append(content, map[string]any{"type": "thinking", "thinking": part.Text, "signature": part.Signature})
				}
				// Anthropic verifies thinking signatures when an assistant turn is
				// sent back during tool use. Unsigned reasoning imported from an
				// OpenAI-style protocol is private model state: omit it instead of
				// forging a signature or exposing it as visible prompt text.
			case "image":
				if part.Data != "" {
					content = append(content, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": defaultMediaType(part.MediaType), "data": part.Data}})
				} else {
					content = append(content, map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": part.ImageURL}})
				}
			case "tool_call":
				content = append(content, map[string]any{"type": "tool_use", "id": part.ID, "name": part.Name, "input": nonNilObject(part.Arguments)})
			case "tool_result":
				content = append(content, map[string]any{"type": "tool_result", "tool_use_id": part.ID, "content": part.Text})
			}
		}
		if len(content) == 0 {
			continue
		}
		result = append(result, map[string]any{"role": role, "content": content})
	}
	return result
}

func coalesceAssistantMessages(messages []adapterMessage) []adapterMessage {
	result := make([]adapterMessage, 0, len(messages))
	for _, message := range messages {
		if normalizeRole(message.Role) == "assistant" && !onlyToolResults(message.Parts) && len(result) > 0 {
			last := &result[len(result)-1]
			if normalizeRole(last.Role) == "assistant" && !onlyToolResults(last.Parts) {
				last.Parts = append(last.Parts, message.Parts...)
				continue
			}
		}
		result = append(result, adapterMessage{Role: message.Role, Parts: append([]adapterPart(nil), message.Parts...)})
	}
	return result
}

func coalesceAnthropicMessages(messages []adapterMessage) []adapterMessage {
	result := make([]adapterMessage, 0, len(messages))
	for _, message := range messages {
		role := normalizeRole(message.Role)
		if len(result) > 0 && result[len(result)-1].Role == role {
			result[len(result)-1].Parts = append(result[len(result)-1].Parts, message.Parts...)
			continue
		}
		result = append(result, adapterMessage{Role: role, Parts: append([]adapterPart(nil), message.Parts...)})
	}
	return result
}

func adaptProtocolResponse(body []byte, source, target string) ([]byte, error) {
	if source == target {
		return body, nil
	}
	response, err := decodeAdapterResponse(body, source)
	if err != nil {
		return nil, err
	}
	return json.Marshal(encodeAdapterResponse(response, target))
}

func decodeAdapterResponse(body []byte, source string) (adapterResponse, error) {
	var root map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return adapterResponse{}, fmt.Errorf("decode upstream %s response: %w", source, err)
	}
	result := adapterResponse{ID: stringValue(root["id"]), Model: stringValue(root["model"]), Created: int64(firstPositiveInt(root["created"], root["created_at"])), Usage: normalizeUsageMap(mapValue(root["usage"]), source)}
	if result.Created == 0 {
		result.Created = time.Now().Unix()
	}
	switch source {
	case "chat_completions":
		choices := sliceValue(root["choices"])
		if len(choices) > 0 {
			choice := mapValue(choices[0])
			message := mapValue(choice["message"])
			result.Text = textFromContent(message["content"])
			result.Reasoning = firstNonEmptyString(message["reasoning_content"], message["reasoning"], message["reasoning_text"])
			result.StopReason = stringValue(choice["finish_reason"])
			result.ToolCalls = decodeChatToolCalls(sliceValue(message["tool_calls"]))
		}
	case "responses":
		result.StopReason = responseStopReason(root)
		for _, raw := range sliceValue(root["output"]) {
			item := mapValue(raw)
			switch stringValue(item["type"]) {
			case "message":
				for _, rawPart := range sliceValue(item["content"]) {
					part := mapValue(rawPart)
					switch stringValue(part["type"]) {
					case "output_text":
						result.Text += stringValue(part["text"])
					case "refusal":
						result.Text += stringValue(part["refusal"])
					}
				}
			case "reasoning":
				for _, rawPart := range sliceValue(item["summary"]) {
					result.Reasoning += stringValue(mapValue(rawPart)["text"])
				}
			case "function_call":
				result.ToolCalls = append(result.ToolCalls, adapterPart{Type: "tool_call", ID: stringValue(item["call_id"]), Name: stringValue(item["name"]), Arguments: decodeJSONValue(item["arguments"])})
			}
		}
	case "messages":
		result.StopReason = stringValue(root["stop_reason"])
		for _, rawPart := range sliceValue(root["content"]) {
			part := mapValue(rawPart)
			switch stringValue(part["type"]) {
			case "text":
				result.Text += stringValue(part["text"])
			case "thinking":
				result.Reasoning += stringValue(part["thinking"])
				if result.ReasoningSignature == "" {
					result.ReasoningSignature = stringValue(part["signature"])
				}
			case "tool_use":
				result.ToolCalls = append(result.ToolCalls, adapterPart{Type: "tool_call", ID: stringValue(part["id"]), Name: stringValue(part["name"]), Arguments: part["input"]})
			}
		}
	default:
		return adapterResponse{}, fmt.Errorf("%w: unknown response source %q", errUnsupportedProtocolConversion, source)
	}
	if len(result.ToolCalls) > 0 && (result.StopReason == "" || result.StopReason == "stop" || result.StopReason == "end_turn") {
		result.StopReason = "tool_use"
	}
	return result, nil
}

func encodeAdapterResponse(response adapterResponse, target string) map[string]any {
	response.ToolCalls = normalizeAdapterToolCalls(response.ID, response.ToolCalls)
	switch target {
	case "chat_completions":
		message := map[string]any{"role": "assistant", "content": response.Text}
		if response.Reasoning != "" {
			message["reasoning_content"] = response.Reasoning
		}
		if len(response.ToolCalls) > 0 {
			message["tool_calls"] = encodeChatToolCalls(response.ToolCalls)
		}
		result := map[string]any{"id": response.ID, "object": "chat.completion", "created": response.Created, "model": response.Model, "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": chatStopReason(response.StopReason)}}}
		setAdapterUsage(result, response.Usage, target)
		return result
	case "responses":
		output := make([]any, 0, 3)
		if response.Reasoning != "" {
			output = append(output, map[string]any{"id": "rs_" + safeID(response.ID), "type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": response.Reasoning}}})
		}
		if response.Text != "" {
			output = append(output, map[string]any{"id": "msg_" + safeID(response.ID), "type": "message", "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": response.Text, "annotations": []any{}}}})
		}
		for _, call := range response.ToolCalls {
			arguments, _ := json.Marshal(nonNilObject(call.Arguments))
			output = append(output, map[string]any{"id": responsesFunctionItemID(call.ID), "type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": string(arguments), "status": "completed"})
		}
		status := "completed"
		result := map[string]any{"id": response.ID, "object": "response", "created_at": response.Created, "status": status, "model": response.Model, "output": output}
		switch chatStopReason(response.StopReason) {
		case "length":
			result["status"] = "incomplete"
			result["incomplete_details"] = map[string]any{"reason": "max_output_tokens"}
		case "content_filter":
			result["status"] = "incomplete"
			result["incomplete_details"] = map[string]any{"reason": "content_filter"}
		}
		setAdapterUsage(result, response.Usage, target)
		return result
	case "messages":
		content := make([]any, 0, 3)
		if response.Reasoning != "" && response.ReasoningSignature != "" {
			content = append(content, map[string]any{"type": "thinking", "thinking": response.Reasoning, "signature": response.ReasoningSignature})
		}
		if response.Text != "" && !duplicatesUnsignedReasoning(response.Text, response.Reasoning, response.ReasoningSignature) {
			content = append(content, map[string]any{"type": "text", "text": response.Text})
		}
		for _, call := range response.ToolCalls {
			content = append(content, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": nonNilObject(call.Arguments)})
		}
		result := map[string]any{"id": response.ID, "type": "message", "role": "assistant", "model": response.Model, "content": content, "stop_reason": anthropicStopReason(response.StopReason), "stop_sequence": nil}
		setAdapterUsage(result, response.Usage, target)
		return result
	}
	return map[string]any{}
}

func duplicatesUnsignedReasoning(text, reasoning, signature string) bool {
	if signature != "" {
		return false
	}
	normalizedText := strings.TrimSpace(text)
	return normalizedText != "" && normalizedText == strings.TrimSpace(reasoning)
}

func normalizeAdapterToolCalls(responseID string, calls []adapterPart) []adapterPart {
	if len(calls) == 0 {
		return nil
	}
	normalized := append([]adapterPart(nil), calls...)
	seen := make(map[string]struct{}, len(normalized))
	base := "call_novro_" + safeID(responseID)
	for index := range normalized {
		callID := strings.TrimSpace(normalized[index].ID)
		if _, duplicate := seen[callID]; callID == "" || duplicate {
			candidate := fmt.Sprintf("%s_%d", base, index)
			callID = candidate
			for suffix := 1; ; suffix++ {
				if _, exists := seen[callID]; !exists {
					break
				}
				callID = fmt.Sprintf("%s_%d", candidate, suffix)
			}
		}
		normalized[index].ID = callID
		seen[callID] = struct{}{}
	}
	return normalized
}

func responsesFunctionItemID(callID string) string {
	return "fc_" + strings.TrimSpace(callID)
}

func normalizeUsageMap(usage map[string]any, source string) map[string]any {
	if usage == nil {
		return map[string]any{}
	}
	input, output := intValue(usage["input_tokens"]), intValue(usage["output_tokens"])
	_, hasInput := usage["input_tokens"]
	_, hasOutput := usage["output_tokens"]
	if source == "chat_completions" {
		if value, exists := usageInt(usage, "prompt_tokens"); exists {
			input, hasInput = value, true
		}
		if value, exists := usageInt(usage, "completion_tokens"); exists {
			output, hasOutput = value, true
		}
	} else {
		if !hasInput {
			input, hasInput = usageInt(usage, "prompt_tokens")
		}
		if !hasOutput {
			output, hasOutput = usageInt(usage, "completion_tokens")
		}
	}
	cached, hasCached := usageInt(usage, "cache_read_input_tokens")
	created, hasCreated := usageInt(usage, "cache_creation_input_tokens")
	if details := mapValue(usage["input_tokens_details"]); details != nil {
		cached, hasCached = usageInt(details, "cached_tokens")
	}
	if details := mapValue(usage["prompt_tokens_details"]); details != nil {
		cached, hasCached = usageInt(details, "cached_tokens")
	}
	if creation := mapValue(usage["cache_creation"]); creation != nil {
		created = intValue(creation["ephemeral_5m_input_tokens"]) + intValue(creation["ephemeral_1h_input_tokens"])
		hasCreated = true
	}
	reasoning := 0
	hasReasoning := false
	if details := mapValue(usage["output_tokens_details"]); details != nil {
		reasoning, hasReasoning = usageInt(details, "reasoning_tokens")
		if !hasReasoning {
			reasoning, hasReasoning = usageInt(details, "thinking_tokens")
		}
	}
	if details := mapValue(usage["completion_tokens_details"]); details != nil {
		reasoning, hasReasoning = usageInt(details, "reasoning_tokens")
	}
	// Anthropic reports uncached input separately from cache reads/writes. The
	// adapter IR keeps a protocol-neutral total so OpenAI targets receive the
	// same logical input token count.
	if source == "messages" && hasInput {
		input += cached + created
	}
	result := map[string]any{}
	if hasInput {
		result["input_tokens"] = input
	}
	if hasOutput {
		result["output_tokens"] = output
	}
	if hasInput && hasOutput {
		result["total_tokens"] = input + output
	}
	if hasCached {
		result["cached_tokens"] = cached
	}
	if hasCreated {
		result["cache_creation_tokens"] = created
	}
	if hasReasoning {
		result["reasoning_tokens"] = reasoning
	}
	return result
}

func usageForTarget(usage map[string]any, target string) map[string]any {
	if len(usage) == 0 {
		return nil
	}
	input, output := intValue(usage["input_tokens"]), intValue(usage["output_tokens"])
	cached, created := intValue(usage["cached_tokens"]), intValue(usage["cache_creation_tokens"])
	reasoning := intValue(usage["reasoning_tokens"])
	switch target {
	case "chat_completions":
		result := map[string]any{"prompt_tokens": input, "completion_tokens": output, "total_tokens": input + output}
		if _, exists := usage["cached_tokens"]; exists {
			result["prompt_tokens_details"] = map[string]any{"cached_tokens": cached}
		}
		if _, exists := usage["reasoning_tokens"]; exists {
			result["completion_tokens_details"] = map[string]any{"reasoning_tokens": reasoning}
		}
		return result
	case "responses":
		result := map[string]any{"input_tokens": input, "output_tokens": output, "total_tokens": input + output}
		if _, exists := usage["cached_tokens"]; exists {
			result["input_tokens_details"] = map[string]any{"cached_tokens": cached}
		}
		if _, exists := usage["reasoning_tokens"]; exists {
			result["output_tokens_details"] = map[string]any{"reasoning_tokens": reasoning}
		}
		return result
	default:
		uncached := input - cached - created
		if uncached < 0 {
			uncached = 0
		}
		result := map[string]any{"input_tokens": uncached, "output_tokens": output}
		if _, exists := usage["cached_tokens"]; exists {
			result["cache_read_input_tokens"] = cached
		}
		if _, exists := usage["cache_creation_tokens"]; exists {
			result["cache_creation_input_tokens"] = created
		}
		if _, exists := usage["reasoning_tokens"]; exists {
			result["output_tokens_details"] = map[string]any{"thinking_tokens": reasoning}
		}
		return result
	}
}

func setAdapterUsage(target map[string]any, usage map[string]any, protocol string) {
	if converted := usageForTarget(usage, protocol); converted != nil {
		target["usage"] = converted
	}
}

func convertTools(tools []any, target string) ([]any, error) {
	result := make([]any, 0, len(tools))
	for _, raw := range tools {
		tool := mapValue(raw)
		if tool == nil {
			return nil, fmt.Errorf("%w: invalid tool definition", errUnsupportedProtocolConversion)
		}
		function := mapValue(tool["function"])
		name, description, schema := stringValue(tool["name"]), stringValue(tool["description"]), tool["parameters"]
		if function != nil {
			name, description, schema = stringValue(function["name"]), stringValue(function["description"]), function["parameters"]
		}
		typeName := stringValue(tool["type"])
		if function == nil && typeName != "" && typeName != "function" && name == "" {
			// Server-side/built-in tools have no portable function schema. The
			// mature adapters drop only that unsupported tool while preserving
			// the rest of the request; rejecting the entire request would also
			// disable ordinary client-side tools that can be converted safely.
			continue
		}
		if name == "" {
			return nil, fmt.Errorf("%w: function tool name is required", errUnsupportedProtocolConversion)
		}
		if schema == nil {
			schema = tool["input_schema"]
		}
		switch target {
		case "chat_completions":
			result = append(result, map[string]any{"type": "function", "function": map[string]any{"name": name, "description": description, "parameters": nonNilObject(schema)}})
		case "responses":
			result = append(result, map[string]any{"type": "function", "name": name, "description": description, "parameters": nonNilObject(schema)})
		case "messages":
			result = append(result, map[string]any{"name": name, "description": description, "input_schema": nonNilObject(schema)})
		}
	}
	return result, nil
}

func convertToolChoice(raw any, target string) any {
	if value, ok := raw.(string); ok {
		switch target {
		case "messages":
			if value == "none" {
				return nil
			}
			if value == "required" {
				return map[string]any{"type": "any"}
			}
			return map[string]any{"type": value}
		default:
			if value == "any" {
				return "required"
			}
			return value
		}
	}
	choice := mapValue(raw)
	typeName, name := stringValue(choice["type"]), stringValue(choice["name"])
	if function := mapValue(choice["function"]); function != nil {
		name = stringValue(function["name"])
	}
	if typeName == "any" || typeName == "required" {
		if target == "messages" {
			return map[string]any{"type": "any"}
		}
		return "required"
	}
	if typeName == "auto" || typeName == "none" {
		return convertToolChoice(typeName, target)
	}
	if typeName == "tool" || typeName == "function" {
		switch target {
		case "chat_completions":
			return map[string]any{"type": "function", "function": map[string]any{"name": name}}
		case "responses":
			return map[string]any{"type": "function", "name": name}
		case "messages":
			return map[string]any{"type": "tool", "name": name}
		}
	}
	return nil
}

func copyOptionalSampling(payload map[string]any, request adapterRequest, target string) {
	if request.Temperature != nil {
		payload["temperature"] = request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = request.TopP
	}
	if request.Stop != nil {
		switch target {
		case "messages":
			if value, ok := request.Stop.(string); ok {
				payload["stop_sequences"] = []any{value}
			} else {
				payload["stop_sequences"] = request.Stop
			}
		case "chat_completions":
			payload["stop"] = request.Stop
		}
	}
}

func copyRequestExtras(target map[string]any, source map[string]any, keys ...string) {
	for _, key := range keys {
		if value, exists := source[key]; exists && value != nil {
			target[key] = value
		}
	}
}

func copyReasoningConfig(target map[string]any, source map[string]any, protocol string) {
	effort := source["reasoning_effort"]
	if reasoning := mapValue(source["reasoning"]); reasoning != nil {
		if effort == nil {
			effort = reasoning["effort"]
		}
		if protocol == "responses" {
			target["reasoning"] = cloneMap(reasoning)
			return
		}
	}
	if outputConfig := mapValue(source["output_config"]); outputConfig != nil && effort == nil {
		effort = outputConfig["effort"]
	}
	switch protocol {
	case "chat_completions":
		if effort != nil {
			target["reasoning_effort"] = effort
		}
	case "responses":
		if effort != nil {
			target["reasoning"] = map[string]any{"effort": effort}
		}
	case "messages":
		if thinking := mapValue(source["thinking"]); thinking != nil {
			target["thinking"] = cloneMap(thinking)
		}
		outputConfig := mapValue(source["output_config"])
		if outputConfig != nil {
			outputConfig = cloneMap(outputConfig)
		} else if effort != nil {
			outputConfig = map[string]any{}
		}
		if outputConfig != nil {
			if effort != nil {
				outputConfig["effort"] = effort
			}
			target["output_config"] = outputConfig
		}
	}
}

func chatResponseFormatToResponses(raw any) any {
	format := mapValue(raw)
	if format == nil {
		return nil
	}
	if stringValue(format["type"]) != "json_schema" {
		return cloneMap(format)
	}
	schema := mapValue(format["json_schema"])
	if schema == nil {
		return nil
	}
	result := cloneMap(schema)
	result["type"] = "json_schema"
	return result
}

func responsesTextToChat(raw any) any {
	text := mapValue(raw)
	format := mapValue(text["format"])
	if format == nil {
		return nil
	}
	if stringValue(format["type"]) != "json_schema" {
		return cloneMap(format)
	}
	result := map[string]any{"type": "json_schema", "json_schema": map[string]any{}}
	schema := mapValue(result["json_schema"])
	for key, value := range format {
		if key != "type" {
			schema[key] = value
		}
	}
	return result
}

func openAIOutputToAnthropic(extra map[string]any) any {
	if responseFormat := mapValue(extra["response_format"]); responseFormat != nil {
		if stringValue(responseFormat["type"]) != "json_schema" {
			return nil
		}
		jsonSchema := mapValue(responseFormat["json_schema"])
		if jsonSchema == nil || mapValue(jsonSchema["schema"]) == nil {
			return nil
		}
		return map[string]any{"type": "json_schema", "schema": jsonSchema["schema"]}
	}
	text := mapValue(extra["text"])
	format := mapValue(text["format"])
	if format == nil || stringValue(format["type"]) != "json_schema" || mapValue(format["schema"]) == nil {
		return nil
	}
	return map[string]any{"type": "json_schema", "schema": format["schema"]}
}

func anthropicOutputToChat(raw any) any {
	format := anthropicOutputFormat(raw)
	if format == nil {
		return nil
	}
	return map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "novro_response", "schema": format["schema"]}}
}

func anthropicOutputToResponses(raw any) any {
	format := anthropicOutputFormat(raw)
	if format == nil {
		return nil
	}
	return map[string]any{"type": "json_schema", "name": "novro_response", "schema": format["schema"]}
}

func anthropicOutputFormat(raw any) map[string]any {
	outputConfig := mapValue(raw)
	format := mapValue(outputConfig["format"])
	if format == nil || stringValue(format["type"]) != "json_schema" || mapValue(format["schema"]) == nil {
		return nil
	}
	return format
}

func usageInt(values map[string]any, key string) (int, bool) {
	value, exists := values[key]
	return intValue(value), exists
}

func textFromParts(parts []adapterPart) string {
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type == "text" && part.Text != "" {
			values = append(values, part.Text)
		}
	}
	return strings.Join(values, "")
}

func decodeAdapterImage(value string) adapterPart {
	part := adapterPart{Type: "image", ImageURL: value}
	if !strings.HasPrefix(strings.ToLower(value), "data:") {
		return part
	}
	header, data, ok := strings.Cut(value, ",")
	if !ok || !strings.HasSuffix(strings.ToLower(header), ";base64") {
		return part
	}
	part.ImageURL = ""
	part.MediaType = strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	part.Data = data
	return part
}

func adapterImageURL(part adapterPart) string {
	if part.Data != "" {
		return "data:" + defaultMediaType(part.MediaType) + ";base64," + part.Data
	}
	return part.ImageURL
}

func defaultMediaType(value string) string {
	if strings.TrimSpace(value) == "" {
		return "image/png"
	}
	return strings.TrimSpace(value)
}

func encodeChatContent(parts []adapterPart) any {
	content := make([]any, 0)
	for _, part := range parts {
		switch part.Type {
		case "text":
			content = append(content, map[string]any{"type": "text", "text": part.Text})
		case "image":
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": adapterImageURL(part)}})
		}
	}
	if len(content) == 0 {
		return nil
	}
	if len(content) == 1 {
		if first := mapValue(content[0]); stringValue(first["type"]) == "text" {
			return first["text"]
		}
	}
	return content
}

func decodeChatToolCalls(values []any) []adapterPart {
	result := make([]adapterPart, 0, len(values))
	for _, raw := range values {
		call := mapValue(raw)
		function := mapValue(call["function"])
		result = append(result, adapterPart{Type: "tool_call", ID: stringValue(call["id"]), Name: stringValue(function["name"]), Arguments: decodeJSONValue(function["arguments"])})
	}
	return result
}

func encodeChatToolCalls(calls []adapterPart) []any {
	result := make([]any, 0, len(calls))
	for _, call := range calls {
		arguments, _ := json.Marshal(nonNilObject(call.Arguments))
		result = append(result, map[string]any{"id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": string(arguments)}})
	}
	return result
}

func cloneMap(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func sliceValue(value any) []any {
	result, _ := value.([]any)
	return result
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func firstPositiveInt(values ...any) int {
	for _, value := range values {
		if parsed := intValue(value); parsed > 0 {
			return parsed
		}
	}
	return 0
}

func positiveOrDefault(value int) int {
	if value > 0 {
		return value
	}
	return defaultMaxOutputTokens
}

func textFromContent(raw any) string {
	if text, ok := raw.(string); ok {
		return text
	}
	parts := make([]string, 0)
	for _, rawPart := range sliceValue(raw) {
		part := mapValue(rawPart)
		if text := stringValue(part["text"]); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "")
}

func decodeJSONValue(raw any) any {
	text, ok := raw.(string)
	if !ok {
		return nonNilObject(raw)
	}
	if strings.TrimSpace(text) == "" {
		return map[string]any{}
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if decoder.Decode(&value) == nil {
		return value
	}
	return map[string]any{"_raw": text}
}

func nonNilObject(value any) any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func nonEmptyStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if text := stringValue(value); text != "" {
			return text
		}
	}
	return ""
}

func onlyToolResults(parts []adapterPart) bool {
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if part.Type != "tool_result" {
			return false
		}
	}
	return true
}

func normalizeRole(role string) string {
	if role == "assistant" {
		return role
	}
	return "user"
}

func responseStopReason(root map[string]any) string {
	if status := stringValue(root["status"]); status == "incomplete" {
		if reason := stringValue(mapValue(root["incomplete_details"])["reason"]); reason != "" {
			return reason
		}
		return "max_output_tokens"
	}
	return "stop"
}

func chatStopReason(reason string) string {
	switch reason {
	case "max_tokens", "max_output_tokens", "model_context_window_exceeded", "length":
		return "length"
	case "tool_use", "tool_calls":
		return "tool_calls"
	case "content_filter", "refusal":
		return "content_filter"
	default:
		return "stop"
	}
}

func anthropicStopReason(reason string) string {
	switch reason {
	case "length", "max_tokens", "max_output_tokens":
		return "max_tokens"
	case "model_context_window_exceeded":
		return "model_context_window_exceeded"
	case "tool_calls", "tool_use":
		return "tool_use"
	case "content_filter", "refusal":
		return "refusal"
	case "stop_sequence", "pause_turn", "compaction":
		return reason
	default:
		return "end_turn"
	}
}

func safeID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "novro"
	}
	return strings.NewReplacer("resp_", "", "msg_", "", "chatcmpl-", "").Replace(value)
}
