//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPelicanPostgresBudgetIdempotencyPublicationAndRetention(t *testing.T) {
	ctx := context.Background()
	repo := &pelicanRepository{db: integrationDB}
	previous, err := repo.Config(ctx)
	require.NoError(t, err)
	group := mustCreateGroup(t, integrationEntClient, &service.Group{Name: "pelican-" + uuid.NewString(), Platform: service.PlatformOpenAI, Status: service.StatusActive})
	plan := &service.PelicanPlan{PelicanPlanInput: service.PelicanPlanInput{GroupID: group.ID, Model: "test-model", Effort: "high", Protocol: "chat", Published: true, Enabled: true, IntervalMinutes: 30, MaxOutputTokens: 1024, BudgetUSD: 0.75}, GroupName: group.Name, NextRunAt: time.Now().UTC().Add(time.Hour)}
	t.Cleanup(func() {
		_, e := integrationDB.ExecContext(ctx, `DELETE FROM pelican_runs WHERE group_id=$1`, group.ID)
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `DELETE FROM pelican_plans WHERE group_id=$1`, group.ID)
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `DELETE FROM groups WHERE id=$1`, group.ID)
		require.NoError(t, e)
		require.NoError(t, repo.SaveConfig(ctx, previous))
	})
	require.NoError(t, repo.SaveConfig(ctx, service.PelicanConfig{Enabled: true, DailyBudgetUSD: 1}))
	require.NoError(t, repo.SavePlan(ctx, plan))
	id := uuid.NewString()
	var wg sync.WaitGroup
	results := make([]*service.PelicanRun, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = repo.Submit(ctx, plan.ID, 7, id, false) }(i)
	}
	wg.Wait()
	for i := range 2 {
		require.NoError(t, errs[i])
		require.Equal(t, id, results[i].ID)
	}
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pelican_runs WHERE id=$1`, id).Scan(&count))
	require.Equal(t, 1, count)
	plans, err := repo.Plans(ctx)
	require.NoError(t, err)
	for _, p := range plans {
		if p.ID == plan.ID {
			require.WithinDuration(t, plan.NextRunAt, p.NextRunAt, time.Millisecond)
		}
	}
	run := results[0]
	now := time.Now().UTC()
	run.State = "unknown"
	run.ErrorKind = "incomplete"
	run.FinishedAt = &now
	run.Output = "<html><svg></svg></html>"
	require.NoError(t, repo.Finish(ctx, run))
	plan.Model = "changed-model"
	require.NoError(t, repo.SavePlan(ctx, plan))
	duplicate, err := repo.Submit(ctx, plan.ID, 7, id, false)
	require.NoError(t, err)
	require.Equal(t, "test-model", duplicate.Model)
	require.Equal(t, "unknown", duplicate.State)
	_, err = repo.Submit(ctx, plan.ID, 7, uuid.NewString(), false)
	require.ErrorIs(t, err, service.ErrPelicanUnavailable)
	filter := service.PelicanFilter{Page: 1, PageSize: 12, PlanID: plan.ID}
	items, total, _, err := repo.Results(ctx, filter, []int64{group.ID}, true)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
	require.Empty(t, items[0].Output)
	_, total, _, err = repo.Results(ctx, filter, []int64{}, true)
	require.NoError(t, err)
	require.Zero(t, total)
	plan.Published = false
	require.NoError(t, repo.SavePlan(ctx, plan))
	_, total, _, err = repo.Results(ctx, filter, []int64{group.ID}, true)
	require.NoError(t, err)
	require.Zero(t, total)
	plan.Published = true
	require.NoError(t, repo.SavePlan(ctx, plan))
	require.NoError(t, repo.DeletePlan(ctx, plan.ID))
	_, total, _, err = repo.Results(ctx, filter, []int64{group.ID}, true)
	require.NoError(t, err)
	require.Zero(t, total)
	// Expiring history removes payload visibility but retains the reserved ledger and UUID.
	_, err = integrationDB.ExecContext(ctx, `UPDATE pelican_runs SET created_at=NOW()-INTERVAL '8 days' WHERE id=$1`, id)
	require.NoError(t, err)
	require.NoError(t, repo.Prune(ctx))
	_, err = repo.Result(ctx, id)
	require.ErrorIs(t, err, service.ErrPelicanNotFound)
	var reserved float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT reserved_usd FROM pelican_runs WHERE id=$1`, id).Scan(&reserved))
	require.Equal(t, 0.75, reserved)
	duplicate, err = repo.Submit(ctx, plan.ID, 7, id, false)
	require.NoError(t, err)
	require.Equal(t, "test-model", duplicate.Frozen.Model)
}
