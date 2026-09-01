//go:build unit

package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestDiffSettings_DetectsGlobalPlatformQuotaChange(t *testing.T) {
	five := 5.0
	ten := 10.0
	before := &service.SystemSettings{
		DefaultPlatformQuotas: map[string]*service.DefaultPlatformQuotaSetting{
			"anthropic": {DailyLimitUSD: &five},
		},
	}
	after := &service.SystemSettings{
		DefaultPlatformQuotas: map[string]*service.DefaultPlatformQuotaSetting{
			"anthropic": {DailyLimitUSD: &ten},
		},
	}

	changed := diffSettings(before, after, UpdateSettingsRequest{})
	found := false
	for _, key := range changed {
		if key == service.SettingKeyDefaultPlatformQuotas {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected change detection for default platform quotas, got %v", changed)
	}
}

func TestDiffSettings_NoChangeWhenEqual(t *testing.T) {
	five := 5.0
	before := &service.SystemSettings{
		DefaultPlatformQuotas: map[string]*service.DefaultPlatformQuotaSetting{
			"anthropic": {DailyLimitUSD: &five},
		},
	}
	after := &service.SystemSettings{
		DefaultPlatformQuotas: map[string]*service.DefaultPlatformQuotaSetting{
			"anthropic": {DailyLimitUSD: &five},
		},
	}

	changed := diffSettings(before, after, UpdateSettingsRequest{})
	for _, key := range changed {
		if key == service.SettingKeyDefaultPlatformQuotas {
			t.Error("equal values should not be detected as changed")
		}
	}
}

func TestEqualNullableFloat(t *testing.T) {
	five := 5.0
	five2 := 5.0
	ten := 10.0
	cases := []struct {
		a, b *float64
		want bool
	}{
		{nil, nil, true},
		{&five, nil, false},
		{nil, &five, false},
		{&five, &five2, true},
		{&five, &ten, false},
	}
	for _, c := range cases {
		if got := equalNullableFloat(c.a, c.b); got != c.want {
			t.Errorf("equalNullableFloat(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestEqualPlatformQuotaSettings_DetectsPerWindowChange(t *testing.T) {
	five := 5.0
	ten := 10.0
	before := map[string]*service.DefaultPlatformQuotaSetting{
		"anthropic": {DailyLimitUSD: &five},
	}
	after := map[string]*service.DefaultPlatformQuotaSetting{
		"anthropic": {DailyLimitUSD: &ten},
	}
	if equalPlatformQuotaSettings(before, after) {
		t.Error("expected unequal")
	}
}
