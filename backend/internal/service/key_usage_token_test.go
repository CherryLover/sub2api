package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestKeyUsageTokenService(t *testing.T) *KeyUsageTokenService {
	t.Helper()
	key := DeriveKeyUsageTokenSigningKey("test-master-secret-that-is-long-enough-0123456789")
	require.NotEmpty(t, key)
	return NewKeyUsageTokenService(key, 30*24*time.Hour)
}

func TestKeyUsageTokenRoundTrip(t *testing.T) {
	svc := newTestKeyUsageTokenService(t)
	now := time.Now()

	token, expiresAt, err := svc.Issue(42, "sk-secret-value", now)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.WithinDuration(t, now.Add(30*24*time.Hour), expiresAt, time.Second)

	claims, err := svc.Parse(token, now)
	require.NoError(t, err)
	require.Equal(t, int64(42), claims.APIKeyID)
	require.Equal(t, keyUsageTokenType, claims.TokenType)
	require.Equal(t, svc.Fingerprint("sk-secret-value"), claims.Fingerprint)
}

// 令牌不能反推原 key，也不能拿去当网关凭证使用。
func TestKeyUsageTokenDoesNotLeakRawKey(t *testing.T) {
	svc := newTestKeyUsageTokenService(t)
	const rawKey = "sk-secret-value"

	token, _, err := svc.Issue(42, rawKey, time.Now())
	require.NoError(t, err)
	require.NotContains(t, token, rawKey)

	claims, err := svc.Parse(token, time.Now())
	require.NoError(t, err)
	require.NotContains(t, claims.Fingerprint, rawKey)
	// 指纹是密钥相关的单向 HMAC：换一把签名密钥就算不出同样的指纹。
	other := NewKeyUsageTokenService(DeriveKeyUsageTokenSigningKey("another-master-secret-0123456789abcdef"), time.Hour)
	require.NotEqual(t, svc.Fingerprint(rawKey), other.Fingerprint(rawKey))
}

func TestKeyUsageTokenRejectsExpired(t *testing.T) {
	svc := NewKeyUsageTokenService(DeriveKeyUsageTokenSigningKey("test-master-secret-0123456789abcdef"), time.Minute)
	issuedAt := time.Now().Add(-2 * time.Hour)

	token, _, err := svc.Issue(7, "sk-x", issuedAt)
	require.NoError(t, err)

	_, err = svc.Parse(token, issuedAt.Add(30*time.Second))
	require.NoError(t, err, "未过期时应可解析")

	_, err = svc.Parse(token, time.Now())
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
}

func TestKeyUsageTokenRejectsTampering(t *testing.T) {
	svc := newTestKeyUsageTokenService(t)
	token, _, err := svc.Issue(7, "sk-x", time.Now())
	require.NoError(t, err)

	parts := strings.Split(token, ".")
	require.Len(t, parts, 2)

	// 篡改 payload（例如把 kid 换成别人的 Key）后签名对不上
	tamperedPayload := "eyJ0eXAiOiJrZXlfdXNhZ2VfdjEiLCJraWQiOjk5OTk5fQ" + "." + parts[1]
	_, err = svc.Parse(tamperedPayload, time.Now())
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized)

	// 篡改签名
	tamperedSignature := parts[0] + "." + flipLastRune(parts[1])
	_, err = svc.Parse(tamperedSignature, time.Now())
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized)

	// 结构不合法
	for _, bad := range []string{"", ".", "abc", parts[0], parts[0] + "." + parts[1] + "." + parts[1]} {
		_, err = svc.Parse(bad, time.Now())
		require.ErrorIs(t, err, ErrKeyUsageUnauthorized, "input=%q", bad)
	}
}

func TestKeyUsageTokenRejectsForeignSigningKey(t *testing.T) {
	issuer := newTestKeyUsageTokenService(t)
	verifier := NewKeyUsageTokenService(DeriveKeyUsageTokenSigningKey("different-master-secret-0123456789ab"), time.Hour)

	token, _, err := issuer.Issue(7, "sk-x", time.Now())
	require.NoError(t, err)

	_, err = verifier.Parse(token, time.Now())
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
}

// 没有签名密钥时必须 fail-closed：既签不出令牌，也不接受任何令牌。
func TestKeyUsageTokenNotConfiguredFailsClosed(t *testing.T) {
	svc := NewKeyUsageTokenService(nil, time.Hour)
	require.False(t, svc.Configured())

	_, _, err := svc.Issue(1, "sk-x", time.Now())
	require.Error(t, err)

	_, err = svc.Parse("anything.anything", time.Now())
	require.ErrorIs(t, err, ErrKeyUsageUnauthorized)
	require.Empty(t, svc.Fingerprint("sk-x"))
}

func TestDeriveKeyUsageTokenSigningKeyIsDeterministicAndDomainSeparated(t *testing.T) {
	const master = "master-secret-0123456789abcdefghij"
	require.Equal(t, DeriveKeyUsageTokenSigningKey(master), DeriveKeyUsageTokenSigningKey(master))
	require.NotEqual(t, []byte(master), DeriveKeyUsageTokenSigningKey(master))
	require.Nil(t, DeriveKeyUsageTokenSigningKey("   "))
}

func flipLastRune(s string) string {
	if s == "" {
		return "x"
	}
	last := s[len(s)-1]
	if last == 'A' {
		return s[:len(s)-1] + "B"
	}
	return s[:len(s)-1] + "A"
}
