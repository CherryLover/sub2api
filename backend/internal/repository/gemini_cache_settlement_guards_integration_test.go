//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGeminiCacheSettlementLegacyNullWindowsDoNotOverspend(t *testing.T) {
	for _, tc := range []struct {
		name, limit, usage, start string
		want                      error
	}{
		{"5h", "rate_limit_5h", "usage_5h", "window_5h_start", service.ErrAPIKeyRateLimit5hExceeded},
		{"1d", "rate_limit_1d", "usage_1d", "window_1d_start", service.ErrAPIKeyRateLimit1dExceeded},
		{"7d", "rate_limit_7d", "usage_7d", "window_7d_start", service.ErrAPIKeyRateLimit7dExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGeminiCacheSettlementFixture(t, 10)
			ctx := context.Background()
			_, err := integrationDB.ExecContext(ctx, `UPDATE api_keys SET `+tc.limit+`=1,`+tc.usage+`=0.8,`+tc.start+`=NULL WHERE id=$1`, f.key.ID)
			require.NoError(t, err)
			r, q := f.quote(0.3)
			_, err = f.repo.CreateWithSettlement(ctx, r, q)
			require.ErrorIs(t, err, tc.want)
			require.Zero(t, f.balance(t))
			r, q = f.quote(0.1)
			_, err = f.repo.CreateWithSettlement(ctx, r, q)
			require.NoError(t, err)
			var used float64
			require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT `+tc.usage+` FROM api_keys WHERE id=$1`, f.key.ID).Scan(&used))
			require.InDelta(t, 0.9, used, 1e-8)
		})
	}
}

func TestGeminiCacheSettlementRechecksLatestKeyState(t *testing.T) {
	for _, tc := range []struct {
		name, assignment string
		want             error
	}{
		{"disabled", "status='disabled'", service.ErrAPIKeyQuotaExhausted},
		{"deleted", "deleted_at=NOW()", service.ErrAPIKeyNotFound},
		{"expired", "expires_at=NOW()-INTERVAL '1 second'", service.ErrAPIKeyExpired},
		{"group removed", "group_id=NULL", service.ErrGeminiCachedContentNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newGeminiCacheSettlementFixture(t, 1)
			r, q := f.quote(0.1)
			_, err := integrationDB.ExecContext(ctx, `UPDATE api_keys SET `+tc.assignment+` WHERE id=$1`, f.key.ID)
			require.NoError(t, err)
			_, err = f.repo.CreateWithSettlement(ctx, r, q)
			require.ErrorIs(t, err, tc.want)
			require.Zero(t, f.balance(t))
			require.Equal(t, 0, geminiCacheCount(t, `SELECT count(*) FROM gemini_cached_contents WHERE public_id=$1`, r.PublicID))
			require.Equal(t, 0, geminiCacheCount(t, `SELECT count(*) FROM usage_logs WHERE api_key_id=$1`, f.key.ID))
		})
	}
}
