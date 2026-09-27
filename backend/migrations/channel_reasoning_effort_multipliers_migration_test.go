package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelReasoningEffortMultipliersMigration(t *testing.T) {
	content, err := FS.ReadFile("241_channel_reasoning_effort_multipliers.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")

	require.Contains(t, sql, "ALTER TABLE channel_model_pricing ADD COLUMN IF NOT EXISTS reasoning_effort_multipliers JSONB NOT NULL DEFAULT '{}'::jsonb")
	require.Contains(t, sql, "ALTER TABLE channel_account_stats_model_pricing ADD COLUMN IF NOT EXISTS reasoning_effort_multipliers JSONB NOT NULL DEFAULT '{}'::jsonb")
	require.Contains(t, sql, "COMMENT ON COLUMN channel_model_pricing.reasoning_effort_multipliers")
	require.Contains(t, sql, "COMMENT ON COLUMN channel_account_stats_model_pricing.reasoning_effort_multipliers")

	// Slim never shipped the upstream legacy single-max column/key. Referencing it
	// would make this migration fail against the real slim pre-241 schema.
	require.NotContains(t, sql, "max_reasoning_effort_multiplier")
	require.NotContains(t, strings.ToLower(sql), "model_plaza")
	require.NotContains(t, strings.ToLower(sql), "fable")
}
