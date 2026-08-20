package gateway

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// ConversionLostFieldsHeader follows the convention used by lmm-adapter-go.
// The public value is a count only; prompt text, field values, tool arguments,
// URLs, file data, and all other request/response content are excluded. Exact
// schema paths remain available in the server's structured warning log.
const ConversionLostFieldsHeader = "X-Conversion-Lost-Fields"

func requestConversionLostFields(payload map[string]any, source, target string) []string {
	if source == target || payload == nil {
		return nil
	}
	report := newConversionLossReport()

	// Top-level options that the current protocol-neutral request model cannot
	// preserve across protocol families.
	for _, field := range lossyRequestFields[source][target] {
		if value, exists := payload[field]; exists && value != nil {
			report.add(field)
		}
	}

	for _, rawTool := range sliceValue(payload["tools"]) {
		tool := mapValue(rawTool)
		if tool == nil {
			continue
		}
		typeName := stringValue(tool["type"])
		if source == "messages" && typeName == "" {
			typeName = "function"
		}
		portableType := normalizePortableToolType(typeName, source)
		unsupportedType := portableType != "" && ((target == "chat_completions" && portableType != "function" && portableType != "web_search") || (target == "responses" && portableType != "function" && !responsesSupportsToolType(portableType)))
		if unsupportedType {
			report.add("tools[].type")
		}
		if portableType == "web_search" && target == "chat_completions" {
			if filters := mapValue(tool["filters"]); filters != nil && len(filters) > 0 {
				report.add("tools[].filters")
			}
			for _, field := range []string{"allowed_domains", "blocked_domains", "max_uses"} {
				if value, exists := tool[field]; exists && value != nil {
					report.add("tools[]." + field)
				}
			}
		}
		if portableType == "web_search" && source == "messages" && target == "responses" {
			for _, field := range []string{"max_uses", "blocked_domains"} {
				if value, exists := tool[field]; exists && value != nil {
					report.add("tools[]." + field)
				}
			}
		}
		if portableType == "web_search" && source == "responses" && target == "messages" {
			if filters := mapValue(tool["filters"]); filters != nil {
				for key, value := range filters {
					if key != "allowed_domains" && value != nil {
						report.add("tools[].filters")
						break
					}
				}
			}
		}
		if target == "messages" {
			if function := mapValue(tool["function"]); function != nil {
				if _, exists := function["strict"]; exists {
					report.add("tools[].function.strict")
				}
			} else if _, exists := tool["strict"]; exists {
				report.add("tools[].strict")
			}
		}
	}

	switch source {
	case "chat_completions":
		inspectChatMessagesForLosses(report, sliceValue(payload["messages"]), target)
	case "responses":
		inspectResponsesInputForLosses(report, payload["input"], target)
	case "messages":
		inspectAnthropicMessagesForLosses(report, sliceValue(payload["messages"]), "messages[]", target)
		inspectAnthropicSystemForLosses(report, payload["system"])
	}
	return report.fields()
}

var lossyRequestFields = map[string]map[string][]string{
	"chat_completions": {
		"responses": {
			"audio", "frequency_penalty", "logit_bias", "logprobs", "modalities", "n",
			"prediction", "presence_penalty", "seed", "stop", "top_logprobs", "web_search_options",
		},
		"messages": {
			"audio", "frequency_penalty", "logit_bias", "logprobs", "modalities", "n",
			"prediction", "presence_penalty", "seed", "top_logprobs", "user", "web_search_options", "store",
		},
	},
	"responses": {
		"chat_completions": {
			"background", "conversation", "include", "max_tool_calls", "prompt", "prompt_cache_key",
			"safety_identifier", "truncation",
		},
		"messages": {
			"background", "conversation", "include", "max_tool_calls", "prompt", "prompt_cache_key",
			"safety_identifier", "store", "truncation", "user",
		},
	},
	"messages": {
		"chat_completions": {
			"container", "context_management", "mcp_servers", "top_k",
		},
		"responses": {
			"container", "context_management", "mcp_servers", "stop_sequences", "top_k",
		},
	},
}

func inspectChatMessagesForLosses(report *conversionLossReport, messages []any, target string) {
	for _, rawMessage := range messages {
		message := mapValue(rawMessage)
		if message == nil {
			continue
		}
		for _, field := range []string{"function_call", "name"} {
			if value, exists := message[field]; exists && value != nil {
				report.add("messages[]." + field)
			}
		}
		for _, rawPart := range sliceValue(message["content"]) {
			part := mapValue(rawPart)
			typeName := stringValue(part["type"])
			switch typeName {
			case "input_audio":
				if target == "messages" {
					report.add("messages[].content[]." + typeName)
				} else if target == "responses" {
					if audio := mapValue(part["input_audio"]); audio != nil && audio["format"] != nil {
						report.add("messages[].content[].input_audio.format")
					}
				}
			case "file":
				file := mapValue(part["file"])
				if file == nil || (stringValue(file["file_data"]) == "" && stringValue(file["file_id"]) == "" && stringValue(file["file_url"]) == "") {
					report.add("messages[].content[].file")
				}
			case "image_url":
				if image := mapValue(part["image_url"]); image != nil {
					if _, exists := image["detail"]; exists && target == "messages" {
						report.add("messages[].content[].image_url.detail")
					}
				}
			}
		}
	}
}

func inspectResponsesInputForLosses(report *conversionLossReport, input any, target string) {
	for _, rawItem := range sliceValue(input) {
		item := mapValue(rawItem)
		if item == nil {
			continue
		}
		typeName := stringValue(item["type"])
		switch typeName {
		case "item_reference", "computer_call", "computer_call_output", "file_search_call", "code_interpreter_call", "local_shell_call", "local_shell_call_output", "mcp_call", "mcp_approval_request", "mcp_approval_response":
			report.add("input[]." + typeName)
		case "web_search_call":
			if target == "chat_completions" {
				report.add("input[].web_search_call")
			}
		}
		for _, rawPart := range sliceValue(item["content"]) {
			part := mapValue(rawPart)
			typeName := stringValue(part["type"])
			switch typeName {
			case "input_file":
				if target == "chat_completions" {
					if stringValue(part["file_data"]) == "" && stringValue(part["file_id"]) == "" {
						report.add("input[].content[].input_file")
					}
				} else if target == "messages" && stringValue(part["file_id"]) != "" {
					report.add("input[].content[].input_file.file_id")
				}
			case "input_audio":
				if target == "messages" {
					report.add("input[].content[].input_audio")
				} else if target == "responses" {
					if audio := mapValue(part["input_audio"]); audio != nil && audio["format"] != nil {
						report.add("input[].content[].input_audio.format")
					}
				}
			}
			if target != "messages" {
				if _, exists := part["annotations"]; exists {
					report.add("input[].content[].annotations")
				}
			}
		}
	}
}

func inspectAnthropicSystemForLosses(report *conversionLossReport, content any) {
	for _, rawPart := range sliceValue(content) {
		part := mapValue(rawPart)
		if part == nil {
			continue
		}
		if typeName := stringValue(part["type"]); typeName != "" && typeName != "text" {
			report.add("system[].type")
		}
	}
}

func inspectAnthropicMessagesForLosses(report *conversionLossReport, messages []any, prefix, target string) {
	for _, rawMessage := range messages {
		message := mapValue(rawMessage)
		if message == nil {
			continue
		}
		inspectAnthropicContentForLosses(report, message["content"], prefix+".content[]", target)
	}
}

func inspectAnthropicContentForLosses(report *conversionLossReport, content any, prefix, target string) {
	for _, rawPart := range sliceValue(content) {
		part := mapValue(rawPart)
		typeName := stringValue(part["type"])
		switch typeName {
		case "document":
			if target == "chat_completions" {
				source := mapValue(part["source"])
				if stringValue(source["type"]) != "base64" || stringValue(source["data"]) == "" {
					report.add(prefix + ".document")
				}
			}
		case "redacted_thinking":
			report.add(prefix + ".redacted_thinking")
		case "server_tool_use", "web_search_tool_result":
			if target == "chat_completions" {
				report.add(prefix + "." + typeName)
			}
		case "tool_result":
			if target != "messages" && boolValue(part["is_error"]) {
				report.add(prefix + ".tool_result.is_error")
			}
			if target != "messages" {
				for _, rawNested := range sliceValue(part["content"]) {
					nested := mapValue(rawNested)
					if nested != nil && stringValue(nested["type"]) != "text" {
						report.add(prefix + ".tool_result.content[]")
						break
					}
				}
			}
		case "text":
			if target != "messages" {
				if _, exists := part["citations"]; exists {
					report.add(prefix + ".text.citations")
				}
			}
		}
	}
}

func responseConversionLostFields(body []byte, source, target string) []string {
	if source == target || len(body) == 0 {
		return nil
	}
	var payload map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil || payload == nil {
		return nil
	}
	report := newConversionLossReport()
	switch source {
	case "chat_completions":
		if target == "messages" && payload["service_tier"] != nil {
			report.add("service_tier")
		}
		if payload["system_fingerprint"] != nil {
			report.add("system_fingerprint")
		}
		choices := sliceValue(payload["choices"])
		if len(choices) > 1 {
			report.add("choices[]")
		}
		for _, rawChoice := range choices {
			choice := mapValue(rawChoice)
			if choice["logprobs"] != nil {
				report.add("choices[].logprobs")
			}
			message := mapValue(choice["message"])
			if value, exists := message["audio"]; exists && value != nil {
				report.add("choices[].message.audio")
			}
			if target == "messages" && firstNonEmptyString(message["reasoning_content"], message["reasoning"], message["reasoning_text"]) != "" {
				report.add("choices[].message.reasoning")
			}
			for _, rawPart := range sliceValue(message["content"]) {
				part := mapValue(rawPart)
				typeName := stringValue(part["type"])
				if typeName != "" && typeName != "text" && typeName != "output_text" && typeName != "refusal" {
					report.add("choices[].message.content[]." + typeName)
				}
			}
		}
		inspectOpenAIUsageForLosses(report, mapValue(payload["usage"]), "prompt_tokens", "completion_tokens", target)
	case "responses":
		if target == "chat_completions" {
			if details := mapValue(payload["incomplete_details"]); details != nil {
				switch stringValue(details["reason"]) {
				case "", "max_output_tokens", "max_tokens", "length", "content_filter", "refusal":
				default:
					report.add("incomplete_details")
				}
			}
			if payload["metadata"] != nil {
				report.add("metadata")
			}
		}
		if target == "messages" && payload["service_tier"] != nil {
			report.add("service_tier")
		}
		for _, rawItem := range sliceValue(payload["output"]) {
			item := mapValue(rawItem)
			typeName := stringValue(item["type"])
			if typeName == "function_call_output" {
				report.add("output[].function_call_output")
				continue
			}
			if target == "messages" && typeName == "reasoning" {
				report.add("output[].reasoning")
			}
			if target == "chat_completions" && typeName != "message" && typeName != "reasoning" && typeName != "function_call" && typeName != "" {
				switch typeName {
				case "web_search_call", "file_search_call", "computer_call", "code_interpreter_call", "local_shell_call", "mcp_call":
					report.add("output[]." + typeName)
				default:
					report.add("output[].type")
				}
			}
			if typeName == "message" {
				for _, rawPart := range sliceValue(item["content"]) {
					part := mapValue(rawPart)
					contentType := stringValue(part["type"])
					if contentType != "" && contentType != "output_text" && contentType != "text" && contentType != "refusal" {
						report.add("output[].message.content[]." + contentType)
					}
				}
			}
		}
		inspectOpenAIUsageForLosses(report, mapValue(payload["usage"]), "input_tokens", "output_tokens", target)
	case "messages":
		if target == "chat_completions" && payload["metadata"] != nil {
			report.add("metadata")
		}
		if payload["stop_sequence"] != nil {
			report.add("stop_sequence")
		}
		inspectAnthropicResponseContentForLosses(report, payload["content"], "content[]", target)
		usage := mapValue(payload["usage"])
		if value, exists := usage["server_tool_use_tokens"]; exists && value != nil {
			report.add("usage.server_tool_use_tokens")
		}
	}
	return report.fields()
}

func inspectAnthropicResponseContentForLosses(report *conversionLossReport, content any, prefix, target string) {
	for _, rawPart := range sliceValue(content) {
		part := mapValue(rawPart)
		if part == nil {
			continue
		}
		switch typeName := stringValue(part["type"]); typeName {
		case "image", "audio", "document":
			report.add(prefix + "." + typeName)
		case "thinking":
			if part["signature"] != nil {
				report.add(prefix + ".thinking.signature")
			}
		case "tool_result":
			if target == "chat_completions" {
				report.add(prefix + ".tool_result")
			}
		case "redacted_thinking":
			report.add(prefix + ".redacted_thinking")
		case "server_tool_use", "web_search_tool_result":
			if target == "chat_completions" {
				report.add(prefix + "." + typeName)
			}
		}
	}
}

func inspectOpenAIUsageForLosses(report *conversionLossReport, usage map[string]any, inputKey, outputKey, target string) {
	if usage == nil {
		return
	}
	if target == "messages" {
		total, hasTotal := usageInt(usage, "total_tokens")
		input, hasInput := usageInt(usage, inputKey)
		output, hasOutput := usageInt(usage, outputKey)
		if hasTotal && (!hasInput || !hasOutput || total != input+output) {
			report.add("usage.total_tokens")
		}
	}
	for _, detailsKey := range []string{"prompt_tokens_details", "input_tokens_details"} {
		for key, value := range mapValue(usage[detailsKey]) {
			if key != "cached_tokens" && value != nil {
				report.add("usage." + detailsKey + "." + key)
			}
		}
	}
	for _, detailsKey := range []string{"completion_tokens_details", "output_tokens_details"} {
		for key, value := range mapValue(usage[detailsKey]) {
			if key != "reasoning_tokens" && key != "thinking_tokens" && value != nil {
				report.add("usage." + detailsKey + "." + key)
			}
		}
	}
}

func setConversionLostFieldsHeader(w http.ResponseWriter, fields []string) {
	count := len(uniqueConversionLostFields(fields))
	if count == 0 {
		w.Header().Del(ConversionLostFieldsHeader)
		return
	}
	w.Header().Set(ConversionLostFieldsHeader, strconv.Itoa(count))
}

func mergeConversionLostFieldsHeader(w http.ResponseWriter, fields []string) {
	previous, _ := strconv.Atoi(strings.TrimSpace(w.Header().Get(ConversionLostFieldsHeader)))
	count := previous + len(uniqueConversionLostFields(fields))
	if count == 0 {
		w.Header().Del(ConversionLostFieldsHeader)
		return
	}
	w.Header().Set(ConversionLostFieldsHeader, strconv.Itoa(count))
}

func uniqueConversionLostFields(fields []string) []string {
	report := newConversionLossReport()
	for _, field := range fields {
		report.add(field)
	}
	return report.fields()
}

type conversionLossReport struct {
	set map[string]struct{}
}

func newConversionLossReport() *conversionLossReport {
	return &conversionLossReport{set: make(map[string]struct{})}
}

func (r *conversionLossReport) add(field string) {
	field = strings.TrimSpace(field)
	if field == "" || !safeConversionFieldPath(field) {
		return
	}
	r.set[field] = struct{}{}
}

func safeConversionFieldPath(field string) bool {
	for _, char := range field {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		switch char {
		case '.', '_', '-', '[', ']':
			continue
		default:
			return false
		}
	}
	return true
}

func (r *conversionLossReport) fields() []string {
	result := make([]string, 0, len(r.set))
	for field := range r.set {
		result = append(result, field)
	}
	sort.Strings(result)
	return result
}
