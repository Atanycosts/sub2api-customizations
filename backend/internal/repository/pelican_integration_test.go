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

func TestPelicanScheduledAdmissionSkipsUnaffordablePlansAndClaimsOnce(t *testing.T) {
	repo, group := newPelicanScheduleFixture(t)
	ctx := context.Background()
	for i := range 11 {
		newDuePelicanPlan(t, repo, group, 1.50, time.Duration(i-20)*time.Minute)
	}
	plan := newDuePelicanPlan(t, repo, group, 0.25, -time.Minute)
	ids, err := repo.Due(ctx)
	require.NoError(t, err)
	require.Equal(t, []int64{plan.ID}, ids)
	run, err := repo.Submit(ctx, plan.ID, 0, uuid.NewString(), true)
	require.NoError(t, err)
	require.Zero(t, run.RequestedBy)
	var lastRun *time.Time
	var nextRun time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT last_run_at,next_run_at FROM pelican_plans WHERE id=$1`, plan.ID).Scan(&lastRun, &nextRun))
	require.Nil(t, lastRun)
	require.WithinDuration(t, run.CreatedAt.Add(30*time.Minute), nextRun, time.Millisecond)
	ids, err = repo.Due(ctx)
	require.NoError(t, err)
	require.Empty(t, ids)

	// Separate worker instances use independent connections and the same global slot.
	workers := []*pelicanRepository{repo, {db: integrationDB}, {db: integrationDB}}
	results := make([]*service.PelicanRun, len(workers))
	errs := make([]error, len(workers))
	var wg sync.WaitGroup
	for i, worker := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = worker.Next(ctx)
		}()
	}
	wg.Wait()
	claims := 0
	for i, err := range errs {
		require.NoError(t, err)
		if results[i] != nil {
			claims++
			require.Equal(t, run.ID, results[i].ID)
		}
	}
	require.Equal(t, 1, claims)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT last_run_at FROM pelican_plans WHERE id=$1`, plan.ID).Scan(&lastRun))
	require.NotNil(t, lastRun)
}

func TestPelicanQueuedRunRechecksControlsAtClaim(t *testing.T) {
	for _, tt := range []struct {
		name     string
		manual   bool
		change   func(t *testing.T, repo *pelicanRepository, plan *service.PelicanPlan)
		wantSend bool
	}{
		{name: "daily budget zero", change: func(t *testing.T, repo *pelicanRepository, _ *service.PelicanPlan) {
			require.NoError(t, repo.SaveConfig(t.Context(), service.PelicanConfig{Enabled: true}))
		}},
		{name: "daily budget lowered", change: func(t *testing.T, repo *pelicanRepository, _ *service.PelicanPlan) {
			require.NoError(t, repo.SaveConfig(t.Context(), service.PelicanConfig{Enabled: true, DailyBudgetUSD: 0.49}))
		}},
		{name: "disabled globally", change: func(t *testing.T, repo *pelicanRepository, _ *service.PelicanPlan) {
			require.NoError(t, repo.SaveConfig(t.Context(), service.PelicanConfig{DailyBudgetUSD: 1}))
		}},
		{name: "schedule paused", change: func(t *testing.T, repo *pelicanRepository, plan *service.PelicanPlan) {
			plan.Enabled = false
			require.NoError(t, repo.SavePlan(t.Context(), plan))
		}},
		{name: "manual run on a paused schedule", manual: true, wantSend: true, change: func(t *testing.T, repo *pelicanRepository, plan *service.PelicanPlan) {
			plan.Enabled = false
			require.NoError(t, repo.SavePlan(t.Context(), plan))
		}},
		{name: "manual plan deleted", manual: true, change: func(t *testing.T, repo *pelicanRepository, plan *service.PelicanPlan) {
			require.NoError(t, repo.DeletePlan(t.Context(), plan.ID))
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			repo, group := newPelicanScheduleFixture(t)
			plan := newDuePelicanPlan(t, repo, group, 0.50, -time.Minute)
			var userID int64
			if tt.manual {
				userID = 7
			}
			run, err := repo.Submit(ctx, plan.ID, userID, uuid.NewString(), !tt.manual)
			require.NoError(t, err)
			tt.change(t, repo, plan)
			claimed, err := repo.Next(ctx)
			require.NoError(t, err)
			if tt.wantSend {
				require.Equal(t, run.ID, claimed.ID)
				return
			}
			require.Nil(t, claimed)
			canceled, err := repo.Result(ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, "failed", canceled.State)
			require.Equal(t, "unavailable", canceled.ErrorKind)
			require.NotNil(t, canceled.FinishedAt)
			var started, lease *time.Time
			require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT r.started_at,p.running_until FROM pelican_runs r JOIN pelican_plans p ON p.id=r.plan_id WHERE r.id=$1`, run.ID).Scan(&started, &lease))
			require.Nil(t, started)
			require.Nil(t, lease)
			require.NoError(t, repo.SaveConfig(ctx, service.PelicanConfig{Enabled: true, DailyBudgetUSD: 0.5}))
			duplicate, err := repo.Submit(ctx, plan.ID, userID, run.ID, !tt.manual)
			require.NoError(t, err)
			require.Equal(t, "failed", duplicate.State)
			next := newDuePelicanPlan(t, repo, group, .5, -time.Minute)
			_, err = repo.Submit(ctx, next.ID, 0, uuid.NewString(), true)
			require.NoError(t, err, "an unsent run must release its budget")
		})
	}
}

func TestPelicanRestartRecoversLeasesWithoutReplayOrCatchup(t *testing.T) {
	repo, group := newPelicanScheduleFixture(t)
	ctx := context.Background()
	missed := newDuePelicanPlan(t, repo, group, 0.5, -time.Hour)
	paused := newDuePelicanPlan(t, repo, group, 0.5, -time.Hour)
	paused.Enabled = false
	require.NoError(t, repo.SavePlan(ctx, paused))
	expire := func(planID int64) {
		t.Helper()
		_, err := integrationDB.ExecContext(ctx, `UPDATE pelican_plans SET running_until=NOW()-INTERVAL '1 second' WHERE id=$1`, planID)
		require.NoError(t, err)
	}
	queued, err := repo.Submit(ctx, missed.ID, 7, uuid.NewString(), false)
	require.NoError(t, err)
	expire(missed.ID)
	require.NoError(t, repo.Recover(ctx, true))
	unsent, err := repo.Result(ctx, queued.ID)
	require.NoError(t, err)
	require.Equal(t, "failed", unsent.State)
	require.Contains(t, unsent.Error, "before execution")
	var pausedNext, missedNext time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT next_run_at FROM pelican_plans WHERE id=$1`, paused.ID).Scan(&pausedNext))
	require.WithinDuration(t, paused.NextRunAt, pausedNext, time.Millisecond)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT next_run_at FROM pelican_plans WHERE id=$1`, missed.ID).Scan(&missedNext))
	require.WithinDuration(t, time.Now().Add(30*time.Minute), missedNext, 5*time.Second)
	ids, err := repo.Due(ctx)
	require.NoError(t, err)
	require.Empty(t, ids)

	claimed, err := repo.Submit(ctx, missed.ID, 7, uuid.NewString(), false)
	require.NoError(t, err)
	got, err := repo.Next(ctx)
	require.NoError(t, err)
	require.Equal(t, claimed.ID, got.ID)
	require.NoError(t, repo.Recover(ctx, true))
	active, err := repo.Result(ctx, claimed.ID)
	require.NoError(t, err)
	require.Equal(t, "running", active.State, "another worker may still own a live lease")
	expire(missed.ID)
	require.NoError(t, repo.Recover(ctx, false))
	got, err = repo.Next(ctx)
	require.NoError(t, err)
	require.Nil(t, got)
	unknown, err := repo.Result(ctx, claimed.ID)
	require.NoError(t, err)
	require.Equal(t, "unknown", unknown.State)
	require.Equal(t, "interrupted", unknown.ErrorKind)
	var lease *time.Time
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT running_until FROM pelican_plans WHERE id=$1`, missed.ID).Scan(&lease))
	require.Nil(t, lease)
	claimed.State = "completed"
	require.NoError(t, repo.Finish(ctx, claimed))
	duplicate, err := repo.Submit(ctx, missed.ID, 7, claimed.ID, false)
	require.NoError(t, err)
	require.Equal(t, "unknown", duplicate.State)
	require.NoError(t, repo.SaveConfig(ctx, service.PelicanConfig{Enabled: true, DailyBudgetUSD: .5}))
	_, err = repo.Submit(ctx, missed.ID, 7, uuid.NewString(), false)
	require.ErrorIs(t, err, service.ErrPelicanUnavailable, "an unknown execution must keep its reservation")
}

func newPelicanScheduleFixture(t *testing.T) (*pelicanRepository, *service.Group) {
	t.Helper()
	ctx := context.Background()
	repo := &pelicanRepository{db: integrationDB}
	previous, err := repo.Config(ctx)
	require.NoError(t, err)
	group := mustCreateGroup(t, integrationEntClient, &service.Group{Name: "pelican-schedule-" + uuid.NewString(), Platform: service.PlatformOpenAI, Status: service.StatusActive})
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, `DELETE FROM pelican_runs WHERE group_id=$1`, group.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM pelican_plans WHERE group_id=$1`, group.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM groups WHERE id=$1`, group.ID)
		require.NoError(t, err)
		require.NoError(t, repo.SaveConfig(ctx, previous))
	})
	require.NoError(t, repo.SaveConfig(ctx, service.PelicanConfig{Enabled: true, DailyBudgetUSD: 1}))
	return repo, group
}

func newDuePelicanPlan(t *testing.T, repo *pelicanRepository, group *service.Group, reserve float64, due time.Duration) *service.PelicanPlan {
	t.Helper()
	plan := &service.PelicanPlan{
		PelicanPlanInput: service.PelicanPlanInput{GroupID: group.ID, Model: "gpt-6.2-sol", Effort: "high", Protocol: "responses", Published: true, Enabled: true, IntervalMinutes: 30, MaxOutputTokens: 1024, BudgetUSD: reserve},
		GroupName:        group.Name, NextRunAt: time.Now().UTC().Add(due),
	}
	require.NoError(t, repo.SavePlan(t.Context(), plan))
	return plan
}
