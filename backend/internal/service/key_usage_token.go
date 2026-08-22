package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	// keyUsageTokenType 写进 payload 的类型标记，避免同一把签名密钥下的令牌被跨用途重放。
	keyUsageTokenType = "key_usage_v1"
	// keyUsageTokenKeyLabel HKDF 风格的域分隔标签：从 JWT 主密钥派生一把只用于本功能的子密钥，
	// 这样即使子密钥泄露也推不回 JWT 主密钥，反之亦然。
	keyUsageTokenKeyLabel = "sub2api/key-usage-report-token/v1"
	// keyUsageFingerprintLabel 指纹标签：令牌里只放 HMAC 指纹而不是 API Key 本身，
	// 保证令牌不可逆推原 key；同时 key 值一旦轮换，旧令牌指纹对不上即失效。
	keyUsageFingerprintLabel = "sub2api/key-usage-fingerprint/v1"
	// keyUsageFingerprintBytes 指纹截断长度（字节）。16 字节足够抗碰撞，令牌也更短。
	keyUsageFingerprintBytes = 16
)

// KeyUsageTokenClaims 是免登录用量页 URL 令牌的载荷。
// 刻意只放 API Key 的自增 ID + 指纹：ID 单独出现在 URL 里可枚举，但它被 HMAC 签名
// 覆盖，改一位签名即失效；指纹用于把令牌钉死在"签发时的那一把 key 值"上。
type KeyUsageTokenClaims struct {
	TokenType   string `json:"typ"`
	APIKeyID    int64  `json:"kid"`
	Fingerprint string `json:"fp"`
	IssuedAt    int64  `json:"iat"`
	ExpiresAt   int64  `json:"exp"`
}

// KeyUsageTokenService 负责签发/校验用量页令牌（HMAC-SHA256，无状态、不落库）。
type KeyUsageTokenService struct {
	signingKey []byte
	ttl        time.Duration
}

// DeriveKeyUsageTokenSigningKey 从服务端主密钥（JWT secret，已由 security_secrets 持久化并跨实例一致）
// 派生用量令牌签名密钥。不直接复用主密钥，避免一处泄露牵连另一处。
func DeriveKeyUsageTokenSigningKey(masterSecret string) []byte {
	masterSecret = strings.TrimSpace(masterSecret)
	if masterSecret == "" {
		return nil
	}
	mac := hmac.New(sha256.New, []byte(masterSecret))
	_, _ = mac.Write([]byte(keyUsageTokenKeyLabel))
	return mac.Sum(nil)
}

// NewKeyUsageTokenService 创建令牌服务。signingKey 为空时服务处于"未配置"状态：
// 签发返回 503，校验一律失败（fail-closed，绝不因为缺密钥就放行）。
func NewKeyUsageTokenService(signingKey []byte, ttl time.Duration) *KeyUsageTokenService {
	svc := &KeyUsageTokenService{ttl: ttl}
	if ttl <= 0 {
		svc.ttl = DefaultKeyUsageTokenTTL
	}
	if len(signingKey) > 0 {
		svc.signingKey = append([]byte(nil), signingKey...)
	}
	return svc
}

// TTL 返回令牌有效期。
func (s *KeyUsageTokenService) TTL() time.Duration {
	if s == nil || s.ttl <= 0 {
		return DefaultKeyUsageTokenTTL
	}
	return s.ttl
}

// Configured 返回签名密钥是否可用。
func (s *KeyUsageTokenService) Configured() bool {
	return s != nil && len(s.signingKey) > 0
}

// Fingerprint 计算某个 API Key 明文的不可逆指纹。
func (s *KeyUsageTokenService) Fingerprint(rawKey string) string {
	if !s.Configured() || rawKey == "" {
		return ""
	}
	mac := hmac.New(sha256.New, s.signingKey)
	_, _ = mac.Write([]byte(keyUsageFingerprintLabel))
	_, _ = mac.Write([]byte(rawKey))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:keyUsageFingerprintBytes])
}

// Issue 为指定 API Key 签发令牌，返回令牌与绝对过期时间。
func (s *KeyUsageTokenService) Issue(apiKeyID int64, rawKey string, now time.Time) (string, time.Time, error) {
	if !s.Configured() {
		return "", time.Time{}, infraerrors.ServiceUnavailable("KEY_USAGE_TOKEN_NOT_CONFIGURED", "key usage tokens require a configured signing key")
	}
	if apiKeyID <= 0 || rawKey == "" {
		return "", time.Time{}, ErrKeyUsageUnauthorized
	}
	if now.IsZero() {
		now = time.Now()
	}
	expiresAt := now.Add(s.TTL())
	claims := KeyUsageTokenClaims{
		TokenType:   keyUsageTokenType,
		APIKeyID:    apiKeyID,
		Fingerprint: s.Fingerprint(rawKey),
		IssuedAt:    now.Unix(),
		ExpiresAt:   expiresAt.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + s.sign(encoded), expiresAt, nil
}

// Parse 校验签名与有效期并返回 claims。
// 任何失败都返回同一个 ErrKeyUsageUnauthorized：篡改、过期、格式错误对外不可区分，
// 避免把令牌结构或 key 是否存在的信息泄露给探测者。
func (s *KeyUsageTokenService) Parse(token string, now time.Time) (*KeyUsageTokenClaims, error) {
	if !s.Configured() {
		return nil, ErrKeyUsageUnauthorized
	}
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, ErrKeyUsageUnauthorized
	}
	if !hmac.Equal([]byte(parts[1]), []byte(s.sign(parts[0]))) {
		return nil, ErrKeyUsageUnauthorized
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrKeyUsageUnauthorized
	}
	var claims KeyUsageTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, ErrKeyUsageUnauthorized
	}
	if claims.TokenType != keyUsageTokenType || claims.APIKeyID <= 0 || claims.Fingerprint == "" {
		return nil, ErrKeyUsageUnauthorized
	}
	if now.IsZero() {
		now = time.Now()
	}
	if claims.ExpiresAt <= 0 || now.Unix() > claims.ExpiresAt {
		return nil, ErrKeyUsageUnauthorized
	}
	return &claims, nil
}

func (s *KeyUsageTokenService) sign(payload string) string {
	mac := hmac.New(sha256.New, s.signingKey)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
