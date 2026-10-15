package service

import (
	"bytes"
	"context"
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

func newCodexDirectProtocolContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	return c, w
}
func TestCodexDirectImagesForwardJSONToSSEGenerationAndEdit(t *testing.T) {
	for _, edit := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("edit=%t/stream=%t", edit, stream), func(t *testing.T) {
				endpoint := "/v1/images/generations"
				if edit {
					endpoint = "/v1/images/edits"
				}
				body := []byte(fmt.Sprintf(`{"model":"gpt-image-2","prompt":"draw a cat","size":"1024x1024","stream":%t,"n":2}`, stream))
				c, w := newCodexDirectProtocolContext()
				c.Request = httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				png := encodeOpenAIImageTestPNG(t, 32, 24)
				response := fmt.Sprintf(`{
 "created":1710000000,"model":"gpt-image-2.5-flare","data":[{"b64_json":%q,"size":"1024x1024"},{"b64_json":%q,"size":"1024x1024"}],
 "usage":{
 "input_tokens":15,"output_tokens":24,"input_tokens_details":{"cached_tokens":4}}
}`, png, png)
				up := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}, "Content-Length": []string{fmt.Sprint(len(response))}}, Body: io.NopCloser(strings.NewReader(response))}}
				s := &OpenAIGatewayService{httpUpstream: up}
				parsed := &OpenAIImagesRequest{Endpoint: endpoint, Model: "gpt-image-2", Prompt: "draw a cat", Stream: stream, N: 2, Size: "1024x1024", SizeTier: ImageBillingSize1K, ResponseFormat: "b64_json", Body: body}
				if edit {
					parsed.InputImageURLs = []string{"data:image/png;base64," + png}
				}
				account := &Account{ID: 17, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "test-token", "model_mapping": map[string]any{"gpt-image-2": "gpt-image-2.5-flare"}}, Extra: map[string]any{"codex_direct_images_enabled": true}}
				result, err := s.ForwardImages(context.Background(), c, account, body, parsed, "")
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 2, result.ImageCount)
				require.Equal(t, 15, result.Usage.InputTokens)
				require.Equal(t, 24, result.Usage.OutputTokens)
				require.Equal(t, 24, result.Usage.ImageOutputTokens)
				require.Equal(t, 4, result.Usage.CacheReadInputTokens)
				require.Equal(t, "gpt-image-2.5-flare", result.UpstreamModel)
				require.Equal(t, []string{"32x24", "32x24"}, result.ImageOutputSizes)
				wantPath := "/backend-api/codex/images/generations"
				if edit {
					wantPath = "/backend-api/codex/images/edits"
				}
				require.Equal(t, wantPath, up.lastReq.URL.Path)
				require.Empty(t, up.lastReq.Header.Get("OpenAI-Beta"))
				require.Equal(t, "gpt-image-2.5-flare", gjson.GetBytes(up.lastBody, "model").String())
				if edit {
					require.Equal(t, "data:image/png;base64,"+png, gjson.GetBytes(up.lastBody, "images.0.image_url").String())
				}
				if stream {
					require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
					require.Empty(t, w.Header().Get("Content-Length"))
					events := parseOpenAIImageTestSSEEvents(w.Body.String())
					require.Len(t, events, 2)
					prefix := "image_generation.completed"
					if edit {
						prefix = "image_edit.completed"
					}
					for _, e := range events {
						require.True(t, gjson.Valid(e.Data))
						require.Equal(t, prefix, gjson.Get(e.Data, "type").String())
						require.Equal(t, "gpt-image-2", gjson.Get(e.Data, "model").String())
						require.Equal(t, png, gjson.Get(e.Data, "b64_json").String())
						require.Equal(t, int64(24), gjson.Get(e.Data, "usage.output_tokens").Int())
						require.NotContains(t, e.Data, "\n")
					}
				} else {
					require.Contains(t, w.Header().Get("Content-Type"), "application/json")
					require.Len(t, gjson.Get(w.Body.String(), "data").Array(), 2)
				}
			})
		}
	}
}

func TestCodexDirectImagesJSONErrorsAreUncommitted(t *testing.T) {
	for _, payload := range []string{`not-json`, `{"data":[]}`, `{"data":[{"b64_json":"aGVsbG8="},{"b64_json":""}]}`, `{"data":[{"b64_json":17}]}`, `{"data":[{"b64_json":"not-base64!"}]}`, `{"error":{"message":"request failed"}}`} {
		t.Run(payload, func(t *testing.T) {
			c, w := newCodexDirectProtocolContext()
			resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}
			usage, count, _, _, err := (&OpenAIGatewayService{}).handleCodexDirectImagesJSONAsStream(resp, c, time.Now(), &OpenAIImagesRequest{Stream: true, Model: "gpt-image-2"})
			require.Error(t, err)
			require.Zero(t, count)
			require.Equal(t, OpenAIUsage{}, usage)
			require.False(t, c.Writer.Written())
			require.Empty(t, w.Body.String())
		})
	}
	c, w := newCodexDirectProtocolContext()
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{UpstreamResponseReadMaxBytes: 8}}}
	_, _, _, _, err := svc.handleCodexDirectImagesJSONAsStream(&http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"aGVsbG8="}]}`))}, c, time.Now(), &OpenAIImagesRequest{Stream: true})
	require.ErrorIs(t, err, ErrUpstreamResponseBodyTooLarge)
	require.False(t, c.Writer.Written())
	require.Empty(t, w.Body.String())
}

func TestCodexDirectImagesJSONStreamDisconnectStillReturnsBillableUsage(t *testing.T) {
	c, _ := newCodexDirectProtocolContext()
	c.Writer = &failingOpenAIImageWriter{ResponseWriter: c.Writer, failAfter: 0}
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"aGVsbG8="}],"usage":{"input_tokens":9,"output_tokens":12}}`))}
	usage, count, _, _, err := (&OpenAIGatewayService{}).handleCodexDirectImagesJSONAsStream(resp, c, time.Now(), &OpenAIImagesRequest{Stream: true, Model: "gpt-image-2"})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, 12, usage.ImageOutputTokens)
}
