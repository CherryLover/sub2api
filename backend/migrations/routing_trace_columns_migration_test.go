package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsErrorLogsRoutingDiagnosisMigration(t *testing.T) {
	content, err := FS.ReadFile("247_ops_error_logs_routing_diagnosis.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE ops_error_logs ADD COLUMN IF NOT EXISTS routing_diagnosis JSONB")
	lower := strings.ToLower(sql)
	require.NotContains(t, lower, "update ops_error_logs")
	require.NotContains(t, lower, "default")
	require.NotContains(t, lower, "not null")
}

func TestUsageLogsRoutingInfoMigration(t *testing.T) {
	content, err := FS.ReadFile("248_usage_logs_routing_info.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS routing_info JSONB")
	lower := strings.ToLower(sql)
	require.NotContains(t, lower, "update usage_logs")
	require.NotContains(t, lower, "default")
	require.NotContains(t, lower, "not null")
}
