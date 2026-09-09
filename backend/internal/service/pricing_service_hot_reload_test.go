package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func hotReloadModelJSON(name string, input, output float64) string {
	return fmt.Sprintf(`"%s":{"input_cost_per_token":%g,"output_cost_per_token":%g,"litellm_provider":"openai","mode":"chat"}`, name, input, output)
}

func newHotReloadPricingService(t *testing.T, catalogJSON, fallbackJSON string) *PricingService {
	t.Helper()
	dir := t.TempDir()
	svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{DataDir: dir}}, nil)
	require.NoError(t, os.WriteFile(svc.getPricingFilePath(), []byte(catalogJSON), 0644))
	if fallbackJSON != "" {
		svc.cfg.Pricing.FallbackFile = filepath.Join(dir, "fallback.json")
		require.NoError(t, os.WriteFile(svc.cfg.Pricing.FallbackFile, []byte(fallbackJSON), 0644))
	}
	require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
	return svc
}

func TestPricingCustomFilesFingerprint(t *testing.T) {
	dir := t.TempDir()
	svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{DataDir: dir}}, nil)
	require.Empty(t, svc.customPricingFilesFingerprint())

	svc.cfg.Pricing.FallbackFile = filepath.Join(dir, "fallback.json")
	empty := svc.customPricingFilesFingerprint()
	require.NotEmpty(t, empty)
	require.NoError(t, os.WriteFile(svc.cfg.Pricing.FallbackFile, []byte(`{"a":{}}`), 0644))
	require.NotEqual(t, empty, svc.customPricingFilesFingerprint())
}

func TestPricingHotReload_FallbackChangeRebuildsWithoutTouchingSyncAnchor(t *testing.T) {
	svc := newHotReloadPricingService(t,
		`{`+hotReloadModelJSON("remote-model", 1e-6, 2e-6)+`}`,
		`{`+hotReloadModelJSON("custom-a", 3e-6, 4e-6)+`}`,
	)
	require.NotNil(t, svc.pricingData["custom-a"])
	anchor := svc.localHash

	require.NoError(t, os.WriteFile(svc.cfg.Pricing.FallbackFile, []byte(`{`+hotReloadModelJSON("custom-b", 5e-6, 6e-6)+`}`), 0644))
	svc.reloadIfCustomFilesChanged()
	require.Nil(t, svc.pricingData["custom-a"])
	require.NotNil(t, svc.pricingData["custom-b"])
	require.Equal(t, anchor, svc.localHash)
}

func TestPricingHotReload_UnchangedFilesSkipRebuild(t *testing.T) {
	svc := newHotReloadPricingService(t,
		`{`+hotReloadModelJSON("remote-model", 1e-6, 2e-6)+`}`,
		`{`+hotReloadModelJSON("custom-a", 3e-6, 4e-6)+`}`,
	)
	before := svc.customFilesHash
	svc.reloadIfCustomFilesChanged()
	require.Equal(t, before, svc.customFilesHash)
}

func TestPricingHotReload_InvalidFileKeepsCurrentDataUntilFixed(t *testing.T) {
	svc := newHotReloadPricingService(t,
		`{`+hotReloadModelJSON("remote-model", 1e-6, 2e-6)+`}`,
		`{`+hotReloadModelJSON("custom-a", 3e-6, 4e-6)+`}`,
	)
	require.NoError(t, os.WriteFile(svc.cfg.Pricing.FallbackFile, []byte("not-json"), 0644))
	svc.reloadIfCustomFilesChanged()
	require.NotNil(t, svc.pricingData["custom-a"])

	require.NoError(t, os.WriteFile(svc.cfg.Pricing.FallbackFile, []byte(`{`+hotReloadModelJSON("custom-b", 5e-6, 6e-6)+`}`), 0644))
	svc.reloadIfCustomFilesChanged()
	require.Nil(t, svc.pricingData["custom-a"])
	require.NotNil(t, svc.pricingData["custom-b"])
}

func TestPricingHotReload_DeletedFileDropsItsLayer(t *testing.T) {
	svc := newHotReloadPricingService(t,
		`{`+hotReloadModelJSON("remote-model", 1e-6, 2e-6)+`}`,
		`{`+hotReloadModelJSON("custom-a", 3e-6, 4e-6)+`}`,
	)
	require.NoError(t, os.Remove(svc.cfg.Pricing.FallbackFile))
	svc.reloadIfCustomFilesChanged()
	require.Nil(t, svc.pricingData["custom-a"])
	require.NotNil(t, svc.pricingData["remote-model"])
}

func TestPricingSchedulerStartsForCustomFilesWithoutRemoteURL(t *testing.T) {
	svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{
		FallbackFile: filepath.Join(t.TempDir(), "fallback.json"),
	}}, nil)
	require.True(t, svc.hasCustomPricingFiles())
	svc.startUpdateScheduler()
	svc.Stop()
}
