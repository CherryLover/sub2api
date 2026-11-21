package config

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestBackportResourceDefaultsPreserveLitePolicy(t *testing.T) {
	resetViperWithJWTSecret(t)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 5, cfg.Server.ShutdownDrainTimeout)
	require.Zero(t, cfg.Gateway.OpenAIWS.NativeReplayMaxSessionBytes)
	require.EqualValues(t, 64<<20, cfg.Gateway.OpenAIWS.NativeReplayMaxTotalBytes)
	require.Equal(t, 8192, cfg.Gateway.OpenAIWS.NativeReplayMaxItems)
	require.Zero(t, cfg.Gateway.RequestBodyMemoryBudgetBytes)
}
func TestBackportResourceExplicitLimitsAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"explicit", "server:\n  shutdown_drain_timeout: 25\ngateway:\n  openai_ws:\n    native_replay_max_session_bytes: 4096\n    native_replay_max_total_bytes: 8192\n    native_replay_max_items: 32\n", true},
		{"negative session", "gateway:\n  openai_ws:\n    native_replay_max_session_bytes: -1\n", false},
		{"negative total", "gateway:\n  openai_ws:\n    native_replay_max_total_bytes: -1\n", false},
		{"negative items", "gateway:\n  openai_ws:\n    native_replay_max_items: -1\n", false},
		{"zero drain", "server:\n  shutdown_drain_timeout: 0\n", false},
		{"overflow drain", "server:\n  shutdown_drain_timeout: 9223372036854775807\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			p := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(p, []byte(tc.body), 0600))
			t.Setenv("CONFIG_FILE", p)
			cfg, err := Load()
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 25, cfg.Server.ShutdownDrainTimeout)
			require.EqualValues(t, 4096, cfg.Gateway.OpenAIWS.NativeReplayMaxSessionBytes)
			require.EqualValues(t, 8192, cfg.Gateway.OpenAIWS.NativeReplayMaxTotalBytes)
			require.Equal(t, 32, cfg.Gateway.OpenAIWS.NativeReplayMaxItems)
		})
	}
}
