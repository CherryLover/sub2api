//go:build unit

package service

import "testing"

// TestSettingKeyDefaultPlatformQuotas 验证新的系统层 JSON key 常量值正确。
func TestSettingKeyDefaultPlatformQuotas(t *testing.T) {
	if SettingKeyDefaultPlatformQuotas != "default_platform_quotas" {
		t.Errorf("SettingKeyDefaultPlatformQuotas = %q, want %q",
			SettingKeyDefaultPlatformQuotas, "default_platform_quotas")
	}
}
