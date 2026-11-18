package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesPythonRegexBackport(t *testing.T) {
	const original = `{"model":"gpt-6","tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"bad":{"type":"string","pattern":"^\\p{L}+$","default":{"pattern":"(?=should_stay)"}},"valid":{"type":"string","pattern":"^[a-z]+\\d+$"}},"patternProperties":{"^\\p{L}+$":{"type":"string"},"^[a-z]+$":{"type":"string","pattern":"(?=reject)"}},"additionalProperties":false}}]}`
	cleaned, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns([]byte(original))
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, json.Valid(cleaned))
	require.False(t, gjson.GetBytes(cleaned, "tools.0.parameters.properties.bad.pattern").Exists())
	require.Equal(t, "(?=should_stay)", gjson.GetBytes(cleaned, "tools.0.parameters.properties.bad.default.pattern").String())
	require.Equal(t, `^[a-z]+\d+$`, gjson.GetBytes(cleaned, "tools.0.parameters.properties.valid.pattern").String())
	var doc struct {
		Tools []struct {
			Parameters struct {
				PatternProperties    map[string]json.RawMessage `json:"patternProperties"`
				AdditionalProperties bool                       `json:"additionalProperties"`
			} `json:"parameters"`
		} `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(cleaned, &doc))
	require.NotContains(t, doc.Tools[0].Parameters.PatternProperties, `^\p{L}+$`)
	require.Contains(t, doc.Tools[0].Parameters.PatternProperties, `^[a-z]+$`)
	require.True(t, doc.Tools[0].Parameters.AdditionalProperties)
	require.False(t, gjson.GetBytes(cleaned, "tools.0.parameters.patternProperties.\\^[a-z]+$\\.pattern").Exists())
	again, next, err := sanitizeOpenAIResponsesToolSchemaPatterns(cleaned)
	require.NoError(t, err)
	require.False(t, next)
	require.Equal(t, cleaned, again)
}

func TestOpenAIResponsesPythonRegexBackportNoChangeForValidSchema(t *testing.T) {
	const original = `{"tools":[{"parameters":{"type":"object","patternProperties":{"^[a-z]+$":{"type":"string","pattern":"^\\d+$"}},"additionalProperties":false}}]}`
	cleaned, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns([]byte(original))
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, original, string(cleaned))
}

func TestOpenAIResponsesPythonRegexNestedToolsAndEscapes(t *testing.T) {
	original := []byte(`{"input":[{"tools":[{"type":"namespace","tools":[{"type":"function","parameters":{"type":"object","properties":{"bad":{"pattern":"(?<name>[A-Z]+)"},"ok":{"pattern":"\\\\p{L}+"}},"patternProperties":{"^(?=bad)":{"type":"string"},"^good$":{"type":"string"}},"additionalProperties":{"type":"string"}}}]}]}],"metadata":{"pattern":"(?=untouched)"},"max_output_tokens":1234567890123456789}`)
	cleaned, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(original)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(cleaned, "input.0.tools.0.tools.0.parameters.properties.bad.pattern").Exists())
	require.Equal(t, `\\p{L}+`, gjson.GetBytes(cleaned, "input.0.tools.0.tools.0.parameters.properties.ok.pattern").String())
	require.Equal(t, "(?=untouched)", gjson.GetBytes(cleaned, "metadata.pattern").String())
	require.Contains(t, string(cleaned), `1234567890123456789`)
	require.True(t, gjson.GetBytes(cleaned, "input.0.tools.0.tools.0.parameters.additionalProperties").Bool())
	cleanedAgain, changedAgain, err := sanitizeOpenAIResponsesToolSchemaPatterns(cleaned)
	require.NoError(t, err)
	require.False(t, changedAgain)
	require.Equal(t, cleaned, cleanedAgain)
}

func TestOpenAIRegexUnsupportedPatterns(t *testing.T) {
	for _, pattern := range []string{`\p{L}`, `\R`, `(?=a)`, `(?!a)`, `(?<n>a)`, `(?<=a)`, `\Q`} {
		require.True(t, openAIRegexUnsupported(pattern), pattern)
	}
	for _, pattern := range []string{`^\d+$`, `^[a-z]+$`, `(?:foo|bar)`, `\\p{L}`, `^foo$`} {
		require.False(t, openAIRegexUnsupported(pattern), pattern)
	}
}
