package gateway

import (
	"container/list"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	maxCachedResponseHistories     = 4096
	maxCachedResponseHistoryBytes  = 64 << 20
	maxSingleResponseHistoryBytes  = 8 << 20
	maxResponseHistoryChainLength  = 64
	responseHistoryRetentionPeriod = 24 * time.Hour
)

var errPreviousResponseUnavailable = errors.New("previous response is unavailable")

type responseHistoryKey struct {
	APIKeyID   uuid.UUID
	ResponseID string
}

type cachedResponseHistory struct {
	previousResponseID string
	input              []any
	output             []any
	retainedBytes      int
	expiresAt          time.Time
	element            *list.Element
}

// responseHistoryStore retains the minimum Responses input/output chain needed
// to expand previous_response_id for non-Responses upstreams. Entries are
// scoped to the authenticated API key and bounded by count, bytes, chain
// length, and time so one client cannot turn protocol compatibility into an
// unbounded prompt archive.
type responseHistoryStore struct {
	mu             sync.Mutex
	entries        map[responseHistoryKey]*cachedResponseHistory
	insertionOrder *list.List
	retainedBytes  int
}

func newResponseHistoryStore() *responseHistoryStore {
	return &responseHistoryStore{
		entries:        make(map[responseHistoryKey]*cachedResponseHistory),
		insertionOrder: list.New(),
	}
}

func (s *responseHistoryStore) expand(apiKeyID uuid.UUID, payload map[string]any, now time.Time) (map[string]any, error) {
	previousResponseID := strings.TrimSpace(stringValue(payload["previous_response_id"]))
	if previousResponseID == "" {
		return payload, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)

	currentResponseID := previousResponseID
	visited := make(map[string]struct{}, maxResponseHistoryChainLength)
	chain := make([]*cachedResponseHistory, 0, 4)
	for {
		if _, duplicate := visited[currentResponseID]; duplicate {
			return nil, fmt.Errorf("%w: response history contains a cycle at %s", errPreviousResponseUnavailable, currentResponseID)
		}
		visited[currentResponseID] = struct{}{}
		key := responseHistoryKey{APIKeyID: apiKeyID, ResponseID: currentResponseID}
		entry := s.entries[key]
		if entry == nil {
			return nil, fmt.Errorf("%w: %s", errPreviousResponseUnavailable, currentResponseID)
		}
		chain = append(chain, entry)
		if entry.previousResponseID == "" {
			break
		}
		if len(chain) >= maxResponseHistoryChainLength {
			return nil, fmt.Errorf("%w: response history exceeds %d turns", errPreviousResponseUnavailable, maxResponseHistoryChainLength)
		}
		currentResponseID = entry.previousResponseID
	}

	capacity := len(normalizeResponseInput(payload["input"]))
	for _, entry := range chain {
		capacity += len(entry.input) + len(entry.output)
	}
	expandedInput := make([]any, 0, capacity)
	for index := len(chain) - 1; index >= 0; index-- {
		expandedInput = append(expandedInput, cloneJSONValues(chain[index].input)...)
		expandedInput = append(expandedInput, cloneJSONValues(chain[index].output)...)
	}
	expandedInput = append(expandedInput, cloneJSONValues(normalizeResponseInput(payload["input"]))...)

	expanded := cloneMap(payload)
	delete(expanded, "previous_response_id")
	expanded["input"] = expandedInput
	return expanded, nil
}

func (s *responseHistoryStore) remember(apiKeyID uuid.UUID, payload, response map[string]any, now time.Time) error {
	if !strings.EqualFold(strings.TrimSpace(stringValue(response["status"])), "completed") {
		return nil
	}
	responseID := strings.TrimSpace(stringValue(response["id"]))
	if responseID == "" {
		return fmt.Errorf("completed Responses result has no id")
	}
	output, ok := response["output"].([]any)
	if !ok {
		return fmt.Errorf("completed Responses result %s has no output array", responseID)
	}
	input := normalizeResponseInput(payload["input"])
	previousResponseID := strings.TrimSpace(stringValue(payload["previous_response_id"]))
	input = cloneJSONValues(input)
	output = cloneJSONValues(output)
	serializedInput, inputErr := json.Marshal(input)
	serializedOutput, outputErr := json.Marshal(output)
	if inputErr != nil || outputErr != nil {
		return fmt.Errorf("serialize Responses history %s: input=%v output=%v", responseID, inputErr, outputErr)
	}
	retainedBytes := len(serializedInput) + len(serializedOutput) + len(responseID) + len(previousResponseID)
	if retainedBytes > maxSingleResponseHistoryBytes {
		return fmt.Errorf("Responses history %s requires %d bytes, exceeding %d", responseID, retainedBytes, maxSingleResponseHistoryBytes)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)
	key := responseHistoryKey{APIKeyID: apiKeyID, ResponseID: responseID}
	if existing := s.entries[key]; existing != nil {
		s.removeLocked(key, existing)
	}
	for len(s.entries) >= maxCachedResponseHistories || s.retainedBytes+retainedBytes > maxCachedResponseHistoryBytes {
		oldest := s.insertionOrder.Front()
		if oldest == nil {
			break
		}
		oldestKey := oldest.Value.(responseHistoryKey)
		if entry := s.entries[oldestKey]; entry != nil {
			s.removeLocked(oldestKey, entry)
		} else {
			s.insertionOrder.Remove(oldest)
		}
	}
	element := s.insertionOrder.PushBack(key)
	s.entries[key] = &cachedResponseHistory{
		previousResponseID: previousResponseID,
		input:              input,
		output:             output,
		retainedBytes:      retainedBytes,
		expiresAt:          now.Add(responseHistoryRetentionPeriod),
		element:            element,
	}
	s.retainedBytes += retainedBytes
	return nil
}

func (s *responseHistoryStore) pruneExpiredLocked(now time.Time) {
	for element := s.insertionOrder.Front(); element != nil; {
		next := element.Next()
		key := element.Value.(responseHistoryKey)
		entry := s.entries[key]
		if entry == nil {
			s.insertionOrder.Remove(element)
		} else if !entry.expiresAt.After(now) {
			s.removeLocked(key, entry)
		}
		element = next
	}
}

func (s *responseHistoryStore) removeLocked(key responseHistoryKey, entry *cachedResponseHistory) {
	delete(s.entries, key)
	s.retainedBytes -= entry.retainedBytes
	if s.retainedBytes < 0 {
		s.retainedBytes = 0
	}
	if entry.element != nil {
		s.insertionOrder.Remove(entry.element)
	}
}

func normalizeResponseInput(raw any) []any {
	switch value := raw.(type) {
	case nil:
		return nil
	case []any:
		return value
	case string:
		return []any{map[string]any{
			"type":    "message",
			"role":    "user",
			"content": []any{map[string]any{"type": "input_text", "text": value}},
		}}
	default:
		return []any{value}
	}
}

func cloneJSONValues(values []any) []any {
	if len(values) == 0 {
		return nil
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return append([]any(nil), values...)
	}
	var cloned []any
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	if decoder.Decode(&cloned) != nil {
		return append([]any(nil), values...)
	}
	return cloned
}

func responsesResultFromJSON(body []byte) (map[string]any, error) {
	var result map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("Responses result is not an object")
	}
	if response := mapValue(result["response"]); response != nil {
		return response, nil
	}
	return result, nil
}

func (h *Handler) rememberResponsesHistory(apiKeyID uuid.UUID, payload, response map[string]any, at time.Time) bool {
	if h.responseHistories == nil || response == nil {
		return false
	}
	if err := h.responseHistories.remember(apiKeyID, payload, response, at); err != nil {
		h.logger.Warn("cache Responses history", "response_id", stringValue(response["id"]), "error", err)
		return false
	}
	return strings.EqualFold(strings.TrimSpace(stringValue(response["status"])), "completed")
}
