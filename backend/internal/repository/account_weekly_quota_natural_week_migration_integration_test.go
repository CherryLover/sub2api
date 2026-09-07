//go:build integration

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

const weeklyQuotaNaturalWeekMigration = "239_account_weekly_quota_natural_week.sql"

// pgUTCStamp 与迁移里 to_char(... AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') 同格式。
func pgUTCStamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// startOfWeekIn 返回 now 所在自然周的周一 00:00（loc 本地时间）。
func startOfWeekIn(now time.Time, loc *time.Location) time.Time {
	local := now.In(loc)
	weekday := int(local.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return time.Date(local.Year(), local.Month(), local.Day()-weekday+1, 0, 0, 0, 0, loc)
}

func insertMigration239Account(t *testing.T, tx *sql.Tx, name string, extra map[string]any, deleted bool) int64 {
	t.Helper()
	raw, err := json.Marshal(extra)
	require.NoError(t, err)
	var id int64
	if deleted {
		require.NoError(t, tx.QueryRowContext(context.Background(), `
INSERT INTO accounts (name, platform, type, extra, deleted_at)
VALUES ($1, 'anthropic', 'apikey', $2::jsonb, NOW())
RETURNING id`, name, string(raw)).Scan(&id))
		return id
	}
	require.NoError(t, tx.QueryRowContext(context.Background(), `
INSERT INTO accounts (name, platform, type, extra)
VALUES ($1, 'anthropic', 'apikey', $2::jsonb)
RETURNING id`, name, string(raw)).Scan(&id))
	return id
}

func readMigration239Extra(t *testing.T, tx *sql.Tx, id int64) (map[string]any, string) {
	t.Helper()
	var text string
	require.NoError(t, tx.QueryRowContext(context.Background(),
		`SELECT extra::text FROM accounts WHERE id = $1`, id).Scan(&text))
	var extra map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &extra))
	return extra, text
}

// TestMigration239AnchorsAccountWeeklyQuotaToNaturalWeek 覆盖 239 的全部分支：
// 谁会被改成自然周固定重置、改成什么、用量什么时候保留 / 清零、谁绝不能被动，
// 以及重复执行不再改动任何一行。
func TestMigration239AnchorsAccountWeeklyQuotaToNaturalWeek(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	migrationSQL, err := dbmigrations.FS.ReadFile(weeklyQuotaNaturalWeekMigration)
	require.NoError(t, err)

	// 生产连库 DSN 带 TimeZone=<项目时区>；harness 是 UTC，这里切成上海，
	// 才能证明「缺省时区 = 会话时区」和「本周一按该时区算」都真的生效。
	_, err = tx.ExecContext(ctx, "SET LOCAL TimeZone = 'Asia/Shanghai'")
	require.NoError(t, err)

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err)

	now := time.Now()
	weekStartSH := startOfWeekIn(now, shanghai)
	nextWeekSH := time.Date(weekStartSH.Year(), weekStartSH.Month(), weekStartSH.Day()+7, 0, 0, 0, 0, shanghai)
	weekStartTokyo := startOfWeekIn(now, tokyo)
	nextWeekTokyo := time.Date(weekStartTokyo.Year(), weekStartTokyo.Month(), weekStartTokyo.Day()+7, 0, 0, 0, 0, tokyo)

	// 1. 典型存量账号：滚动窗口、起点在上周 → 改固定 + 清零
	staleID := insertMigration239Account(t, tx, "m239-stale", map[string]any{
		"quota_weekly_limit": 500,
		"quota_weekly_used":  76.5,
		"quota_weekly_start": weekStartSH.Add(-24 * time.Hour).UTC().Format(time.RFC3339),
	}, false)

	// 2. 滚动窗口、起点在本周内 → 改固定，但本周用量保留
	activeStart := weekStartSH.Add(time.Second).UTC().Format(time.RFC3339)
	activeID := insertMigration239Account(t, tx, "m239-active", map[string]any{
		"quota_weekly_limit": 500,
		"quota_weekly_used":  30,
		"quota_weekly_start": activeStart,
	}, false)

	// 3. 日限额早已配过固定重置（时区东京）、周限额还是滚动 → 时区沿用东京，本周一按东京算
	tokyoID := insertMigration239Account(t, tx, "m239-tokyo", map[string]any{
		"quota_daily_limit":      50,
		"quota_daily_reset_mode": "fixed",
		"quota_daily_reset_hour": 9,
		"quota_reset_timezone":   "Asia/Tokyo",
		"quota_weekly_limit":     100,
		"quota_weekly_used":      5,
		"quota_weekly_start":     weekStartTokyo.Add(-time.Hour).UTC().Format(time.RFC3339),
	}, false)

	// 4. 存了脏时区名 → 不能让整次迁移报错，退回会话时区
	badTzID := insertMigration239Account(t, tx, "m239-bad-tz", map[string]any{
		"quota_reset_timezone": "Mars/Olympus",
		"quota_weekly_limit":   100,
		"quota_weekly_used":    1,
	}, false)

	// 以下全部必须原样不动
	fixedID := insertMigration239Account(t, tx, "m239-fixed", map[string]any{
		"quota_weekly_limit":      200,
		"quota_weekly_used":       42,
		"quota_weekly_start":      weekStartSH.Add(-72 * time.Hour).UTC().Format(time.RFC3339),
		"quota_weekly_reset_mode": "fixed",
		"quota_weekly_reset_day":  3,
		"quota_weekly_reset_hour": 9,
		"quota_reset_timezone":    "America/New_York",
		"quota_weekly_reset_at":   "2099-01-06T14:00:00Z",
	}, false)
	rollingID := insertMigration239Account(t, tx, "m239-explicit-rolling", map[string]any{
		"quota_weekly_limit":      200,
		"quota_weekly_used":       42,
		"quota_weekly_reset_mode": "rolling",
	}, false)
	zeroLimitID := insertMigration239Account(t, tx, "m239-zero-limit", map[string]any{
		"quota_weekly_limit": 0,
		"quota_weekly_used":  42,
	}, false)
	noWeeklyID := insertMigration239Account(t, tx, "m239-no-weekly", map[string]any{
		"quota_daily_limit": 10,
		"quota_daily_used":  3,
	}, false)
	deletedID := insertMigration239Account(t, tx, "m239-deleted", map[string]any{
		"quota_weekly_limit": 500,
		"quota_weekly_used":  76,
	}, true)

	untouched := map[string]int64{
		"already fixed":    fixedID,
		"explicit rolling": rollingID,
		"limit = 0":        zeroLimitID,
		"no weekly quota":  noWeeklyID,
		"soft deleted":     deletedID,
	}
	before := map[string]string{}
	for label, id := range untouched {
		_, before[label] = readMigration239Extra(t, tx, id)
	}

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	assertNaturalWeek := func(extra map[string]any, tz string, nextWeek time.Time) {
		t.Helper()
		require.Equal(t, "fixed", extra["quota_weekly_reset_mode"])
		require.EqualValues(t, 1, extra["quota_weekly_reset_day"])
		require.EqualValues(t, 0, extra["quota_weekly_reset_hour"])
		require.Equal(t, tz, extra["quota_reset_timezone"])
		require.Equal(t, pgUTCStamp(nextWeek), extra["quota_weekly_reset_at"])
	}

	stale, _ := readMigration239Extra(t, tx, staleID)
	assertNaturalWeek(stale, "Asia/Shanghai", nextWeekSH)
	require.EqualValues(t, 0, stale["quota_weekly_used"], "上周用量清零")
	require.Equal(t, pgUTCStamp(weekStartSH), stale["quota_weekly_start"], "起点对齐到本周一 00:00（上海）")
	require.EqualValues(t, 500, stale["quota_weekly_limit"], "限额本身不动")

	active, _ := readMigration239Extra(t, tx, activeID)
	assertNaturalWeek(active, "Asia/Shanghai", nextWeekSH)
	require.EqualValues(t, 30, active["quota_weekly_used"], "本周用量保留")
	require.Equal(t, activeStart, active["quota_weekly_start"], "本周内的起点不动")

	tokyoExtra, _ := readMigration239Extra(t, tx, tokyoID)
	assertNaturalWeek(tokyoExtra, "Asia/Tokyo", nextWeekTokyo)
	require.EqualValues(t, 0, tokyoExtra["quota_weekly_used"])
	require.Equal(t, pgUTCStamp(weekStartTokyo), tokyoExtra["quota_weekly_start"])
	require.Equal(t, "fixed", tokyoExtra["quota_daily_reset_mode"], "日限额配置不受影响")
	require.EqualValues(t, 9, tokyoExtra["quota_daily_reset_hour"])

	badTz, _ := readMigration239Extra(t, tx, badTzID)
	assertNaturalWeek(badTz, "Asia/Shanghai", nextWeekSH)
	require.EqualValues(t, 0, badTz["quota_weekly_used"], "没有起点 = 从未计费，残留用量清零")
	require.Equal(t, pgUTCStamp(weekStartSH), badTz["quota_weekly_start"])

	for label, id := range untouched {
		_, after := readMigration239Extra(t, tx, id)
		require.Equalf(t, before[label], after, "%s 的 extra 不该被 239 改动", label)
	}

	// 幂等：第二次执行不再改任何一行
	snapshot := map[int64]string{}
	for _, id := range []int64{staleID, activeID, tokyoID, badTzID, fixedID, rollingID, zeroLimitID, noWeeklyID, deletedID} {
		_, snapshot[id] = readMigration239Extra(t, tx, id)
	}
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	for id, want := range snapshot {
		_, got := readMigration239Extra(t, tx, id)
		require.Equalf(t, want, got, "account %d 在第二次执行后发生了变化", id)
	}
}

// TestMigration239ResetAtAgreesWithBillingExpiry 证明迁移写出的 reset_at / start
// 和计费 SQL（weeklyExpiredExpr）是同一口径：升级后的第一笔计费不会把本周用量清零，
// 而是继续累加；等固定重置点一过才重置。
func TestMigration239ResetAtAgreesWithBillingExpiry(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	migrationSQL, err := dbmigrations.FS.ReadFile(weeklyQuotaNaturalWeekMigration)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "SET LOCAL TimeZone = 'Asia/Shanghai'")
	require.NoError(t, err)

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	weekStartSH := startOfWeekIn(time.Now(), shanghai)

	id := insertMigration239Account(t, tx, "m239-billing", map[string]any{
		"quota_weekly_limit": 500,
		"quota_weekly_used":  30,
		"quota_weekly_start": weekStartSH.Add(time.Second).UTC().Format(time.RFC3339),
	}, false)

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	var expired bool
	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT `+weeklyExpiredExpr+` FROM accounts WHERE id = $1`, id).Scan(&expired))
	require.False(t, expired, "迁移写入的 reset_at 在未来，计费不应把本周判成过期")

	var nextResetAt string
	require.NoError(t, tx.QueryRowContext(ctx,
		`SELECT `+nextWeeklyResetAtExpr+` FROM accounts WHERE id = $1`, id).Scan(&nextResetAt))
	extra, _ := readMigration239Extra(t, tx, id)
	require.Equal(t, extra["quota_weekly_reset_at"], nextResetAt,
		"迁移算出的下次重置点必须和计费 SQL 自己算的一致")
}
