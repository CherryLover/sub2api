package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRoutingTraceDiagnosisCandidatesAndChecks(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	resetAt := now.Add(10 * time.Minute)
	trace := NewRoutingTrace(RoutingTraceMeta{
		RequestID: "req-diagnosis",
		GroupID:   7,
		GroupName: "group-a",
		Model:     "gpt-5",
	})
	trace.now = func() time.Time { return now }

	excluded := &Account{ID: 1, Name: "excluded", Platform: PlatformOpenAI}
	rateLimited := &Account{ID: 2, Name: "rate-limited", Platform: PlatformOpenAI, RateLimitResetAt: &resetAt}
	unsupported := &Account{ID: 3, Name: "unsupported", Platform: PlatformOpenAI}

	trace.beginCandidates(3)
	trace.exclude(excluded, OpsNoAccountReasonExcluded)
	trace.exclude(rateLimited, OpsNoAccountReasonModelRateLimited)
	trace.exclude(unsupported, OpsNoAccountReasonModelNotSupported)
	trace.finishSelection(nil, false, errors.New("no available accounts (pool=3, filtered: model_not_supported=1 model_rate_limited=1 excluded=1)"))

	diagnosis := trace.Diagnosis("")
	require.NotNil(t, diagnosis)
	require.Equal(t, int64(7), diagnosis.GroupID)
	require.Equal(t, "group-a", diagnosis.GroupName)
	require.Equal(t, "gpt-5", diagnosis.Model)
	require.Equal(t, 3, diagnosis.PoolSize)
	require.Equal(t, OpsNoAccountReasonExcluded, diagnosis.Reason)
	require.Contains(t, diagnosis.Raw, "pool=3")
	require.Len(t, diagnosis.Candidates, 3)

	require.Nil(t, diagnosis.Candidates[0].Checks)
	require.Equal(t, OpsNoAccountReasonExcluded, diagnosis.Candidates[0].Reason)

	require.Equal(t, map[string]bool{
		RoutingGatePlatform:    true,
		RoutingGateSchedulable: true,
		RoutingGateQuota:       false,
	}, diagnosis.Candidates[1].Checks)
	require.Equal(t, OpsNoAccountReasonModelRateLimited, diagnosis.Candidates[1].Reason)
	require.NotNil(t, diagnosis.Candidates[1].RecoverAt)
	require.Equal(t, resetAt, *diagnosis.Candidates[1].RecoverAt)

	require.Equal(t, map[string]bool{
		RoutingGatePlatform:    true,
		RoutingGateSchedulable: true,
		RoutingGateQuota:       true,
		RoutingGateModel:       false,
	}, diagnosis.Candidates[2].Checks)
	require.Equal(t, OpsNoAccountReasonModelNotSupported, diagnosis.Candidates[2].Reason)
}

func TestRoutingTraceMethodDetection(t *testing.T) {
	tests := []struct {
		name           string
		accounts       int
		sticky         bool
		wantMethod     string
		wantCandidates int
	}{
		{name: "single", accounts: 1, wantMethod: RoutingMethodSingle, wantCandidates: 1},
		{name: "load balance", accounts: 3, wantMethod: RoutingMethodLoadBalance, wantCandidates: 3},
		{name: "sticky wins over load balance", accounts: 3, sticky: true, wantMethod: RoutingMethodSticky, wantCandidates: 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trace := NewRoutingTrace(RoutingTraceMeta{RequestID: "req-method"})
			accounts := make([]*Account, 0, tc.accounts)
			trace.beginCandidates(tc.accounts)
			for i := 0; i < tc.accounts; i++ {
				account := &Account{ID: int64(i + 1), Name: "account", Platform: PlatformOpenAI}
				accounts = append(accounts, account)
				trace.pass(account)
			}
			if tc.sticky {
				trace.markSticky()
			}
			trace.finishSelection(&AccountSelectionResult{Account: accounts[0]}, tc.sticky, nil)

			info := trace.Info()
			require.NotNil(t, info)
			require.Equal(t, tc.wantMethod, info.Method)
			require.Equal(t, tc.wantCandidates, info.CandidateCount)
			require.NotEmpty(t, info.Candidates)
			require.True(t, info.Candidates[0].Chosen)
		})
	}
}

func TestRoutingTraceAttemptNextActionsAndEvents(t *testing.T) {
	rec := &recordingRoutingObserver{}
	unregister := RegisterRoutingObserver(rec)
	defer unregister()

	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	cooldownUntil := now.Add(time.Minute)
	trace := NewRoutingTrace(RoutingTraceMeta{RequestID: "req-attempts"})
	trace.now = func() time.Time { return now }
	account := &Account{ID: 9, Name: "account-9", Platform: PlatformOpenAI}

	trace.BeginAttempt(account)
	now = now.Add(150 * time.Millisecond)
	trace.EndAttempt(http.StatusTooManyRequests, true, &cooldownUntil)
	trace.ResolveAttempt(RoutingNextSameAccount)

	trace.BeginAttempt(account)
	now = now.Add(250 * time.Millisecond)
	trace.EndAttempt(http.StatusBadGateway, true, nil)
	trace.ResolveAttempt(RoutingNextSwitch)

	trace.BeginAttempt(account)
	now = now.Add(50 * time.Millisecond)
	trace.EndAttempt(http.StatusOK, false, nil)
	trace.Finish(http.StatusOK)

	info := trace.Info()
	require.NotNil(t, info)
	require.Len(t, info.Attempts, 3)
	require.Equal(t, RoutingNextSameAccount, info.Attempts[0].Next)
	require.Equal(t, int64(150), info.Attempts[0].DurationMs)
	require.Equal(t, RoutingNextSwitch, info.Attempts[1].Next)
	require.Equal(t, int64(250), info.Attempts[1].DurationMs)
	require.Empty(t, info.Attempts[2].Next)

	require.Len(t, rec.events, 6)
	require.Equal(t, RoutingEventAttempt, rec.events[0].Type)
	require.Equal(t, RoutingEventAttemptFailed, rec.events[1].Type)
	require.Equal(t, RoutingNextSameAccount, rec.events[1].Next)
	require.NotNil(t, rec.events[1].CooldownUntil)
	require.Equal(t, cooldownUntil, *rec.events[1].CooldownUntil)
	require.Equal(t, RoutingEventAttempt, rec.events[2].Type)
	require.Equal(t, RoutingEventAttemptFailed, rec.events[3].Type)
	require.Equal(t, RoutingNextSwitch, rec.events[3].Next)
	require.Equal(t, RoutingEventAttempt, rec.events[4].Type)
	require.Equal(t, RoutingEventRequestDone, rec.events[5].Type)
	require.Equal(t, http.StatusOK, rec.events[5].StatusCode)
}

func TestRoutingTraceLifecycleEventsUseRecordingObserver(t *testing.T) {
	rec := &recordingRoutingObserver{}
	unregister := RegisterRoutingObserver(rec)
	defer unregister()

	trace := NewRoutingTrace(RoutingTraceMeta{
		RequestID: "req-lifecycle",
		APIKeyID:  11,
		GroupID:   22,
		Model:     "gpt-5",
	})
	trace.EmitStart()
	trace.beginSelection()
	trace.beginCandidates(1)
	trace.exclude(&Account{ID: 33, Name: "blocked"}, OpsNoAccountReasonNotSchedulable)
	trace.finishSelection(nil, false, errors.New("no available accounts (pool=1, filtered: not_schedulable=1)"))
	trace.Finish(0)

	require.Len(t, rec.events, 3)
	require.Equal(t, RoutingEventRequestStart, rec.events[0].Type)
	require.Equal(t, "req-lifecycle", rec.events[0].RequestID)
	require.Equal(t, int64(11), rec.events[0].APIKeyID)
	require.Equal(t, int64(22), rec.events[0].GroupID)
	require.Equal(t, RoutingEventBlocked, rec.events[1].Type)
	require.Equal(t, OpsNoAccountReasonNotSchedulable, rec.events[1].Reason)
	require.Equal(t, RoutingEventRequestDone, rec.events[2].Type)
	require.Equal(t, http.StatusServiceUnavailable, rec.events[2].StatusCode)
}

func TestRoutingTraceDiagnosisAndInfoCapsAndJSONShape(t *testing.T) {
	diagnosisTrace := NewRoutingTrace(RoutingTraceMeta{RequestID: "req-caps"})
	diagnosisTrace.beginCandidates(RoutingDiagnosisMaxCandidates + 5)
	for i := 0; i < RoutingDiagnosisMaxCandidates+5; i++ {
		diagnosisTrace.exclude(&Account{ID: int64(i + 1), Name: "candidate"}, OpsNoAccountReasonNotSchedulable)
	}
	for i := 0; i < RoutingDiagnosisMaxAttempts+3; i++ {
		account := &Account{ID: int64(i + 1), Name: "attempt-account"}
		diagnosisTrace.BeginAttempt(account)
		diagnosisTrace.EndAttempt(http.StatusBadGateway, true, nil)
		diagnosisTrace.ResolveAttempt(RoutingNextSwitch)
	}
	diagnosisTrace.finishSelection(nil, false, errors.New("no available accounts (pool=25, filtered: not_schedulable=25)"))

	diagnosis := diagnosisTrace.Diagnosis(OpsNoAccountReasonNotSchedulable)
	require.NotNil(t, diagnosis)
	require.Len(t, diagnosis.Candidates, RoutingDiagnosisMaxCandidates)
	require.Len(t, diagnosis.Attempts, RoutingDiagnosisMaxAttempts)
	diagnosisJSON, err := json.Marshal(diagnosis)
	require.NoError(t, err)
	require.True(t, json.Valid(diagnosisJSON))

	var diagnosisShape map[string]any
	require.NoError(t, json.Unmarshal(diagnosisJSON, &diagnosisShape))
	require.Contains(t, diagnosisShape, "pool_size")
	require.Contains(t, diagnosisShape, "candidates")
	require.Contains(t, diagnosisShape, "attempts")
	require.Contains(t, diagnosisShape, "reason")
	require.Contains(t, diagnosisShape, "raw")

	infoTrace := NewRoutingTrace(RoutingTraceMeta{RequestID: "req-info-caps"})
	infoTrace.beginCandidates(RoutingInfoMaxCandidates + 5)
	var selected *Account
	for i := 0; i < RoutingInfoMaxCandidates+5; i++ {
		account := &Account{ID: int64(i + 1), Name: "eligible"}
		if selected == nil {
			selected = account
		}
		infoTrace.pass(account)
	}
	infoTrace.finishSelection(&AccountSelectionResult{Account: selected}, false, nil)

	info := infoTrace.Info()
	require.NotNil(t, info)
	require.Equal(t, RoutingInfoMaxCandidates+5, info.CandidateCount)
	require.Len(t, info.Candidates, RoutingInfoMaxCandidates)
	infoJSON, err := json.Marshal(info)
	require.NoError(t, err)
	require.True(t, json.Valid(infoJSON))

	var infoShape map[string]any
	require.NoError(t, json.Unmarshal(infoJSON, &infoShape))
	require.Contains(t, infoShape, "method")
	require.Contains(t, infoShape, "candidate_count")
	require.Contains(t, infoShape, "candidates")
}

func TestRoutingTraceRawIsTruncated(t *testing.T) {
	trace := NewRoutingTrace(RoutingTraceMeta{RequestID: "req-raw"})
	trace.beginSelection()
	raw := strings.Repeat("x", routingTraceRawMaxLen+25)
	trace.finishSelection(nil, false, errors.New(raw))

	diagnosis := trace.Diagnosis("")
	require.NotNil(t, diagnosis)
	require.Len(t, diagnosis.Raw, routingTraceRawMaxLen)
}

func TestRoutingTraceContextRoundTrip(t *testing.T) {
	trace := NewRoutingTrace(RoutingTraceMeta{RequestID: "req-context"})
	ctx := WithRoutingTrace(context.Background(), trace)
	require.Same(t, trace, RoutingTraceFromContext(ctx))
	require.Nil(t, RoutingTraceFromContext(context.Background()))
}
