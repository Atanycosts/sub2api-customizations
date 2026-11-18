//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// This test uses PostgreSQL's real row lock and reservation query. SQL mocks
// cannot detect an invalid JSONB expression or a reservation lost after commit.
func TestGatewayOperationsPostgresReservationAndDuplicate(t *testing.T) {
	ctx := context.Background()
	user := mustCreateUser(t, integrationEntClient, &service.User{Email: uuid.NewString() + "@example.test"})
	repo := &gatewayOperationsRepository{db: integrationDB}
	previous, err := repo.Config(ctx)
	require.NoError(t, err)
	id := uuid.NewString()
	t.Cleanup(func() {
		_, e := integrationDB.ExecContext(ctx, "DELETE FROM gateway_operations_experiments WHERE requested_by=$1", user.ID)
		require.NoError(t, e)
		require.NoError(t, repo.SaveConfig(ctx, previous))
		_, e = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id=$1", user.ID)
		require.NoError(t, e)
	})
	require.NoError(t, repo.SaveConfig(ctx, service.GatewayOperationsConfig{RefreshMinutes: 30, DailyBudgetUSD: 1}))
	run := &service.GatewayExperiment{ID: id, RequestedBy: user.ID, ReservedUSD: 0.75, State: "running"}
	require.NoError(t, repo.CreateExperiment(ctx, run))
	require.ErrorIs(t, repo.CreateExperiment(ctx, run), service.ErrGatewayExperimentExists)
	second := &service.GatewayExperiment{ID: uuid.NewString(), RequestedBy: user.ID, ReservedUSD: 0.5, State: "running"}
	require.ErrorContains(t, repo.CreateExperiment(ctx, second), "budget exhausted")
	// Unknown execution retains the reservation; it cannot free money for replay.
	run.State = "unknown"
	require.NoError(t, repo.FinishExperiment(ctx, run))
	require.ErrorContains(t, repo.CreateExperiment(ctx, second), "budget exhausted")
	stored, err := repo.Experiment(ctx, id, user.ID)
	require.NoError(t, err)
	require.Equal(t, "unknown", stored.State)
	require.InDelta(t, 0.75, stored.ReservedUSD, 0.000001)
	other, err := repo.Experiment(ctx, id, user.ID+1)
	require.NoError(t, err)
	require.Nil(t, other)
}
