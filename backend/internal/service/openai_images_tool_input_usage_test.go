package service

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

func TestOpenAIImagesToolImageInputTokens(t *testing.T) {
	for _, tc := range []struct {
		name, details string
		want          int
	}{
		{"missing", "", 0}, {"valid", `,"input_tokens_details":{"image_tokens":30}`, 30},
		{"capped", `,"input_tokens_details":{"image_tokens":101}`, 100},
		{"negative", `,"input_tokens_details":{"image_tokens":-1}`, 0},
		{"string", `,"input_tokens_details":{"image_tokens":"30"}`, 0},
		{"fraction", `,"input_tokens_details":{"image_tokens":1.5}`, 0},
		{"exponent", `,"input_tokens_details":{"image_tokens":3e1}`, 30},
		{"hugeExponent", `,"input_tokens_details":{"image_tokens":1e999999999}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"input_tokens":100,"output_tokens":60,"output_tokens_details":{"image_tokens":40}%s}`, tc.details)
			usage, ok := openAIImagesToolUsageFromGJSON(gjson.Parse(raw))
			require.True(t, ok)
			require.Equal(t, tc.want, usage.ImageInputTokens)
			require.Equal(t, 100, usage.InputTokens)
			require.Equal(t, 60, usage.OutputTokens)
			require.Equal(t, 40, usage.ImageOutputTokens)
			s := &OpenAIGatewayService{}
			var streamUsage OpenAIUsage
			s.parseOpenAIImagesSSEUsageBytes([]byte(`{"type":"response.completed","response":{"tool_usage":{"image_gen":`+raw+`}}}`), &streamUsage)
			require.Equal(t, usage, streamUsage)
		})
	}
}
