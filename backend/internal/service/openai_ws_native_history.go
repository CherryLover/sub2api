package service

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tidwall/gjson"
)

const (
	openAIWSNativeHistorySessionDefault = 0
	openAIWSNativeHistoryItemsDefault   = 8192
	// Reserve space for the item descriptors as well as their owned JSON bodies.
	openAIWSNativeHistoryItemOverhead = 64
)

// History belongs to one native ingress attempt. Only a complete, current
// request may cross accounts; unknown chains and exhausted budgets fail closed.
type openAIWSNativeHistory struct {
	used         *atomic.Int64
	scope        *openAIWSNativeReplayBudget
	maxSession   int64
	maxTotal     int64
	maxItems     int
	reserved     int64
	items        []json.RawMessage
	exists       bool
	complete     bool
	ready        bool
	lastResponse string
	reason       string
}

func (s *OpenAIGatewayService) newOpenAIWSNativeHistory(contexts ...context.Context) *openAIWSNativeHistory {
	h := &openAIWSNativeHistory{
		used:       &s.openaiWSNativeHistoryBytes,
		maxSession: openAIWSNativeHistorySessionDefault,
		maxTotal:   64 << 20,
		maxItems:   openAIWSNativeHistoryItemsDefault,
	}
	if s.cfg != nil {
		c := s.cfg.Gateway.OpenAIWS
		h.maxSession = c.NativeReplayMaxSessionBytes
		h.maxTotal = c.NativeReplayMaxTotalBytes
		h.maxItems = c.NativeReplayMaxItems
	}
	if len(contexts) > 0 {
		if contexts[0] != nil {
			h.scope, _ = contexts[0].Value(openAIWSNativeReplayBudgetKey{}).(*openAIWSNativeReplayBudget)
		}
		// Transparent handoff requires a reservation owned by the whole client
		// connection, not just the old upstream attempt.
		if h.scope == nil {
			h.maxSession = 0
		}
	}
	return h
}

func (h *openAIWSNativeHistory) clear(reason string) {
	if h.reserved != 0 && h.scope == nil {
		h.used.Add(-h.reserved)
	}
	h.reserved = 0
	h.items = nil
	h.exists = false
	h.complete = false
	h.ready = false
	h.lastResponse = ""
	h.reason = reason
}

func (h *openAIWSNativeHistory) close() { h.clear("closed") }

func (h *openAIWSNativeHistory) resize(n int64) bool {
	if n < 0 || n > h.maxSession || h.maxSession <= 0 || h.maxTotal <= 0 || h.maxItems <= 0 {
		h.clear("session_budget")
		return false
	}
	if h.scope != nil {
		if !h.scope.reserveAtLeast(n) {
			h.clear("total_budget")
			return false
		}
		h.reserved = n
		return true
	}
	delta := n - h.reserved
	if delta <= 0 {
		h.used.Add(delta)
		h.reserved = n
		return true
	}
	for {
		used := h.used.Load()
		if used < 0 || delta > h.maxTotal-used {
			h.clear("total_budget")
			return false
		}
		if h.used.CompareAndSwap(used, used+delta) {
			h.reserved = n
			return true
		}
	}
}

func (h *openAIWSNativeHistory) replace(next []json.RawMessage, detach bool) bool {
	if len(next) > h.maxItems {
		h.clear("item_budget")
		return false
	}
	if int64(len(next)) > h.maxSession/openAIWSNativeHistoryItemOverhead {
		h.clear("session_budget")
		return false
	}
	n := int64(len(next)) * openAIWSNativeHistoryItemOverhead
	for _, item := range next {
		if int64(len(item)) > h.maxSession-n {
			h.clear("session_budget")
			return false
		}
		n += int64(len(item))
	}
	if !h.resize(n) {
		return false
	}
	// Detach newly received items once, so small slices cannot pin a much larger
	// request buffer. Previously owned bodies are immutable and remain shared.
	if detach {
		for i, item := range next {
			if i < len(h.items) && len(item) > 0 && len(h.items[i]) > 0 && &item[0] == &h.items[i][0] {
				continue
			}
			next[i] = bytes.Clone(item)
		}
	}
	h.items = next
	return true
}

func (h *openAIWSNativeHistory) prepare(items []json.RawMessage, exists bool, previous string) {
	h.ready = false
	if h.maxSession <= 0 || h.maxItems <= 0 {
		h.clear("disabled")
		return
	}
	previous = strings.TrimSpace(previous)
	if previous != "" && (!h.complete || previous != h.lastResponse) {
		reason := h.reason
		if h.complete || reason == "" {
			reason = "incomplete_chain"
		}
		h.clear(reason)
		return
	}
	if previous == "" {
		h.clear("")
	}
	if !exists {
		h.clear("missing_input")
		return
	}
	for _, item := range items {
		value := parseRawJSONView(item)
		if !value.IsObject() {
			h.clear("unsupported_input_shape")
			return
		}
		if value.Get("type").String() == "item_reference" {
			h.clear("unresolved_item_reference")
			return
		}
	}
	next := items
	if previous != "" && !openAIWSRawItemsHasPrefix(items, h.items) {
		// Unlike tool-only transport recovery, full history must not discard
		// assistant calls whose results have not arrived yet.
		next = combineOpenAIWSReplayItems(h.items, items)
	}
	// Never mutate the caller's item descriptors when detaching input bodies.
	next = append([]json.RawMessage(nil), next...)
	if !h.replace(next, true) {
		return
	}
	h.exists = true
	h.complete = true
	h.ready = true
	h.reason = ""
}

// The terminal output array is authoritative for ordering and final item bodies.
// A missing array is not evidence of an empty response: do not migrate it.
func openAIWSNativeCompletedOutput(payload []byte) []json.RawMessage {
	output := parseRawJSONView(payload).Get("response.output")
	if !output.IsArray() || !gjson.Valid(output.Raw) {
		return nil
	}
	values := output.Array()
	items := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		itemType := value.Get("type").String()
		if !value.IsObject() || itemType == "" || itemType == "item_reference" {
			return nil
		}
		items = append(items, json.RawMessage(value.Raw))
	}
	return items
}

func (h *openAIWSNativeHistory) commit(result *OpenAIForwardResult) {
	if !h.ready {
		return
	}
	if result == nil || result.RequestID == "" || result.wsAccountFailoverReplayInput == nil ||
		(result.UpstreamTerminalEvent != "response.completed" && result.UpstreamTerminalEvent != "response.done") {
		h.clear("incomplete_response")
		return
	}
	// Completed output bodies are independently owned; do not copy them again.
	next := combineOpenAIWSReplayItems(h.items, result.wsAccountFailoverReplayInput)
	if h.replace(next, false) {
		h.lastResponse = strings.Clone(result.RequestID)
	}
	h.ready = false
}

func (h *openAIWSNativeHistory) retryPayload(source []byte, model string) ([]byte, bool, error) {
	if !h.ready || !h.complete || !nativeWSReplaySourceIsIndependent(source) {
		return nil, false, nil
	}
	if int64(len(source)) > h.maxSession {
		h.reason = "retry_payload_budget"
		return nil, false, nil
	}
	if h.scope != nil && (h.reserved > h.maxTotal/3 || !h.scope.reserveAtLeast(3*h.reserved)) {
		h.reason = "retry_processing_budget"
		return nil, false, nil
	}
	payload, safe, err := buildOpenAIWSCurrentTurnRetryPayload(source, h.items, h.exists, model)
	if err != nil {
		h.reason = "retry_payload_build"
	} else if !safe {
		h.reason = "unpaired_tool_output"
	}
	if err == nil && safe && int64(len(payload)) > h.maxSession {
		h.reason = "retry_payload_budget"
		return nil, false, nil
	}
	return payload, safe, err
}

// The per-process reservation is acquired before any completed output is
// copied. Accounting-only counters would allow simultaneous responses to
// allocate unbounded history even with a nominal aggregate cap.
func (h *openAIWSNativeHistory) captureCompletedOutput(payload []byte) []json.RawMessage {
	if !h.ready {
		return nil
	}
	root := parseRawJSONView(payload)
	status := root.Get("response.status").String()
	if (status != "" && status != "completed") || root.Get("response.error").IsObject() {
		h.clear("incomplete_response")
		return nil
	}
	output := root.Get("response.output")
	if !output.IsArray() || !gjson.Valid(output.Raw) {
		h.clear("missing_output")
		return nil
	}
	count := len(h.items)
	n := h.reserved
	valid := true
	output.ForEach(func(_, item gjson.Result) bool {
		count++
		if !item.IsObject() {
			valid = false
			return false
		}
		switch item.Get("type").String() {
		case "message", "reasoning", "function_call", "custom_tool_call":
		default:
			valid = false
			return false
		}
		if count > h.maxItems || int64(len(item.Raw))+openAIWSNativeHistoryItemOverhead > h.maxSession-n {
			valid = false
			return false
		}
		n += int64(len(item.Raw)) + openAIWSNativeHistoryItemOverhead
		return true
	})
	if !valid {
		h.clear("output_budget_or_shape")
		return nil
	}
	if h.scope != nil && (n > h.maxTotal/3 || !h.scope.reserveAtLeast(3*n)) {
		h.clear("total_budget")
		return nil
	}
	if !h.resize(n) {
		return nil
	}
	return openAIWSNativeCompletedOutput(payload)
}

func (h *openAIWSNativeHistory) preparePayload(payload []byte, previous string) {
	if h.maxSession <= 0 || h.maxTotal <= 0 || h.maxItems <= 0 {
		h.clear("disabled")
		return
	}
	if int64(len(payload)) > h.maxSession {
		h.clear("unsafe_or_oversized_source")
		return
	}
	// Count descriptors using a borrowed view before Array/Unmarshal copies.
	input := parseRawJSONView(payload).Get("input")
	count := int64(0)
	if input.IsArray() {
		input.ForEach(func(_, _ gjson.Result) bool { count++; return count <= int64(h.maxItems) })
	} else if input.Exists() {
		count = 1
	}
	if count > int64(h.maxItems) || count > h.maxTotal/64 {
		h.clear("item_budget")
		return
	}
	cost := count * 64
	if int64(len(payload)) > h.maxTotal-cost {
		h.clear("total_budget")
		return
	}
	cost += int64(len(payload))
	if h.reserved > h.maxTotal || cost > (h.maxTotal-h.reserved)/2 {
		h.clear("total_budget")
		return
	}
	if h.scope != nil && !h.scope.reserveAtLeast(h.reserved+2*cost) {
		h.clear("total_budget")
		return
	}
	if !nativeWSReplaySourceIsIndependent(payload) {
		h.clear("unsafe_or_oversized_source")
		return
	}
	items, exists, err := openAIWSExtractNormalizedInputSequence(payload)
	if err != nil {
		h.clear("invalid_input")
		return
	}
	h.prepare(items, exists, previous)
}

func combineOpenAIWSReplayItems(prefix, items []json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(prefix)+len(items))
	out = append(out, prefix...)
	return append(out, items...)
}

func nativeWSReplaySourceIsIndependent(source []byte) bool {
	if !gjson.ValidBytes(source) {
		return false
	}
	for _, key := range []string{"input", "model", "previous_response_id", "conversation", "background", "prompt", "tools"} {
		if HasDuplicateTopLevelKey(source, key) {
			return false
		}
	}
	root := parseRawJSONView(source)
	if root.Get("conversation").Exists() || root.Get("prompt").Exists() || root.Get("background").Bool() {
		return false
	}
	tools := root.Get("tools")
	if tools.Exists() && !tools.IsArray() {
		return false
	}
	clientTools := true
	tools.ForEach(func(_, tool gjson.Result) bool {
		switch tool.Get("type").String() {
		case "function", "custom":
		default:
			clientTools = false
		}
		return clientTools
	})
	if !clientTools {
		return false
	}
	return !nativeWSHistoryHasAccountReference(root.Get("input"), 0)
}

func nativeWSHistoryHasAccountReference(value gjson.Result, depth int) bool {
	if depth > 32 {
		return true
	}
	if !value.IsObject() && !value.IsArray() {
		return false
	}
	if value.Get("type").String() == "item_reference" || value.Get("file_id").Exists() || value.Get("container_id").Exists() {
		return true
	}
	found := false
	value.ForEach(func(_, child gjson.Result) bool {
		found = nativeWSHistoryHasAccountReference(child, depth+1)
		return !found
	})
	return found
}

// A client-connection high-water reservation spans replacement-account attempts
// and the intervening retry payload. It is released only after the handler has
// stopped using that payload, preventing handoff from bypassing the total cap.
type openAIWSNativeReplayBudgetKey struct{}
type openAIWSNativeReplayBudget struct {
	mu     sync.Mutex
	used   *atomic.Int64
	limit  int64
	held   int64
	closed bool
}

func (b *openAIWSNativeReplayBudget) reserveAtLeast(n int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || n < 0 {
		return false
	}
	if n <= b.held {
		return true
	}
	delta := n - b.held
	for {
		used := b.used.Load()
		if used < 0 || delta > b.limit-used {
			return false
		}
		if b.used.CompareAndSwap(used, used+delta) {
			b.held = n
			return true
		}
	}
}
func (b *openAIWSNativeReplayBudget) release() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		b.used.Add(-b.held)
		b.held = 0
	}
}

// WithOpenAIWSNativeReplayBudget establishes the native replay reservation for
// one downstream WebSocket. The caller must defer release until every upstream
// attempt and retry payload for that connection has finished.
func (s *OpenAIGatewayService) WithOpenAIWSNativeReplayBudget(ctx context.Context) (context.Context, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.cfg == nil {
		return ctx, func() {}
	}
	cfg := s.cfg.Gateway.OpenAIWS
	if cfg.NativeReplayMaxSessionBytes <= 0 || cfg.NativeReplayMaxItems <= 0 || cfg.NativeReplayMaxTotalBytes <= 0 {
		return ctx, func() {}
	}
	if prior, ok := ctx.Value(openAIWSNativeReplayBudgetKey{}).(*openAIWSNativeReplayBudget); ok && prior.used == &s.openaiWSNativeHistoryBytes {
		return ctx, func() {}
	}
	b := &openAIWSNativeReplayBudget{used: &s.openaiWSNativeHistoryBytes, limit: cfg.NativeReplayMaxTotalBytes}
	return context.WithValue(ctx, openAIWSNativeReplayBudgetKey{}, b), b.release
}
