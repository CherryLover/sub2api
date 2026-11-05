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
	} {
		if ValidateQuotaResetConfig(extra) == nil {
			t.Errorf("accepted invalid config %#v", extra)
		}
	}
	if err := ValidateQuotaResetConfig(map[string]any{"quota_monthly_reset_mode": "fixed", "quota_monthly_reset_day": 31, "quota_monthly_reset_hour": 23}); err != nil {
		t.Fatal(err)
	}
}
