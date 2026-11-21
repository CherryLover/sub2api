package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCodexForkCacheIdentityMatchesParentHeader(t *testing.T) {
	for _, mode := range []codexFingerprintMode{codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull} {
		for _, cacheKey := range []string{"parent-session", "child-session", "custom-cache"} {
			for _, raw := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/raw=%t", mode, cacheKey, raw), func(t *testing.T) {
					account := newTestOAuthAccount(739, map[string]any{codexFingerprintModeExtraKey: string(mode)})
					ids := resolveCodexFingerprintIDs(account, "parent-session", mode)
					body := []byte(fmt.Sprintf(`{"model":"gpt-5.4","marker":9007199254740993,"escaped":"\u0061","prompt_cache_key":%q,"client_metadata":{"session_id":"child-session"},"input":[]}`, cacheKey))
					original := bytes.Clone(body)
					var out []byte
					if raw {
						var err error
						out, _, err = applyCodexFingerprintClientMetadataRaw(body, ids)
						require.NoError(t, err)
					} else {
						decoder := json.NewDecoder(bytes.NewReader(body))
						decoder.UseNumber()
						var request map[string]any
						require.NoError(t, decoder.Decode(&request))
						applyCodexFingerprintClientMetadata(request, ids)
						var err error
						out, err = json.Marshal(request)
						require.NoError(t, err)
					}
					want := cacheKey
					if (mode == codexFingerprintSession || mode == codexFingerprintFull) && cacheKey != "custom-cache" {
						want = ids.sessionID
					}
					require.Equal(t, want, gjson.GetBytes(out, "prompt_cache_key").String())
					require.Equal(t, "9007199254740993", gjson.GetBytes(out, "marker").Raw)
					if raw {
						require.Equal(t, `"\u0061"`, gjson.GetBytes(out, "escaped").Raw)
					}
					require.Equal(t, original, body)
				})
			}
		}
	}
}
func TestCodexForkCacheIdentityForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{"session", "full"} {
		for _, passthrough := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/passthrough=%t", mode, passthrough), func(t *testing.T) {
				account := instructionsEchoOAuthAccount(passthrough)
				if account.Extra == nil {
					account.Extra = map[string]any{}
				}
				account.Extra[codexFingerprintModeExtraKey] = mode
				account.Extra[codexFingerprintSeedExtraKey] = testCodexFingerprintSeed
				upstream := instructionsEchoUpstream(`"client"`)
				body := []byte(`{"model":"gpt-5.4","stream":true,"instructions":"client","prompt_cache_key":"parent-session","client_metadata":{"session_id":"child-session"},"input":[{"role":"user","content":"Hi"}]}`)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				c.Request.Header.Set("session-id", "parent-session")
				SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
				svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
				result, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				require.NotNil(t, result)
				require.NotNil(t, upstream.lastReq)
				require.Equal(t, resolveConvergedSessionID(testCodexFingerprintSeed), gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
				require.Equal(t, "parent-session", c.Request.Header.Get("session-id"), "client header must not be overwritten")
			})
		}
	}
}
