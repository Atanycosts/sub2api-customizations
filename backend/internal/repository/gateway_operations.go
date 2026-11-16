package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type gatewayOperationsRepository struct{ db *sql.DB }

func NewGatewayOperationsRepository(db *sql.DB) service.GatewayOperationsRepository {
	return &gatewayOperationsRepository{db: db}
}
func (r *gatewayOperationsRepository) Config(ctx context.Context) (c service.GatewayOperationsConfig, err error) {
	err = r.db.QueryRowContext(ctx, `SELECT worker_enabled,refresh_minutes,daily_budget_usd FROM gateway_operations_config WHERE id=1`).Scan(&c.WorkerEnabled, &c.RefreshMinutes, &c.DailyBudgetUSD)
	return
}
func (r *gatewayOperationsRepository) SaveConfig(ctx context.Context, c service.GatewayOperationsConfig) error {
	_, err := r.db.ExecContext(ctx, `UPDATE gateway_operations_config SET worker_enabled=$1,refresh_minutes=$2,daily_budget_usd=$3 WHERE id=1`, c.WorkerEnabled, c.RefreshMinutes, c.DailyBudgetUSD)
	return err
}
func scanGatewayWallet(scanner interface{ Scan(...any) error }) (w service.GatewayWallet, err error) {
	var configuration, observation []byte
	err = scanner.Scan(&w.ID, &w.Provider, &w.BaseURL, &w.RemoteUserID, &w.CredentialCipher, &configuration, &observation, &w.LastNotifiedAt)
	if err != nil {
		return
	}
	var config service.GatewayWallet
	err = json.Unmarshal(configuration, &config)
	if err != nil {
		return
	}
	w.Name = config.Name
	w.AccountIDs = config.AccountIDs
	w.Threshold = config.Threshold
	w.Currency = config.Currency
	w.NotifyEmail = config.NotifyEmail
	var obs service.GatewayWallet
	err = json.Unmarshal(observation, &obs)
	w.Balance = obs.Balance
	w.RefreshedAt = obs.RefreshedAt
	w.LastAttemptAt = obs.LastAttemptAt
	w.LastError = obs.LastError
	return
}

const gatewayWalletColumns = `id,provider,base_url,remote_user_id,credential_cipher,configuration,observation,last_notified_at`

func (r *gatewayOperationsRepository) Wallets(ctx context.Context, limit, offset int) ([]service.GatewayWallet, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+gatewayWalletColumns+` FROM gateway_operations_wallets ORDER BY id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []service.GatewayWallet{}
	for rows.Next() {
		w, e := scanGatewayWallet(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
func (r *gatewayOperationsRepository) Wallet(ctx context.Context, id int64) (*service.GatewayWallet, error) {
	w, err := scanGatewayWallet(r.db.QueryRowContext(ctx, `SELECT `+gatewayWalletColumns+` FROM gateway_operations_wallets WHERE id=$1`, id))
	return &w, err
}
func (r *gatewayOperationsRepository) SaveWallet(ctx context.Context, w *service.GatewayWallet) error {
	b, err := json.Marshal(w)
	if err != nil {
		return err
	}
	if w.ID == 0 {
		err = r.db.QueryRowContext(ctx, `INSERT INTO gateway_operations_wallets(provider,base_url,remote_user_id,credential_cipher,configuration) VALUES($1,$2,$3,$4,$5) RETURNING id`, w.Provider, w.BaseURL, w.RemoteUserID, w.CredentialCipher, b).Scan(&w.ID)
	} else {
		_, err = r.db.ExecContext(ctx, `UPDATE gateway_operations_wallets SET provider=$2,base_url=$3,remote_user_id=$4,credential_cipher=$5,configuration=$6,observation='{}',last_notified_at=NULL WHERE id=$1`, w.ID, w.Provider, w.BaseURL, w.RemoteUserID, w.CredentialCipher, b)
	}
	if isUniqueViolation(err) {
		return errors.New("this remote wallet already exists; link accounts to the existing shared wallet")
	}
	return err
}
func (r *gatewayOperationsRepository) DeleteWallet(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM gateway_operations_wallets WHERE id=$1`, id)
	return err
}
func (r *gatewayOperationsRepository) SaveWalletObservation(ctx context.Context, w *service.GatewayWallet) error {
	b, err := json.Marshal(w)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE gateway_operations_wallets SET observation=$2 WHERE id=$1`, w.ID, b)
	return err
}
func (r *gatewayOperationsRepository) ClaimWalletNotification(ctx context.Context, id int64, now time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE gateway_operations_wallets SET last_notified_at=$2 WHERE id=$1 AND (last_notified_at IS NULL OR last_notified_at<$2-INTERVAL '24 hours')`, id, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
func (r *gatewayOperationsRepository) CreateExperiment(ctx context.Context, e *service.GatewayExperiment) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var budget, used float64
	if err = tx.QueryRowContext(ctx, `SELECT daily_budget_usd FROM gateway_operations_config WHERE id=1 FOR UPDATE`).Scan(&budget); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gateway_operations_experiments WHERE id=$1)`, e.ID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return service.ErrGatewayExperimentExists
	}
	// Unknown and in-flight execution retain their full reservation; no replay can spend it twice.
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN state='completed' AND result->>'cost_usd' IS NOT NULL THEN GREATEST((result->>'cost_usd')::numeric,0) ELSE reserved_usd END),0) FROM gateway_operations_experiments WHERE created_at>=date_trunc('day',NOW() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'`).Scan(&used); err != nil {
		return err
	}
	if budget <= 0 || used+e.ReservedUSD > budget {
		return errors.New("daily experiment budget exhausted or disabled")
	}
	frozen, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO gateway_operations_experiments(id,requested_by,frozen,reserved_usd,state) VALUES($1,$2,$3,$4,'running')`, e.ID, e.RequestedBy, frozen, e.ReservedUSD)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (r *gatewayOperationsRepository) FinishExperiment(ctx context.Context, e *service.GatewayExperiment) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE gateway_operations_experiments SET state=$2,result=$3 WHERE id=$1 AND state='running'`, e.ID, e.State, b)
	return err
}
func (r *gatewayOperationsRepository) Experiments(ctx context.Context, limit, offset int) ([]service.GatewayExperiment, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT frozen,result,state,created_at FROM gateway_operations_experiments ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []service.GatewayExperiment{}
	for rows.Next() {
		var frozen, result []byte
		var state string
		var created time.Time
		if err = rows.Scan(&frozen, &result, &state, &created); err != nil {
			return nil, err
		}
		var e service.GatewayExperiment
		if err = json.Unmarshal(frozen, &e); err != nil {
			return nil, err
		}
		if string(result) != "{}" {
			if err = json.Unmarshal(result, &e); err != nil {
				return nil, err
			}
		}
		e.State = state
		e.CreatedAt = created
		if state == "running" && time.Since(created) > 5*time.Minute {
			e.State = "unknown"
			e.Error = "execution did not persist a terminal result; reservation retained; never automatically replayed"
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (r *gatewayOperationsRepository) CacheUsage(ctx context.Context, userID, keyID int64, ids []string) ([]service.GatewayCacheUsage, error) {
	var own bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM api_keys WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL)`, keyID, userID).Scan(&own); err != nil {
		return nil, err
	}
	if !own {
		return nil, errors.New("API key must belong to the current user")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,request_id,account_id,group_id,model,input_tokens,output_tokens,cache_read_tokens,cache_creation_tokens,actual_cost FROM usage_logs WHERE user_id=$1 AND api_key_id=$2 AND request_id=ANY($3) ORDER BY id`, userID, keyID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []service.GatewayCacheUsage{}
	for rows.Next() {
		var u service.GatewayCacheUsage
		if err = rows.Scan(&u.ID, &u.RequestID, &u.AccountID, &u.GroupID, &u.Model, &u.InputTokens, &u.OutputTokens, &u.CacheReadTokens, &u.CacheCreationTokens, &u.ActualCost); err != nil {
			return nil, err
		}
		u.CacheFieldsKnown = u.CacheReadTokens > 0 || u.CacheCreationTokens > 0
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *gatewayOperationsRepository) Experiment(ctx context.Context, id string, userID int64) (*service.GatewayExperiment, error) {
	var frozen, result []byte
	var state string
	var created time.Time
	err := r.db.QueryRowContext(ctx, `SELECT frozen,result,state,created_at FROM gateway_operations_experiments WHERE id=$1 AND requested_by=$2`, id, userID).Scan(&frozen, &result, &state, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var e service.GatewayExperiment
	if err = json.Unmarshal(frozen, &e); err != nil {
		return nil, err
	}
	if string(result) != "{}" {
		if err = json.Unmarshal(result, &e); err != nil {
			return nil, err
		}
	}
	e.State = state
	e.CreatedAt = created
	if state == "running" && time.Since(created) > 5*time.Minute {
		e.State = "unknown"
		e.Error = "execution did not persist a terminal result; reservation retained; never automatically replayed"
	}
	return &e, nil
}
func (r *gatewayOperationsRepository) ReleaseWalletNotification(ctx context.Context, id int64, claimed time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE gateway_operations_wallets SET last_notified_at=NULL WHERE id=$1 AND last_notified_at=$2`, id, claimed)
	return err
}
