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
	Refusal   string
	ID        string
	Name      string
	Arguments any
	ImageURL  string
	MediaType string
	Data      string
	Detail    string
	FileID    string
	FileURL   string
	Filename  string
	Title     string
	Signature string
	Content   []adapterPart
	Citations []adapterCitation
	IsError   bool
	ErrorCode string
	Status    string
	Extra     map[string]any
}

// adapterCitation is the portable subset shared by Anthropic citations and
// OpenAI Chat/Responses annotations. Protocol-specific fields which cannot be
// represented by a peer protocol stay in Raw so same-semantic encoders can
// retain them without flattening the entire content block into text.
type adapterCitation struct {
	Type          string
	Title         string
	URL           string
	CitedText     string
	FileID        string
	Start         *int
	End           *int
	DocumentIndex *int
	Raw           map[string]any
}

type adapterMessage struct {
	Role  string
	Parts []adapterPart
}

type adapterRequest struct {
	Source      string
	Model       string
	System      string
	SystemParts []adapterPart
	Messages    []adapterMessage
	MaxTokens   int
	Stream      bool
	Temperature any
	TopP        any
	TopK        any
	Stop        any
	Tools       []any
	ToolChoice  any
	Parallel    *bool
	Extra       map[string]any
}

type adapterResponse struct {
	ID                 string
	Model              string
	Created            int64
	Text               string
	Refusal            string
	Reasoning          string
	ReasoningSignature string
	ToolCalls          []adapterPart
	Parts              []adapterPart
	SpecialParts       []adapterPart
	Citations          []adapterCitation
	Annotations        []any
	StopReason         string
	StopSequence       any
	IncompleteDetails  map[string]any
	Metadata           any
	ServiceTier        string
	SystemFingerprint  string
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
		Source: source, Model: stringValue(payload["model"]), Stream: boolValue(payload["stream"]),
		Temperature: payload["temperature"], TopP: payload["top_p"], TopK: payload["top_k"], Stop: payload["stop"],
		Tools: sliceValue(payload["tools"]), ToolChoice: payload["tool_choice"], Extra: map[string]any{},
	}
	if parallel, ok := payload["parallel_tool_calls"].(bool); ok {
		request.Parallel = &parallel
	}
	for _, key := range []string{
		"metadata", "reasoning_effort", "reasoning", "thinking", "output_config", "service_tier", "response_format", "text",
		"frequency_penalty", "presence_penalty", "seed", "user", "store", "truncation", "include", "background", "max_tool_calls",
		"web_search_options", "safety_identifier", "prompt_cache_key", "verbosity", "modalities", "audio",
	} {
		if value, exists := payload[key]; exists {
			request.Extra[key] = value
		}
	}
	var err error
	switch source {
	case "chat_completions":
		request.MaxTokens = firstPositiveInt(payload["max_completion_tokens"], payload["max_tokens"])
		request.Messages, request.System, err = decodeChatMessages(sliceValue(payload["messages"]))
		request.SystemParts = textParts(request.System)
	case "responses":
		request.MaxTokens = intValue(payload["max_output_tokens"])
		request.System = stringValue(payload["instructions"])
		request.SystemParts = textParts(request.System)
		var systemParts []adapterPart
		request.Messages, systemParts, err = decodeResponsesInput(payload["input"])
		request.SystemParts = append(request.SystemParts, systemParts...)
		request.System = textFromParts(request.SystemParts)
	case "messages":
		request.MaxTokens = intValue(payload["max_tokens"])
		request.Stop = payload["stop_sequences"]
		request.SystemParts, err = decodeAnthropicContent(payload["system"])
		if err != nil {
			return adapterRequest{}, err
		}
		request.System = textFromParts(request.SystemParts)
		var messagesErr error
		request.Messages, _, messagesErr = decodeAnthropicMessages(sliceValue(payload["messages"]))
		err = messagesErr
		if choice := mapValue(request.ToolChoice); choice != nil {
			if disabled, ok := choice["disable_parallel_tool_use"].(bool); ok {
				parallel := !disabled
				request.Parallel = &parallel
			}
		}
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
			tools, err := convertTools(request.Tools, request.Source, target)
			if err != nil {
				return nil, err
			}
			if len(tools) > 0 {
				payload["tools"] = tools
			}
		}
		if request.ToolChoice != nil && len(sliceValue(payload["tools"])) > 0 {
			if choice := sanitizeConvertedToolChoice(convertToolChoice(request.ToolChoice, target), sliceValue(payload["tools"]), target); choice != nil {
				payload["tool_choice"] = choice
			}
		}
		if request.Parallel != nil && len(sliceValue(payload["tools"])) > 0 {
			payload["parallel_tool_calls"] = *request.Parallel
		}
		if request.Stream {
			payload["stream_options"] = map[string]any{"include_usage": true}
		}
		copyRequestExtras(payload, request.Extra, "service_tier", "response_format", "frequency_penalty", "presence_penalty", "seed", "user", "store", "metadata", "modalities", "audio")
		if options := chatWebSearchOptions(request); options != nil {
			payload["web_search_options"] = options
		}
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
			tools, err := convertTools(request.Tools, request.Source, target)
			if err != nil {
				return nil, err
			}
			if len(tools) > 0 {
				payload["tools"] = tools
			}
		}
		if request.ToolChoice != nil && len(sliceValue(payload["tools"])) > 0 {
			if choice := sanitizeConvertedToolChoice(convertToolChoice(request.ToolChoice, target), sliceValue(payload["tools"]), target); choice != nil {
				payload["tool_choice"] = choice
			}
		}
		if request.Parallel != nil && len(sliceValue(payload["tools"])) > 0 {
			payload["parallel_tool_calls"] = *request.Parallel
		}
		copyRequestExtras(payload, request.Extra, "metadata", "service_tier", "text", "user", "store", "truncation", "include", "background", "max_tool_calls", "safety_identifier", "prompt_cache_key")
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
		if system := encodeAnthropicSystem(request.SystemParts); system != nil {
			payload["system"] = system
		}
		if len(request.Tools) > 0 {
			tools, err := convertTools(request.Tools, request.Source, target)
			if err != nil {
				return nil, err
			}
			if len(tools) > 0 {
				payload["tools"] = tools
			}
		}
		if len(sliceValue(payload["tools"])) > 0 {
			rawChoice := request.ToolChoice
			if rawChoice == nil && request.Parallel != nil {
				rawChoice = "auto"
			}
			if choice := sanitizeConvertedToolChoice(convertToolChoice(rawChoice, target), sliceValue(payload["tools"]), target); choice != nil {
				if choiceMap := mapValue(choice); choiceMap != nil && request.Parallel != nil {
					choiceMap = cloneMap(choiceMap)
					choiceMap["disable_parallel_tool_use"] = !*request.Parallel
					choice = choiceMap
				}
				payload["tool_choice"] = choice
			}
		}
		copyRequestExtras(payload, request.Extra, "metadata")
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
		delete(payload, "tool_choice")
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
			systems = append(systems, textFromParts(decodeCommonContent(message["content"])))
			continue
		}
		parts := decodeCommonContent(message["content"])
		if reasoning := firstNonEmptyString(message["reasoning_content"], message["reasoning"], message["reasoning_text"]); reasoning != "" {
			parts = append([]adapterPart{{Type: "reasoning", Text: reasoning}}, parts...)
		}
		if refusal := stringValue(message["refusal"]); refusal != "" {
			parts = append(parts, adapterPart{Type: "refusal", Refusal: refusal})
		}
		for _, rawCall := range sliceValue(message["tool_calls"]) {
			call := mapValue(rawCall)
			function := mapValue(call["function"])
			if function == nil {
				continue
			}
			parts = append(parts, adapterPart{Type: "tool_call", ID: stringValue(call["id"]), Name: stringValue(function["name"]), Arguments: decodeJSONValue(function["arguments"])})
		}
		if role == "tool" {
			content := decodeCommonContent(message["content"])
			parts = []adapterPart{{Type: "tool_result", ID: stringValue(message["tool_call_id"]), Text: textFromParts(content), Content: content, IsError: boolValue(message["is_error"])}}
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
		parts, err := decodeAnthropicContent(message["content"])
		if err != nil {
			return nil, "", fmt.Errorf("%w: invalid Anthropic message content: %v", errUnsupportedProtocolConversion, err)
		}
		role := stringValue(message["role"])
		if role == "user" && onlyToolResults(parts) {
			role = "tool"
		}
		messages = append(messages, adapterMessage{Role: role, Parts: parts})
	}
	return messages, "", nil
}

func decodeResponsesInput(raw any) ([]adapterMessage, []adapterPart, error) {
	if text, ok := raw.(string); ok {
		return []adapterMessage{{Role: "user", Parts: textParts(text)}}, nil, nil
	}
	messages := make([]adapterMessage, 0)
	systemParts := make([]adapterPart, 0, 2)
	for _, rawItem := range sliceValue(raw) {
		item := mapValue(rawItem)
		if item == nil {
			return nil, nil, fmt.Errorf("%w: invalid Responses input item", errUnsupportedProtocolConversion)
		}
		switch stringValue(item["type"]) {
		case "", "message":
			role := stringValue(item["role"])
			parts := decodeCommonContent(item["content"])
			if role == "system" || role == "developer" {
				systemParts = append(systemParts, parts...)
				continue
			}
			messages = append(messages, adapterMessage{Role: role, Parts: parts})
		case "function_call":
			messages = append(messages, adapterMessage{Role: "assistant", Parts: []adapterPart{{Type: "tool_call", ID: stringValue(item["call_id"]), Name: stringValue(item["name"]), Arguments: decodeJSONValue(item["arguments"])}}})
		case "function_call_output":
			content := decodeCommonContent(item["output"])
			messages = append(messages, adapterMessage{Role: "tool", Parts: []adapterPart{{Type: "tool_result", ID: stringValue(item["call_id"]), Text: textFromParts(content), Content: content}}})
		case "reasoning":
			var summary strings.Builder
			for _, rawPart := range sliceValue(item["summary"]) {
				summary.WriteString(stringValue(mapValue(rawPart)["text"]))
			}
			if summary.Len() > 0 {
				messages = append(messages, adapterMessage{Role: "assistant", Parts: []adapterPart{{Type: "reasoning", Text: summary.String()}}})
			}
		case "web_search_call":
			parts := responsesWebSearchParts(item)
			if len(parts) > 0 {
				messages = append(messages, adapterMessage{Role: "assistant", Parts: parts})
			}
		default:
			// Cross-protocol history references and future provider-side items do
			// not have a safe portable representation. Keep the rest of the turn
			// instead of rejecting the complete request.
			continue
		}
	}
	return messages, systemParts, nil
}

func decodeAnthropicContent(raw any) ([]adapterPart, error) {
	return decodePortableContent(raw, "messages"), nil
}

func decodeCommonContent(raw any) []adapterPart {
	return decodePortableContent(raw, "openai")
}

func decodePortableContent(raw any, source string) []adapterPart {
	if text, ok := raw.(string); ok {
		return textParts(text)
	}
	var values []any
	switch value := raw.(type) {
	case []any:
		values = value
	case map[string]any:
		values = []any{value}
	default:
		return nil
	}
	parts := make([]adapterPart, 0, len(values))
	for _, rawPart := range values {
		part := mapValue(rawPart)
		if part == nil {
			continue
		}
		typeName := stringValue(part["type"])
		if typeName == "" && stringValue(part["text"]) != "" {
			typeName = "text"
		}
		switch typeName {
		case "text", "input_text", "output_text":
			citationsRaw := part["citations"]
			if citationsRaw == nil {
				citationsRaw = part["annotations"]
			}
			parts = append(parts, adapterPart{Type: "text", Text: stringValue(part["text"]), Citations: decodeAdapterCitations(citationsRaw)})
		case "refusal":
			parts = append(parts, adapterPart{Type: "refusal", Refusal: firstNonEmptyString(part["refusal"], part["text"])})
		case "image_url":
			imageURL := stringValue(part["image_url"])
			detail := stringValue(part["detail"])
			if image := mapValue(part["image_url"]); image != nil {
				imageURL, detail = stringValue(image["url"]), stringValue(image["detail"])
			}
			image := decodeAdapterImage(imageURL)
			image.Detail = detail
			parts = append(parts, image)
		case "input_image", "image":
			if typeName == "image" && source == "messages" {
				image := adapterPart{Type: "image"}
				sourceValue := mapValue(part["source"])
				switch stringValue(sourceValue["type"]) {
				case "base64":
					image.MediaType, image.Data = stringValue(sourceValue["media_type"]), stringValue(sourceValue["data"])
				case "url":
					image.ImageURL = stringValue(sourceValue["url"])
				}
				parts = append(parts, image)
				continue
			}
			image := decodeAdapterImage(stringValue(part["image_url"]))
			image.FileID = stringValue(part["file_id"])
			image.Detail = stringValue(part["detail"])
			parts = append(parts, image)
		case "input_audio":
			audio := mapValue(part["input_audio"])
			if audio == nil {
				audio = part
			}
			parts = append(parts, adapterPart{Type: "audio", Data: stringValue(audio["data"]), MediaType: firstNonEmptyString(audio["format"], audio["media_type"])})
		case "input_file":
			document := adapterPart{Type: "document", FileID: stringValue(part["file_id"]), FileURL: stringValue(part["file_url"]), Data: stringValue(part["file_data"]), Filename: stringValue(part["filename"])}
			if strings.HasPrefix(strings.ToLower(document.Data), "data:") {
				decoded := decodeAdapterImage(document.Data)
				document.Data, document.MediaType = decoded.Data, decoded.MediaType
			}
			parts = append(parts, document)
		case "document":
			document := adapterPart{Type: "document", Filename: firstNonEmptyString(part["title"], part["filename"])}
			sourceValue := mapValue(part["source"])
			switch stringValue(sourceValue["type"]) {
			case "base64":
				document.MediaType, document.Data = stringValue(sourceValue["media_type"]), stringValue(sourceValue["data"])
			case "url":
				document.FileURL = stringValue(sourceValue["url"])
			}
			parts = append(parts, document)
		case "thinking":
			parts = append(parts, adapterPart{Type: "reasoning", Text: stringValue(part["thinking"]), Signature: stringValue(part["signature"])})
		case "redacted_thinking":
			parts = append(parts, adapterPart{Type: "redacted_thinking", Data: stringValue(part["data"])})
		case "tool_use", "tool_call":
			arguments := part["input"]
			if arguments == nil {
				arguments = decodeJSONValue(part["arguments"])
			}
			parts = append(parts, adapterPart{Type: "tool_call", ID: firstNonEmptyString(part["id"], part["call_id"]), Name: stringValue(part["name"]), Arguments: arguments})
		case "server_tool_use":
			parts = append(parts, adapterPart{Type: "server_tool_use", ID: stringValue(part["id"]), Name: firstNonEmptyString(part["name"], "web_search"), Arguments: nonNilObject(part["input"])})
		case "tool_result":
			content := decodePortableContent(part["content"], source)
			parts = append(parts, adapterPart{Type: "tool_result", ID: stringValue(part["tool_use_id"]), Text: textFromParts(content), Content: content, IsError: boolValue(part["is_error"])})
		case "web_search_tool_result":
			parts = append(parts, decodeWebSearchToolResult(part))
		case "web_search_result":
			parts = append(parts, adapterPart{Type: "web_search_result", Text: firstNonEmptyString(part["title"], part["url"]), ImageURL: stringValue(part["url"]), Extra: cloneMap(part)})
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
		remaining := make([]adapterPart, 0, len(message.Parts))
		for _, part := range message.Parts {
			if part.Type == "tool_result" {
				result = append(result, map[string]any{"role": "tool", "tool_call_id": part.ID, "content": toolResultText(part)})
				continue
			}
			remaining = append(remaining, part)
		}
		if len(remaining) == 0 {
			continue
		}
		entry := map[string]any{"role": normalizeRole(message.Role)}
		content := encodeChatContent(remaining)
		entry["content"] = content
		calls := make([]any, 0)
		for _, part := range remaining {
			if part.Type == "tool_call" {
				arguments, _ := json.Marshal(nonNilObject(part.Arguments))
				calls = append(calls, map[string]any{"id": part.ID, "type": "function", "function": map[string]any{"name": part.Name, "arguments": string(arguments)}})
			}
			if part.Type == "reasoning" {
				entry["reasoning_content"] = part.Text
			}
			if part.Type == "refusal" && part.Refusal != "" {
				entry["refusal"] = part.Refusal
			}
		}
		if len(calls) > 0 {
			entry["tool_calls"] = calls
		}
		result = append(result, entry)
	}
	return result
}

func encodeResponsesInput(messages []adapterMessage) []any {
	result := make([]any, 0, len(messages))
	for _, message := range messages {
		content := make([]any, 0)
		special := make([]any, 0)
		role := normalizeRole(message.Role)
		for _, part := range message.Parts {
			switch part.Type {
			case "text":
				textType := "input_text"
				if role == "assistant" {
					textType = "output_text"
				}
				content = append(content, map[string]any{"type": textType, "text": part.Text})
			case "refusal":
				if role == "assistant" && part.Refusal != "" {
					content = append(content, map[string]any{"type": "refusal", "refusal": part.Refusal})
				}
			case "reasoning":
				if part.Text != "" {
					special = append(special, map[string]any{"type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": part.Text}}})
				}
			case "image":
				if value := encodeResponsesImage(part); value != nil {
					content = append(content, value)
				}
			case "document":
				if value := encodeResponsesDocument(part); value != nil {
					content = append(content, value)
				}
			case "audio":
				if part.Data != "" {
					content = append(content, map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": part.Data, "format": part.MediaType}})
				}
			case "tool_call":
				arguments, _ := json.Marshal(nonNilObject(part.Arguments))
				special = append(special, map[string]any{"type": "function_call", "call_id": part.ID, "name": part.Name, "arguments": string(arguments)})
			case "tool_result":
				special = append(special, map[string]any{"type": "function_call_output", "call_id": part.ID, "output": toolResultText(part)})
			case "server_tool_use":
				if part.Name == "web_search" || part.Name == "" {
					special = append(special, encodeResponsesWebSearchHistory(part, findPartByTypeAndID(message.Parts, "web_search_tool_result", part.ID)))
				}
			}
		}
		if len(content) > 0 {
			result = append(result, map[string]any{"type": "message", "role": role, "content": content})
		}
		result = append(result, special...)
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
					block := map[string]any{"type": "text", "text": part.Text}
					if citations := encodeAnthropicCitations(part.Citations); len(citations) > 0 {
						block["citations"] = citations
					}
					content = append(content, block)
				}
			case "refusal":
				if part.Refusal != "" {
					content = append(content, map[string]any{"type": "text", "text": part.Refusal})
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
				if block := encodeAnthropicMediaBlock("image", part); block != nil {
					content = append(content, block)
				}
			case "document":
				if block := encodeAnthropicMediaBlock("document", part); block != nil {
					content = append(content, block)
				}
			case "tool_call":
				content = append(content, map[string]any{"type": "tool_use", "id": part.ID, "name": part.Name, "input": nonNilObject(part.Arguments)})
			case "tool_result":
				block := map[string]any{"type": "tool_result", "tool_use_id": part.ID, "content": encodeAnthropicToolResultContent(part)}
				if part.IsError {
					block["is_error"] = true
				}
				content = append(content, block)
			case "server_tool_use":
				content = append(content, map[string]any{"type": "server_tool_use", "id": part.ID, "name": firstNonEmptyString(part.Name, "web_search"), "input": nonNilObject(part.Arguments)})
			case "web_search_tool_result":
				content = append(content, encodeAnthropicWebSearchToolResult(part))
			case "redacted_thinking":
				// Opaque thinking is provider-bound state. Same-protocol requests
				// bypass this adapter byte-for-byte; cross-protocol paths must not
				// inject foreign opaque state into Anthropic history.
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
	result := adapterResponse{ID: stringValue(root["id"]), Model: stringValue(root["model"]), Created: int64(firstPositiveInt(root["created"], root["created_at"])), Usage: normalizeUsageMap(mapValue(root["usage"]), source), Metadata: root["metadata"], ServiceTier: stringValue(root["service_tier"]), SystemFingerprint: stringValue(root["system_fingerprint"])}
	if result.Created == 0 {
		result.Created = time.Now().Unix()
	}
	switch source {
	case "chat_completions":
		choices := sliceValue(root["choices"])
		if len(choices) > 0 {
			choice := mapValue(choices[0])
			message := mapValue(choice["message"])
			result.Parts = decodeCommonContent(message["content"])
			result.Text = textFromParts(result.Parts)
			result.Refusal = stringValue(message["refusal"])
			result.Reasoning = firstNonEmptyString(message["reasoning_content"], message["reasoning"], message["reasoning_text"])
			result.StopReason = stringValue(choice["finish_reason"])
			result.ToolCalls = decodeChatToolCalls(sliceValue(message["tool_calls"]))
			result.Annotations = sliceValue(message["annotations"])
			result.Citations = decodeAdapterCitations(result.Annotations)
			for _, part := range result.Parts {
				if len(part.Citations) > 0 {
					result.Citations = append(result.Citations, part.Citations...)
				}
			}
		}
	case "responses":
		result.StopReason = responseStopReason(root)
		if details := mapValue(root["incomplete_details"]); details != nil {
			result.IncompleteDetails = cloneMap(details)
		}
		for _, raw := range sliceValue(root["output"]) {
			item := mapValue(raw)
			switch stringValue(item["type"]) {
			case "message":
				for _, rawPart := range sliceValue(item["content"]) {
					part := mapValue(rawPart)
					switch stringValue(part["type"]) {
					case "output_text":
						decoded := adapterPart{Type: "text", Text: stringValue(part["text"]), Citations: decodeAdapterCitations(part["annotations"])}
						result.Parts = append(result.Parts, decoded)
						result.Text += decoded.Text
						result.Citations = append(result.Citations, decoded.Citations...)
						result.Annotations = append(result.Annotations, sliceValue(part["annotations"])...)
					case "refusal":
						result.Refusal += stringValue(part["refusal"])
						result.Parts = append(result.Parts, adapterPart{Type: "refusal", Refusal: stringValue(part["refusal"])})
					}
				}
			case "reasoning":
				var summary strings.Builder
				for _, rawPart := range sliceValue(item["summary"]) {
					text := stringValue(mapValue(rawPart)["text"])
					result.Reasoning += text
					summary.WriteString(text)
				}
				if summary.Len() > 0 {
					result.Parts = append(result.Parts, adapterPart{Type: "reasoning", Text: summary.String()})
				}
			case "function_call":
				call := adapterPart{Type: "tool_call", ID: stringValue(item["call_id"]), Name: stringValue(item["name"]), Arguments: decodeJSONValue(item["arguments"])}
				result.ToolCalls = append(result.ToolCalls, call)
				result.Parts = append(result.Parts, call)
			case "web_search_call":
				parts := responsesWebSearchParts(item)
				result.SpecialParts = append(result.SpecialParts, parts...)
				result.Parts = append(result.Parts, parts...)
			}
		}
	case "messages":
		result.StopReason = stringValue(root["stop_reason"])
		result.StopSequence = root["stop_sequence"]
		result.Parts, _ = decodeAnthropicContent(root["content"])
		for _, part := range result.Parts {
			switch part.Type {
			case "text":
				result.Text += part.Text
				result.Citations = append(result.Citations, part.Citations...)
			case "refusal":
				result.Refusal += part.Refusal
			case "reasoning":
				result.Reasoning += part.Text
				if result.ReasoningSignature == "" {
					result.ReasoningSignature = part.Signature
				}
			case "tool_call":
				result.ToolCalls = append(result.ToolCalls, part)
			case "server_tool_use", "web_search_tool_result":
				result.SpecialParts = append(result.SpecialParts, part)
			}
		}
		if result.StopReason == "refusal" && result.Refusal == "" && result.Text != "" {
			result.Refusal, result.Text = result.Text, ""
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
		message := map[string]any{"role": "assistant", "content": nil}
		if response.Text != "" {
			message["content"] = response.Text
		}
		if response.Refusal != "" {
			message["refusal"] = response.Refusal
		}
		if response.Reasoning != "" {
			message["reasoning_content"] = response.Reasoning
		}
		if annotations := responseOpenAIAnnotations(response); len(annotations) > 0 {
			message["annotations"] = annotations
		}
		if len(response.ToolCalls) > 0 {
			message["tool_calls"] = encodeChatToolCalls(response.ToolCalls)
		}
		finishReason := chatStopReason(response.StopReason)
		if response.Refusal != "" {
			finishReason = "content_filter"
		}
		result := map[string]any{"id": response.ID, "object": "chat.completion", "created": response.Created, "model": response.Model, "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finishReason}}}
		if response.ServiceTier != "" {
			result["service_tier"] = response.ServiceTier
		}
		if response.SystemFingerprint != "" {
			result["system_fingerprint"] = response.SystemFingerprint
		}
		setAdapterUsage(result, response.Usage, target)
		return result
	case "responses":
		output := make([]any, 0, 3)
		if response.Reasoning != "" {
			output = append(output, map[string]any{"id": "rs_" + safeID(response.ID), "type": "reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": response.Reasoning}}})
		}
		messageContent := make([]any, 0, 2)
		if response.Text != "" {
			messageContent = append(messageContent, map[string]any{"type": "output_text", "text": response.Text, "annotations": responseOpenAIAnnotations(response)})
		}
		if response.Refusal != "" {
			messageContent = append(messageContent, map[string]any{"type": "refusal", "refusal": response.Refusal})
		}
		if len(messageContent) > 0 {
			output = append(output, map[string]any{"id": "msg_" + safeID(response.ID), "type": "message", "status": "completed", "role": "assistant", "content": messageContent})
		}
		for _, part := range response.SpecialParts {
			if part.Type != "server_tool_use" {
				continue
			}
			resultPart := findPartByTypeAndID(response.SpecialParts, "web_search_tool_result", part.ID)
			output = append(output, encodeResponsesWebSearchHistory(part, resultPart))
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
		case "content_filter", "refusal":
			result["status"] = "incomplete"
			result["incomplete_details"] = map[string]any{"reason": "content_filter"}
		}
		if response.IncompleteDetails != nil {
			result["incomplete_details"] = cloneMap(response.IncompleteDetails)
			result["status"] = "incomplete"
		}
		if response.Metadata != nil {
			result["metadata"] = response.Metadata
		}
		if response.ServiceTier != "" {
			result["service_tier"] = response.ServiceTier
		}
		setAdapterUsage(result, response.Usage, target)
		return result
	case "messages":
		content := make([]any, 0, 3)
		if response.Reasoning != "" && response.ReasoningSignature != "" {
			content = append(content, map[string]any{"type": "thinking", "thinking": response.Reasoning, "signature": response.ReasoningSignature})
		}
		if response.Text != "" && !duplicatesUnsignedReasoning(response.Text, response.Reasoning, response.ReasoningSignature) {
			block := map[string]any{"type": "text", "text": response.Text}
			if citations := encodeAnthropicCitations(response.Citations); len(citations) > 0 {
				block["citations"] = citations
			}
			content = append(content, block)
		}
		if response.Refusal != "" {
			content = append(content, map[string]any{"type": "text", "text": response.Refusal})
		}
		for _, call := range response.ToolCalls {
			content = append(content, map[string]any{"type": "tool_use", "id": call.ID, "name": call.Name, "input": nonNilObject(call.Arguments)})
		}
		for _, part := range response.SpecialParts {
			switch part.Type {
			case "server_tool_use":
				content = append(content, map[string]any{"type": "server_tool_use", "id": part.ID, "name": firstNonEmptyString(part.Name, "web_search"), "input": nonNilObject(part.Arguments)})
			case "web_search_tool_result":
				content = append(content, encodeAnthropicWebSearchToolResult(part))
			}
		}
		stopSequence := response.StopSequence
		result := map[string]any{"id": response.ID, "type": "message", "role": "assistant", "model": response.Model, "content": content, "stop_reason": anthropicStopReason(response.StopReason), "stop_sequence": stopSequence}
		if response.Metadata != nil {
			result["metadata"] = response.Metadata
		}
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

func convertTools(tools []any, source, target string) ([]any, error) {
	result := make([]any, 0, len(tools))
	for _, raw := range tools {
		tool := mapValue(raw)
		if tool == nil {
			return nil, fmt.Errorf("%w: invalid tool definition", errUnsupportedProtocolConversion)
		}
		function := mapValue(tool["function"])
		name, description, schema := stringValue(tool["name"]), stringValue(tool["description"]), tool["parameters"]
		strict, hasStrict := tool["strict"]
		if function != nil {
			name, description, schema = stringValue(function["name"]), stringValue(function["description"]), function["parameters"]
			strict, hasStrict = function["strict"]
		}
		typeName := normalizePortableToolType(stringValue(tool["type"]), source)
		if schema == nil {
			schema = tool["input_schema"]
		}
		if typeName == "function" && name == "" {
			return nil, fmt.Errorf("%w: function tool name is required", errUnsupportedProtocolConversion)
		}
		if typeName != "function" && name == "" {
			name = typeName
		}
		switch target {
		case "chat_completions":
			if typeName != "function" {
				continue
			}
			encodedFunction := map[string]any{"name": name, "description": description, "parameters": nonNilObject(schema)}
			if hasStrict {
				encodedFunction["strict"] = strict
			}
			result = append(result, map[string]any{"type": "function", "function": encodedFunction})
		case "responses":
			if typeName == "function" {
				encoded := map[string]any{"type": "function", "name": name, "description": description, "parameters": nonNilObject(schema)}
				if hasStrict {
					encoded["strict"] = strict
				}
				result = append(result, encoded)
				continue
			}
			if !responsesSupportsToolType(typeName) {
				continue
			}
			encoded := copyToolExtraFields(tool, source)
			encoded["type"] = responsesToolType(typeName)
			delete(encoded, "name")
			delete(encoded, "description")
			delete(encoded, "parameters")
			delete(encoded, "input_schema")
			delete(encoded, "strict")
			if typeName == "web_search" {
				filters := mapValue(encoded["filters"])
				if filters == nil {
					filters = map[string]any{}
				} else {
					filters = cloneMap(filters)
				}
				if allowed := encoded["allowed_domains"]; allowed != nil {
					filters["allowed_domains"] = allowed
				}
				delete(encoded, "allowed_domains")
				delete(encoded, "blocked_domains")
				delete(encoded, "max_uses")
				if len(filters) > 0 {
					encoded["filters"] = filters
				}
			}
			result = append(result, encoded)
		case "messages":
			if typeName == "function" {
				result = append(result, map[string]any{"name": name, "description": description, "input_schema": nonNilObject(schema)})
				continue
			}
			encoded := copyToolExtraFields(tool, source)
			encoded["type"] = anthropicToolType(typeName)
			encoded["name"] = name
			delete(encoded, "description")
			delete(encoded, "parameters")
			delete(encoded, "input_schema")
			delete(encoded, "strict")
			if typeName == "web_search" {
				if filters := mapValue(encoded["filters"]); filters != nil && filters["allowed_domains"] != nil {
					encoded["allowed_domains"] = filters["allowed_domains"]
				}
				delete(encoded, "filters")
			}
			result = append(result, encoded)
		}
	}
	return result, nil
}

func normalizePortableToolType(raw, source string) string {
	switch raw {
	case "", "function", "custom":
		return "function"
	case "web_search", "web_search_preview", "web_search_preview_2025_03_11", "web_search_20250305":
		return "web_search"
	default:
		return raw
	}
}

func responsesToolType(typeName string) string {
	if typeName == "web_search" {
		return "web_search"
	}
	return typeName
}

func anthropicToolType(typeName string) string {
	if typeName == "web_search" {
		return "web_search_20250305"
	}
	if typeName == "function" {
		return "custom"
	}
	return typeName
}

func responsesSupportsToolType(typeName string) bool {
	switch responsesToolType(typeName) {
	case "web_search", "file_search", "code_interpreter", "computer_use", "mcp", "image_generation", "local_shell":
		return true
	default:
		return false
	}
}

func copyToolExtraFields(tool map[string]any, source string) map[string]any {
	result := cloneMap(tool)
	if source == "chat_completions" {
		if function := mapValue(tool["function"]); function != nil {
			result = cloneMap(function)
		}
	}
	delete(result, "function")
	delete(result, "type")
	return result
}

func convertToolChoice(raw any, target string) any {
	if raw == nil {
		return nil
	}
	if value, ok := raw.(string); ok {
		switch target {
		case "messages":
			if value == "required" {
				return map[string]any{"type": "any"}
			}
			if value == "web_search" || strings.HasPrefix(value, "web_search_") {
				return map[string]any{"type": "tool", "name": "web_search"}
			}
			return map[string]any{"type": value}
		default:
			if value == "any" {
				return "required"
			}
			if value == "web_search" || strings.HasPrefix(value, "web_search_") {
				if target == "responses" {
					return map[string]any{"type": "web_search"}
				}
				return "auto"
			}
			return value
		}
	}
	choice := mapValue(raw)
	if choice == nil {
		return nil
	}
	typeName, name := stringValue(choice["type"]), stringValue(choice["name"])
	if typeName == "allowed_tools" {
		mode := firstNonEmptyString(choice["mode"], "auto")
		selectors := sliceValue(choice["tools"])
		names := make([]string, 0, len(selectors))
		for _, rawSelector := range selectors {
			selector := mapValue(rawSelector)
			selectorType := normalizePortableToolType(stringValue(selector["type"]), "responses")
			selectorName := stringValue(selector["name"])
			if selectorName == "" && selectorType != "function" {
				selectorName = selectorType
			}
			if selectorName != "" {
				names = append(names, selectorName)
			}
		}
		switch target {
		case "messages":
			if len(names) == 1 {
				return map[string]any{"type": "tool", "name": names[0]}
			}
			if mode == "required" {
				return map[string]any{"type": "any"}
			}
			return map[string]any{"type": "auto"}
		case "chat_completions":
			if len(names) == 1 {
				return map[string]any{"type": "function", "function": map[string]any{"name": names[0]}}
			}
			if mode == "required" {
				return "required"
			}
			return "auto"
		case "responses":
			return cloneMap(choice)
		}
	}
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
		if name == "web_search" {
			switch target {
			case "responses":
				return map[string]any{"type": "web_search"}
			case "chat_completions":
				return "auto"
			}
		}
		switch target {
		case "chat_completions":
			return map[string]any{"type": "function", "function": map[string]any{"name": name}}
		case "responses":
			return map[string]any{"type": "function", "name": name}
		case "messages":
			return map[string]any{"type": "tool", "name": name}
		}
	}
	portableType := normalizePortableToolType(typeName, "responses")
	if portableType != "function" && portableType != "" {
		switch target {
		case "messages":
			return map[string]any{"type": "tool", "name": portableType}
		case "responses":
			return map[string]any{"type": responsesToolType(portableType)}
		case "chat_completions":
			return "auto"
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
	if target == "messages" && request.TopK != nil {
		payload["top_k"] = request.TopK
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

func textParts(text string) []adapterPart {
	if text == "" {
		return nil
	}
	return []adapterPart{{Type: "text", Text: text}}
}

func decodeAdapterCitations(raw any) []adapterCitation {
	values := sliceValue(raw)
	if len(values) == 0 {
		return nil
	}
	result := make([]adapterCitation, 0, len(values))
	for _, value := range values {
		citation := mapValue(value)
		if citation == nil {
			continue
		}
		decoded := adapterCitation{
			Type:      stringValue(citation["type"]),
			Title:     firstNonEmptyString(citation["title"], citation["document_title"], citation["filename"]),
			URL:       stringValue(citation["url"]),
			CitedText: stringValue(citation["cited_text"]),
			FileID:    firstNonEmptyString(citation["file_id"], citation["source_id"]),
			Raw:       cloneMap(citation),
		}
		decoded.Start = optionalInt(citation, "start_index", "start_char_index", "start")
		decoded.End = optionalInt(citation, "end_index", "end_char_index", "end")
		decoded.DocumentIndex = optionalInt(citation, "document_index")
		result = append(result, decoded)
	}
	return result
}

func responseOpenAIAnnotations(response adapterResponse) []any {
	result := make([]any, 0, len(response.Annotations)+len(response.Citations))
	if len(response.Annotations) > 0 {
		for _, raw := range response.Annotations {
			if annotation := openAIAnnotation(raw); annotation != nil {
				result = append(result, annotation)
			}
		}
		return result
	}
	for _, citation := range response.Citations {
		if annotation := openAIAnnotation(citation); annotation != nil {
			result = append(result, annotation)
		}
	}
	return result
}

func openAIAnnotation(raw any) map[string]any {
	var citation adapterCitation
	switch value := raw.(type) {
	case adapterCitation:
		citation = value
	case map[string]any:
		decoded := decodeAdapterCitations([]any{value})
		if len(decoded) == 0 {
			return nil
		}
		citation = decoded[0]
	default:
		return nil
	}
	result := map[string]any{}
	if citation.Raw != nil {
		result = cloneMap(citation.Raw)
	}
	if citation.URL != "" {
		result["type"] = "url_citation"
		result["url"] = citation.URL
		if citation.Title != "" {
			result["title"] = citation.Title
		}
	} else if citation.FileID != "" {
		result["type"] = "file_citation"
		result["file_id"] = citation.FileID
	} else if len(result) == 0 {
		return nil
	}
	delete(result, "document_title")
	delete(result, "document_index")
	delete(result, "start_char_index")
	delete(result, "end_char_index")
	delete(result, "cited_text")
	if citation.Start != nil {
		result["start_index"] = *citation.Start
	}
	if citation.End != nil {
		result["end_index"] = *citation.End
	}
	return result
}

func optionalInt(value map[string]any, keys ...string) *int {
	for _, key := range keys {
		if raw, exists := value[key]; exists && raw != nil {
			parsed := intValue(raw)
			return &parsed
		}
	}
	return nil
}

func responsesWebSearchParts(item map[string]any) []adapterPart {
	id := stringValue(item["id"])
	action := mapValue(item["action"])
	arguments := map[string]any{}
	if action != nil {
		if query := firstNonEmptyString(action["query"], firstStringValue(action["queries"]), action["url"], action["pattern"]); query != "" {
			arguments["query"] = query
		}
	}
	serverUse := adapterPart{Type: "server_tool_use", ID: id, Name: "web_search", Arguments: arguments, Status: stringValue(item["status"]), Extra: cloneMap(action)}
	result := adapterPart{Type: "web_search_tool_result", ID: id, Status: stringValue(item["status"])}
	if result.Status == "failed" {
		result.IsError = true
		result.ErrorCode = firstNonEmptyString(mapValue(item["error"])["code"], item["error_code"], "failed")
	}
	if action != nil {
		for _, rawSource := range sliceValue(action["sources"]) {
			source := mapValue(rawSource)
			url := stringValue(source["url"])
			if url == "" {
				continue
			}
			result.Content = append(result.Content, adapterPart{Type: "web_search_result", Text: firstNonEmptyString(source["title"], url), ImageURL: url, Extra: cloneMap(source)})
		}
	}
	return []adapterPart{serverUse, result}
}

func decodeWebSearchToolResult(part map[string]any) adapterPart {
	result := adapterPart{Type: "web_search_tool_result", ID: stringValue(part["tool_use_id"]), IsError: boolValue(part["is_error"]), ErrorCode: stringValue(part["error_code"])}
	if payload := mapValue(part["content"]); payload != nil {
		if stringValue(payload["type"]) == "web_search_tool_result_error" {
			result.IsError = true
			result.ErrorCode = firstNonEmptyString(payload["error_code"], result.ErrorCode, "failed")
		}
		return result
	}
	for _, raw := range sliceValue(part["content"]) {
		item := mapValue(raw)
		if item == nil {
			continue
		}
		if stringValue(item["type"]) == "web_search_tool_result_error" {
			result.IsError = true
			result.ErrorCode = firstNonEmptyString(item["error_code"], "failed")
			continue
		}
		if url := stringValue(item["url"]); url != "" {
			result.Content = append(result.Content, adapterPart{Type: "web_search_result", Text: firstNonEmptyString(item["title"], url), ImageURL: url, Extra: cloneMap(item)})
		}
	}
	return result
}

func findPartByTypeAndID(parts []adapterPart, typeName, id string) adapterPart {
	for _, part := range parts {
		if part.Type == typeName && (id == "" || part.ID == id) {
			return part
		}
	}
	return adapterPart{}
}

func toolResultText(part adapterPart) string {
	text := strings.TrimSpace(part.Text)
	if text == "" {
		text = strings.TrimSpace(textFromParts(part.Content))
	}
	if part.IsError && text == "" {
		return "Tool execution failed."
	}
	return text
}

func encodeResponsesImage(part adapterPart) any {
	result := map[string]any{"type": "input_image"}
	if part.FileID != "" {
		result["file_id"] = part.FileID
	} else if url := adapterImageURL(part); url != "" {
		result["image_url"] = url
	} else {
		return nil
	}
	if part.Detail != "" {
		result["detail"] = part.Detail
	}
	return result
}

func encodeResponsesDocument(part adapterPart) any {
	result := map[string]any{"type": "input_file"}
	switch {
	case part.FileID != "":
		result["file_id"] = part.FileID
	case part.FileURL != "":
		result["file_url"] = part.FileURL
	case part.Data != "":
		result["file_data"] = part.Data
	default:
		return nil
	}
	if part.Filename != "" {
		result["filename"] = part.Filename
	}
	return result
}

func encodeResponsesWebSearchHistory(serverUse, resultPart adapterPart) map[string]any {
	action := cloneMap(serverUse.Extra)
	if action == nil {
		action = map[string]any{}
	}
	if action["type"] == nil {
		action["type"] = "search"
	}
	arguments := mapValue(serverUse.Arguments)
	if action["query"] == nil && arguments != nil && stringValue(arguments["query"]) != "" {
		action["query"] = stringValue(arguments["query"])
	}
	if action["sources"] == nil && len(resultPart.Content) > 0 {
		sources := make([]any, 0, len(resultPart.Content))
		for _, part := range resultPart.Content {
			if part.ImageURL == "" {
				continue
			}
			sources = append(sources, map[string]any{"type": "url", "url": part.ImageURL, "title": firstNonEmptyString(part.Text, part.ImageURL)})
		}
		action["sources"] = sources
	}
	status := firstNonEmptyString(serverUse.Status, resultPart.Status, "completed")
	if resultPart.IsError {
		status = "failed"
	}
	return map[string]any{"id": serverUse.ID, "type": "web_search_call", "status": status, "action": action}
}

func encodeAnthropicSystem(parts []adapterPart) any {
	blocks := make([]any, 0, len(parts))
	for _, part := range parts {
		if part.Type == "text" && part.Text != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": part.Text})
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	if len(blocks) == 1 {
		return stringValue(mapValue(blocks[0])["text"])
	}
	return blocks
}

func encodeAnthropicMediaBlock(typeName string, part adapterPart) map[string]any {
	block := map[string]any{"type": typeName}
	switch {
	case part.Data != "":
		mediaType := strings.TrimSpace(part.MediaType)
		if mediaType == "" {
			if typeName == "document" {
				mediaType = "application/pdf"
			} else {
				mediaType = defaultMediaType(mediaType)
			}
		}
		block["source"] = map[string]any{"type": "base64", "media_type": mediaType, "data": part.Data}
	case typeName == "image" && part.ImageURL != "":
		block["source"] = map[string]any{"type": "url", "url": part.ImageURL}
	case typeName == "document" && part.FileURL != "":
		block["source"] = map[string]any{"type": "url", "url": part.FileURL}
	default:
		return nil
	}
	if typeName == "document" && part.Filename != "" {
		block["title"] = part.Filename
	}
	return block
}

func encodeAnthropicToolResultContent(part adapterPart) any {
	if len(part.Content) == 0 {
		return toolResultText(part)
	}
	if len(part.Content) == 1 && part.Content[0].Type == "text" {
		return part.Content[0].Text
	}
	blocks := make([]any, 0, len(part.Content))
	for _, nested := range part.Content {
		switch nested.Type {
		case "text":
			blocks = append(blocks, map[string]any{"type": "text", "text": nested.Text})
		case "image":
			if block := encodeAnthropicMediaBlock("image", nested); block != nil {
				blocks = append(blocks, block)
			}
		case "document":
			if block := encodeAnthropicMediaBlock("document", nested); block != nil {
				blocks = append(blocks, block)
			}
		}
	}
	if len(blocks) == 0 {
		return toolResultText(part)
	}
	return blocks
}

func encodeAnthropicWebSearchToolResult(part adapterPart) map[string]any {
	block := map[string]any{"type": "web_search_tool_result", "tool_use_id": part.ID}
	if part.IsError {
		block["content"] = map[string]any{"type": "web_search_tool_result_error", "error_code": firstNonEmptyString(part.ErrorCode, "failed")}
		return block
	}
	results := make([]any, 0, len(part.Content))
	for _, item := range part.Content {
		if item.ImageURL == "" {
			continue
		}
		results = append(results, map[string]any{"type": "web_search_result", "url": item.ImageURL, "title": firstNonEmptyString(item.Text, item.ImageURL)})
	}
	block["content"] = results
	return block
}

func encodeAnthropicCitations(citations []adapterCitation) []any {
	result := make([]any, 0, len(citations))
	for _, citation := range citations {
		wire := map[string]any{}
		if citation.URL != "" {
			wire["type"] = "web_search_result"
			wire["url"] = citation.URL
			if citation.Title != "" {
				wire["title"] = citation.Title
			}
			if citation.CitedText != "" {
				wire["cited_text"] = citation.CitedText
			}
		} else {
			wire["type"] = firstNonEmptyString(citation.Type, "char_location")
			if citation.Title != "" {
				wire["document_title"] = citation.Title
			}
			if citation.CitedText != "" {
				wire["cited_text"] = citation.CitedText
			}
			if citation.DocumentIndex != nil {
				wire["document_index"] = *citation.DocumentIndex
			}
			if citation.Start != nil {
				wire["start_char_index"] = *citation.Start
			}
			if citation.End != nil {
				wire["end_char_index"] = *citation.End
			}
		}
		result = append(result, wire)
	}
	return result
}

func chatWebSearchOptions(request adapterRequest) any {
	if options := mapValue(request.Extra["web_search_options"]); options != nil {
		return cloneMap(options)
	}
	for _, raw := range request.Tools {
		tool := mapValue(raw)
		typeName := normalizePortableToolType(stringValue(tool["type"]), request.Source)
		if typeName != "web_search" {
			continue
		}
		options := map[string]any{}
		for _, key := range []string{"search_context_size", "user_location"} {
			if value, exists := tool[key]; exists {
				options[key] = value
			}
		}
		return options
	}
	return nil
}

func sanitizeConvertedToolChoice(choice any, tools []any, target string) any {
	if choice == nil || len(tools) == 0 {
		return nil
	}
	names := make(map[string]struct{}, len(tools))
	types := make(map[string]struct{}, len(tools))
	for _, raw := range tools {
		tool := mapValue(raw)
		function := mapValue(tool["function"])
		name := stringValue(tool["name"])
		if function != nil {
			name = stringValue(function["name"])
		}
		if name != "" {
			names[name] = struct{}{}
		}
		if typeName := stringValue(tool["type"]); typeName != "" {
			types[typeName] = struct{}{}
		}
	}
	choiceMap := mapValue(choice)
	if choiceMap == nil {
		return choice
	}
	name := stringValue(choiceMap["name"])
	if function := mapValue(choiceMap["function"]); function != nil {
		name = stringValue(function["name"])
	}
	if stringValue(choiceMap["type"]) == "tool" {
		name = stringValue(choiceMap["name"])
	}
	if name != "" {
		if _, exists := names[name]; !exists {
			if target == "messages" {
				return map[string]any{"type": "auto"}
			}
			return "auto"
		}
	}
	return choice
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
			if url := adapterImageURL(part); url != "" {
				image := map[string]any{"url": url}
				if part.Detail != "" {
					image["detail"] = part.Detail
				}
				content = append(content, map[string]any{"type": "image_url", "image_url": image})
			}
		case "audio":
			if part.Data != "" {
				content = append(content, map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": part.Data, "format": part.MediaType}})
			}
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
