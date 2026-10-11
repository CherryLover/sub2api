//go:build unit

package service

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestOpenAIModelRefusalClassifier(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	cases := []struct {
		status int
		body   string
		want   bool
	}{
		{http.StatusNotFound, ` {"error":{"code":"model_not_found","message":"model not found"}}`, true},
		{http.StatusBadGateway, ` {"error":{"message":"bad gateway"}}`, false},
		{http.StatusUnauthorized, ` {"error":{"message":"invalid key"}}`, false},
	}
	for _, tc := range cases {
		_, _, got := upstreamModelUnavailableCooldown(account, tc.status, []byte(tc.body))
		require.Equal(t, tc.want, got)
	}
}
