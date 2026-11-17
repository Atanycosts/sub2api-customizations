//go:build integration

package repository

import (
	"context"
	"encoding/json"
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

func TestPelicanLatestPerGroupPostgresSummaries(t *testing.T) {
	ctx := context.Background()
	repo := &pelicanRepository{db: integrationDB}
	previous, err := repo.Config(ctx)
	require.NoError(t, err)

	newGroup := func() *service.Group {
		return mustCreateGroup(t, integrationEntClient, &service.Group{
			Name:     "pelican-dense-" + uuid.NewString(),
			Platform: service.PlatformOpenAI,
			Status:   service.StatusActive,
		})
	}
	groupA, groupB, groupC, groupD, groupE, groupF := newGroup(), newGroup(), newGroup(), newGroup(), newGroup(), newGroup()
	groups := []*service.Group{groupA, groupB, groupC, groupD, groupE, groupF}
	t.Cleanup(func() {
		for _, group := range groups {
			_, e := integrationDB.ExecContext(ctx, `DELETE FROM pelican_runs WHERE group_id=$1`, group.ID)
			require.NoError(t, e)
			_, e = integrationDB.ExecContext(ctx, `DELETE FROM pelican_plans WHERE group_id=$1`, group.ID)
			require.NoError(t, e)
			_, e = integrationDB.ExecContext(ctx, `DELETE FROM groups WHERE id=$1`, group.ID)
			require.NoError(t, e)
		}
		require.NoError(t, repo.SaveConfig(ctx, previous))
	})
	require.NoError(t, repo.SaveConfig(ctx, service.PelicanConfig{Enabled: true, DailyBudgetUSD: 1}))

	newPlan := func(group *service.Group, published bool) *service.PelicanPlan {
		plan := &service.PelicanPlan{
			PelicanPlanInput: service.PelicanPlanInput{
				GroupID:         group.ID,
				Model:           "gpt-5",
				Effort:          "high",
				Protocol:        "chat",
				Enabled:         true,
				Published:       published,
				IntervalMinutes: 30,
				MaxOutputTokens: 1024,
				BudgetUSD:       0.25,
			},
			GroupName: group.Name,
			NextRunAt: time.Now().UTC().Add(time.Hour),
		}
		require.NoError(t, repo.SavePlan(ctx, plan))
		return plan
	}
	planA := newPlan(groupA, true)
	planB := newPlan(groupB, true)
	planC := newPlan(groupC, true)
	planD := newPlan(groupD, true)
	planE := newPlan(groupE, true)
	planF := newPlan(groupF, false)

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	seed := func(plan *service.PelicanPlan, id, model, effort, state string, offset time.Duration, published, pruned bool) {
		run := service.PelicanRun{
			PelicanPublicRun: service.PelicanPublicRun{
				ID:              id,
				Source:          "group",
				GroupID:         plan.GroupID,
				GroupName:       plan.GroupName,
				Model:           model,
				Effort:          effort,
				Protocol:        plan.Protocol,
				PromptVersion:   service.GatewayPelicanVersion,
				MaxOutputTokens: plan.MaxOutputTokens,
				State:           state,
				CreatedAt:       base.Add(offset),
			},
			Prompt:      service.GatewayPelicanPrompt,
			PlanID:      plan.ID,
			RequestedBy: 7,
			Frozen:      plan.PelicanPlanInput,
			Published:   published,
			ReservedUSD: plan.BudgetUSD,
		}
		payload, e := json.Marshal(run)
		require.NoError(t, e)
		_, e = integrationDB.ExecContext(ctx, `INSERT INTO pelican_runs(id,plan_id,requested_by,group_id,published,created_at,state,result,reserved_usd,payload_pruned) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			id, plan.ID, 7, plan.GroupID, published, run.CreatedAt, state, payload, plan.BudgetUSD, pruned)
		require.NoError(t, e)
	}

	seed(planA, "10000000-0000-0000-0000-000000000001", "gpt-5", "high", "completed", 0, true, false)
	seed(planA, "10000000-0000-0000-0000-000000000002", "gpt-5", "high", "unknown", 350*time.Second, true, false)
	seed(planA, "10000000-0000-0000-0000-000000000003", "gpt-5", "high", "failed", 400*time.Second, true, false)
	seed(planA, "10000000-0000-0000-0000-000000000004", "o4-mini", "high", "completed", 500*time.Second, true, false)
	seed(planA, "10000000-0000-0000-0000-000000000005", "gpt-5", "high", "completed", 600*time.Second, true, true)
	seed(planA, "10000000-0000-0000-0000-000000000006", "gpt-5", "high", "completed", 700*time.Second, false, false)
	seed(planF, "40000000-0000-0000-0000-000000000007", "gpt-5", "high", "completed", 800*time.Second, true, false)
	seed(planB, "f0000000-0000-0000-0000-000000000001", "gpt-5", "high", "completed", 300*time.Second, true, false)
	seed(planB, "f0000000-0000-0000-0000-000000000002", "gpt-5", "high", "failed", 300*time.Second, true, false)
	seed(planC, "30000000-0000-0000-0000-000000000001", "gpt-5", "high", "running", 900*time.Second, true, false)
	seed(planD, "20000000-0000-0000-0000-000000000001", "gpt-5", "high", "completed", 100*time.Second, true, false)
	seed(planD, "20000000-0000-0000-0000-000000000002", "gpt-5", "high", "unknown", 200*time.Second, true, false)
	seed(planE, "e0000000-0000-0000-0000-000000000001", "gpt-5", "high", "running", 300*time.Second, true, false)
	seed(planA, "10000000-0000-0000-0000-000000000008", "gpt-5", "low", "failed", 1000*time.Second, true, false)
	seed(planA, "10000000-0000-0000-0000-000000000009", "o4-mini", "high", "failed", 1100*time.Second, true, false)

	authorized := []int64{groupA.ID, groupB.ID, groupD.ID, groupE.ID, groupF.ID}
	filter := service.PelicanFilter{Model: "gpt-5", Effort: "high", Page: 1, PageSize: 2}
	history, historyTotal, _, err := repo.Results(ctx, filter, authorized, true)
	require.NoError(t, err)
	require.Equal(t, 8, historyTotal)
	require.Len(t, history, 2)
	require.Equal(t, groupA.ID, history[0].GroupID)
	require.Equal(t, groupA.ID, history[1].GroupID)
	require.Zero(t, history[0].HistoryCount)

	tieFilter := service.PelicanFilter{GroupID: groupB.ID, Model: "gpt-5", Effort: "high", Page: 1, PageSize: 12}
	tiedHistory, tiedHistoryTotal, _, err := repo.Results(ctx, tieFilter, authorized, true)
	require.NoError(t, err)
	require.Equal(t, 2, tiedHistoryTotal)
	require.Len(t, tiedHistory, 2)
	require.Equal(t, []string{
		"f0000000-0000-0000-0000-000000000002",
		"f0000000-0000-0000-0000-000000000001",
	}, []string{tiedHistory[0].ID, tiedHistory[1].ID})
	tieFilter.LatestPerGroup = true
	tiedSummary, tiedSummaryTotal, _, err := repo.Results(ctx, tieFilter, authorized, true)
	require.NoError(t, err)
	require.Equal(t, 1, tiedSummaryTotal)
	require.Len(t, tiedSummary, 1)
	require.Equal(t, tiedHistory[0].ID, tiedSummary[0].ID)

	filter.LatestPerGroup = true
	first, total, stats, err := repo.Results(ctx, filter, authorized, true)
	require.NoError(t, err)
	require.Equal(t, 4, total)
	require.Equal(t, service.PelicanStats{Completed: 0, Failed: 2, Unknown: 1, Running: 1}, stats)
	require.Len(t, first, 2)
	require.Equal(t, "10000000-0000-0000-0000-000000000003", first[0].ID)
	require.Equal(t, "failed", first[0].State)
	require.Equal(t, 3, first[0].HistoryCount)
	require.Equal(t, "f0000000-0000-0000-0000-000000000002", first[1].ID)
	require.Equal(t, "failed", first[1].State)
	require.Equal(t, 2, first[1].HistoryCount)
	require.NotEqual(t, first[0].GroupID, first[1].GroupID)

	filter.Page = 2
	second, pageTwoTotal, pageTwoStats, err := repo.Results(ctx, filter, authorized, true)
	require.NoError(t, err)
	require.Equal(t, total, pageTwoTotal)
	require.Equal(t, stats, pageTwoStats)
	require.Len(t, second, 2)
	require.ElementsMatch(t, []string{"unknown", "running"}, []string{second[0].State, second[1].State})
	require.ElementsMatch(t, []int{2, 1}, []int{second[0].HistoryCount, second[1].HistoryCount})
	seenGroups := map[int64]bool{first[0].GroupID: true, first[1].GroupID: true, second[0].GroupID: true, second[1].GroupID: true}
	require.Len(t, seenGroups, 4)
	require.False(t, seenGroups[groupC.ID])

	filter.Page = 3
	beyond, finalTotal, finalStats, err := repo.Results(ctx, filter, authorized, true)
	require.NoError(t, err)
	require.Empty(t, beyond)
	require.Equal(t, total, finalTotal)
	require.Equal(t, stats, finalStats)

	filter.Page, filter.State = 1, "failed"
	failed, failedTotal, failedStats, err := repo.Results(ctx, filter, authorized, true)
	require.NoError(t, err)
	require.Equal(t, 2, failedTotal)
	require.Equal(t, service.PelicanStats{Failed: 2}, failedStats)
	require.Len(t, failed, 2)
	require.Equal(t, []int{1, 1}, []int{failed[0].HistoryCount, failed[1].HistoryCount})

	require.NoError(t, repo.SaveConfig(ctx, service.PelicanConfig{Enabled: false, DailyBudgetUSD: 1}))
	hidden, hiddenTotal, hiddenStats, err := repo.Results(ctx, service.PelicanFilter{Page: 1, PageSize: 12, LatestPerGroup: true}, authorized, true)
	require.NoError(t, err)
	require.Empty(t, hidden)
	require.Zero(t, hiddenTotal)
	require.Equal(t, service.PelicanStats{}, hiddenStats)
}

func TestPelicanPlanFlagsPreserveScheduleAndRunningMetadata(t *testing.T) {
	ctx := context.Background()
	repo := &pelicanRepository{db: integrationDB}
	group := mustCreateGroup(t, integrationEntClient, &service.Group{
		Name: "pelican-plan-flags-" + uuid.NewString(), Platform: service.PlatformOpenAI, Status: service.StatusActive,
	})
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, `DELETE FROM pelican_plans WHERE group_id=$1`, group.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM groups WHERE id=$1`, group.ID)
		require.NoError(t, err)
	})
	now := time.Now().UTC().Truncate(time.Microsecond)
	next := now.Add(37 * time.Minute)
	last := now.Add(-time.Minute)
	lease := now.Add(4 * time.Minute)
	plan := &service.PelicanPlan{
		PelicanPlanInput: service.PelicanPlanInput{GroupID: group.ID, Model: "custom-model", Effort: "high", Protocol: "responses", Enabled: true, Published: false, IntervalMinutes: 60, MaxOutputTokens: 8192, BudgetUSD: .5},
		GroupName:        group.Name, NextRunAt: next,
	}
	require.NoError(t, repo.SavePlan(ctx, plan))
	_, err := integrationDB.ExecContext(ctx, `UPDATE pelican_plans SET last_run_at=$2,running_until=$3 WHERE id=$1`, plan.ID, last, lease)
	require.NoError(t, err)

	// A publishing-only update must not postpone the scheduled test or lose its lease.
	plan.Published = true
	plan.NextRunAt = now.Add(time.Hour)
	require.NoError(t, repo.SavePlan(ctx, plan))
	require.Equal(t, next, plan.NextRunAt)
	require.Equal(t, &last, plan.LastRunAt)
	require.Equal(t, &lease, plan.RunningUntil)

	// Pausing preserves the schedule; re-enabling starts a fresh interval.
	plan.Enabled = false
	plan.NextRunAt = now.Add(2 * time.Hour)
	require.NoError(t, repo.SavePlan(ctx, plan))
	require.Equal(t, next, plan.NextRunAt)
	plan.Enabled = true
	plan.NextRunAt = now.Add(3 * time.Hour)
	require.NoError(t, repo.SavePlan(ctx, plan))
	require.Equal(t, now.Add(3*time.Hour), plan.NextRunAt)

	// An explicit interval change adopts the newly calculated schedule.
	plan.IntervalMinutes = 120
	plan.NextRunAt = now.Add(4 * time.Hour)
	require.NoError(t, repo.SavePlan(ctx, plan))
	require.Equal(t, now.Add(4*time.Hour), plan.NextRunAt)
	require.Equal(t, &last, plan.LastRunAt)
	require.Equal(t, &lease, plan.RunningUntil)

	require.NoError(t, repo.DeletePlan(ctx, plan.ID))
	require.ErrorIs(t, repo.SavePlan(ctx, plan), service.ErrPelicanNotFound)
}
