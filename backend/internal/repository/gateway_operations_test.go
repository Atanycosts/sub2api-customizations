package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGatewayOperationsBudgetReservationIsAtomic(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &gatewayOperationsRepository{db: db}
	run := &service.GatewayExperiment{ID: "42d8894f-c026-4dc5-8e0a-10a88af16832", RequestedBy: 7, ReservedUSD: 2}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT daily_budget_usd.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"budget"}).AddRow(3))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(run.ID).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("SELECT COALESCE\\(SUM").WillReturnRows(sqlmock.NewRows([]string{"used"}).AddRow(2))
	mock.ExpectRollback()
	require.ErrorContains(t, repo.CreateExperiment(context.Background(), run), "budget exhausted")
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestGatewayOperationsDuplicateSubmissionCannotReserveTwice(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &gatewayOperationsRepository{db: db}
	run := &service.GatewayExperiment{ID: "42d8894f-c026-4dc5-8e0a-10a88af16832", RequestedBy: 7, ReservedUSD: 2}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT daily_budget_usd.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"budget"}).AddRow(3))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(run.ID).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()
	require.ErrorIs(t, repo.CreateExperiment(context.Background(), run), service.ErrGatewayExperimentExists)
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestGatewayOperationsExactUsageRequiresKeyOwnership(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &gatewayOperationsRepository{db: db}
	mock.ExpectQuery("SELECT EXISTS.*api_keys").WithArgs(int64(3), int64(7)).WillReturnRows(sqlmock.NewRows([]string{"own"}).AddRow(false))
	_, err = repo.CacheUsage(context.Background(), 7, 3, []string{"client:unique"})
	require.ErrorContains(t, err, "belong")
	require.NoError(t, mock.ExpectationsWereMet())
}
func TestGatewayOperationsCrashRetainsUnknownReservation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &gatewayOperationsRepository{db: db}
	run := &service.GatewayExperiment{ID: "42d8894f-c026-4dc5-8e0a-10a88af16832", RequestedBy: 7, ReservedUSD: 2, State: "running"}
	frozen, err := json.Marshal(run)
	require.NoError(t, err)
	mock.ExpectQuery("SELECT frozen,result,state,created_at").WithArgs(run.ID, int64(7)).WillReturnRows(sqlmock.NewRows([]string{"frozen", "result", "state", "created_at"}).AddRow(frozen, []byte(`{}`), "running", time.Now().Add(-10*time.Minute)))
	result, err := repo.Experiment(context.Background(), run.ID, 7)
	require.NoError(t, err)
	require.Equal(t, "unknown", result.State)
	require.Equal(t, 2.0, result.ReservedUSD)
	require.NoError(t, mock.ExpectationsWereMet())
}
