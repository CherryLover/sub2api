package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsAlertEventDeliveryMigration(t *testing.T) {
	content, err := FS.ReadFile("242_ops_alert_event_delivery.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")

	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS urgency VARCHAR(16) NOT NULL DEFAULT 'observe'")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS delivery VARCHAR(32) NOT NULL DEFAULT 'none'")
	require.Contains(t, sql, "FROM ops_alert_rules AS r WHERE e.rule_id = r.id")
	require.Contains(t, sql, "COALESCE(r.notify_email, true)")
	require.Contains(t, sql, "THEN 'bark_realtime'")
	require.Contains(t, sql, "WHEN e.urgency IN ('immediate', 'observe') THEN 'in_app'")
}
