package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsErrorLogsErrorReasonMigration(t *testing.T) {
	content, err := FS.ReadFile("245_ops_error_logs_error_reason.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS error_reason TEXT")
	require.Contains(t, sql, "ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS request_body_bytes BIGINT")
	require.NotContains(t, strings.ToLower(sql), "update ops_error_logs")
	require.NotContains(t, strings.ToLower(sql), "default")
}
