package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPelicanDuplicateSubmissionReturnsFrozenResultBeforeEnabledCheck(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	r := &pelicanRepository{db: db}
	run := service.PelicanRun{PelicanPublicRun: service.PelicanPublicRun{ID: "00000000-0000-0000-0000-000000000001", Model: "original", State: "unknown"}, PlanID: 2, RequestedBy: 3, Frozen: service.PelicanPlanInput{Model: "original"}, ReservedUSD: 0.5}
	b, _ := json.Marshal(run)
	m.ExpectBegin()
	m.ExpectQuery("SELECT enabled,daily_budget_usd.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"enabled", "budget"}).AddRow(false, 0))
	m.ExpectQuery("SELECT result FROM pelican_runs WHERE id").WithArgs(run.ID).WillReturnRows(sqlmock.NewRows([]string{"result"}).AddRow(b))
	m.ExpectRollback()
	got, err := r.Submit(context.Background(), 2, 3, run.ID, false)
	require.NoError(t, err)
	require.Equal(t, run.Frozen, got.Frozen)
	require.Equal(t, "unknown", got.State)
	require.NoError(t, m.ExpectationsWereMet())
}
func TestPelicanBudgetReservationUsesLockedDailyBudget(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	r := &pelicanRepository{db: db}
	input := service.PelicanPlanInput{GroupID: 1, Model: "alias", BudgetUSD: 0.75, IntervalMinutes: 30}
	b, _ := json.Marshal(input)
	m.ExpectBegin()
	m.ExpectQuery("SELECT enabled,daily_budget_usd.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"enabled", "budget"}).AddRow(true, 1))
	m.ExpectQuery("SELECT result FROM pelican_runs WHERE id").WillReturnError(sql.ErrNoRows)
	m.ExpectQuery("SELECT EXISTS.*state='running'").WillReturnRows(sqlmock.NewRows([]string{"busy"}).AddRow(false))
	m.ExpectQuery("SELECT p.id.*FOR UPDATE OF p").WillReturnRows(sqlmock.NewRows([]string{"id", "config", "name", "last", "next", "lease"}).AddRow(2, b, "public", nil, time.Now().Add(time.Hour), nil))
	m.ExpectQuery("SELECT COALESCE\\(SUM\\(GREATEST").WillReturnRows(sqlmock.NewRows([]string{"spent"}).AddRow(0.5))
	m.ExpectRollback()
	_, err = r.Submit(context.Background(), 2, 3, "00000000-0000-0000-0000-000000000001", false)
	require.ErrorIs(t, err, service.ErrPelicanUnavailable)
	require.NoError(t, m.ExpectationsWereMet())
}
func TestPelicanPublicVisibilityUsesEmptyPermissionSetAndThinList(t *testing.T) {
	where, args := pelicanWhere(service.PelicanFilter{}, []int64{}, true)
	require.Contains(t, where, "r.group_id=ANY($1)")
	require.Contains(t, where, "p.published")
	require.Contains(t, where, "enabled")
	require.Len(t, args, 1)
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	r := &pelicanRepository{db: db}
	m.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total", "completed", "failed", "unknown", "running"}).AddRow(0, 0, 0, 0, 0))
	m.ExpectQuery("SELECT r.result-'output'-'raw_response'-'usage'-'timing'.*ORDER BY r.created_at DESC,r.id DESC LIMIT").WillReturnRows(sqlmock.NewRows([]string{"result"}))
	items, total, _, err := r.Results(context.Background(), service.PelicanFilter{Page: 1, PageSize: 12}, []int64{}, true)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Zero(t, total)
	require.NoError(t, m.ExpectationsWereMet())
}

func TestPelicanLatestPerGroupRanksFilteredRowsBeforePaging(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	run := service.PelicanRun{PelicanPublicRun: service.PelicanPublicRun{
		ID:           "00000000-0000-0000-0000-000000000002",
		GroupID:      9,
		Model:        "gpt-5",
		Effort:       "high",
		State:        "failed",
		HistoryCount: 5,
	}}
	b, _ := json.Marshal(run)
	m.ExpectQuery(`WITH filtered AS .*COUNT\(\*\) OVER\(PARTITION BY r\.group_id\).*ROW_NUMBER\(\) OVER\(PARTITION BY r\.group_id ORDER BY r\.created_at DESC,r\.id DESC\).*WHERE NOT r\.payload_pruned.*r\.group_id=ANY\(\$1\).*p\.published.*enabled.*SELECT COUNT\(\*\).*state='failed'.*FROM filtered WHERE group_rank=1`).
		WillReturnRows(sqlmock.NewRows([]string{"total", "completed", "failed", "unknown", "running"}).AddRow(2, 1, 1, 0, 0))
	m.ExpectQuery(`WITH filtered AS .*SELECT result,history_count FROM filtered WHERE group_rank=1 ORDER BY created_at DESC,id DESC LIMIT \$4 OFFSET \$5`).
		WillReturnRows(sqlmock.NewRows([]string{"result", "history_count"}).AddRow(b, 5))

	r := &pelicanRepository{db: db}
	items, total, stats, err := r.Results(context.Background(), service.PelicanFilter{
		Model:          "gpt-5",
		Effort:         "high",
		Page:           1,
		PageSize:       1,
		LatestPerGroup: true,
	}, []int64{9}, true)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Equal(t, service.PelicanStats{Failed: 1, Completed: 1}, stats)
	require.Len(t, items, 1)
	require.Equal(t, 5, items[0].HistoryCount)
	require.Equal(t, "failed", items[0].State)
	require.NoError(t, m.ExpectationsWereMet())
}
