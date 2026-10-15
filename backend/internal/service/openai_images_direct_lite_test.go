package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexDirectImagesLiteJSONValidationAndUsage(t *testing.T) {
	valid := []byte(`{"created":1710000000,"data":[{"b64_json":"aGVsbG8=","size":"1024x1024"}],"usage":{"input_tokens":15,"output_tokens":24,"input_tokens_details":{"cached_tokens":4}}}`)
	images, err := parseCodexDirectImagesResponse(valid)
	require.NoError(t, err)
	require.Len(t, images, 1)
	usage, ok := codexDirectImagesUsage(valid)
	require.True(t, ok)
	require.Equal(t, 15, usage.InputTokens)
	require.Equal(t, 24, usage.OutputTokens)
	require.Equal(t, 24, usage.ImageOutputTokens)
	require.Equal(t, 4, usage.CacheReadInputTokens)

	for _, malformed := range [][]byte{
		[]byte(`{"data":[]}`),
		[]byte(`{"error":{"message":"blocked"}}`),
		[]byte(`{"data":[{"b64_json":""}]}`),
		[]byte(`not-json`),
	} {
		_, err := parseCodexDirectImagesResponse(malformed)
		require.Error(t, err)
	}
	require.True(t, isCodexDirectImagesJSONResponse(&http.Response{Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}}))
	require.False(t, isCodexDirectImagesJSONResponse(&http.Response{Header: http.Header{"Content-Type": []string{"text/event-stream"}}}))
}

func TestCodexDirectImagesAccountOptInLite(t *testing.T) {
	model := "gpt-image-2"
	require.True(t, usesCodexDirectImages(model))
	require.False(t, (&Account{}).getExtraBool("codex_direct_images_enabled"))
	require.True(t, (&Account{Extra: map[string]any{"codex_direct_images_enabled": true}}).getExtraBool("codex_direct_images_enabled"))
}
