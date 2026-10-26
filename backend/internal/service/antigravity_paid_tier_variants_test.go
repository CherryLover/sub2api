package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAntigravityPaidTierVariantsGate(t *testing.T) {
	tests := []struct {
		tier string
		paid bool
	}{
		{"Pro", true},
		{"Ultra", true},
		{"g1-pro-plus-tier", true},
		{"g1-ultra-lite-tier", true},
		{"free-tier", false},
		{"unknown-tier", false},
	}
	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			account := &Account{Platform: PlatformAntigravity, Credentials: map[string]any{"plan_type": tt.tier}}
			require.Equal(t, tt.paid, accountHasAntigravityPaidTier(account))
		})
	}
}
