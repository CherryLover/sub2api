//go:build unit

package repository

import (
	"database/sql"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpsInsertErrorLogArgsPreservesExplicitZeroUpstreamStatus(t *testing.T) {
	zero := 0
	args := opsInsertErrorLogArgs(&service.OpsInsertErrorLogInput{UpstreamStatusCode: &zero})

	require.Len(t, args, 40)
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
