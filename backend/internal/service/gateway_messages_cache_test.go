package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMustJSONStringQuotesControlCharactersAsJSON(t *testing.T) {
	quoted := mustJSONString("a\x7f\nb")
	var decoded string
	require.NoError(t, json.Unmarshal([]byte(quoted), &decoded))
	require.Equal(t, "a\x7f\nb", decoded)
	require.NotContains(t, quoted, `\x7f`)
}
