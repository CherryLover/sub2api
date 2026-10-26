//go:build unit

package handler

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/stretchr/testify/require"
)

func TestAppendUpstreamGeminiModelsPreservesNativeEnvelopeAndMetadata(t *testing.T) {
	native := []byte(`{"models":[{"name":"models/gemini-3.1-pro-preview","native_metadata":{"tokenLimit":4096}}],"nextPageToken":"next"}`)
	out, ok := appendUpstreamGeminiModels(native, []gemini.Model{
		gemini.FallbackModel("gemini-3.1-pro-preview"),
		gemini.FallbackModel("gemini-custom"),
		gemini.FallbackModel("gemini-custom"),
	})
	require.True(t, ok)
	var envelope struct {
		Models        []json.RawMessage `json:"models"`
		NextPageToken string            `json:"nextPageToken"`
	}
	require.NoError(t, json.Unmarshal(out, &envelope))
	require.Len(t, envelope.Models, 2)
	require.Equal(t, "next", envelope.NextPageToken)
	require.Contains(t, string(envelope.Models[0]), "native_metadata")
	require.Contains(t, string(envelope.Models[1]), "gemini-custom")
	require.Equal(t, []byte("not-json"), func() []byte {
		x, ok := appendUpstreamGeminiModels([]byte("not-json"), nil)
		require.False(t, ok)
		return x
	}())
}
