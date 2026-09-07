-- 239_account_weekly_quota_natural_week.sql
--
-- 批次 6 / A3：上游账号的「周限额」改为按自然周（周一 00:00，项目时区）重置。
--
-- 背景：accounts.extra 里的周限额（quota_weekly_limit）此前默认是「滚动窗口」——
-- 从该账号首次计费起每 168 小时重置一次，重置点跟着首次使用时间漂移，站长看到的
-- 「本周用了多少」和上游账单 / 自己的直觉里的「本周」对不上。用户平台额度早已按
-- 自然周（timezone.StartOfWeek）计算，本次把账号侧对齐过去。
--
-- 做什么：对「已设周限额（quota_weekly_limit > 0）且从未选过重置方式
-- （quota_weekly_reset_mode 缺失）」的账号，一次性改成固定重置：
--   quota_weekly_reset_mode = 'fixed'、quota_weekly_reset_day = 1（周一）、
--   quota_weekly_reset_hour = 0；
--   quota_reset_timezone 若已有（日限额可能早已配过固定重置）则原样保留，否则取
--   数据库会话时区 current_setting('TimeZone')——应用连库 DSN 带 TimeZone=<项目时区>
--   （repository/ent.go），迁移与业务走同一条 DSN，所以这里拿到的就是项目时区；
--   quota_weekly_reset_at = 下周一 00:00（该时区）——必须写：计费 SQL 的
--   weeklyExpiredExpr 把 fixed 模式下缺失的 reset_at 当成 1970，升级后的第一笔
--   计费会把本周用量清零（少扣）；写了之后 Go 侧展示与 SQL 侧计费才是同一口径；
--   quota_weekly_start 早于本周一 00:00（或缺失）的，用量属于上一个自然周：
--   quota_weekly_used 清零、quota_weekly_start 对齐到本周一 00:00；否则本周用量保留。
--
-- 不动谁：已显式选过 fixed / rolling 的账号、没设周限额的账号、软删账号。
--
-- 幂等：条件里的「reset_mode 缺失」在第一次执行后不再成立，重复执行为空更新。
-- 时区名一律先在 pg_timezone_names 里核对，脏值退回会话时区，避免 AT TIME ZONE 报错
-- 把整次启动卡死。

WITH target AS (
    SELECT
        a.id,
        CASE
            -- 没存过时区（绝大多数存量账号）：直接取会话时区，不去扫 pg_timezone_names
            WHEN NULLIF(a.extra->>'quota_reset_timezone', '') IS NULL THEN current_setting('TimeZone')
            ELSE COALESCE(
                (
                    SELECT n.name
                    FROM pg_timezone_names n
                    WHERE n.name = a.extra->>'quota_reset_timezone'
                    LIMIT 1
                ),
                current_setting('TimeZone')
            )
        END AS tz
    FROM accounts a
    WHERE a.deleted_at IS NULL
      AND COALESCE((a.extra->>'quota_weekly_limit')::numeric, 0) > 0
      AND NULLIF(a.extra->>'quota_weekly_reset_mode', '') IS NULL
),
calc AS (
    SELECT
        t.id,
        t.tz,
        -- 本周一 00:00，tz 的本地时间（无时区戳）；下面再用 AT TIME ZONE tz 还原成 timestamptz
        date_trunc('week', NOW() AT TIME ZONE t.tz) AS week_start_local
    FROM target t
)
UPDATE accounts a
SET extra = COALESCE(a.extra, '{}'::jsonb)
    || jsonb_build_object(
        'quota_weekly_reset_mode', 'fixed',
        'quota_weekly_reset_day', 1,
        'quota_weekly_reset_hour', 0,
        'quota_reset_timezone', c.tz,
        'quota_weekly_reset_at', to_char(
            ((c.week_start_local + interval '7 days') AT TIME ZONE c.tz) AT TIME ZONE 'UTC',
            'YYYY-MM-DD"T"HH24:MI:SS"Z"'
        )
    )
    || CASE
        WHEN COALESCE(NULLIF(a.extra->>'quota_weekly_start', '')::timestamptz, '1970-01-01'::timestamptz)
             < (c.week_start_local AT TIME ZONE c.tz)
        THEN jsonb_build_object(
            'quota_weekly_used', 0,
            'quota_weekly_start', to_char(
                (c.week_start_local AT TIME ZONE c.tz) AT TIME ZONE 'UTC',
                'YYYY-MM-DD"T"HH24:MI:SS"Z"'
            )
        )
        ELSE '{}'::jsonb
    END,
    updated_at = NOW()
FROM calc c
WHERE a.id = c.id;
