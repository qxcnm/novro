package gateway

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// These tests describe the cross-protocol behavior shared by the mature
// adapters used as compatibility references.  They intentionally exercise the
// public wire payloads through adaptProtocolRequest/adaptProtocolResponse so
// the adapter's internal representation can evolve without coupling the
// contract to one particular set of Go structs.

func TestExtendedProtocolContractDocumentsAndAudio(t *testing.T) {
	t.Run("messages base64 document becomes responses input_file", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64,
			"messages": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "document", "source": map[string]any{
					"type": "base64", "media_type": "application/pdf", "data": "ZmFrZS1wZGY=",
				}},
				map[string]any{"type": "text", "text": "summarize it"},
			}}},
		}, "messages", "responses")

		message := contractFindItem(t, contractSlice(t, got["input"], "input"), "type", "message")
		content := contractSlice(t, message["content"], "input message content")
		file := contractFindItem(t, content, "type", "input_file")
		if file["file_data"] != "ZmFrZS1wZGY=" {
			t.Fatalf("input_file.file_data = %#v, want base64 document payload; body=%s", file["file_data"], contractJSON(got))
		}
		if !contractContentHasText(content, "summarize it") {
			t.Fatalf("adjacent document prompt was lost; body=%s", contractJSON(got))
		}
	})

	t.Run("responses URL file becomes messages document", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_output_tokens": 64,
			"input": []any{map[string]any{"type": "message", "role": "user", "content": []any{
				map[string]any{"type": "input_file", "file_url": "https://example.test/guide.pdf", "filename": "guide.pdf"},
				map[string]any{"type": "input_text", "text": "summarize it"},
			}}},
		}, "responses", "messages")

		message := contractMap(t, contractSlice(t, got["messages"], "messages")[0], "messages[0]")
		content := contractSlice(t, message["content"], "messages[0].content")
		document := contractFindItem(t, content, "type", "document")
		source := contractMap(t, document["source"], "document.source")
		if source["type"] != "url" || source["url"] != "https://example.test/guide.pdf" {
			t.Fatalf("document source = %#v, want URL source; body=%s", source, contractJSON(got))
		}
	})

	t.Run("responses file id becomes chat file", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_output_tokens": 64,
			"input": []any{map[string]any{"type": "message", "role": "user", "content": []any{
				map[string]any{"type": "input_file", "file_id": "file_123"},
				map[string]any{"type": "input_text", "text": "continue safely"},
			}}},
		}, "responses", "chat_completions")

		messages := contractSlice(t, got["messages"], "messages")
		if !strings.Contains(contractJSON(messages), "continue safely") {
			t.Fatalf("representable text was dropped with the file; body=%s", contractJSON(got))
		}
		message := contractMap(t, messages[0], "messages[0]")
		filePart := contractFindItem(t, contractSlice(t, message["content"], "messages[0].content"), "type", "file")
		file := contractMap(t, filePart["file"], "file part")
		if file["file_id"] != "file_123" {
			t.Fatalf("Chat file_id = %#v, want file_123; body=%s", file["file_id"], contractJSON(got))
		}
	})

	t.Run("chat inline file becomes responses input file", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64,
			"messages": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "file", "file": map[string]any{"file_data": "ZmFrZQ==", "filename": "note.txt"}},
			}}},
		}, "chat_completions", "responses")

		message := contractFindItem(t, contractSlice(t, got["input"], "input"), "type", "message")
		file := contractFindItem(t, contractSlice(t, message["content"], "input content"), "type", "input_file")
		if file["file_data"] != "ZmFrZQ==" || file["filename"] != "note.txt" {
			t.Fatalf("Responses inline file = %#v", file)
		}
	})

	t.Run("chat file id becomes messages document losslessly when possible", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64,
			"messages": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "file", "file": map[string]any{"file_data": "ZmFrZQ==", "filename": "note.txt"}},
			}}},
		}, "chat_completions", "messages")

		message := contractMap(t, contractSlice(t, got["messages"], "messages")[0], "messages[0]")
		document := contractFindItem(t, contractSlice(t, message["content"], "messages content"), "type", "document")
		source := contractMap(t, document["source"], "document.source")
		if source["type"] != "base64" || source["data"] != "ZmFrZQ==" {
			t.Fatalf("Messages document = %#v", document)
		}
	})

	t.Run("unrepresentable chat audio is omitted without rejecting the turn", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64,
			"messages": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "UklGRg==", "format": "wav"}},
				map[string]any{"type": "text", "text": "transcribe if supported"},
			}}},
		}, "chat_completions", "messages")

		encoded := contractJSON(got)
		if !strings.Contains(encoded, "transcribe if supported") {
			t.Fatalf("representable text was dropped with audio; body=%s", encoded)
		}
		if strings.Contains(encoded, "UklGRg==") {
			t.Fatalf("audio bytes were emitted in a target block that cannot express audio; body=%s", encoded)
		}
	})

	t.Run("chat audio becomes responses input file", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64,
			"messages": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "UklGRg==", "format": "wav"}},
			}}},
		}, "chat_completions", "responses")
		message := contractFindItem(t, contractSlice(t, got["input"], "input"), "type", "message")
		file := contractFindItem(t, contractSlice(t, message["content"], "input content"), "type", "input_file")
		if file["file_data"] != "UklGRg==" {
			t.Fatalf("Responses audio file = %#v", file)
		}
	})
}

func TestExtendedProtocolContractRefusalResponses(t *testing.T) {
	t.Run("responses refusal stays typed in chat", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "resp_refusal", "object": "response", "created_at": 1700000000,
			"model": "source", "status": "incomplete",
			"incomplete_details": map[string]any{"reason": "content_filter"},
			"output": []any{map[string]any{
				"id": "msg_refusal", "type": "message", "role": "assistant", "status": "completed",
				"content": []any{map[string]any{"type": "refusal", "refusal": "I cannot help with that."}},
			}},
		}, "responses", "chat_completions")

		choice := contractMap(t, contractSlice(t, got["choices"], "choices")[0], "choices[0]")
		message := contractMap(t, choice["message"], "choices[0].message")
		if message["refusal"] != "I cannot help with that." {
			t.Fatalf("chat refusal = %#v, want typed refusal; body=%s", message["refusal"], contractJSON(got))
		}
		if choice["finish_reason"] != "content_filter" {
			t.Fatalf("finish_reason = %#v, want content_filter", choice["finish_reason"])
		}
	})

	t.Run("chat refusal becomes responses refusal part", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "chatcmpl-refusal", "object": "chat.completion", "created": 1700000000, "model": "source",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "content_filter",
				"message": map[string]any{"role": "assistant", "content": nil, "refusal": "I cannot help with that."},
			}},
		}, "chat_completions", "responses")

		message := contractFindItem(t, contractSlice(t, got["output"], "output"), "type", "message")
		refusal := contractFindItem(t, contractSlice(t, message["content"], "message.content"), "type", "refusal")
		if refusal["refusal"] != "I cannot help with that." {
			t.Fatalf("responses refusal = %#v; body=%s", refusal, contractJSON(got))
		}
		if got["status"] != "incomplete" || contractMap(t, got["incomplete_details"], "incomplete_details")["reason"] != "content_filter" {
			t.Fatalf("refusal status/details not preserved; body=%s", contractJSON(got))
		}
	})
}

func TestExtendedProtocolContractCitationsAndAnnotations(t *testing.T) {
	t.Run("messages web citation becomes chat annotation", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "msg_cite", "type": "message", "role": "assistant", "model": "source",
			"content": []any{map[string]any{
				"type": "text", "text": "The source confirms it.",
				"citations": []any{map[string]any{
					"type": "web_search_result", "url": "https://example.test/source", "title": "Source", "cited_text": "confirms it",
				}},
			}},
			"stop_reason": "end_turn", "stop_sequence": nil,
		}, "messages", "chat_completions")

		choice := contractMap(t, contractSlice(t, got["choices"], "choices")[0], "choices[0]")
		message := contractMap(t, choice["message"], "message")
		annotations := contractSlice(t, message["annotations"], "message.annotations")
		annotation := contractMap(t, annotations[0], "annotations[0]")
		if annotation["type"] != "url_citation" || annotation["url"] != "https://example.test/source" || annotation["title"] != "Source" {
			t.Fatalf("chat annotation = %#v, want URL citation; body=%s", annotation, contractJSON(got))
		}
	})

	t.Run("responses annotations survive in messages citations", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "resp_cite", "object": "response", "created_at": 1700000000, "model": "source", "status": "completed",
			"output": []any{map[string]any{
				"id": "msg_cite", "type": "message", "role": "assistant", "status": "completed",
				"content": []any{map[string]any{
					"type": "output_text", "text": "The source confirms it.",
					"annotations": []any{map[string]any{
						"type": "url_citation", "url": "https://example.test/source", "title": "Source", "start_index": 4, "end_index": 10,
					}},
				}},
			}},
		}, "responses", "messages")

		text := contractFindItem(t, contractSlice(t, got["content"], "content"), "type", "text")
		citations := contractSlice(t, text["citations"], "text.citations")
		citation := contractMap(t, citations[0], "citations[0]")
		if citation["type"] != "web_search_result" || citation["url"] != "https://example.test/source" || citation["title"] != "Source" {
			t.Fatalf("messages citation = %#v, want Web Search citation; body=%s", citation, contractJSON(got))
		}
	})

	t.Run("chat annotations survive in responses output_text", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "chatcmpl-cite", "object": "chat.completion", "created": 1700000000, "model": "source",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{
					"role": "assistant", "content": "The source confirms it.",
					"annotations": []any{map[string]any{
						"type": "url_citation", "url": "https://example.test/source", "title": "Source", "start_index": 4, "end_index": 10,
					}},
				},
			}},
		}, "chat_completions", "responses")

		message := contractFindItem(t, contractSlice(t, got["output"], "output"), "type", "message")
		text := contractFindItem(t, contractSlice(t, message["content"], "message.content"), "type", "output_text")
		annotations := contractSlice(t, text["annotations"], "output_text.annotations")
		annotation := contractMap(t, annotations[0], "annotations[0]")
		if annotation["url"] != "https://example.test/source" || annotation["start_index"] == nil || annotation["end_index"] == nil {
			t.Fatalf("responses annotations not preserved; body=%s", contractJSON(got))
		}
	})
}

func TestExtendedProtocolContractRedactedThinkingIsSafe(t *testing.T) {
	t.Run("request history omits opaque thinking but keeps tools", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64,
			"messages": []any{
				map[string]any{"role": "assistant", "content": []any{
					map[string]any{"type": "redacted_thinking", "data": "opaque-secret-state"},
					map[string]any{"type": "tool_use", "id": "call_1", "name": "lookup", "input": map[string]any{"q": "weather"}},
				}},
				map[string]any{"role": "user", "content": []any{
					map[string]any{"type": "tool_result", "tool_use_id": "call_1", "content": "sunny"},
				}},
			},
		}, "messages", "responses")

		encoded := contractJSON(got)
		if strings.Contains(encoded, "opaque-secret-state") {
			t.Fatalf("redacted thinking must not be exposed to a protocol without a safe typed representation; body=%s", encoded)
		}
		for _, want := range []string{"call_1", "lookup", "sunny"} {
			if !strings.Contains(encoded, want) {
				t.Fatalf("representable tool history %q was dropped with redacted thinking; body=%s", want, encoded)
			}
		}
	})

	t.Run("response omits opaque thinking but keeps visible text", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "msg_redacted", "type": "message", "role": "assistant", "model": "source",
			"content": []any{
				map[string]any{"type": "redacted_thinking", "data": "opaque-secret-state"},
				map[string]any{"type": "text", "text": "Visible answer"},
			},
			"stop_reason": "end_turn", "stop_sequence": nil,
		}, "messages", "chat_completions")

		encoded := contractJSON(got)
		if strings.Contains(encoded, "opaque-secret-state") || !strings.Contains(encoded, "Visible answer") {
			t.Fatalf("unsafe redacted-thinking degradation; body=%s", encoded)
		}
	})
}

func TestExtendedProtocolContractRichToolResults(t *testing.T) {
	t.Run("messages rich error result degrades to text for responses", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64,
			"messages": []any{
				map[string]any{"role": "assistant", "content": []any{
					map[string]any{"type": "tool_use", "id": "call_err", "name": "inspect", "input": map[string]any{}},
				}},
				map[string]any{"role": "user", "content": []any{
					map[string]any{
						"type": "tool_result", "tool_use_id": "call_err", "is_error": true,
						"content": []any{
							map[string]any{"type": "text", "text": "permission denied"},
							map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "aW1hZ2U="}},
						},
					},
				}},
			},
		}, "messages", "responses")

		output := contractFindItem(t, contractSlice(t, got["input"], "input"), "type", "function_call_output")
		if output["call_id"] != "call_err" || !strings.Contains(contractJSON(output["output"]), "permission denied") {
			t.Fatalf("tool result text/call ID not preserved; body=%s", contractJSON(got))
		}
	})

	t.Run("messages rich error result stays structured when target supports it", func(t *testing.T) {
		// A same-protocol call is a byte-level passthrough contract.  This case
		// protects is_error and mixed tool-result blocks while the cross-protocol
		// paths use best-effort degradation.
		payload := map[string]any{
			"model": "source", "max_tokens": 64,
			"messages": []any{map[string]any{"role": "user", "content": []any{
				map[string]any{
					"type": "tool_result", "tool_use_id": "call_err", "is_error": true,
					"content": []any{
						map[string]any{"type": "text", "text": "permission denied"},
						map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "aW1hZ2U="}},
					},
				},
			}}},
		}
		got := contractAdaptRequest(t, payload, "messages", "messages")
		if !strings.Contains(contractJSON(got), `"is_error":true`) || !strings.Contains(contractJSON(got), "aW1hZ2U=") {
			t.Fatalf("same-protocol rich tool result changed; body=%s", contractJSON(got))
		}
	})
}

func TestExtendedProtocolContractToolDefinitionsAndChoice(t *testing.T) {
	functionSchema := map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}}

	t.Run("chat strict function remains strict in responses", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64,
			"messages": []any{map[string]any{"role": "user", "content": "search"}},
			"tools": []any{map[string]any{"type": "function", "function": map[string]any{
				"name": "lookup", "description": "Lookup", "parameters": functionSchema, "strict": true,
			}}},
		}, "chat_completions", "responses")

		tool := contractFindItem(t, contractSlice(t, got["tools"], "tools"), "name", "lookup")
		if tool["strict"] != true {
			t.Fatalf("responses function strict = %#v, want true; body=%s", tool["strict"], contractJSON(got))
		}
	})

	t.Run("responses strict function remains strict in chat", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_output_tokens": 64, "input": "search",
			"tools": []any{map[string]any{"type": "function", "name": "lookup", "description": "Lookup", "parameters": functionSchema, "strict": true}},
		}, "responses", "chat_completions")

		tool := contractMap(t, contractSlice(t, got["tools"], "tools")[0], "tools[0]")
		function := contractMap(t, tool["function"], "tools[0].function")
		if function["strict"] != true {
			t.Fatalf("chat function strict = %#v, want true; body=%s", function["strict"], contractJSON(got))
		}
	})

	t.Run("chat parallel false inverts into messages tool choice", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64,
			"messages":    []any{map[string]any{"role": "user", "content": "search"}},
			"tools":       []any{map[string]any{"type": "function", "function": map[string]any{"name": "lookup", "parameters": functionSchema}}},
			"tool_choice": "auto", "parallel_tool_calls": false,
		}, "chat_completions", "messages")

		choice := contractMap(t, got["tool_choice"], "tool_choice")
		if choice["type"] != "auto" || choice["disable_parallel_tool_use"] != true {
			t.Fatalf("messages tool_choice = %#v, want auto with disable_parallel_tool_use=true", choice)
		}
	})

	t.Run("messages parallel disable inverts into responses", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64,
			"messages":    []any{map[string]any{"role": "user", "content": "search"}},
			"tools":       []any{map[string]any{"name": "lookup", "input_schema": functionSchema}},
			"tool_choice": map[string]any{"type": "auto", "disable_parallel_tool_use": true},
		}, "messages", "responses")

		if got["parallel_tool_calls"] != false {
			t.Fatalf("responses parallel_tool_calls = %#v, want false; body=%s", got["parallel_tool_calls"], contractJSON(got))
		}
	})

	t.Run("responses single allowed tool degrades to named messages choice", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_output_tokens": 64, "input": "search",
			"tools": []any{
				map[string]any{"type": "function", "name": "lookup", "parameters": functionSchema},
				map[string]any{"type": "function", "name": "other", "parameters": functionSchema},
			},
			"tool_choice": map[string]any{
				"type": "allowed_tools", "mode": "required",
				"tools": []any{map[string]any{"type": "function", "name": "lookup"}},
			},
		}, "responses", "messages")

		choice := contractMap(t, got["tool_choice"], "tool_choice")
		if choice["type"] != "tool" || choice["name"] != "lookup" {
			t.Fatalf("messages allowed-tool degradation = %#v, want named lookup choice", choice)
		}
	})

	t.Run("allowed tools drops unnamed function selector", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_output_tokens": 64, "input": "search",
			"tools": []any{map[string]any{"type": "function", "name": "lookup", "parameters": functionSchema}},
			"tool_choice": map[string]any{
				"type": "allowed_tools", "mode": "required",
				"tools": []any{map[string]any{"type": "function"}},
			},
		}, "responses", "chat_completions")
		if got["tool_choice"] != "auto" {
			t.Fatalf("invalid empty allowlist choice = %#v, want auto; body=%s", got["tool_choice"], contractJSON(got))
		}
	})

	t.Run("allowed tools filters dropped builtins before degrading", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_output_tokens": 64, "input": "search",
			"tools": []any{
				map[string]any{"type": "web_search"},
				map[string]any{"type": "function", "name": "lookup", "parameters": functionSchema},
			},
			"tool_choice": map[string]any{
				"type": "allowed_tools", "mode": "required",
				"tools": []any{
					map[string]any{"type": "web_search"},
					map[string]any{"type": "function", "name": "lookup"},
				},
			},
		}, "responses", "chat_completions")
		choice := contractMap(t, got["tool_choice"], "tool_choice")
		function := contractMap(t, choice["function"], "tool_choice.function")
		if choice["type"] != "function" || function["name"] != "lookup" {
			t.Fatalf("filtered Chat choice = %#v, want lookup; body=%s", choice, contractJSON(got))
		}
	})
}

func TestExtendedProtocolContractWebSearch(t *testing.T) {
	t.Run("responses web search tool becomes anthropic server tool", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_output_tokens": 64, "input": "search",
			"tools": []any{map[string]any{
				"type":    "web_search_preview",
				"filters": map[string]any{"allowed_domains": []any{"example.test"}},
			}},
			"tool_choice": map[string]any{"type": "web_search_preview"},
		}, "responses", "messages")

		tool := contractMap(t, contractSlice(t, got["tools"], "tools")[0], "tools[0]")
		if tool["type"] != "web_search_20250305" {
			t.Fatalf("messages web search type = %#v; body=%s", tool["type"], contractJSON(got))
		}
		if !reflect.DeepEqual(tool["allowed_domains"], []any{"example.test"}) {
			t.Fatalf("allowed_domains = %#v, want example.test; body=%s", tool["allowed_domains"], contractJSON(got))
		}
	})

	t.Run("anthropic web search tool becomes responses built-in", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64,
			"messages": []any{map[string]any{"role": "user", "content": "search"}},
			"tools": []any{map[string]any{
				"type": "web_search_20250305", "name": "web_search", "max_uses": 3,
				"allowed_domains": []any{"example.test"},
			}},
			"tool_choice": map[string]any{"type": "tool", "name": "web_search"},
		}, "messages", "responses")

		tool := contractMap(t, contractSlice(t, got["tools"], "tools")[0], "tools[0]")
		if tool["type"] != "web_search" {
			t.Fatalf("responses Web Search type = %#v; body=%s", tool["type"], contractJSON(got))
		}
		filters := contractMap(t, tool["filters"], "tools[0].filters")
		if !reflect.DeepEqual(filters["allowed_domains"], []any{"example.test"}) {
			t.Fatalf("responses Web Search filters = %#v", filters)
		}
	})

	t.Run("responses web_search_call expands to messages server use and result", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "resp_search", "object": "response", "created_at": 1700000000, "model": "source", "status": "completed",
			"output": []any{map[string]any{
				"id": "ws_1", "type": "web_search_call", "status": "completed",
				"action": map[string]any{
					"type": "search", "query": "novro protocol adapter",
					"sources": []any{map[string]any{"title": "Novro", "url": "https://example.test/novro"}},
				},
			}},
		}, "responses", "messages")

		content := contractSlice(t, got["content"], "content")
		serverUse := contractFindItem(t, content, "type", "server_tool_use")
		if serverUse["id"] != "ws_1" || serverUse["name"] != "web_search" {
			t.Fatalf("server_tool_use = %#v; body=%s", serverUse, contractJSON(got))
		}
		input := contractMap(t, serverUse["input"], "server_tool_use.input")
		if input["query"] != "novro protocol adapter" {
			t.Fatalf("server tool query = %#v", input)
		}
		result := contractFindItem(t, content, "type", "web_search_tool_result")
		if result["tool_use_id"] != "ws_1" || !strings.Contains(contractJSON(result["content"]), "https://example.test/novro") {
			t.Fatalf("web_search_tool_result = %#v; body=%s", result, contractJSON(got))
		}
	})

	t.Run("failed responses web search becomes typed messages error", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "resp_search_failed", "object": "response", "created_at": 1700000000, "model": "source", "status": "completed",
			"output": []any{map[string]any{
				"id": "ws_failed", "type": "web_search_call", "status": "failed",
				"action": map[string]any{"type": "search", "query": "novro"},
			}},
		}, "responses", "messages")

		content := contractSlice(t, got["content"], "content")
		result := contractFindItem(t, content, "type", "web_search_tool_result")
		errorPayload := contractMap(t, result["content"], "web_search_tool_result.content")
		if errorPayload["type"] != "web_search_tool_result_error" || errorPayload["error_code"] == nil {
			t.Fatalf("typed Web Search failure missing; body=%s", contractJSON(got))
		}
	})
}

func TestExtendedProtocolContractRequestFieldsAndStopSemantics(t *testing.T) {
	t.Run("messages stop sequences are omitted for responses", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64, "stop_sequences": []any{"END"},
			"messages": []any{map[string]any{"role": "user", "content": "hello"}},
		}, "messages", "responses")
		if _, exists := got["stop"]; exists {
			t.Fatalf("Responses request contains unsupported stop: %s", contractJSON(got))
		}
	})

	t.Run("chat stop is omitted for responses", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64, "stop": "END",
			"messages": []any{map[string]any{"role": "user", "content": "hello"}},
		}, "chat_completions", "responses")
		if _, exists := got["stop"]; exists {
			t.Fatalf("Responses request contains unsupported stop: %s", contractJSON(got))
		}
	})
	t.Run("messages stop sequence maps to chat", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64, "stop_sequences": []any{"END", "DONE"},
			"messages": []any{map[string]any{"role": "user", "content": "hello"}},
		}, "messages", "chat_completions")

		if !reflect.DeepEqual(got["stop"], []any{"END", "DONE"}) {
			t.Fatalf("chat stop = %#v, want both stop sequences; body=%s", got["stop"], contractJSON(got))
		}
	})

	t.Run("chat stop maps to messages stop_sequences", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64, "stop": "END",
			"messages": []any{map[string]any{"role": "user", "content": "hello"}},
		}, "chat_completions", "messages")

		if !reflect.DeepEqual(got["stop_sequences"], []any{"END"}) {
			t.Fatalf("messages stop_sequences = %#v, want [END]; body=%s", got["stop_sequences"], contractJSON(got))
		}
	})

	t.Run("responses user field maps to chat user", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_output_tokens": 64, "input": "hello", "user": "user_123",
		}, "responses", "chat_completions")

		if got["user"] != "user_123" {
			t.Fatalf("chat user = %#v, want user_123; body=%s", got["user"], contractJSON(got))
		}
	})

	t.Run("messages top_k is safely omitted for chat", func(t *testing.T) {
		got := contractAdaptRequest(t, map[string]any{
			"model": "source", "max_tokens": 64, "top_k": 40,
			"messages": []any{map[string]any{"role": "user", "content": "hello"}},
		}, "messages", "chat_completions")

		if _, exists := got["top_k"]; exists {
			t.Fatalf("target Chat does not define top_k; body=%s", contractJSON(got))
		}
		if !strings.Contains(contractJSON(got["messages"]), "hello") {
			t.Fatalf("dropping top_k also dropped the request message; body=%s", contractJSON(got))
		}
	})

	t.Run("responses incomplete context limit stays distinct in messages", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "resp_context", "object": "response", "created_at": 1700000000, "model": "source",
			"status": "incomplete", "incomplete_details": map[string]any{"reason": "model_context_window_exceeded"},
			"output": []any{map[string]any{
				"id": "msg_context", "type": "message", "role": "assistant", "status": "incomplete",
				"content": []any{map[string]any{"type": "output_text", "text": "partial", "annotations": []any{}}},
			}},
		}, "responses", "messages")

		if got["stop_reason"] != "model_context_window_exceeded" {
			t.Fatalf("messages stop_reason = %#v, want model_context_window_exceeded; body=%s", got["stop_reason"], contractJSON(got))
		}
	})

	t.Run("chat length becomes responses incomplete details", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "chatcmpl_length", "object": "chat.completion", "created": 1700000000, "model": "source",
			"choices": []any{map[string]any{
				"index": 0, "finish_reason": "length", "message": map[string]any{"role": "assistant", "content": "partial"},
			}},
		}, "chat_completions", "responses")

		if got["status"] != "incomplete" || contractMap(t, got["incomplete_details"], "incomplete_details")["reason"] != "max_output_tokens" {
			t.Fatalf("responses incomplete semantics lost; body=%s", contractJSON(got))
		}
	})
}

func TestExtendedProtocolContractOrderedResponseParts(t *testing.T) {
	t.Run("responses items keep their order in messages", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "resp_order", "object": "response", "created_at": 1700000000, "model": "source", "status": "completed",
			"output": []any{
				map[string]any{"id": "ws_1", "type": "web_search_call", "status": "completed", "action": map[string]any{"type": "search", "query": "novro"}},
				map[string]any{"id": "msg_1", "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": "found", "annotations": []any{}}}},
				map[string]any{"id": "fc_1", "type": "function_call", "call_id": "call_1", "name": "save", "arguments": `{"value":"found"}`, "status": "completed"},
			},
		}, "responses", "messages")

		content := contractSlice(t, got["content"], "content")
		want := []string{"server_tool_use", "web_search_tool_result", "text", "tool_use"}
		if len(content) != len(want) {
			t.Fatalf("ordered Messages content length = %d, want %d; body=%s", len(content), len(want), contractJSON(got))
		}
		for index, typeName := range want {
			if gotType := stringValue(contractMap(t, content[index], "content item")["type"]); gotType != typeName {
				t.Fatalf("content[%d].type = %q, want %q; body=%s", index, gotType, typeName, contractJSON(got))
			}
		}
	})

	t.Run("messages tool result becomes responses function output", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "msg_tool_result", "type": "message", "role": "assistant", "model": "source",
			"content":     []any{map[string]any{"type": "tool_result", "tool_use_id": "call_1", "content": "saved"}},
			"stop_reason": "end_turn", "stop_sequence": nil,
		}, "messages", "responses")

		item := contractFindItem(t, contractSlice(t, got["output"], "output"), "type", "function_call_output")
		if item["call_id"] != "call_1" || item["output"] != "saved" {
			t.Fatalf("function_call_output = %#v; body=%s", item, contractJSON(got))
		}
	})

	t.Run("responses function output is retained in the response IR", func(t *testing.T) {
		body := contractMustMarshal(t, map[string]any{
			"id": "resp_tool_result", "object": "response", "created_at": 1700000000, "model": "source", "status": "completed",
			"output": []any{map[string]any{"id": "fco_1", "type": "function_call_output", "call_id": "call_1", "output": "saved", "status": "completed"}},
		})
		decoded, err := decodeAdapterResponse(body, "responses")
		if err != nil {
			t.Fatalf("decode Responses function output: %v", err)
		}
		if len(decoded.Parts) != 1 || decoded.Parts[0].Type != "tool_result" || decoded.Parts[0].ID != "call_1" || decoded.Parts[0].Text != "saved" {
			t.Fatalf("decoded function output = %#v", decoded.Parts)
		}
	})
}

func TestExtendedProtocolContractCitationUnionAndUsageTotals(t *testing.T) {
	t.Run("chat top level and content citations are merged once", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "chat_citations", "object": "chat.completion", "created": 1700000000, "model": "source",
			"choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{
				"role":        "assistant",
				"content":     []any{map[string]any{"type": "text", "text": "AB", "annotations": []any{map[string]any{"type": "url_citation", "url": "https://part.test", "start_index": 0, "end_index": 1}}}},
				"annotations": []any{map[string]any{"type": "url_citation", "url": "https://top.test", "start_index": 1, "end_index": 2}},
			}}},
		}, "chat_completions", "responses")

		message := contractFindItem(t, contractSlice(t, got["output"], "output"), "type", "message")
		text := contractFindItem(t, contractSlice(t, message["content"], "message.content"), "type", "output_text")
		annotations := contractSlice(t, text["annotations"], "annotations")
		if len(annotations) != 2 {
			t.Fatalf("merged annotations = %#v, want two unique citations; body=%s", annotations, contractJSON(got))
		}
	})

	t.Run("responses block local citation indexes are rebased for chat", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "resp_citations", "object": "response", "created_at": 1700000000, "model": "source", "status": "completed",
			"output": []any{
				map[string]any{
					"id": "msg_1", "type": "message", "role": "assistant",
					"content": []any{map[string]any{
						"type": "output_text", "text": "AB",
						"annotations": []any{map[string]any{"type": "url_citation", "url": "https://one.test", "start_index": 0, "end_index": 1}},
					}},
				},
				map[string]any{
					"id": "msg_2", "type": "message", "role": "assistant",
					"content": []any{map[string]any{
						"type": "output_text", "text": "CD",
						"annotations": []any{map[string]any{"type": "url_citation", "url": "https://two.test", "start_index": 0, "end_index": 1}},
					}},
				},
			},
		}, "responses", "chat_completions")

		choice := contractMap(t, contractSlice(t, got["choices"], "choices")[0], "choice")
		message := contractMap(t, choice["message"], "message")
		annotations := contractSlice(t, message["annotations"], "annotations")
		if len(annotations) != 2 || intValue(contractMap(t, annotations[1], "annotations[1]")["start_index"]) != 2 {
			t.Fatalf("rebased annotations = %#v; body=%s", annotations, contractJSON(got))
		}
	})

	t.Run("chat authoritative total tokens is preserved", func(t *testing.T) {
		got := contractAdaptResponse(t, map[string]any{
			"id": "chat_usage", "object": "chat.completion", "created": 1700000000, "model": "source",
			"choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "ok"}}},
			"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 17},
		}, "chat_completions", "responses")
		if total := intValue(contractMap(t, got["usage"], "usage")["total_tokens"]); total != 17 {
			t.Fatalf("Responses total_tokens = %d, want authoritative 17; body=%s", total, contractJSON(got))
		}
	})
}

func contractAdaptRequest(t *testing.T, payload map[string]any, source, target string) map[string]any {
	t.Helper()
	body, err := adaptProtocolRequest(payload, source, target, "target-model", false)
	if err != nil {
		t.Fatalf("adapt request %s -> %s: %v", source, target, err)
	}
	return contractDecode(t, body)
}

func contractAdaptResponse(t *testing.T, payload map[string]any, source, target string) map[string]any {
	t.Helper()
	body, err := adaptProtocolResponse(contractMustMarshal(t, payload), source, target)
	if err != nil {
		t.Fatalf("adapt response %s -> %s: %v", source, target, err)
	}
	return contractDecode(t, body)
}

func contractDecode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var result map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode adapter body %s: %v", body, err)
	}
	return result
}

func contractMustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal contract fixture: %v", err)
	}
	return body
}

func contractMap(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want object", path, value)
	}
	return result
}

func contractSlice(t *testing.T, value any, path string) []any {
	t.Helper()
	result, ok := value.([]any)
	if !ok || len(result) == 0 {
		t.Fatalf("%s = %#v, want non-empty array", path, value)
	}
	return result
}

func contractFindItem(t *testing.T, values []any, key, want string) map[string]any {
	t.Helper()
	for _, value := range values {
		item, ok := value.(map[string]any)
		if ok && item[key] == want {
			return item
		}
	}
	t.Fatalf("no item with %s=%q in %s", key, want, contractJSON(values))
	return nil
}

func contractContentHasText(content []any, want string) bool {
	for _, value := range content {
		part, ok := value.(map[string]any)
		if ok && (part["type"] == "text" || part["type"] == "input_text") && part["text"] == want {
			return true
		}
	}
	return false
}

func contractJSON(value any) string {
	body, _ := json.Marshal(value)
	return string(body)
}
