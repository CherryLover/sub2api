package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSIngressEndedByClient_BareNormalClosureIsNotAccountFailure(t *testing.T) {
	err := coderws.CloseError{Code: coderws.StatusNormalClosure, Reason: "client done"}

	var closeErr *service.OpenAIWSClientCloseError
	require.False(t, errors.As(err, &closeErr),
		"裸 coderws.CloseError 不是 *OpenAIWSClientCloseError，旧的 errors.As 必然为假")
	require.Equal(t, coderws.StatusNormalClosure, coderws.CloseStatus(err))
	require.True(t, openAIWSIngressEndedByClient(err))
}

func TestOpenAIWSIngressEndedByClient_WrappedBareNormalClosureIsNotAccountFailure(t *testing.T) {
	err := fmt.Errorf("ingress turn 3: %w",
		coderws.CloseError{Code: coderws.StatusNormalClosure, Reason: "client done"})

	require.True(t, openAIWSIngressEndedByClient(err))
}

func TestOpenAIWSIngressEndedByClient_ClientCancelDuringTurnIsNotAccountFailure(t *testing.T) {
	err := service.NewOpenAIWSClientCloseError(
		coderws.StatusGoingAway, "websocket request canceled", context.Canceled)

	var closeErr *service.OpenAIWSClientCloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, coderws.StatusGoingAway, closeErr.StatusCode())
	require.NotEqual(t, coderws.StatusNormalClosure, closeErr.StatusCode())
	require.True(t, openAIWSIngressEndedByClient(err))
}

func TestOpenAIWSIngressEndedByClient_GatewayNormalClosureStillRecognised(t *testing.T) {
	err := service.NewOpenAIWSClientCloseError(
		coderws.StatusNormalClosure, "websocket idle timeout", context.DeadlineExceeded)

	require.True(t, openAIWSIngressEndedByClient(err))
}

func TestOpenAIWSIngressEndedByClient_GoingAwayWithoutCancellationStillReported(t *testing.T) {
	err := service.NewOpenAIWSClientCloseError(
		coderws.StatusGoingAway, "upstream going away", errors.New("upstream closed session"))

	require.False(t, openAIWSIngressEndedByClient(err))
	require.True(t, shouldReportOpenAIWSProxyAccountFailure(err), "真实上游故障仍须归因账号")
}

func TestOpenAIWSIngressEndedByClient_AbnormalClosuresStillReportAccountFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{
			name: "upstream_policy_violation",
			err: service.NewOpenAIWSClientCloseError(
				coderws.StatusPolicyViolation, "upstream websocket authentication failed",
				errors.New("upstream rejected credentials")),
		},
		{
			name: "upstream_internal_error",
			err: service.NewOpenAIWSClientCloseError(
				coderws.StatusInternalError, "upstream websocket proxy failed", nil),
		},
		{
			name: "bare_abnormal_closure",
			err:  coderws.CloseError{Code: coderws.StatusAbnormalClosure, Reason: "connection reset"},
		},
		{
			name: "generic_read_failure",
			err:  errors.New("upstream websocket read failed"),
		},
		{
			name: "deadline_without_normal_close",
			err:  fmt.Errorf("upstream stalled: %w", context.DeadlineExceeded),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.False(t, openAIWSIngressEndedByClient(tc.err))
			require.True(t, shouldReportOpenAIWSProxyAccountFailure(tc.err))
		})
	}
}

func TestOpenAIWSIngressEndedByClient_MatchesCloseCodeReportedInLog(t *testing.T) {
	errs := []error{
		coderws.CloseError{Code: coderws.StatusNormalClosure, Reason: "client done"},
		fmt.Errorf("ingress turn 3: %w", coderws.CloseError{Code: coderws.StatusNormalClosure}),
		service.NewOpenAIWSClientCloseError(coderws.StatusNormalClosure, "websocket idle timeout", context.DeadlineExceeded),
		service.NewOpenAIWSClientCloseError(coderws.StatusGoingAway, "websocket request canceled", context.Canceled),
		coderws.CloseError{Code: coderws.StatusAbnormalClosure, Reason: "connection reset"},
		errors.New("upstream websocket read failed"),
	}

	for _, err := range errs {
		t.Run(err.Error(), func(t *testing.T) {
			closeStatus, _ := summarizeWSCloseErrorForLog(err)
			if closeStatus == "1000(StatusNormalClosure)" {
				require.True(t, openAIWSIngressEndedByClient(err),
					"日志按 1000 归类为正常关闭，归因侧不得同时判为账号故障")
			}
		})
	}
}
