//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type countingModelRateLimitRepo struct {
	accountRepoStub
	calls  int
	scopes []string
}

func (r *countingModelRateLimitRepo) SetModelRateLimit(_ context.Context, _ int64, scope string, _ time.Time, _ ...string) error {
	r.calls++
	r.scopes = append(r.scopes, scope)
	return nil
}

func newImagesCooldownContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	return c, rec
}

func imagesCooldownAccount() *Account {
	return &Account{ID: 77, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Name: "img-oauth"}
}

func TestShouldCoolOpenAIImagesToolForError(t *testing.T) {
	cases := []struct {
		name string
		err  *OpenAIImagesUpstreamError
		want bool
	}{
		{name: "nil_error", err: nil, want: false},
		{
			name: "synthesized_from_model_text",
			err: &OpenAIImagesUpstreamError{
				StatusCode:               http.StatusBadGateway,
				Code:                     "image_generation_unavailable",
				SynthesizedFromModelText: true,
			},
			want: false,
		},
		{
			name: "structured_upstream_error_frame",
			err: &OpenAIImagesUpstreamError{
				StatusCode: http.StatusBadGateway,
				Code:       "image_generation_unavailable",
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, shouldCoolOpenAIImagesToolForError(tc.err))
		})
	}
}

func TestHandleOpenAIImagesOAuthResponseError_TextFallbackDoesNotCoolAccount(t *testing.T) {
	c, _ := newImagesCooldownContext(t)
	repo := &countingModelRateLimitRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := imagesCooldownAccount()

	upstreamErr := openAIImagesTextFallbackErrorForText("Here's a polished image prompt for your request.")
	require.NotNil(t, upstreamErr)
	require.Equal(t, "image_generation_unavailable", upstreamErr.Code)

	err := svc.handleOpenAIImagesOAuthResponseError(
		context.Background(), c, account, "gpt-image-2", "https://upstream.example/v1/responses",
		&http.Response{StatusCode: http.StatusOK, Header: http.Header{}},
		OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c), upstreamErr,
	)

	require.Zero(t, repo.calls, "模型闲聊不构成账号级证据，不得写 30 分钟冷却")

	var failover *UpstreamFailoverError
	require.True(t, errors.As(err, &failover), "仍应触发换号，got %T", err)
}

func TestHandleOpenAIImagesOAuthResponseError_StructuredUnavailableStillCoolsAccount(t *testing.T) {
	c, _ := newImagesCooldownContext(t)
	repo := &countingModelRateLimitRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := imagesCooldownAccount()

	upstreamErr := &OpenAIImagesUpstreamError{
		StatusCode: http.StatusBadGateway,
		ErrorType:  "upstream_error",
		Code:       "image_generation_unavailable",
		Message:    "image generation tool is not available for this account",
	}

	_ = svc.handleOpenAIImagesOAuthResponseError(
		context.Background(), c, account, "gpt-image-2", "https://upstream.example/v1/responses",
		&http.Response{StatusCode: http.StatusOK, Header: http.Header{}},
		OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c), upstreamErr,
	)

	require.Equal(t, 1, repo.calls, "结构化上游证据仍须写冷却")
	require.Equal(t, []string{openAIImageGenerationRateLimitKey}, repo.scopes)
}

func TestOpenAIImagesTextFallback_MarksSynthesizedVerdicts(t *testing.T) {
	t.Run("plain_text_reply_is_synthesized", func(t *testing.T) {
		err := openAIImagesTextFallbackErrorForText("Here's a polished image prompt for your request.")
		require.NotNil(t, err)
		require.True(t, err.SynthesizedFromModelText)
		require.Equal(t, "image_generation_unavailable", err.Code)
		require.Equal(t, http.StatusBadGateway, err.StatusCode)
	})

	t.Run("body_entrypoint_is_synthesized", func(t *testing.T) {
		body := []byte("event: response.completed\n" +
			`data: {"type":"response.completed","response":{"id":"r","status":"completed",` +
			`"output":[{"type":"message","content":[{"type":"output_text","text":"I drafted a prompt for you."}]}]}}` +
			"\n\n")
		err := openAIImagesTextFallbackError(body)
		require.NotNil(t, err)
		require.True(t, err.SynthesizedFromModelText)
	})

	t.Run("content_policy_branch_unchanged", func(t *testing.T) {
		err := openAIImagesTextFallbackErrorForText("Blocked by our content policy.")
		require.NotNil(t, err)
		require.Equal(t, "content_policy_violation", err.Code)
		require.Equal(t, http.StatusBadRequest, err.StatusCode)
		require.False(t, err.SynthesizedFromModelText)
	})

	t.Run("empty_text_yields_no_error", func(t *testing.T) {
		require.Nil(t, openAIImagesTextFallbackErrorForText("   "))
	})
}

func TestOpenAIImagesTextFallback_RemainsRetryableAndThusCascades(t *testing.T) {
	err := openAIImagesTextFallbackErrorForText("Here's a polished image prompt for your request.")
	require.NotNil(t, err)
	require.True(t, IsOpenAIImagesRetryableUpstreamError(err),
		"文字兜底判据是可重试的——正因如此，写账号冷却会沿号池级联")
}
