package service

import "testing"

func TestParseNoAccountReason(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "model not supported",
			text: "no available OpenAI accounts supporting model: gpt-5 (pool=4, filtered: model_not_supported=4)",
			want: OpsNoAccountReasonModelNotSupported,
		},
		{
			name: "highest count wins",
			text: "no available OpenAI accounts supporting model: gpt-5 (pool=8, filtered: excluded=4 model_not_supported=3 not_schedulable=1)",
			want: OpsNoAccountReasonExcluded,
		},
		{
			name: "rate limited",
			text: "no available accounts supporting model: gpt-5 (pool=7, filtered: model_rate_limited=7)",
			want: OpsNoAccountReasonModelRateLimited,
		},
		{
			name: "empty pool",
			text: "no available accounts supporting model: gpt-5 (pool=0)",
			want: OpsNoAccountReasonEmptyPool,
		},
		{
			name: "unknown",
			text: "no available accounts",
			want: OpsNoAccountReasonUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseNoAccountReason(tt.text); got != tt.want {
				t.Fatalf("ParseNoAccountReason(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestParseNoAccountReasonEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "empty", text: "", want: OpsNoAccountReasonUnknown},
		{name: "whitespace only", text: "   ", want: OpsNoAccountReasonUnknown},
		{
			name: "tie breaks alphabetically for a stable kind",
			text: "no available accounts (pool=4, filtered: not_schedulable=2 excluded=2)",
			want: OpsNoAccountReasonExcluded,
		},
		{
			name: "case insensitive",
			text: "No Available OpenAI Accounts (POOL=3, FILTERED: MODEL_RATE_LIMITED=3)",
			want: OpsNoAccountReasonModelRateLimited,
		},
		{
			name: "malformed counts are ignored",
			text: "no available accounts (pool=3, filtered: excluded=abc model_not_supported=3)",
			want: OpsNoAccountReasonModelNotSupported,
		},
		{
			name: "filtered without usable entries falls back to empty pool",
			text: "no available accounts (pool=0, filtered: )",
			want: OpsNoAccountReasonEmptyPool,
		},
		{
			name: "unrecognised reason codes pass through",
			text: "no available accounts (pool=2, filtered: quota_auto_pause_7d=2)",
			want: "quota_auto_pause_7d",
		},
		{
			name: "pool=10 is not mistaken for an empty pool",
			text: "no available accounts (pool=10)",
			want: OpsNoAccountReasonUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseNoAccountReason(tt.text); got != tt.want {
				t.Fatalf("ParseNoAccountReason(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
