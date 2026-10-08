package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsAlertNoiseReductionV2Migration(t *testing.T) {
	content, err := FS.ReadFile("243_ops_alert_noise_reduction_v2.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")

	require.Contains(t, sql, "UPDATE ops_alert_rules SET sustained_minutes = 5")
	require.Contains(t, sql, "name = '错误率极高'")
	require.Contains(t, sql, "threshold = 20.0")
	require.Contains(t, sql, "sustained_minutes = 1")
	require.NotContains(t, sql, "UPDATE ops_alert_events", "历史告警记录必须保留当时真实的 urgency / delivery")
}
