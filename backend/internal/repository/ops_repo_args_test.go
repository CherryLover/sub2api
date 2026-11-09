//go:build unit

package repository

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsInsertErrorLogArgsPreservesExplicitZeroUpstreamStatus(t *testing.T) {
	zero := 0
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{UpstreamStatusCode: &zero})

	require.Len(t, args, 42)
	encoded, ok := args[27].(sql.NullInt64)
	require.True(t, ok)
	require.True(t, encoded.Valid)
	require.Zero(t, encoded.Int64)
}

func TestOpsInsertErrorLogArgsIncludesErrorReason(t *testing.T) {
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{
		ErrorReason: service.OpsNoAccountReasonModelRateLimited,
	})

	encoded, ok := args[38].(sql.NullString)
	require.True(t, ok)
	require.True(t, encoded.Valid)
	require.Equal(t, service.OpsNoAccountReasonModelRateLimited, encoded.String)
}

func TestOpsInsertErrorLogArgsIncludesRequestBodyBytes(t *testing.T) {
	size := int64(8995454)
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{RequestBodyBytes: &size})

	encoded, ok := args[39].(sql.NullInt64)
	require.True(t, ok)
	require.True(t, encoded.Valid)
	require.EqualValues(t, size, encoded.Int64)

	missing, ok := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{})[39].(sql.NullInt64)
	require.True(t, ok)
	require.False(t, missing.Valid)
}

func TestOpsInsertErrorLogArgsIncludesReasoningEffort(t *testing.T) {
	effort := "high"
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{ReasoningEffort: &effort})

	encoded, ok := args[40].(sql.NullString)
	require.True(t, ok)
	require.True(t, encoded.Valid)
	require.Equal(t, effort, encoded.String)

	missing, ok := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{})[40].(sql.NullString)
	require.True(t, ok)
	require.False(t, missing.Valid)
}

func TestOpsNullableIntPointerDistinguishesNilZeroAndStatus(t *testing.T) {
	missing := opsNullableIntPointer(nil).(sql.NullInt64)
	require.False(t, missing.Valid)

	zeroValue := 0
	zero := opsNullableIntPointer(&zeroValue).(sql.NullInt64)
	require.True(t, zero.Valid)
	require.Zero(t, zero.Int64)

	statusValue := 503
	status := opsNullableIntPointer(&statusValue).(sql.NullInt64)
	require.True(t, status.Valid)
	require.EqualValues(t, 503, status.Int64)
}

func TestOpsInsertErrorLogArgsIncludesRoutingDiagnosis(t *testing.T) {
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{
		ErrorReason: service.OpsNoAccountReasonModelRateLimited,
		RoutingDiagnosis: &service.RoutingDiagnosis{
			GroupID:  3,
			PoolSize: 2,
			Reason:   service.OpsNoAccountReasonModelRateLimited,
			Raw:      "no available accounts (pool=2, filtered: model_rate_limited=2)",
			Candidates: []service.RoutingCandidate{
				{AccountID: 11, AccountName: "a", Checks: map[string]bool{"platform": true, "schedulable": true, "quota": false}, Reason: "model_rate_limited"},
			},
		},
	})

	require.Len(t, args, 42)
	encoded, ok := args[41].(sql.NullString)
	require.True(t, ok)
	require.True(t, encoded.Valid)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(encoded.String), &decoded))
	require.EqualValues(t, 2, decoded["pool_size"])
	require.Equal(t, "model_rate_limited", decoded["reason"])
	candidates, ok := decoded["candidates"].([]any)
	require.True(t, ok)
	require.Len(t, candidates, 1)

	missing, ok := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{})[41].(sql.NullString)
	require.True(t, ok)
	require.False(t, missing.Valid)
}

func TestInsertOpsErrorLogSQLIncludesRoutingDiagnosisColumn(t *testing.T) {
	require.Contains(t, insertOpsErrorLogSQL, "routing_diagnosis")
	require.Contains(t, insertOpsErrorLogSQL, "$42")
	require.NotContains(t, insertOpsErrorLogSQL, "$43")
}

func TestOpsDecodeRoutingDiagnosis(t *testing.T) {
	require.Nil(t, opsDecodeRoutingDiagnosis(sql.NullString{}))
	require.Nil(t, opsDecodeRoutingDiagnosis(sql.NullString{String: "not-json", Valid: true}))

	d := opsDecodeRoutingDiagnosis(sql.NullString{String: `{"pool_size":4,"candidates":[],"reason":"excluded"}`, Valid: true})
	require.NotNil(t, d)
	require.Equal(t, 4, d.PoolSize)
	require.Equal(t, "excluded", d.Reason)
}
