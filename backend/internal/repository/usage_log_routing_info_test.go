package repository

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPrepareUsageLogInsertRoutingInfoJSON(t *testing.T) {
	info := &service.RoutingInfo{
		Method:         service.RoutingMethodLoadBalance,
		CandidateCount: 2,
		QueueWaitMs:    37,
		Candidates: []service.RoutingCandidate{
			{AccountID: 11, AccountName: "account-11", Chosen: true},
		},
		Attempts: []service.RoutingAttempt{
			{Attempt: 1, AccountID: 11, AccountName: "account-11", StatusCode: 200},
		},
	}
	prepared := prepareUsageLogInsert(&service.UsageLog{
		UserID:      1,
		APIKeyID:    2,
		AccountID:   11,
		RequestID:   "req-routing-info",
		Model:       "gpt-5",
		RoutingInfo: info,
		CreatedAt:   time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
	})

	require.Len(t, prepared.args, len(usageLogInsertArgTypes))
	require.Equal(t, "jsonb", usageLogInsertArgTypes[len(usageLogInsertArgTypes)-2])
	encoded, ok := prepared.args[len(prepared.args)-2].(string)
	require.True(t, ok)

	var decoded service.RoutingInfo
	require.NoError(t, json.Unmarshal([]byte(encoded), &decoded))
	require.Equal(t, service.RoutingMethodLoadBalance, decoded.Method)
	require.Equal(t, 2, decoded.CandidateCount)
	require.Equal(t, int64(37), decoded.QueueWaitMs)
	require.Len(t, decoded.Candidates, 1)
	require.Len(t, decoded.Attempts, 1)

	absent := prepareUsageLogInsert(&service.UsageLog{
		UserID:    1,
		APIKeyID:  2,
		AccountID: 11,
		RequestID: "req-no-routing-info",
		Model:     "gpt-5",
		CreatedAt: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC),
	})
	require.Nil(t, absent.args[len(absent.args)-2])
}

func TestRoutingInfoFromNullJSON(t *testing.T) {
	require.Nil(t, routingInfoFromNullJSON(sql.NullString{}))
	require.Nil(t, routingInfoFromNullJSON(sql.NullString{String: "not-json", Valid: true}))

	got := routingInfoFromNullJSON(sql.NullString{
		String: `{"method":"sticky","candidate_count":1,"queue_wait_ms":12}`,
		Valid:  true,
	})
	require.NotNil(t, got)
	require.Equal(t, service.RoutingMethodSticky, got.Method)
	require.Equal(t, 1, got.CandidateCount)
	require.Equal(t, int64(12), got.QueueWaitMs)
}

