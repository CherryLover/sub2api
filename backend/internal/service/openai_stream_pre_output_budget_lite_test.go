//go:build unit

package service

import "testing"

func TestOpenAIStreamPreOutputTimeBudgetErrorRetries(t *testing.T) {
	cases := []struct {
		name, payload, message string
		retry                  bool
	}{
		{"pre-output exhaustion", `{"type":"error","status_code":200,"error":{"message":"Upstream pre-output time budget exhausted"}}`, "", true},
		{"nested error", `{"type":"error","response":{"error":{"message":"pre-output time budget exhausted"}}}`, "", true},
		{"wrong event", `{"type":"response.output_text.delta","error":{"message":"pre-output time budget exhausted"}}`, "", false},
		{"context overflow", `{"type":"error","error":{"message":"context window exceeded"}}`, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := openAIStreamErrorEventShouldFailover([]byte(c.payload), c.message); got != c.retry {
				t.Fatalf("retry=%v, want %v", got, c.retry)
			}
		})
	}
}
