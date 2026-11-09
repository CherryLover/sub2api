package service

import (
	"testing"
	"time"
)

func TestMonthlyFixedResetCalendarAndDST(t *testing.T) {
	tests := []struct {
		name, zone, after, want string
		day, hour               int
	}{
		{"short month", "UTC", "2025-02-01T00:00:00Z", "2025-02-28T07:00:00Z", 31, 7},
		{"restore original day", "UTC", "2025-02-28T07:00:00Z", "2025-03-31T07:00:00Z", 31, 7},
		{"leap", "UTC", "2024-02-01T00:00:00Z", "2024-02-29T00:00:00Z", 31, 0},
		{"year rollover", "UTC", "2025-12-31T23:00:00Z", "2026-01-31T23:00:00Z", 31, 23},
		{"DST gap PostgreSQL standard offset", "America/New_York", "2025-03-01T00:00:00Z", "2025-03-09T07:00:00Z", 9, 2},
		{"DST fold PostgreSQL later instant", "America/New_York", "2025-11-01T00:00:00Z", "2025-11-02T06:00:00Z", 2, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tt.zone)
			if err != nil {
				t.Fatal(err)
			}
			after, _ := time.Parse(time.RFC3339, tt.after)
			want, _ := time.Parse(time.RFC3339, tt.want)
			got := nextFixedMonthlyReset(tt.day, tt.hour, loc, after)
			if !got.Equal(want) {
				t.Fatalf("got %s want %s", got, want)
			}
			if last := lastFixedMonthlyReset(tt.day, tt.hour, loc, want); !last.Equal(want) {
				t.Fatalf("boundary last=%s want=%s", last, want)
			}
		})
	}
}

func TestMonthlyQuotaRollingAndScheduling(t *testing.T) {
	now := time.Now()
	a := Account{Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Extra: map[string]any{"quota_monthly_limit": 10.0, "quota_monthly_used": 10.0, "quota_monthly_start": now.Add(-719 * time.Hour).Format(time.RFC3339)}}
	if !a.HasAnyQuotaLimit() || a.IsMonthlyQuotaPeriodExpired() || !a.IsQuotaExceeded() || a.IsSchedulable() {
		t.Fatal("active monthly limit must stop scheduling")
	}
	a.Extra["quota_monthly_start"] = now.Add(-720*time.Hour - time.Second).Format(time.RFC3339)
	if !a.IsMonthlyQuotaPeriodExpired() || a.IsQuotaExceeded() || !a.IsSchedulable() {
		t.Fatal("expired rolling period must be reusable")
	}
	a.Extra["quota_monthly_limit"] = 0.0
	if a.HasAnyQuotaLimit() || a.IsQuotaExceeded() {
		t.Fatal("zero must disable monthly quota")
	}
	if (&Account{}).GetQuotaMonthlyResetMode() != "rolling" || (&Account{}).HasAnyQuotaLimit() {
		t.Fatal("legacy accounts must remain disabled and rolling")
	}
}

func TestMonthlyQuotaNormalizationPreservesActiveUsage(t *testing.T) {
	extra := map[string]any{"quota_monthly_limit": 10.0, "quota_monthly_used": 7.0, "quota_monthly_start": time.Now().UTC().Format(time.RFC3339), "quota_monthly_reset_mode": "fixed", "quota_monthly_reset_day": 1, "quota_monthly_reset_hour": 0, "quota_reset_timezone": "UTC"}
	ComputeQuotaResetAt(extra)
	NormalizeFixedQuotaWindows(extra)
	if extra["quota_monthly_used"] != 7.0 {
		t.Fatal("amount-only edits must preserve active usage")
	}
	extra["quota_monthly_start"] = time.Now().AddDate(0, -2, 0).UTC().Format(time.RFC3339)
	NormalizeFixedQuotaWindows(extra)
	if extra["quota_monthly_used"] != 0.0 {
		t.Fatal("stale usage must reset at new boundary")
	}
	extra["quota_monthly_reset_mode"] = "rolling"
	ComputeQuotaResetAt(extra)
	if _, ok := extra["quota_monthly_reset_at"]; ok {
		t.Fatal("rolling must remove fixed reset_at")
	}
}

func TestMonthlyQuotaComputePersistsServerTimezone(t *testing.T) {
	extra := map[string]any{"quota_monthly_reset_mode": "fixed", "quota_monthly_limit": 1.0}
	ComputeQuotaResetAt(extra)
	if extra["quota_reset_timezone"] != defaultQuotaResetTimezone() {
		t.Fatalf("monthly SQL must retain server timezone: %#v", extra)
	}
}

func TestMonthlyQuotaValidation(t *testing.T) {
	for _, extra := range []map[string]any{
		{"quota_monthly_reset_day": 0}, {"quota_monthly_reset_day": 32}, {"quota_monthly_reset_day": 1.5},
		{"quota_monthly_reset_hour": 24}, {"quota_monthly_reset_hour": -1}, {"quota_monthly_reset_hour": "bad"},
		{"quota_monthly_reset_mode": "other"}, {"quota_monthly_reset_mode": true},
		{"quota_monthly_limit": -1.0}, {"quota_monthly_limit": "bad"},
		{"quota_monthly_prorate": "true"}, {"quota_monthly_prorate": nil},
	} {
		if ValidateQuotaResetConfig(extra) == nil {
			t.Errorf("accepted invalid config %#v", extra)
		}
	}
	if err := ValidateQuotaResetConfig(map[string]any{"quota_monthly_reset_mode": "fixed", "quota_monthly_reset_day": 31, "quota_monthly_reset_hour": 23}); err != nil {
		t.Fatal(err)
	}
}

func TestCalculateProratedMonthlyQuotaLimit(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		limit      float64
		cycleStart time.Time
		cycleEnd   time.Time
		from       time.Time
		want       float64
	}{
		{
			name:       "full period",
			limit:      100,
			cycleStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			cycleEnd:   time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			from:       time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			want:       100,
		},
		{
			name:       "half period",
			limit:      100,
			cycleStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			cycleEnd:   time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			from:       time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC),
			want:       50,
		},
		{
			name:       "short month",
			limit:      100,
			cycleStart: time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
			cycleEnd:   time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC),
			from:       time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC),
			want:       28.57,
		},
		{
			name:       "cross month",
			limit:      123,
			cycleStart: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			cycleEnd:   time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
			from:       time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			want:       55.55,
		},
		{
			name:       "shanghai period has no dst",
			limit:      50,
			cycleStart: time.Date(2026, 11, 1, 0, 0, 0, 0, shanghai),
			cycleEnd:   time.Date(2026, 12, 1, 0, 0, 0, 0, shanghai),
			from:       time.Date(2026, 11, 16, 17, 0, 0, 0, shanghai),
			want:       23.82,
		},
		{
			name:       "exactly at reset point",
			limit:      100,
			cycleStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			cycleEnd:   time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			from:       time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			want:       0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateProratedMonthlyQuotaLimit(tt.limit, tt.cycleStart, tt.cycleEnd, tt.from)
			if got < tt.want-0.000001 || got > tt.want+0.000001 {
				t.Fatalf("got %.2f want %.2f", got, tt.want)
			}
		})
	}
}

func TestQuotaMonthlyEffectiveLimit(t *testing.T) {
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	extra := map[string]any{
		"quota_monthly_limit":          100.0,
		"quota_monthly_reset_mode":     "fixed",
		"quota_monthly_reset_day":      1,
		"quota_monthly_reset_hour":     0,
		"quota_reset_timezone":         "UTC",
		"quota_monthly_prorate":        true,
		"quota_monthly_prorated_limit": 42.5,
		"quota_monthly_prorated_until": now.Add(time.Hour).Format(time.RFC3339),
	}
	a := &Account{Extra: extra}
	if got := a.GetQuotaMonthlyEffectiveLimit(now); got != 42.5 {
		t.Fatalf("before until: got %v", got)
	}
	if got := a.GetQuotaMonthlyEffectiveLimit(now.Add(time.Hour)); got != 100 {
		t.Fatalf("at until: got %v", got)
	}
	if got := a.GetQuotaMonthlyEffectiveLimit(now.Add(2 * time.Hour)); got != 100 {
		t.Fatalf("after until: got %v", got)
	}
	extra["quota_monthly_reset_mode"] = "rolling"
	if got := a.GetQuotaMonthlyEffectiveLimit(now); got != 100 {
		t.Fatalf("rolling must ignore proration: got %v", got)
	}
	extra["quota_monthly_reset_mode"] = "fixed"
	extra["quota_monthly_prorated_limit"] = -1.0
	if got := a.GetQuotaMonthlyEffectiveLimit(now); got != 100 {
		t.Fatalf("negative proration must fall back: got %v", got)
	}
	extra["quota_monthly_prorated_limit"] = 0.0
	if got := a.GetQuotaMonthlyEffectiveLimit(now); got != 0 {
		t.Fatalf("zero is a valid prorated limit: got %v", got)
	}
	delete(extra, "quota_monthly_prorated_limit")
	if got := a.GetQuotaMonthlyEffectiveLimit(now); got != 100 {
		t.Fatalf("missing proration must fall back: got %v", got)
	}
}

func TestIsQuotaExceededUsesMonthlyEffectiveLimit(t *testing.T) {
	now := time.Now()
	periodStart := lastFixedMonthlyReset(1, 0, time.UTC, now)
	extra := map[string]any{
		"quota_monthly_limit":          100.0,
		"quota_monthly_used":           45.0,
		"quota_monthly_start":          periodStart.UTC().Format(time.RFC3339),
		"quota_monthly_reset_mode":     "fixed",
		"quota_monthly_reset_day":      1,
		"quota_monthly_reset_hour":     0,
		"quota_reset_timezone":         "UTC",
		"quota_monthly_reset_at":       now.Add(time.Hour).UTC().Format(time.RFC3339),
		"quota_monthly_prorate":        true,
		"quota_monthly_prorated_limit": 40.0,
		"quota_monthly_prorated_until": now.Add(time.Hour).UTC().Format(time.RFC3339),
	}
	a := &Account{Type: AccountTypeAPIKey, Extra: extra}
	if !a.IsQuotaExceeded() {
		t.Fatal("used above prorated limit must be exceeded")
	}
	extra["quota_monthly_used"] = 39.99
	if a.IsQuotaExceeded() {
		t.Fatal("used below prorated limit must not be exceeded")
	}
	extra["quota_monthly_used"] = 45.0
	extra["quota_monthly_prorated_until"] = now.Add(-time.Hour).UTC().Format(time.RFC3339)
	if a.IsQuotaExceeded() {
		t.Fatal("expired proration must fall back to the raw limit")
	}
}

func TestApplyMonthlyQuotaProrationTriggersAndClears(t *testing.T) {
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	newExtra := func(limit float64, day, hour int, timezone string, prorate bool) map[string]any {
		return map[string]any{
			"quota_monthly_limit":      limit,
			"quota_monthly_reset_mode": "fixed",
			"quota_monthly_reset_day":  day,
			"quota_monthly_reset_hour": hour,
			"quota_reset_timezone":     timezone,
			"quota_monthly_prorate":    prorate,
		}
	}
	base := newExtra(100, 1, 0, "Asia/Shanghai", true)
	base["quota_monthly_prorated_limit"] = 12.34
	base["quota_monthly_prorated_until"] = "old"

	tests := []struct {
		name string
		old  map[string]any
		new  map[string]any
	}{
		{name: "create", old: nil, new: cloneMonthlyQuotaExtra(base)},
		{name: "enable proration", old: newExtra(100, 1, 0, "Asia/Shanghai", false), new: cloneMonthlyQuotaExtra(base)},
		{name: "limit changes", old: newExtra(200, 1, 0, "Asia/Shanghai", true), new: cloneMonthlyQuotaExtra(base)},
		{name: "reset day changes", old: newExtra(100, 2, 0, "Asia/Shanghai", true), new: cloneMonthlyQuotaExtra(base)},
		{name: "reset hour changes", old: newExtra(100, 1, 1, "Asia/Shanghai", true), new: cloneMonthlyQuotaExtra(base)},
		{name: "timezone changes", old: newExtra(100, 1, 0, "UTC", true), new: cloneMonthlyQuotaExtra(base)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyMonthlyQuotaProration(tt.old, tt.new, now)
			if _, ok := tt.new["quota_monthly_prorated_limit"].(float64); !ok {
				t.Fatal("derived limit was not recalculated")
			}
			until, ok := tt.new["quota_monthly_prorated_until"].(string)
			if !ok || until == "" || until == "old" {
				t.Fatalf("derived until was not recalculated: %#v", tt.new)
			}
		})
	}

	unchanged := cloneMonthlyQuotaExtra(base)
	applyMonthlyQuotaProration(base, unchanged, now.Add(24*time.Hour))
	if unchanged["quota_monthly_prorated_limit"] != 12.34 || unchanged["quota_monthly_prorated_until"] != "old" {
		t.Fatal("unrelated edits must preserve derived proration")
	}

	missingDerived := cloneMonthlyQuotaExtra(base)
	delete(missingDerived, "quota_monthly_prorated_limit")
	delete(missingDerived, "quota_monthly_prorated_until")
	applyMonthlyQuotaProration(base, missingDerived, now.Add(24*time.Hour))
	if missingDerived["quota_monthly_prorated_limit"] != 12.34 || missingDerived["quota_monthly_prorated_until"] != "old" {
		t.Fatal("unchanged proration must be copied from the old extra")
	}

	for _, clear := range []map[string]any{
		newExtra(100, 1, 0, "Asia/Shanghai", false),
		func() map[string]any {
			extra := newExtra(100, 1, 0, "Asia/Shanghai", true)
			extra["quota_monthly_reset_mode"] = "rolling"
			return extra
		}(),
		newExtra(0, 1, 0, "Asia/Shanghai", true),
	} {
		clear["quota_monthly_prorated_limit"] = 12.34
		clear["quota_monthly_prorated_until"] = "old"
		applyMonthlyQuotaProration(base, clear, now)
		for _, key := range []string{"quota_monthly_prorate", "quota_monthly_prorated_limit", "quota_monthly_prorated_until"} {
			if _, ok := clear[key]; ok {
				t.Fatalf("%s should be cleared: %#v", key, clear)
			}
		}
	}
}

func TestApplyMonthlyQuotaProrationAtResetPointUsesFullPeriod(t *testing.T) {
	now := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	extra := map[string]any{
		"quota_monthly_limit":      100.0,
		"quota_monthly_reset_mode": "fixed",
		"quota_monthly_reset_day":  1,
		"quota_monthly_reset_hour": 0,
		"quota_reset_timezone":     "UTC",
		"quota_monthly_prorate":    true,
	}
	applyMonthlyQuotaProration(nil, extra, now)
	if extra["quota_monthly_prorated_limit"] != 100.0 {
		t.Fatalf("reset point must start a full period: %#v", extra)
	}
	if extra["quota_monthly_prorated_until"] != "2027-01-01T00:00:00Z" {
		t.Fatalf("unexpected next reset: %#v", extra)
	}
}

func cloneMonthlyQuotaExtra(extra map[string]any) map[string]any {
	cloned := make(map[string]any, len(extra))
	for key, value := range extra {
		cloned[key] = value
	}
	return cloned
}
