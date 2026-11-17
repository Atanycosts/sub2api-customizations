package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type pelicanRepository struct{ db *sql.DB }

func NewPelicanRepository(db *sql.DB) service.PelicanRepository { return &pelicanRepository{db: db} }
func (r *pelicanRepository) Config(ctx context.Context) (c service.PelicanConfig, err error) {
	err = r.db.QueryRowContext(ctx, `SELECT enabled,daily_budget_usd FROM pelican_config WHERE id=1`).Scan(&c.Enabled, &c.DailyBudgetUSD)
	return
}
func (r *pelicanRepository) SaveConfig(ctx context.Context, c service.PelicanConfig) error {
	_, err := r.db.ExecContext(ctx, `UPDATE pelican_config SET enabled=$1,daily_budget_usd=$2 WHERE id=1`, c.Enabled, c.DailyBudgetUSD)
	return err
}
func scanPelicanPlan(row interface{ Scan(...any) error }) (p service.PelicanPlan, err error) {
	var b []byte
	err = row.Scan(&p.ID, &b, &p.GroupName, &p.LastRunAt, &p.NextRunAt, &p.RunningUntil)
	if err == nil {
		err = json.Unmarshal(b, &p.PelicanPlanInput)
	}
	return
}

const pelicanPlanSelect = `SELECT p.id,p.configuration,g.name,p.last_run_at,p.next_run_at,p.running_until FROM pelican_plans p JOIN groups g ON g.id=p.group_id`

func (r *pelicanRepository) Plans(ctx context.Context) ([]service.PelicanPlan, error) {
	rows, err := r.db.QueryContext(ctx, pelicanPlanSelect+` WHERE p.deleted_at IS NULL ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []service.PelicanPlan{}
	for rows.Next() {
		p, e := scanPelicanPlan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (r *pelicanRepository) SavePlan(ctx context.Context, p *service.PelicanPlan) error {
	b, err := json.Marshal(p.PelicanPlanInput)
	if err != nil {
		return err
	}
	if p.ID == 0 {
		return r.db.QueryRowContext(ctx, `INSERT INTO pelican_plans(group_id,configuration,enabled,published,interval_minutes,next_run_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, p.GroupID, b, p.Enabled, p.Published, p.IntervalMinutes, p.NextRunAt).Scan(&p.ID)
	}
	res, err := r.db.ExecContext(ctx, `UPDATE pelican_plans SET group_id=$2,configuration=$3,enabled=$4,published=$5,interval_minutes=$6,next_run_at=$7 WHERE id=$1 AND deleted_at IS NULL`, p.ID, p.GroupID, b, p.Enabled, p.Published, p.IntervalMinutes, p.NextRunAt)
	return pelicanChanged(res, err)
}
func pelicanChanged(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return service.ErrPelicanNotFound
	}
	return err
}
func (r *pelicanRepository) DeletePlan(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE pelican_plans SET enabled=FALSE,published=FALSE,deleted_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id)
	return pelicanChanged(res, err)
}
func (r *pelicanRepository) Submit(ctx context.Context, planID, userID int64, id string, scheduled bool) (*service.PelicanRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize reservations and durable UUID lookup across all workers/processes.
	var enabled bool
	var budget float64
	if err = tx.QueryRowContext(ctx, `SELECT enabled,daily_budget_usd FROM pelican_config WHERE id=1 FOR UPDATE`).Scan(&enabled, &budget); err != nil {
		return nil, err
	}
	var old []byte
	err = tx.QueryRowContext(ctx, `SELECT result FROM pelican_runs WHERE id=$1`, id).Scan(&old)
	if err == nil {
		var run service.PelicanRun
		if err = json.Unmarshal(old, &run); err != nil {
			return nil, err
		}
		if run.PlanID != planID || run.RequestedBy != userID {
			return nil, service.ErrPelicanUnavailable
		}
		return &run, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if !enabled || budget <= 0 {
		return nil, service.ErrPelicanUnavailable
	}
	var busy bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pelican_runs WHERE state='running')`).Scan(&busy); err != nil {
		return nil, err
	}
	if busy {
		return nil, service.ErrPelicanUnavailable
	}
	p, err := scanPelicanPlan(tx.QueryRowContext(ctx, pelicanPlanSelect+` WHERE p.id=$1 AND p.deleted_at IS NULL FOR UPDATE OF p`, planID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrPelicanNotFound
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if p.RunningUntil != nil && p.RunningUntil.After(now) {
		return nil, service.ErrPelicanUnavailable
	}
	if scheduled && (!p.Enabled || p.NextRunAt.After(now)) {
		return nil, service.ErrPelicanUnavailable
	}
	var spent float64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(GREATEST(reserved_usd,COALESCE(cost_usd,0))),0) FROM pelican_runs WHERE created_at >= date_trunc('day',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'`).Scan(&spent); err != nil {
		return nil, err
	}
	if spent+p.BudgetUSD > budget {
		return nil, service.ErrPelicanUnavailable
	}
	run := &service.PelicanRun{PelicanPublicRun: service.PelicanPublicRun{ID: id, Source: "group", GroupID: p.GroupID, GroupName: p.GroupName, Model: p.Model, Effort: p.Effort, Protocol: p.Protocol, PromptVersion: service.GatewayPelicanVersion, MaxOutputTokens: p.MaxOutputTokens, State: "running", CreatedAt: now}, Prompt: service.GatewayPelicanPrompt, PlanID: planID, RequestedBy: userID, Frozen: p.PelicanPlanInput, Published: p.Published, ReservedUSD: p.BudgetUSD}
	b, err := json.Marshal(run)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO pelican_runs(id,plan_id,requested_by,group_id,published,created_at,state,result,reserved_usd) VALUES($1,$2,$3,$4,$5,$6,'running',$7,$8)`, id, planID, userID, p.GroupID, p.Published, now, b, p.BudgetUSD)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE pelican_plans SET last_run_at=$2,next_run_at=CASE WHEN $3 THEN $2+interval_minutes*INTERVAL '1 minute' ELSE next_run_at END,running_until=$2+INTERVAL '5 minutes' WHERE id=$1`, planID, now, scheduled)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return run, nil
}
func (r *pelicanRepository) Result(ctx context.Context, id string) (*service.PelicanRun, error) {
	var b []byte
	err := r.db.QueryRowContext(ctx, `SELECT result FROM pelican_runs WHERE id::text=$1 AND NOT payload_pruned`, id).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrPelicanNotFound
	}
	if err != nil {
		return nil, err
	}
	var run service.PelicanRun
	err = json.Unmarshal(b, &run)
	return &run, err
}
func pelicanWhere(f service.PelicanFilter, ids []int64, public bool) (string, []any) {
	parts := []string{`NOT r.payload_pruned`}
	args := []any{}
	add := func(expr string, v any) { args = append(args, v); parts = append(parts, fmt.Sprintf(expr, len(args))) }
	if public {
		add(`r.group_id=ANY($%d)`, pq.Array(ids))
		parts = append(parts, `r.published AND p.published AND p.deleted_at IS NULL AND EXISTS(SELECT 1 FROM pelican_config WHERE id=1 AND enabled)`)
	}
	if f.GroupID > 0 {
		add(`r.group_id=$%d`, f.GroupID)
	}
	if f.PlanID > 0 {
		add(`r.plan_id=$%d`, f.PlanID)
	}
	if f.Model != "" {
		add(`r.result->>'model'=$%d`, f.Model)
	}
	if f.Effort != "" {
		add(`r.result->>'effort'=$%d`, f.Effort)
	}
	if f.State != "" {
		add(`r.state=$%d`, f.State)
	}
	return strings.Join(parts, " AND "), args
}
func (r *pelicanRepository) Results(ctx context.Context, f service.PelicanFilter, ids []int64, public bool) ([]service.PelicanRun, int, service.PelicanStats, error) {
	where, args := pelicanWhere(f, ids, public)
	from := ` FROM pelican_runs r JOIN pelican_plans p ON p.id=r.plan_id WHERE ` + where
	var total int
	var stats service.PelicanStats
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(*) FILTER(WHERE r.state='completed'),COUNT(*) FILTER(WHERE r.state='failed'),COUNT(*) FILTER(WHERE r.state='unknown'),COUNT(*) FILTER(WHERE r.state='running')`+from, args...).Scan(&total, &stats.Completed, &stats.Failed, &stats.Unknown, &stats.Running)
	if err != nil {
		return nil, 0, stats, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := r.db.QueryContext(ctx, `SELECT r.result-'output'-'raw_response'-'usage'-'timing'`+from+fmt.Sprintf(` ORDER BY r.created_at DESC,r.id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, stats, err
	}
	defer func() { _ = rows.Close() }()
	out := []service.PelicanRun{}
	for rows.Next() {
		var b []byte
		var run service.PelicanRun
		if err = rows.Scan(&b); err != nil {
			return nil, 0, stats, err
		}
		if err = json.Unmarshal(b, &run); err != nil {
			return nil, 0, stats, err
		}
		out = append(out, run)
	}
	return out, total, stats, rows.Err()
}
func (r *pelicanRepository) Next(ctx context.Context) (*service.PelicanRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	// The config lock serializes claims across instances; an active execution
	// owns the sole worker slot until finish or lease expiry.
	var enabled bool
	if err = tx.QueryRowContext(ctx, `SELECT enabled FROM pelican_config WHERE id=1 FOR UPDATE`).Scan(&enabled); err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pelican_runs r JOIN pelican_plans p ON p.id=r.plan_id WHERE r.state='running' AND r.started_at IS NOT NULL AND p.running_until>NOW())`).Scan(&active); err != nil {
		return nil, err
	}
	if active {
		return nil, nil
	}
	var b []byte
	err = tx.QueryRowContext(ctx, `UPDATE pelican_runs SET started_at=NOW() WHERE id=(SELECT r.id FROM pelican_runs r JOIN pelican_plans p ON p.id=r.plan_id WHERE r.state='running' AND r.started_at IS NULL AND p.running_until>NOW() ORDER BY r.created_at LIMIT 1 FOR UPDATE OF r SKIP LOCKED) AND state='running' AND started_at IS NULL RETURNING result`).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var run service.PelicanRun
	if err = json.Unmarshal(b, &run); err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE pelican_plans SET running_until=NOW()+INTERVAL '5 minutes' WHERE id=$1`, run.PlanID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &run, nil
}
func (r *pelicanRepository) Finish(ctx context.Context, run *service.PelicanRun) error {
	b, err := json.Marshal(run)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `UPDATE pelican_runs SET state=$2,finished_at=$3,result=$4,cost_usd=$5 WHERE id=$1 AND state='running'`, run.ID, run.State, run.FinishedAt, b, run.CostUSD)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE pelican_plans SET running_until=NULL WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM pelican_runs WHERE plan_id=$1 AND state='running')`, run.PlanID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (r *pelicanRepository) Due(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM pelican_plans WHERE deleted_at IS NULL AND enabled AND next_run_at<=NOW() AND (running_until IS NULL OR running_until<=NOW()) AND EXISTS(SELECT 1 FROM pelican_config WHERE id=1 AND enabled AND daily_budget_usd>0) ORDER BY next_run_at LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (r *pelicanRepository) Recover(ctx context.Context, startup bool) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `UPDATE pelican_runs r SET state='failed',finished_at=NOW(),result=result||jsonb_build_object('state','failed','finished_at',NOW(),'error_kind','unavailable','error','scheduled plan paused before execution') FROM pelican_plans p WHERE p.id=r.plan_id AND r.state='running' AND r.started_at IS NULL AND r.requested_by=0 AND (NOT p.enabled OR p.deleted_at IS NOT NULL)`)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE pelican_runs r SET state='unknown',finished_at=NOW(),result=result||jsonb_build_object('state','unknown','finished_at',NOW(),'error_kind','interrupted','error','worker lease expired; no replay') FROM pelican_plans p WHERE p.id=r.plan_id AND r.state='running' AND p.running_until<NOW()`)
	if err != nil {
		return err
	}
	if startup {
		_, err = tx.ExecContext(ctx, `UPDATE pelican_plans SET next_run_at=NOW()+interval_minutes*INTERVAL '1 minute' WHERE next_run_at<NOW() AND deleted_at IS NULL`)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (r *pelicanRepository) Prune(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `WITH expired AS (SELECT id FROM (SELECT id,created_at,state,row_number() OVER(PARTITION BY plan_id ORDER BY created_at DESC) AS n FROM pelican_runs WHERE NOT payload_pruned) t WHERE state!='running' AND (created_at<NOW()-INTERVAL '7 days' OR n>100)) UPDATE pelican_runs r SET payload_pruned=TRUE,result=(result-'output'-'raw_response'-'usage') FROM expired e WHERE r.id=e.id`)
	return err
}
