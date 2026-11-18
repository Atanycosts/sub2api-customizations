//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRequestTimingPostgresLateLinkAndFilter(t *testing.T) {
	ctx := context.Background()
	repo := NewRequestTimingRepository(integrationDB)
	id, requestID, usageRequestID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, "DELETE FROM gateway_request_timing_usage WHERE trace_id=$1", id)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, "DELETE FROM gateway_request_timings WHERE trace_id=$1", id)
		require.NoError(t, err)
	})
	trace := requesttiming.Trace{ID: id, RequestID: requestID, StartedAt: time.Now(), Outcome: "upstream_error", Completeness: "partial", Attempts: []requesttiming.Attempt{{Number: 1, AccountID: 19, Outcome: "upstream_error"}}}
	require.NoError(t, repo.Save(ctx, trace))
	require.NoError(t, repo.Save(ctx, trace))
	// A late usage link is accepted after the response trace has frozen.
	require.NoError(t, repo.LinkUsage(ctx, service.RequestTimingLink{TraceID: id, UsageID: 9123456789, RequestID: usageRequestID, APIKeyID: 19}))
	stored, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, int64(9123456789), *stored.UsageID)
	require.Equal(t, usageRequestID, stored.UsageRequestID)
	rows, err := repo.List(ctx, service.RequestTimingFilter{RequestID: usageRequestID, AccountID: 19, Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, id, rows[0].ID)
	rows, err = repo.List(ctx, service.RequestTimingFilter{RequestID: requestID, AccountID: 20, Limit: 10})
	require.NoError(t, err)
	require.Empty(t, rows)
}
