package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesCreatedAtStamperKeepsStreamValue(t *testing.T) {
	var stamper responsesCreatedAtStamper
	for _, tt := range []struct {
		event string
		body  string
		stamp bool
	}{
		{"response.created", `{"type":"response.created","response":{"id":"resp_1"}}`, true},
		{"response.completed", `{"type":"response.completed","response":{"id":"resp_1"}}`, true},
		{"response.failed", `{"type":"response.failed","response":{"id":"resp_1","created_at":1720000000}}`, false},
		{"ping", `{"type":"ping","response":{"id":"resp_1"}}`, false},
	} {
		rewritten, changed := stamper.Stamp([]byte(tt.body), tt.event)
		require.Equal(t, tt.stamp, changed, tt.event)
		if tt.event == "response.completed" {
			require.Equal(t, stamper.Value(), gjson.GetBytes(rewritten, "response.created_at").Int())
		}
		if tt.event == "response.created" {
			require.Positive(t, gjson.GetBytes(rewritten, "response.created_at").Int())
		}
	}
}

func TestEnsureOpenAIResponseObjectCreatedAtOnlyWhenAbsent(t *testing.T) {
	before := time.Now().Unix()
	raw := []byte(`{"object":"response","id":"resp_1"}`)
	rewritten := ensureOpenAIResponseObjectCreatedAt(raw)
	require.GreaterOrEqual(t, gjson.GetBytes(rewritten, "created_at").Int(), before)
	require.Equal(t, string(rewritten), string(ensureOpenAIResponseObjectCreatedAt(rewritten)))
	noRewrite := []byte(`{"object":"response","created_at":12345}`)
	require.Equal(t, string(noRewrite), string(ensureOpenAIResponseObjectCreatedAt(noRewrite)))
	other := []byte(`{"object":"chat.completion","choices":[]}`)
	require.Equal(t, string(other), string(ensureOpenAIResponseObjectCreatedAt(other)))
}
