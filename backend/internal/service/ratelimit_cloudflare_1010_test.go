package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloudflareBotBlockResponse_JSONAndHTML(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		blocked    bool
	}{
		{"html", "<html>Error code: 1010</html>", true},
		{"json_code", `{"cloudflare_error":true,"code":1010}`, true},
		{"nested_error", `{"error":{"cloudflare_error":true,"error_code":"1010"}}`, true},
		{"signature", `{"error_name":"browser_signature_banned"}`, true},
		{"unrelated_1010", `{"error":{"code":1010,"message":"ordinary error"}}`, false},
		{"unrelated_403", `{"error":{"code":403,"message":"permission denied"}}`, false},
		{"random_text", "not available", false},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.blocked, isCloudflareBotBlockResponse([]byte(tc.body))) })
	}
}
