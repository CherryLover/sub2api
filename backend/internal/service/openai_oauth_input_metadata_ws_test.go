package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Exercise the actual relay on both the first and subsequent response.create.
func TestPassthroughLifecycle_OAuthInputMetadataEveryTurn(t *testing.T) {
	for _, oauth := range []bool{false, true} {
		t.Run(fmt.Sprintf("oauth=%t", oauth), func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(context.Canceled)
			cfg := passthroughLifecycleConfig()
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 5
			account := passthroughLifecycleAccount()
			if oauth {
				account.Type = AccountTypeOAuth
				account.Credentials = map[string]any{"access_token": "sk-test"}
				account.Extra = map[string]any{"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough}
			}
			upstream := newStagedPassthroughConn()
			server, serverErr := startPassthroughLifecycleServer(t, ctx, newPassthroughLifecycleService(cfg, upstream), account)
			defer server.Close()
			payload := `{"type":"response.create","model":"gpt-5.1","input":[{"role":"user","content":[{"type":"input_text","text":"hello","internal_chat_message_metadata_passthrough":{"keep":true}}],"internal_chat_message_metadata_passthrough":null}]}`
			client := dialPassthroughLifecycleClientWithPayload(t, server, payload)
			defer func() { _ = client.CloseNow() }()
			for turn := 0; turn < 2; turn++ {
				if turn > 0 {
					writeCtx, writeCancel := context.WithTimeout(ctx, 3*time.Second)
					err := client.Write(writeCtx, coderws.MessageText, []byte(payload))
					writeCancel()
					require.NoError(t, err)
				}
				forwarded := requirePassthroughUpstreamWrite(t, upstream, 3*time.Second)
				require.Equal(t, !oauth, gjson.GetBytes(forwarded, "input.0.internal_chat_message_metadata_passthrough").Exists(), string(forwarded))
				require.True(t, gjson.GetBytes(forwarded, "input.0.content.0.internal_chat_message_metadata_passthrough.keep").Bool())
				upstream.Send(fmt.Sprintf(`{"type":"response.completed","response":{"id":"resp_%d","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`, turn))
				event, err := readPassthroughLifecycleFrame(t, client, 3*time.Second)
				require.NoError(t, err)
				require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
			}
			require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
			select {
			case <-serverErr:
			case <-time.After(3 * time.Second):
				t.Fatal("relay did not exit")
			}
		})
	}
}
