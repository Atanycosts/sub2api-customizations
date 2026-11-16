package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type requestTimingRepository struct{ db *sql.DB }

func NewRequestTimingRepository(db *sql.DB) service.RequestTimingRepository {
	return &requestTimingRepository{db}
}
func (r *requestTimingRepository) Save(ctx context.Context, t requesttiming.Trace) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO gateway_request_timings(trace_id,request_id,started_at,outcome,trace) VALUES($1,$2,$3,$4,$5) ON CONFLICT(trace_id) DO NOTHING`, t.ID, t.RequestID, t.StartedAt, t.Outcome, string(data))
	return err
}
func (r *requestTimingRepository) LinkUsage(ctx context.Context, l service.RequestTimingLink) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO gateway_request_timing_usage(trace_id,usage_id,usage_request_id,api_key_id) VALUES($1,NULLIF($2,0),$3,$4) ON CONFLICT(trace_id) DO UPDATE SET usage_id=COALESCE(EXCLUDED.usage_id,gateway_request_timing_usage.usage_id),usage_request_id=EXCLUDED.usage_request_id,api_key_id=EXCLUDED.api_key_id`, l.TraceID, l.UsageID, l.RequestID, l.APIKeyID)
	return err
}

const timingSelect = `SELECT t.trace, COALESCE(l.usage_id,u.id), COALESCE(l.usage_request_id,'') FROM gateway_request_timings t LEFT JOIN gateway_request_timing_usage l ON l.trace_id=t.trace_id LEFT JOIN LATERAL (SELECT id FROM usage_logs WHERE request_id=l.usage_request_id AND api_key_id=l.api_key_id ORDER BY id DESC LIMIT 1) u ON l.usage_id IS NULL `

func scanRequestTiming(row interface{ Scan(...any) error }) (*requesttiming.Trace, error) {
	var data []byte
	var id sql.NullInt64
	var requestID string
	if err := row.Scan(&data, &id, &requestID); err != nil {
		return nil, err
	}
	var t requesttiming.Trace
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	if id.Valid {
		t.UsageID = &id.Int64
	}
	t.UsageRequestID = requestID
	return &t, nil
}
func (r *requestTimingRepository) Get(ctx context.Context, id string) (*requesttiming.Trace, error) {
	return scanRequestTiming(r.db.QueryRowContext(ctx, timingSelect+` WHERE t.trace_id=$1`, id))
}
func (r *requestTimingRepository) List(ctx context.Context, f service.RequestTimingFilter) ([]requesttiming.Trace, error) {
	var before any
	if !f.Before.IsZero() {
		before = f.Before
	}
	rows, err := r.db.QueryContext(ctx, timingSelect+` WHERE ($1='' OR t.request_id=$1 OR l.usage_request_id=$1) AND ($2='' OR t.outcome=$2) AND ($3::bigint=0 OR EXISTS(SELECT 1 FROM jsonb_array_elements(t.trace->'attempts') a WHERE (a->>'account_id')::bigint=$3)) AND ($4::timestamptz IS NULL OR t.started_at<$4) ORDER BY t.started_at DESC,t.trace_id DESC LIMIT $5`, f.RequestID, f.Outcome, f.AccountID, before, f.Limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []requesttiming.Trace{}
	for rows.Next() {
		t, err := scanRequestTiming(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *t)
	}
	return items, rows.Err()
}
func (r *requestTimingRepository) Prune(ctx context.Context, before time.Time) error {
	_, err := r.db.ExecContext(ctx, `WITH deleted_traces AS (DELETE FROM gateway_request_timings WHERE trace_id IN (SELECT trace_id FROM gateway_request_timings WHERE started_at<$1 LIMIT 10000)) DELETE FROM gateway_request_timing_usage WHERE trace_id IN (SELECT trace_id FROM gateway_request_timing_usage WHERE created_at<$1 LIMIT 10000)`, before)
	return err
}
