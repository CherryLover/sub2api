package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsErrorLogsReasoningEffortMigration(t *testing.T) {
	content, err := FS.ReadFile("246_ops_error_logs_reasoning_effort.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS reasoning_effort VARCHAR(20)")
	require.NotContains(t, strings.ToLower(sql), "update ops_error_logs")
	require.NotContains(t, strings.ToLower(sql), "default")
}
