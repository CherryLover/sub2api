package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"runtime"
	"strings"
	"testing"
)

func metadataMemoryBody(size int) []byte {
	var b strings.Builder
	_, _ = b.WriteString(`{"model":"gpt-5","input":[{"role":"user","content":"`)
	_, _ = b.WriteString(strings.Repeat("x", size))
	_, _ = b.WriteString(`"}`)
	for i := 0; i < 48; i++ {
		_, _ = b.WriteString(`,{"type":"message","role":"user","content":"small","internal_chat_message_metadata_passthrough":null}`)
	}
	_, _ = b.WriteString(`],"integer":9007199254740993}`)
	return []byte(b.String())
}
func legacyStripMetadataForTest(body []byte) ([]byte, bool, error) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
	}
	out := body
	changed := false
	for i, item := range input.Array() {
		if !item.IsObject() || !item.Get("internal_chat_message_metadata_passthrough").Exists() {
			continue
		}
		var err error
		out, err = sjson.DeleteBytes(out, fmt.Sprintf("input.%d.internal_chat_message_metadata_passthrough", i))
		if err != nil {
			return body, false, err
		}
		changed = true
	}
	return out, changed, nil
}
func TestOAuthMetadataCleanupDoesNotCopyWholeBodyPerItem(t *testing.T) {
	body := metadataMemoryBody(4 << 20)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out, changed, err := stripOpenAIOAuthInputMetadataBody(body)
	runtime.ReadMemStats(&after)
	require.NoError(t, err)
	require.True(t, changed)
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(8*len(body)))
	require.Contains(t, string(out), `"integer":9007199254740993`)
	runtime.KeepAlive(body)
	runtime.KeepAlive(out)
}
func TestCompactionSignalDoesNotCopyLargeInput(t *testing.T) {
	body := []byte(`{"input":[{"role":"user","content":"` + strings.Repeat("x", 4<<20) + `"},{"type":"compaction_trigger"}]}`)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	found := HasCompactionTriggerInInput(body)
	runtime.ReadMemStats(&after)
	require.True(t, found)
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(128<<10))
	runtime.KeepAlive(body)
}
func TestOAuthMetadataCleanupMatchesLegacySemantics(t *testing.T) {
	for _, body := range []string{
		` { "input" : [ {"internal_chat_message_metadata_passthrough":null,"n":9007199254740993},null,"text",{"content":{"internal_chat_message_metadata_passthrough":7}} ] ,"other":true} `,
		`{"input":[{"internal_chat_message_metadata_passthrough":1,"internal_chat_message_metadata_passthrough":2}]}`,
		`{"input":[{"internal_chat_message_metadata_passthrough":1}],"input":[{"keep":true}]}`,
		`{"inpu\u0074":[{"internal_chat_message_metadata_passthrough":false}]}`,
		`{"input":[{"internal_chat_message_metadata_pass\u0074hrough":false}]}`,
		`{"input":"text"}`, `{"input":[{"content":"internal_chat_message_metadata_passthrough"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			want, wc, we := legacyStripMetadataForTest([]byte(body))
			got, gc, ge := stripOpenAIOAuthInputMetadataBody([]byte(body))
			require.Equal(t, we, ge)
			require.Equal(t, wc, gc)
			decode := func(b []byte) any {
				var v any
				d := json.NewDecoder(bytes.NewReader(b))
				d.UseNumber()
				require.NoError(t, d.Decode(&v))
				return v
			}
			require.Equal(t, decode(want), decode(got))
			if !gc {
				require.Equal(t, body, string(got))
			}
		})
	}
}
func BenchmarkOAuthMetadataCleanupLargeBody(b *testing.B) {
	body := metadataMemoryBody(4 << 20)
	for _, tc := range []struct {
		name string
		fn   func([]byte) ([]byte, bool, error)
	}{{"legacy", legacyStripMetadataForTest}, {"current", stripOpenAIOAuthInputMetadataBody}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for b.Loop() {
				_, _, err := tc.fn(body)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
