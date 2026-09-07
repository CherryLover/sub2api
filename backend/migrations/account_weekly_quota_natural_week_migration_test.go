package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const accountWeeklyQuotaNaturalWeekMigration = "239_account_weekly_quota_natural_week.sql"

// TestMigration239TouchesExactlyTheWeeklyQuotaKeysGoReads 锁住 239 读写的 extra 键名
// 必须和 service/account.go 的 GetQuotaWeekly* / repository/account_repo.go 的
// weeklyExpiredExpr 用的是同一套；改键名会让计费与展示各说各话。
// 真正的行为（谁被改、改成什么、幂等）由 repository 包的集成测试
// TestMigration239AnchorsAccountWeeklyQuotaToNaturalWeek 覆盖。
func TestMigration239TouchesExactlyTheWeeklyQuotaKeysGoReads(t *testing.T) {
	content, err := FS.ReadFile(accountWeeklyQuotaNaturalWeekMigration)
	require.NoError(t, err)
	sql := string(content)

	for _, key := range []string{
		"quota_weekly_limit",
		"quota_weekly_reset_mode",
		"quota_weekly_reset_day",
		"quota_weekly_reset_hour",
		"quota_reset_timezone",
		"quota_weekly_reset_at",
		"quota_weekly_used",
		"quota_weekly_start",
	} {
		require.Containsf(t, sql, "'"+key+"'", "239 必须直接使用键名 %s", key)
	}

	// 缺省时区来自会话时区（DSN TimeZone=<项目时区>），不能再硬编码 UTC
	require.Contains(t, sql, "current_setting('TimeZone')")
	require.NotContains(t, strings.ToUpper(sql), "'UTC' AS TZ")
	// 自然周 = date_trunc('week')（ISO 周，周一起算）
	require.Contains(t, sql, "date_trunc('week'")
	// 只碰「设了周限额且从未选过重置方式」的活账号
	require.Contains(t, sql, "deleted_at IS NULL")
	require.Contains(t, sql, "NULLIF(a.extra->>'quota_weekly_reset_mode', '') IS NULL")
	// 写下次重置点是硬要求（否则 fixed 模式下缺 reset_at 会被计费当 1970 直接清零）
	require.Contains(t, sql, "'quota_weekly_reset_at', to_char(")
}
