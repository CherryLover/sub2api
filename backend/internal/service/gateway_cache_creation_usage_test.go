//go:build unit

package service

import (
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAnthropicCacheCreationSnapshots(t *testing.T) {
	tests := []struct {
		name       string
		snapshots  []string
		wantTotals []int
		want5m     int
		want1h     int
		billed5m   int
		billed1h   int
	}{
		{
			name:       "aggregate 100 with 1h 200",
			snapshots:  []string{`{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, `{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_1h_input_tokens":200}}`},
			wantTotals: []int{100, 100}, want1h: 200, billed1h: 100,
		},
		{
			name:       "contradictory explicit aggregate",
			snapshots:  []string{`{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_5m_input_tokens":50,"ephemeral_1h_input_tokens":200}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":300}}`},
			wantTotals: []int{100, 100}, want5m: 50, want1h: 300, billed5m: 14, billed1h: 86,
		},
		{
			name:       "partial TTL snapshots derive missing aggregate",
			snapshots:  []string{`{"cache_creation":{"ephemeral_5m_input_tokens":20}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":0}}`},
			wantTotals: []int{20, 50, 30}, want1h: 30, billed1h: 30,
		},
		{
			name:       "aggregate only delta replaces derived total",
			snapshots:  []string{`{"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`, `{"cache_creation_input_tokens":40}`, `{"output_tokens":7}`, `{"cache_creation":{"ephemeral_1h_input_tokens":200}}`},
			wantTotals: []int{50, 40, 40, 40}, want5m: 20, want1h: 200, billed5m: 4, billed1h: 36,
		},
		{
			name:       "explicit zero falls back to TTL updates",
			snapshots:  []string{`{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":50}}`},
			wantTotals: []int{200, 250}, want5m: 50, want1h: 200, billed5m: 50, billed1h: 200,
		},
		{
			name:       "explicit zero delta preserves aggregate",
			snapshots:  []string{`{"cache_creation_input_tokens":100}`, `{"cache_creation_input_tokens":0}`, `{"cache_creation":{"ephemeral_1h_input_tokens":200}}`},
			wantTotals: []int{100, 100, 100}, want1h: 200, billed1h: 100,
		},
		{
			name:       "repeated cumulative snapshots do not double count",
			snapshots:  []string{`{"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`},
			wantTotals: []int{50, 50, 50}, want5m: 20, want1h: 30, billed5m: 20, billed1h: 30,
		},
		{
			name:       "null aggregate derives merged partial TTL",
			snapshots:  []string{`{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":20}}`, `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":null}}`},
			wantTotals: []int{20, 50, 50}, want5m: 20, want1h: 30, billed5m: 20, billed1h: 30,
		},
		{
			name:       "null aggregate does not clear explicit aggregate",
			snapshots:  []string{`{"cache_creation_input_tokens":100,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_1h_input_tokens":null}}`},
			wantTotals: []int{100, 100}, want1h: 200, billed1h: 100,
		},
		{
			name:       "null does not clear explicit zero",
			snapshots:  []string{`{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":200}}`, `{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":20,"ephemeral_1h_input_tokens":null}}`},
			wantTotals: []int{200, 220}, want5m: 20, want1h: 200, billed5m: 20, billed1h: 200,
		},
		{
			name:       "zero to positive retains partial TTL history",
			snapshots:  []string{`{"cache_creation":{"ephemeral_5m_input_tokens":20}}`, `{"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":30}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":50}}`, `{"cache_creation_input_tokens":35}`},
			wantTotals: []int{20, 50, 70, 35}, want5m: 20, want1h: 50, billed5m: 10, billed1h: 25,
		},
		{
			name:       "all null is absent before partial TTL",
			snapshots:  []string{`{"cache_creation_input_tokens":null,"cache_creation":{"ephemeral_5m_input_tokens":null,"ephemeral_1h_input_tokens":null}}`, `{"cache_creation":{"ephemeral_5m_input_tokens":20}}`, `{"cache_creation":{"ephemeral_1h_input_tokens":30}}`},
			wantTotals: []int{0, 20, 50}, want5m: 20, want1h: 30, billed5m: 20, billed1h: 30,
		},
	}
	for _, test := range tests {
		for _, parser := range []string{"shared merge", "raw passthrough", "generic SSE"} {
			t.Run(test.name+"/"+parser, func(t *testing.T) {
				usage := &ClaudeUsage{}
				for index, snapshot := range test.snapshots {
					if parser == "shared merge" {
						var source apicompat.AnthropicUsage
						require.NoError(t, json.Unmarshal([]byte(snapshot), &source))
						mergeAnthropicUsage(usage, source)
					} else if parser == "generic SSE" {
						if index == 0 {
							(&GatewayService{}).parseSSEUsage(`{"type":"message_start","message":{"usage":`+snapshot+`}}`, usage)
						} else {
							(&GatewayService{}).parseSSEUsage(`{"type":"message_delta","usage":`+snapshot+`}`, usage)
						}
					} else if index == 0 {
						parseSSEUsagePassthrough(`{"type":"message_start","message":{"usage":`+snapshot+`}}`, usage)
					} else {
						parseSSEUsagePassthrough(`{"type":"message_delta","usage":`+snapshot+`}`, usage)
					}
					require.Equal(t, test.wantTotals[index], usage.CacheCreationInputTokens)
				}
				require.Equal(t, test.want5m, usage.CacheCreation5mTokens)
				require.Equal(t, test.want1h, usage.CacheCreation1hTokens)
				cache5m, cache1h := normalizeCacheCreationBreakdown(UsageTokens{CacheCreationTokens: usage.CacheCreationInputTokens, CacheCreation5mTokens: usage.CacheCreation5mTokens, CacheCreation1hTokens: usage.CacheCreation1hTokens})
				require.Equal(t, test.billed5m, cache5m)
				require.Equal(t, test.billed1h, cache1h)
				// Billing weights are tested separately; this checks TTL bucket normalization without changing policy.
			})
		}
	}
}
