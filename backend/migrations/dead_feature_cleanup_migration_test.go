package migrations

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// deadFeatureTables 是批次 5（A5 数据库压平）删掉的全部表。
// 这些功能的业务代码与 ent 实体都已移除，任何迁移都不应再重建它们。
var deadFeatureTables = []string{
	"announcement_reads",
	"announcements",
	"batch_image_events",
	"batch_image_items",
	"batch_image_jobs",
	"channel_monitor_aggregation_watermark",
	"channel_monitor_daily_rollups",
	"channel_monitor_histories",
	"channel_monitor_request_templates",
	"channel_monitors",
	"content_moderation_logs",
	"payment_audit_logs",
	"payment_orders",
	"payment_provider_instances",
	"promo_code_usages",
	"promo_codes",
	"redeem_codes",
	"subscription_plans",
	"user_affiliate_ledger",
	"user_affiliates",
	"user_subscriptions",
}

const dropDeadTablesMigration = "235_drop_dead_feature_tables.sql"

func TestMigration235DropsEveryDeadFeatureTable(t *testing.T) {
	content, err := FS.ReadFile(dropDeadTablesMigration)
	require.NoError(t, err)
	sql := string(content)

	for _, table := range deadFeatureTables {
		// CASCADE 是必需的：usage_logs.subscription_id 上挂着指向 user_subscriptions
		// 的外键，promo_code_usages / announcement_reads 等表之间也互相引用。
		require.Containsf(t, sql, "DROP TABLE IF EXISTS "+table+" CASCADE;",
			"%s 必须以幂等 + CASCADE 的方式删除 %s", dropDeadTablesMigration, table)
	}
}

func TestMigration235KeepsChannelMonitorV2Tables(t *testing.T) {
	content, err := FS.ReadFile(dropDeadTablesMigration)
	require.NoError(t, err)
	// V2 被动监控仍在使用 channel_monitor_v2_* 系列表，绝不能被 V1 清理误伤。
	// 只检查真正的 SQL 语句，注释里提到 V2 是为了说明「不删它」。
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		require.NotContains(t, line, "channel_monitor_v2")
	}
}

// TestNoMigrationRecreatesDeadFeatureTables 保证 235 之后不会有人再把这些表建回来。
func TestNoMigrationRecreatesDeadFeatureTables(t *testing.T) {
	names, err := fs.Glob(FS, "*.sql")
	require.NoError(t, err)
	sort.Strings(names)

	for _, name := range names {
		if name <= dropDeadTablesMigration {
			continue
		}
		content, err := FS.ReadFile(name)
		require.NoError(t, err)
		body := strings.ToLower(string(content))
		for _, table := range deadFeatureTables {
			pattern := regexp.MustCompile(`create table\s+(if not exists\s+)?` + table + `\b`)
			require.Falsef(t, pattern.MatchString(body),
				"迁移 %s 重建了已删除的表 %s", name, table)
		}
	}
}

// TestMigration237KeepsDeliberatelyRetainedSettingKeys 锁住三个「看着像残留、
// 但必须保留」的设置键，避免后续清理误删。
func TestMigration237KeepsDeliberatelyRetainedSettingKeys(t *testing.T) {
	content, err := FS.ReadFile("237_drop_orphan_feature_settings.sql")
	require.NoError(t, err)
	sql := string(content)

	for _, key := range []string{
		// migration 229 靠它兜底「回滚后不会退化成开放注册」
		"registration_enabled",
		// migration 231 靠它兜底「回滚后旧代码直接走 V2 被动聚合」
		"channel_monitor_mode",
		// InitializeDefaultSettings 用它判断是否已种过默认设置
		"registration_email_suffix_whitelist",
	} {
		require.NotContainsf(t, sql, "'"+key+"'",
			"237 不应删除刻意保留的设置键 %s", key)
	}
}
