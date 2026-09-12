//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// The disabled fallback must not create a new account block, but must not clear
// an existing block or suppress explicit upstream quota-reset deadlines.
func TestBackportOAuth429Fallback(t *testing.T) {
	for _, tc := range []struct {
		name        string
		enabled     bool
		body        string
		preblocked  bool
		wantBlocked bool
	}{
		{"disabled", false, `{"error":{"message":"slow down"}}`, false, false},
		{"enabled", true, `{"error":{"message":"slow down"}}`, false, true},
		{"expired reset disabled", false, `{"error":{"type":"usage_limit_reached","resets_at":1}}`, false, false},
		{"explicit future reset", false, fmt.Sprintf(`{"error":{"type":"usage_limit_reached","resets_at":%d}}`, time.Now().Add(time.Hour).Unix()), false, true},
		{"preserve existing block", false, `{"error":{"message":"slow down"}}`, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockSettingRepo()
			data, err := json.Marshal(RateLimit429CooldownSettings{Enabled: tc.enabled, CooldownSeconds: 12})
			require.NoError(t, err)
			repo.data[SettingKeyRateLimit429CooldownSettings] = string(data)
			limits := NewRateLimitService(&rateLimit429AccountRepoStub{}, nil, &config.Config{}, nil, nil)
			limits.SetSettingService(NewSettingService(repo, &config.Config{}))
			svc := &OpenAIGatewayService{rateLimitService: limits}
			account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			if tc.preblocked {
				svc.BlockAccountScheduling(account, time.Now().Add(time.Minute), "transport_error")
			}
			svc.markOpenAIOAuth429RateLimited(context.Background(), account, http.Header{}, []byte(tc.body))
			require.Equal(t, tc.wantBlocked, svc.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-5.4"))
		})
	}
}

type backportFailingCooldownRepo struct {
	AccountRepository
	calls int
}

func (r *backportFailingCooldownRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	r.calls++
	return fmt.Errorf("database unavailable")
}

func TestBackportTransportCooldownSurvivesDatabaseFailure(t *testing.T) {
	repo := &backportFailingCooldownRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 43, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc.tempUnscheduleOpenAITransportError(context.Background(), account, "proxy connection refused")
	require.Equal(t, 1, repo.calls)
	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(account, "gpt-5.4"))
}

func TestBackportRawChatUnsupportedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{PlatformGrok, PlatformOpenAI} {
		t.Run(platform, func(t *testing.T) {
			body := []byte(`{"model":"grok-4","messages":[{"role":"user","content":"hello"}],"stream":false,"external_web_access":true,"metadata":{"external_web_access":true,"keep":"ok"}}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{"id":"chat_1","object":"chat.completion","model":"grok-4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)),
			}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			account := rawChatCompletionsTestAccount()
			account.Platform = platform
			_, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
			require.NoError(t, err)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, platform != PlatformGrok, gjson.GetBytes(upstream.lastBody, "external_web_access").Exists())
			require.Equal(t, platform != PlatformGrok, gjson.GetBytes(upstream.lastBody, "metadata.external_web_access").Exists())
			require.Equal(t, "ok", gjson.GetBytes(upstream.lastBody, "metadata.keep").String())
		})
	}
}
