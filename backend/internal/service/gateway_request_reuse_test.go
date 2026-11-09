//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayRequestParseSnapshotReusesOnlyUnchangedBody(t *testing.T) {
	body := []byte(`{"model":"gpt-5","stream":true,"metadata":{"user_id":"abc"},"input":[{"type":"message","content":"hello"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "openai")
	require.NoError(t, err)
	require.NotNil(t, parsed.Body.parsed)

	// The same immutable body may be cloned without another full parse.
	parsed.Model = "temporary-overridden-route-model"
	clone, err := parsed.CloneForBody(body)
	require.NoError(t, err)
	require.Equal(t, "gpt-5", clone.Model)
	require.True(t, clone.Stream)
	require.Equal(t, "abc", clone.MetadataUserID)
	require.Same(t, parsed.Body.parsed, clone.Body.parsed)

	// In-place mutation must be detected, not treated as a valid snapshot.
	copy(body[10:15], []byte("gpt-6"))
	mutated, err := parsed.CloneForBody(body)
	require.NoError(t, err)
	require.Equal(t, "gpt-6", mutated.Model)
	require.NotSame(t, parsed.Body.parsed, mutated.Body.parsed)
	parsed.Model = "stale-override"
	require.NoError(t, parsed.ReplaceBody(body))
	require.Equal(t, "gpt-6", parsed.Model)
}

func TestGatewayRequestParseSnapshotInvalidatedByReplaceAndInvalidJSON(t *testing.T) {
	parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(`{"model":"gpt-5","input":[]}`)), "openai")
	require.NoError(t, err)
	initial := parsed.Body.parsed
	require.NotNil(t, initial)
	require.NoError(t, parsed.ReplaceBody(parsed.Body.Bytes()))
	require.Same(t, initial, parsed.Body.parsed)

	require.NoError(t, parsed.ReplaceBody([]byte(`{"model":"gpt-6","input":[]}`)))
	require.NotSame(t, initial, parsed.Body.parsed)
	require.Equal(t, "gpt-6", parsed.Model)
	require.Error(t, parsed.ReplaceBody([]byte(`{"model":`)))
	require.Nil(t, parsed.Body.parsed, "invalid request cannot retain a valid parse snapshot")
}
