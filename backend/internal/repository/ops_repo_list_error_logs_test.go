//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestListErrorLogsReadsRequestBodyBytes(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM ops_error_logs e`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	createdAt := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id",
		"created_at",
		"error_phase",
		"error_type",
		"error_owner",
		"error_source",
		"severity",
		"status_code",
		"platform",
		"model",
		"resolved",
		"resolved_at",
		"resolved_by_user_id",
		"resolved_by_user_name",
		"client_request_id",
		"request_id",
		"error_message",
		"user_id",
		"user_email",
		"api_key_id",
		"account_id",
		"account_name",
		"group_id",
		"group_name",
		"client_ip",
		"request_path",
		"stream",
		"inbound_endpoint",
		"upstream_endpoint",
		"requested_model",
		"upstream_model",
		"user_agent",
		"request_type",
		"api_key_name",
		"api_key_deleted_at",
		"request_body_bytes",
	}).
		AddRow(
			1, createdAt, "request", "invalid_request_error", "client", "client_request",
			"warning", 413, "openai", "gpt-5", false, nil, nil, "",
			"client-1", "request-1", "request body too large", nil, "",
			nil, nil, "", nil, "", nil, "/v1/responses", false,
			"/v1/responses", "", "gpt-5", "", "test-agent", nil, "", nil, int64(8995454),
		).
		AddRow(
			2, createdAt, "routing", "api_error", "platform", "gateway",
			"error", 503, "openai", "gpt-5", false, nil, nil, "",
			"client-2", "request-2", "Service temporarily unavailable", nil, "",
			nil, nil, "", nil, "", nil, "/v1/responses", false,
			"/v1/responses", "", "gpt-5", "", "test-agent", nil, "", nil, nil,
		)

	mock.ExpectQuery(`(?s)SELECT\s+e\.id,.*e\.request_body_bytes.*FROM ops_error_logs e.*LIMIT \$1 OFFSET \$2`).
		WithArgs(20, 0).
		WillReturnRows(rows)

	repo := &opsRepository{db: db}
	got, err := repo.ListErrorLogs(context.Background(), &service.OpsErrorLogFilter{})
	require.NoError(t, err)
	require.Equal(t, 2, got.Total)
	require.Len(t, got.Errors, 2)
	require.NotNil(t, got.Errors[0].RequestBodyBytes)
	require.EqualValues(t, 8995454, *got.Errors[0].RequestBodyBytes)
	require.Nil(t, got.Errors[1].RequestBodyBytes)
	require.NoError(t, mock.ExpectationsWereMet())
}
