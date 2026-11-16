package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GatewayOperations is an administrator observation tool. It never owns routing,
// customer billing, account status, or automatic account recovery.
type GatewayOperationsConfig struct {
	WorkerEnabled  bool    `json:"worker_enabled"`
	RefreshMinutes int     `json:"refresh_minutes"`
	DailyBudgetUSD float64 `json:"daily_budget_usd"`
}

type GatewayWallet struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	Provider         string     `json:"provider"`
	BaseURL          string     `json:"base_url"`
	RemoteUserID     string     `json:"remote_user_id"`
	AccountIDs       []int64    `json:"account_ids"`
	Currency         string     `json:"currency"`
	Threshold        *float64   `json:"threshold"`
	NotifyEmail      string     `json:"notify_email"`
	Balance          *float64   `json:"balance"`
	RefreshedAt      *time.Time `json:"refreshed_at"`
	LastError        string     `json:"last_error"`
	LastAttemptAt    *time.Time `json:"last_attempt_at"`
	Stale            bool       `json:"stale"`
	CredentialCipher string     `json:"-"`
	LastNotifiedAt   *time.Time `json:"-"`
}

type GatewayWalletInput struct {
	Name         string   `json:"name"`
	Provider     string   `json:"provider"`
	BaseURL      string   `json:"base_url"`
	RemoteUserID string   `json:"remote_user_id"`
	AccountIDs   []int64  `json:"account_ids"`
	Credential   string   `json:"credential_value"`
	Threshold    *float64 `json:"threshold"`
	Currency     string   `json:"currency"`
	NotifyEmail  string   `json:"notify_email"`
}

var ErrGatewayExperimentExists = errors.New("experiment submission already exists")

const GatewayPelicanPrompt = "Create a complete self-contained HTML document showing a pelican riding a bicycle in a continuous smooth animation. Use inline SVG/CSS/JavaScript only, with no external resources. Show a clearly recognizable pelican, two rotating bicycle wheels, pedals and legs moving together, and a coherent environment. Return only the HTML document."
const GatewayPelicanVersion = "pelican-bicycle-v1"

type GatewayExperimentInput struct {
	SubmitID        string  `json:"submit_id"`
	Kind            string  `json:"kind"`
	AccountID       int64   `json:"account_id"`
	Model           string  `json:"model"`
	Effort          string  `json:"effort"`
	Protocol        string  `json:"protocol"`
	Prompt          string  `json:"prompt"`
	PromptVersion   string  `json:"prompt_version"`
	MaxOutputTokens int     `json:"max_output_tokens"`
	BudgetUSD       float64 `json:"budget_usd"`
}

type GatewayExperiment struct {
	ID            string                 `json:"id"`
	CreatedAt     time.Time              `json:"created_at"`
	FinishedAt    *time.Time             `json:"finished_at"`
	RequestedBy   int64                  `json:"requested_by"`
	Frozen        GatewayExperimentInput `json:"frozen"`
	UpstreamModel string                 `json:"upstream_model"`
	Endpoint      string                 `json:"endpoint"`
	State         string                 `json:"state"`
	Output        string                 `json:"output"`
	Error         string                 `json:"error"`
	RawResponse   json.RawMessage        `json:"raw_response"`
	Terminal      string                 `json:"terminal"`
	Usage         json.RawMessage        `json:"usage"`
	CostUSD       *float64               `json:"cost_usd"`
	ReservedUSD   float64                `json:"reserved_usd"`
	DurationMS    int64                  `json:"duration_ms"`
	HTTPStatus    int                    `json:"http_status"`
}

type GatewayCacheUsage struct {
	ID                  int64   `json:"id"`
	RequestID           string  `json:"request_id"`
	AccountID           int64   `json:"account_id"`
	GroupID             *int64  `json:"group_id"`
	Model               string  `json:"model"`
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheReadTokens     int     `json:"cache_read_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	ActualCost          float64 `json:"actual_cost"`
	CacheFieldsKnown    bool    `json:"cache_fields_known"`
}

type GatewayOperationsRepository interface {
	Config(context.Context) (GatewayOperationsConfig, error)
	SaveConfig(context.Context, GatewayOperationsConfig) error
	Wallets(context.Context, int, int) ([]GatewayWallet, error)
	Wallet(context.Context, int64) (*GatewayWallet, error)
	SaveWallet(context.Context, *GatewayWallet) error
	DeleteWallet(context.Context, int64) error
	SaveWalletObservation(context.Context, *GatewayWallet) error
	ClaimWalletNotification(context.Context, int64, time.Time) (bool, error)
	CreateExperiment(context.Context, *GatewayExperiment) error
	Experiment(context.Context, string, int64) (*GatewayExperiment, error)
	ReleaseWalletNotification(context.Context, int64, time.Time) error
	FinishExperiment(context.Context, *GatewayExperiment) error
	Experiments(context.Context, int, int) ([]GatewayExperiment, error)
	CacheUsage(context.Context, int64, int64, []string) ([]GatewayCacheUsage, error)
}

type GatewayOperationsService struct {
	repo          GatewayOperationsRepository
	encryptor     SecretEncryptor
	accounts      AccountRepository
	tests         *AccountTestService
	billing       *BillingService
	notifications *NotificationEmailService
	walletClient  *http.Client
	workerMu      sync.Mutex
	workerCancel  context.CancelFunc
	workerDone    chan struct{}
}

func NewGatewayOperationsService(repo GatewayOperationsRepository, encryptor SecretEncryptor, accounts AccountRepository, tests *AccountTestService, billing *BillingService, notifications *NotificationEmailService) *GatewayOperationsService {
	return &GatewayOperationsService{repo: repo, encryptor: encryptor, accounts: accounts, tests: tests, billing: billing, notifications: notifications, walletClient: gatewayWalletClient()}
}
func (s *GatewayOperationsService) Config(ctx context.Context) (GatewayOperationsConfig, error) {
	return s.repo.Config(ctx)
}
func (s *GatewayOperationsService) SaveConfig(ctx context.Context, c GatewayOperationsConfig) error {
	if c.RefreshMinutes < 5 || c.RefreshMinutes > 1440 || math.IsNaN(c.DailyBudgetUSD) || math.IsInf(c.DailyBudgetUSD, 0) || c.DailyBudgetUSD < 0 || c.DailyBudgetUSD > 10000 {
		return errors.New("refresh interval must be 5–1440 minutes and daily budget 0–10000 USD")
	}
	return s.repo.SaveConfig(ctx, c)
}
func (s *GatewayOperationsService) Wallets(ctx context.Context, limit, offset int) ([]GatewayWallet, error) {
	wallets, err := s.repo.Wallets(ctx, limit, offset)
	for i := range wallets {
		wallets[i].Stale = wallets[i].RefreshedAt == nil || time.Since(*wallets[i].RefreshedAt) > 30*time.Minute || wallets[i].LastError != ""
	}
	return wallets, err
}
func (s *GatewayOperationsService) SaveWallet(ctx context.Context, id int64, in GatewayWalletInput) (*GatewayWallet, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	in.RemoteUserID = strings.TrimSpace(in.RemoteUserID)
	if n, err := strconv.ParseInt(in.RemoteUserID, 10, 64); err != nil || n <= 0 {
		return nil, errors.New("remote user ID must be a positive integer")
	} else {
		in.RemoteUserID = strconv.FormatInt(n, 10)
	}
	parsed, err := url.Parse(in.BaseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("wallet base URL must be HTTPS without credentials, query or fragment")
	}
	if in.Name == "" || len(in.Name) > 100 || len(in.BaseURL) > 2048 || in.RemoteUserID == "" || len(in.RemoteUserID) > 100 || (in.Provider != "sub2api" && in.Provider != "newapi") {
		return nil, errors.New("name, provider (sub2api/newapi), remote user ID and base URL are required")
	}
	if in.Threshold != nil && (math.IsNaN(*in.Threshold) || math.IsInf(*in.Threshold, 0) || *in.Threshold < 0) {
		return nil, errors.New("invalid low balance threshold")
	}
	if in.Currency != "USD" && in.Currency != "CNY" && in.Currency != "quota_units" {
		return nil, errors.New("currency must be USD, CNY or quota_units")
	}
	if in.NotifyEmail != "" {
		addr, e := mail.ParseAddress(in.NotifyEmail)
		if e != nil || addr.Address != in.NotifyEmail {
			return nil, errors.New("invalid notification email")
		}
	}
	if len(in.AccountIDs) > 100 {
		return nil, errors.New("at most 100 linked accounts")
	}
	for _, accountID := range in.AccountIDs {
		if _, err = s.accounts.GetByID(ctx, accountID); err != nil {
			return nil, fmt.Errorf("linked account %d: %w", accountID, err)
		}
	}
	wallet := &GatewayWallet{ID: id, Name: in.Name, Provider: in.Provider, BaseURL: in.BaseURL, RemoteUserID: in.RemoteUserID, AccountIDs: in.AccountIDs, Threshold: in.Threshold, Currency: in.Currency, NotifyEmail: in.NotifyEmail}
	if id > 0 {
		old, e := s.repo.Wallet(ctx, id)
		if e != nil {
			return nil, e
		}
		wallet.CredentialCipher = old.CredentialCipher
		// A credential belongs to its original remote identity; require explicit replacement when changing it.
		if (old.BaseURL != in.BaseURL || old.Provider != in.Provider || old.RemoteUserID != in.RemoteUserID) && in.Credential == "" {
			return nil, errors.New("changing remote identity requires a new credential")
		}
	}
	if in.Credential != "" {
		if len(in.Credential) > 8192 || strings.IndexFunc(in.Credential, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			return nil, errors.New("invalid credential")
		}
		wallet.CredentialCipher, err = s.encryptor.Encrypt(in.Credential)
		if err != nil {
			return nil, err
		}
	}
	if wallet.CredentialCipher == "" {
		return nil, errors.New("read-only provider access token required")
	}
	if err = s.repo.SaveWallet(ctx, wallet); err != nil {
		return nil, err
	}
	wallet.Stale = true
	return wallet, nil
}
func (s *GatewayOperationsService) DeleteWallet(ctx context.Context, id int64) error {
	return s.repo.DeleteWallet(ctx, id)
}
func (s *GatewayOperationsService) Experiments(ctx context.Context, limit, offset int) ([]GatewayExperiment, error) {
	return s.repo.Experiments(ctx, limit, offset)
}
func (s *GatewayOperationsService) CacheUsage(ctx context.Context, userID, keyID int64, ids []string) ([]GatewayCacheUsage, error) {
	if keyID <= 0 || len(ids) == 0 || len(ids) > 20 {
		return nil, errors.New("one own API key and 1–20 request IDs required")
	}
	for _, id := range ids {
		if len(id) < 8 || len(id) > 128 || strings.ContainsAny(id, "\r\n") {
			return nil, errors.New("invalid request ID")
		}
	}
	return s.repo.CacheUsage(ctx, userID, keyID, ids)
}
