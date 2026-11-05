//go:build unit

package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildOpsErrorLogsWhereOtherErrorKindMatchesNothing(t *testing.T) {
	where, args := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{ErrorKind: "other"})

	require.Contains(t, where, "1=0")
	require.NotContains(t, where, "("+opsErrorKindExpr+") = $")
	require.Empty(t, args)
}

func TestBuildOpsErrorLogsWhereErrorKindAndRequestSizePlaceholdersMatchArgs(t *testing.T) {
	kind := "routing|503|model_rate_limited"
	minRequestBodyBytes := int64(4 * 1024 * 1024)
	where, args := buildOpsErrorLogsWhere(&service.OpsErrorLogFilter{
		ErrorKind:           kind,
		MinRequestBodyBytes: minRequestBodyBytes,
	})

	require.Contains(t, where, "e.request_body_bytes >= $1")
	require.Contains(t, where, "("+opsErrorKindExpr+") = $2")
	require.Equal(t, []any{minRequestBodyBytes, kind}, args)
}
