//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func floatPtrPQ(v float64) *float64 { return &v }

func newSettingServiceForPlatformQuotaTest(seed map[string]string) *SettingService {
	repo := newMockSettingRepo()
	for k, v := range seed {
		repo.data[k] = v
	}
	return NewSettingService(repo, &config.Config{})
}

func TestGetDefaultPlatformQuotas_ReturnsAllowedPlatforms(t *testing.T) {
	zero := 0.0
	svc := newSettingServiceForPlatformQuotaTest(map[string]string{
		// 新 JSON 格式：anthropic daily=10.5, openai monthly=0, 其他平台无配置
		SettingKeyDefaultPlatformQuotas: `{"anthropic":{"daily":10.5},"openai":{"monthly":0}}`,
	})
	got, err := svc.GetDefaultPlatformQuotas(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 必须包含全部允许 platform key（补齐契约）
	for _, platform := range AllowedQuotaPlatforms {
		if _, ok := got[platform]; !ok {
			t.Errorf("missing platform key: %q", platform)
		}
	}
	// anthropic daily = 10.5
	if v := got["anthropic"].DailyLimitUSD; v == nil || *v != 10.5 {
		t.Errorf("anthropic daily want 10.5, got %v", v)
	}
	// openai monthly = 0（显式禁用）
	if v := got["openai"].MonthlyLimitUSD; v == nil || *v != zero {
		t.Errorf("openai monthly want 0 (explicit disable), got %v", v)
	}
	// gemini 无配置 → weekly = nil
	if v := got["gemini"].WeeklyLimitUSD; v != nil {
		t.Errorf("gemini weekly want nil (not configured), got %v", *v)
	}
	// antigravity 无配置 → daily = nil
	if v := got["antigravity"].DailyLimitUSD; v != nil {
		t.Errorf("antigravity daily want nil (not configured), got %v", *v)
	}
}

// TestSystemPlatformQuotas_WriteReadRoundTrip 验证系统层 platform quota 经 buildSystemSettingsUpdates（写）
// 再由 GetDefaultPlatformQuotas（读）正确往返，覆盖真实 write→read 路径并锁住平台补齐契约。
func TestSystemPlatformQuotas_WriteReadRoundTrip(t *testing.T) {
	svc := newSettingServiceForPlatformQuotaTest(nil)
	ctx := context.Background()

	ten := 10.0
	ss := &SystemSettings{
		DefaultPlatformQuotas: map[string]*DefaultPlatformQuotaSetting{
			"anthropic": {DailyLimitUSD: &ten, WeeklyLimitUSD: nil, MonthlyLimitUSD: nil},
		},
	}
	if err := svc.UpdateSettings(ctx, ss); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	got, err := svc.GetDefaultPlatformQuotas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// 平台补齐契约：无论写了几个 platform，读回必须含全部允许平台
	for _, p := range AllowedQuotaPlatforms {
		if _, ok := got[p]; !ok {
			t.Errorf("allowed-platform contract violated: missing platform %q", p)
		}
	}
	// 写入值正确往返
	if v := got["anthropic"].DailyLimitUSD; v == nil || *v != ten {
		t.Fatalf("anthropic daily round-trip failed: got %v, want 10", v)
	}
	// 未写入的平台字段为 nil
	if got["openai"].DailyLimitUSD != nil {
		t.Errorf("openai daily should be nil (not written), got %v", got["openai"].DailyLimitUSD)
	}
}

// TestSystemPlatformQuotas_EmptyMapClearsAll 验证空 map 的整体替换语义：
// 写入 DefaultPlatformQuotas={} 后，GetDefaultPlatformQuotas 返回全部允许平台、所有字段均为 nil，
// 明确文档化"空 map = 清空全部配额"是有意为之的 whole-replace 语义。
func TestSystemPlatformQuotas_EmptyMapClearsAll(t *testing.T) {
	svc := newSettingServiceForPlatformQuotaTest(nil)
	ctx := context.Background()

	// 先写入有值的配置
	ten := 10.0
	if err := svc.UpdateSettings(ctx, &SystemSettings{
		DefaultPlatformQuotas: map[string]*DefaultPlatformQuotaSetting{
			"anthropic": {DailyLimitUSD: &ten},
		},
	}); err != nil {
		t.Fatalf("initial write: %v", err)
	}

	// 再写入空 map（整体替换语义：清空全部）
	if err := svc.UpdateSettings(ctx, &SystemSettings{
		DefaultPlatformQuotas: map[string]*DefaultPlatformQuotaSetting{},
	}); err != nil {
		t.Fatalf("empty map write: %v", err)
	}

	got, err := svc.GetDefaultPlatformQuotas(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// 全部允许平台 key 仍然存在（补齐契约）
	for _, p := range AllowedQuotaPlatforms {
		if _, ok := got[p]; !ok {
			t.Errorf("allowed-platform contract violated after empty write: missing %q", p)
		}
	}
	// 所有字段 nil（全部已清空）
	for _, p := range AllowedQuotaPlatforms {
		pq := got[p]
		if pq == nil {
			continue
		}
		if pq.DailyLimitUSD != nil || pq.WeeklyLimitUSD != nil || pq.MonthlyLimitUSD != nil {
			t.Errorf("platform %q should have all-nil limits after empty-map write, got %+v", p, pq)
		}
	}
}
