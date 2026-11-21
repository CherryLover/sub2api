package service

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func nativeHistoryItems(raw ...string) []json.RawMessage {
	items := make([]json.RawMessage, len(raw))
	for i := range raw {
		items[i] = json.RawMessage(raw[i])
	}
	return items
}

func nativeHistoryResult(id string, items []json.RawMessage) *OpenAIForwardResult {
	if items == nil {
		items = []json.RawMessage{}
	}
	return &OpenAIForwardResult{RequestID: id, UpstreamTerminalEvent: "response.completed", wsAccountFailoverReplayInput: items}
}

func TestOpenAIWSNativeHistoryCompletedOutput(t *testing.T) {
	require.Nil(t, openAIWSNativeCompletedOutput([]byte(`{"response":{"id":"resp_missing_output"}}`)))
	require.Nil(t, openAIWSNativeCompletedOutput([]byte(`{"response":{"output":[null]}}`)))
	require.NotNil(t, openAIWSNativeCompletedOutput([]byte(`{"response":{"output":[]}}`)))
	payload := []byte(`{"response":{"output":[{"type":"reasoning","id":"first"},{"type":"message","id":"second"}]}}`)
	items := openAIWSNativeCompletedOutput(payload)
	require.Len(t, items, 2)
	require.Equal(t, "first", gjson.GetBytes(items[0], "id").String())
	clear(payload)
	require.True(t, json.Valid(items[0]), "cache must own its body independently of the wire buffer")
	h := newNativeHistoryTestService().newOpenAIWSNativeHistory()
	defer h.close()
	h.prepare(nativeHistoryItems(`{"role":"user","content":"hi"}`), true, "")
	h.commit(&OpenAIForwardResult{RequestID: "resp_partial", UpstreamTerminalEvent: "response.completed"})
	require.False(t, h.complete)
}

func TestOpenAIWSNativeHistoryCompleteReplay(t *testing.T) {
	s := newNativeHistoryTestService()
	h := s.newOpenAIWSNativeHistory()
	defer h.close()
	root := nativeHistoryItems(`{"type":"message","role":"user","content":"window-67"}`)
	h.prepare(root, true, "")
	outputs := nativeHistoryItems(
		`{"type":"reasoning","id":"rs_1","encrypted_content":"ciphertext","summary":[]}`,
		`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"working"}]}`,
		`{"type":"custom_tool_call","call_id":"call_1","name":"apply_patch","input":"patch"}`,
	)
	h.commit(nativeHistoryResult("resp_1", outputs))
	oldBody := h.items[0]
	delta := nativeHistoryItems(`{"type":"custom_tool_call_output","call_id":"call_1","output":"done"}`)
	h.prepare(delta, true, "resp_1")
	require.True(t, h.ready)
	require.Same(t, &oldBody[0], &h.items[0][0], "owned history bodies must be shared")
	require.NotSame(t, &delta[0][0], &h.items[4][0], "new input must not pin the request buffer")
	source := []byte(`{"type":"response.create","model":"mapped-model","previous_response_id":"resp_1","prompt_cache_key":"client-key","client_metadata":{"x-codex-window-id":"67"},"input":[]}`)
	retry, safe, err := h.retryPayload(source, "original-model")
	require.NoError(t, err)
	require.True(t, safe)
	require.False(t, gjson.GetBytes(retry, "previous_response_id").Exists())
	require.Equal(t, "original-model", gjson.GetBytes(retry, "model").String())
	require.Equal(t, "client-key", gjson.GetBytes(retry, "prompt_cache_key").String())
	require.Equal(t, int64(5), gjson.GetBytes(retry, "input.#").Int())
	require.Equal(t, "ciphertext", gjson.GetBytes(retry, "input.1.encrypted_content").String())
	require.Contains(t, string(retry), "custom_tool_call_output")
	require.Equal(t, "resp_1", gjson.GetBytes(source, "previous_response_id").String())
	// A second account's failure must replay exactly the same current request.
	h2 := s.newOpenAIWSNativeHistory()
	defer h2.close()
	items, exists, err := openAIWSExtractNormalizedInputSequence(retry)
	require.NoError(t, err)
	h2.prepare(items, exists, "")
	retry2, safe, err := h2.retryPayload(retry, "original-model")
	require.NoError(t, err)
	require.True(t, safe)
	require.JSONEq(t, string(retry), string(retry2))

	// Compaction replaces the window, including assistant and encrypted history.
	h.prepare(nativeHistoryItems(`{"type":"message","role":"user","content":"summary-window-68"}`), true, "")
	require.Len(t, h.items, 1)
	require.NotContains(t, string(h.items[0]), "67")
	require.Less(t, h.reserved, int64(256))
}

func TestOpenAIWSNativeHistoryFullPrefixAndEmptyDelta(t *testing.T) {
	h := newNativeHistoryTestService().newOpenAIWSNativeHistory()
	defer h.close()
	root := nativeHistoryItems(`{"role":"user","content":"hi"}`)
	h.prepare(root, true, "")
	h.commit(nativeHistoryResult("resp_1", nativeHistoryItems(`{"type":"message","role":"assistant","content":"hello"}`)))
	full := combineOpenAIWSReplayItems(h.items, nativeHistoryItems(`{"role":"user","content":"next"}`))
	h.prepare(full, true, "resp_1")
	require.Len(t, h.items, 3)
	h.commit(nativeHistoryResult("resp_2", nil))
	h.prepare(nil, true, "resp_2")
	require.Len(t, h.items, 3)
	require.True(t, h.ready)
}

func TestOpenAIWSNativeHistoryFailClosed(t *testing.T) {
	for _, scenario := range []string{"unknown_chain", "missing_input", "incomplete", "failed", "missing_response_id", "orphan_output", "item_reference", "shorthand_input", "closed"} {
		t.Run(scenario, func(t *testing.T) {
			s := newNativeHistoryTestService()
			h := s.newOpenAIWSNativeHistory()
			defer h.close()
			h.prepare(nativeHistoryItems(`{"role":"user","content":"hi"}`), true, "")
			switch scenario {
			case "unknown_chain":
				h.prepare(nil, true, "resp_unknown")
			case "missing_input":
				h.prepare(nil, false, "")
			case "incomplete", "failed":
				r := nativeHistoryResult("resp_1", nil)
				r.UpstreamTerminalEvent = "response." + scenario
				h.commit(r)
			case "missing_response_id":
				h.commit(nativeHistoryResult("", nil))
			case "orphan_output":
				h.prepare(nativeHistoryItems(`{"type":"function_call_output","call_id":"missing","output":"x"}`), true, "")
			case "item_reference":
				h.prepare(nativeHistoryItems(`{"type":"item_reference","id":"msg_remote"}`), true, "")
			case "shorthand_input":
				h.prepare(nativeHistoryItems(`"hello"`), true, "")
			case "closed":
				h.close()
			}
			_, safe, err := h.retryPayload([]byte(`{"type":"response.create","input":[]}`), "m")
			require.NoError(t, err)
			require.False(t, safe)
			h.close()
			require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
		})
	}
}

func TestOpenAIWSNativeHistoryBudgets(t *testing.T) {
	for _, limit := range []string{"session", "items", "disabled", "disabled_items"} {
		t.Run(limit, func(t *testing.T) {
			s := newNativeHistoryTestService()
			h := s.newOpenAIWSNativeHistory()
			defer h.close()
			h.prepare(nativeHistoryItems(`{"role":"user","content":"hi"}`), true, "")
			h.commit(nativeHistoryResult("resp_1", nil))
			switch limit {
			case "session":
				h.maxSession = 100
			case "items":
				h.maxItems = 1
			case "disabled":
				h.maxSession = 0
			case "disabled_items":
				h.maxItems = 0
			}
			h.prepare(nativeHistoryItems(`{"role":"user","content":"next"}`), true, "resp_1")
			require.False(t, h.ready)
			require.Empty(t, h.items)
			require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
			h.maxSession, h.maxItems = 1024, 8
			h.prepare(nativeHistoryItems(`{"role":"user","content":"fresh"}`), true, "")
			require.True(t, h.ready, "a later full root can recover caching")
		})
	}
}

func TestOpenAIWSNativeHistorySessionLimitBoundary(t *testing.T) {
	s := newNativeHistoryTestService()
	h := s.newOpenAIWSNativeHistory()
	defer h.close()
	require.EqualValues(t, 64<<20, h.maxSession)
	require.Equal(t, 32768, h.maxItems)
	root := nativeHistoryItems(`{"role":"user","content":"hi"}`)
	h.maxSession = int64(len(root[0])) + openAIWSNativeHistoryItemOverhead
	h.prepare(root, true, "")
	require.True(t, h.ready, "exactly the byte limit is allowed")
	require.Equal(t, h.maxSession, h.reserved)

	h.prepare(nativeHistoryItems(string(root[0])+" "), true, "")
	require.False(t, h.ready, "one byte over the limit must release history")
	require.Equal(t, "session_budget", h.reason)
	require.Nil(t, h.items)
	require.Zero(t, s.openaiWSNativeHistoryBytes.Load())

	h.prepare(root, true, "")
	require.True(t, h.ready, "a smaller full root can restart caching")
	h.commit(nativeHistoryResult("resp_overflow", nativeHistoryItems(`{"type":"message","role":"assistant","content":"hello"}`)))
	require.False(t, h.complete, "upstream output also counts toward the limit")
	require.Equal(t, "session_budget", h.reason)
	require.Nil(t, h.items)
	require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
	h.prepare(root, true, "resp_overflow")
	require.False(t, h.ready, "a delta cannot recover discarded history")
}

func newNativeHistoryTestService() *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.NativeReplayMaxSessionBytes = 64 << 20
	cfg.Gateway.OpenAIWS.NativeReplayMaxTotalBytes = 64 << 20
	cfg.Gateway.OpenAIWS.NativeReplayMaxItems = 32768
	return &OpenAIGatewayService{cfg: cfg}
}

func TestOpenAIWSNativeHistoryDefaultsRemainFailClosed(t *testing.T) {
	for _, s := range []*OpenAIGatewayService{{}, {cfg: &config.Config{}}} {
		ctx, release := s.WithOpenAIWSNativeReplayBudget(context.Background())
		defer release()
		h := s.newOpenAIWSNativeHistory(ctx)
		defer h.close()
		h.preparePayload([]byte(`{"input":[{"role":"user","content":"hi"}]}`), "")
		require.False(t, h.ready)
		require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
	}
}

func TestNativeReplayBudgetSurvivesAccountHandoff(t *testing.T) {
	s := newNativeHistoryTestService()
	s.cfg.Gateway.OpenAIWS.NativeReplayMaxTotalBytes = 250
	ctx, release := s.WithOpenAIWSNativeReplayBudget(t.Context())
	defer release()
	root := nativeHistoryItems(`{"role":"user","content":"hi"}`)
	first := s.newOpenAIWSNativeHistory(ctx)
	first.prepare(root, true, "")
	require.True(t, first.ready)
	reserved := first.reserved
	first.close()
	require.Equal(t, reserved, s.openaiWSNativeHistoryBytes.Load(), "handoff must not free the downstream connection reservation")
	second := s.newOpenAIWSNativeHistory(ctx)
	second.prepare(root, true, "")
	require.True(t, second.ready)
	require.Equal(t, reserved, s.openaiWSNativeHistoryBytes.Load())
	otherCtx, otherRelease := s.WithOpenAIWSNativeReplayBudget(t.Context())
	defer otherRelease()
	other := s.newOpenAIWSNativeHistory(otherCtx)
	other.prepare(nativeHistoryItems(`{"role":"user","content":"`+strings.Repeat("x", 150)+`"}`), true, "")
	require.False(t, other.ready)
	require.Equal(t, "total_budget", other.reason)
	second.close()
	release()
	release()
	require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
	other.prepare(root, true, "")
	require.True(t, other.ready)
	other.close()
	otherRelease()
	require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
}

func TestNativeReplayReservesCompletedOutputBeforeRetainingIt(t *testing.T) {
	s := newNativeHistoryTestService()
	s.cfg.Gateway.OpenAIWS.NativeReplayMaxTotalBytes = 230
	ctx, release := s.WithOpenAIWSNativeReplayBudget(t.Context())
	defer release()
	h := s.newOpenAIWSNativeHistory(ctx)
	defer h.close()
	h.prepare(nativeHistoryItems(`{"role":"user","content":"hi"}`), true, "")
	before := s.openaiWSNativeHistoryBytes.Load()
	output := []byte(`{"response":{"status":"completed","output":[{"type":"message","role":"assistant","content":"` + strings.Repeat("x", 200) + `"}]}}`)
	require.Nil(t, h.captureCompletedOutput(output))
	require.False(t, h.ready)
	require.Equal(t, before, s.openaiWSNativeHistoryBytes.Load(), "failed growth does not consume additional process budget")
	release()
	require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
}

func TestNativeReplayRejectsAccountBoundOrAmbiguousRequests(t *testing.T) {
	for _, body := range []string{
		`{"input":[],"conversation":"conv_1"}`, `{"input":[],"prompt":{"id":"p1"}}`,
		`{"input":[],"background":true}`, `{"input":[],"tools":[{"type":"web_search"}]}`,
		`{"input":[{"type":"item_reference","id":"remote"}]}`,
		`{"input":[{"role":"user","content":[{"type":"input_file","file_id":"private"}]}]}`,
		`{"input":[],"input":[{"type":"item_reference","id":"remote"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			s := newNativeHistoryTestService()
			h := s.newOpenAIWSNativeHistory()
			defer h.close()
			h.preparePayload([]byte(body), "")
			require.False(t, h.ready)
		})
	}
}

func TestNativeReplayConcurrentConnectionBudgetIsBounded(t *testing.T) {
	s := newNativeHistoryTestService()
	s.cfg.Gateway.OpenAIWS.NativeReplayMaxTotalBytes = 4096
	var ready sync.WaitGroup
	ready.Add(32)
	releaseAll := make(chan struct{})
	var done sync.WaitGroup
	for i := 0; i < 32; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			ctx, release := s.WithOpenAIWSNativeReplayBudget(context.Background())
			defer release()
			h := s.newOpenAIWSNativeHistory(ctx)
			defer h.close()
			h.prepare(nativeHistoryItems(`{"role":"user","content":"`+strings.Repeat("x", 900)+`"}`), true, "")
			if used := s.openaiWSNativeHistoryBytes.Load(); used > 4096 || used < 0 {
				t.Errorf("invalid process reservation: %d", used)
			}
			ready.Done()
			<-releaseAll
		}()
	}
	ready.Wait()
	require.LessOrEqual(t, s.openaiWSNativeHistoryBytes.Load(), int64(4096))
	close(releaseAll)
	done.Wait()
	require.Zero(t, s.openaiWSNativeHistoryBytes.Load())
}

func BenchmarkOpenAIWSNativeHistoryRebuild4MiB(b *testing.B) {
	s := newNativeHistoryTestService()
	h := s.newOpenAIWSNativeHistory()
	defer h.close()
	h.prepare(nativeHistoryItems(`{"role":"user","content":"`+strings.Repeat("x", 4<<20)+`"}`), true, "")
	source := []byte(`{"type":"response.create","model":"gpt-5.1","input":[]}`)
	b.ReportAllocs()
	b.SetBytes(4 << 20)
	b.ResetTimer()
	for b.Loop() {
		_, safe, err := h.retryPayload(source, "gpt-5.1")
		if !safe || err != nil {
			b.Fatalf("rebuild: %v", err)
		}
	}
}

func TestNativeHistoryAdmissionPrecedesLargeInputMaterialization(t *testing.T) {
	svc := newNativeHistoryTestService()
	svc.cfg.Gateway.OpenAIWS.NativeReplayMaxTotalBytes = 128
	ctx, release := svc.WithOpenAIWSNativeReplayBudget(t.Context())
	defer release()
	h := svc.newOpenAIWSNativeHistory(ctx)
	defer h.close()
	body := []byte(`{"input":[{"role":"user","content":"` + strings.Repeat("x", 2<<20) + `"}]}`)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	h.preparePayload(body, "")
	runtime.ReadMemStats(&after)
	require.False(t, h.ready)
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(128<<10), "rejected history must be admitted before parsing/copying its large input")
	runtime.KeepAlive(body)
}
