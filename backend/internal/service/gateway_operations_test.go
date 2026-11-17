package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type gatewayOpsRepoStub struct {
	GatewayOperationsRepository
	run              *GatewayExperiment
	wallet           *GatewayWallet
	finished         int
	createdTransport string
	configReads      atomic.Int64
}

func (r *gatewayOpsRepoStub) Experiment(_ context.Context, id string, userID int64) (*GatewayExperiment, error) {
	if r.run != nil && r.run.ID == id && r.run.RequestedBy == userID {
		return r.run, nil
	}
	return nil, nil
}
func (r *gatewayOpsRepoStub) CreateExperiment(_ context.Context, e *GatewayExperiment) error {
	copy := *e
	r.run = &copy
	r.createdTransport = e.Transport
	return nil
}
func (r *gatewayOpsRepoStub) FinishExperiment(_ context.Context, e *GatewayExperiment) error {
	copy := *e
	r.run = &copy
	r.finished++
	return nil
}
func (r *gatewayOpsRepoStub) Wallet(_ context.Context, _ int64) (*GatewayWallet, error) {
	copy := *r.wallet
	return &copy, nil
}
func (r *gatewayOpsRepoStub) SaveWalletObservation(_ context.Context, w *GatewayWallet) error {
	copy := *w
	r.wallet = &copy
	return nil
}
func (r *gatewayOpsRepoStub) Config(context.Context) (GatewayOperationsConfig, error) {
	r.configReads.Add(1)
	return GatewayOperationsConfig{WorkerEnabled: false}, nil
}

type gatewayOpsAccountStub struct {
	AccountRepository
	account *Account
}

func (r gatewayOpsAccountStub) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

type gatewayOpsTransport struct {
	HTTPUpstream
	calls    int
	request  *http.Request
	response string
	err      error
}

func (t *gatewayOpsTransport) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	t.calls++
	t.request = req
	if t.err != nil {
		return nil, t.err
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(t.response)), Header: http.Header{}}, nil
}

type gatewayOpsCipher struct{}

func (gatewayOpsCipher) Encrypt(s string) (string, error) { return s, nil }
func (gatewayOpsCipher) Decrypt(s string) (string, error) { return s, nil }
func TestGatewayOperationsSingleSubmitAndUnknown(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			repo := &gatewayOpsRepoStub{}
			transport := &gatewayOpsTransport{err: errors.New("unexpected EOF")}
			accounts := gatewayOpsAccountStub{account: &Account{ID: 7, Platform: "openai", Type: "apikey", Credentials: map[string]any{"api_key": "only-this-account", "base_url": "https://upstream.example"}}}
			tests := &AccountTestService{httpUpstream: transport, cfg: &config.Config{}}
			svc := NewGatewayOperationsService(repo, gatewayOpsCipher{}, accounts, tests, NewBillingService(&config.Config{}, &PricingService{pricingData: map[string]*LiteLLMModelPricing{"gpt-4o": {InputCostPerToken: 0.000001, OutputCostPerToken: 0.000002, Mode: "chat", LiteLLMProvider: "openai"}}}), nil)
			result, err := svc.RunExperiment(context.Background(), 8, GatewayExperimentInput{SubmitID: uuid.NewString(), Kind: "pelican", AccountID: 7, Model: "gpt-4o", Protocol: protocol, MaxOutputTokens: 64, BudgetUSD: 10})
			require.NoError(t, err)
			require.Equal(t, 1, transport.calls)
			require.Equal(t, "unknown", result.State)
			require.Equal(t, 1, repo.finished)
			repeat, repeatErr := svc.RunExperiment(context.Background(), 8, result.Frozen)
			require.NoError(t, repeatErr)
			require.Equal(t, result.ID, repeat.ID)
			require.Equal(t, 1, transport.calls)
			require.Equal(t, GatewayPelicanVersion, result.Frozen.PromptVersion)
			expected := "/v1/responses"
			if protocol == "chat" {
				expected = "/v1/chat/completions"
			}
			require.Equal(t, expected, transport.request.URL.Path)
			require.Equal(t, "Bearer only-this-account", transport.request.Header.Get("Authorization"))
			require.True(t, HTTPUpstreamRedirectsDisabled(transport.request.Context()))
		})
	}
}
func TestGatewayOperationsWalletCurrency(t *testing.T) {
	var profile, status map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(`{"quota":700000}`), &profile))
	require.NoError(t, json.Unmarshal([]byte(`{"quota_per_unit":100000,"quota_display_type":"CNY","usd_exchange_rate":7}`), &status))
	balance, currency, err := gatewayWalletBalance("newapi", profile, status)
	require.NoError(t, err)
	require.Equal(t, 49.0, balance)
	require.Equal(t, "CNY", currency)
	delete(status, "usd_exchange_rate")
	_, currency, err = gatewayWalletBalance("newapi", profile, status)
	require.NoError(t, err)
	require.Equal(t, "quota_units", currency)
	delete(status, "quota_per_unit")
	_, _, err = gatewayWalletBalance("newapi", profile, status)
	require.Error(t, err)
}
func TestGatewayOperationsWalletFailureRetainsBalance(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	balance := 12.0
	repo := &gatewayOpsRepoStub{wallet: &GatewayWallet{ID: 1, Provider: "sub2api", BaseURL: "https://127.0.0.1", CredentialCipher: "secret", Balance: &balance, RefreshedAt: &old}}
	svc := NewGatewayOperationsService(repo, gatewayOpsCipher{}, nil, nil, nil, nil)
	wallet, err := svc.RefreshWallet(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 12.0, *wallet.Balance)
	require.True(t, wallet.Stale)
	require.Contains(t, wallet.LastError, "public HTTPS")
	require.NotContains(t, wallet.LastError, "secret")
}
func TestGatewayOperationsUsageDoesNotDoubleChargeCachedInput(t *testing.T) {
	tokens, known := gatewayExperimentTokens("chat", gjson.Parse(`{"prompt_tokens":100,"completion_tokens":10,"prompt_tokens_details":{"cached_tokens":80}}`))
	require.True(t, known)
	require.Equal(t, 20, tokens.InputTokens)
	require.Equal(t, 80, tokens.CacheReadTokens)
}
func TestGatewayOperationsWorkerLifecycleDisabled(t *testing.T) {
	repo := &gatewayOpsRepoStub{}
	svc := NewGatewayOperationsService(repo, gatewayOpsCipher{}, nil, nil, nil, nil)
	svc.Start(context.Background())
	svc.Start(context.Background())
	svc.Stop()
	svc.refreshDueWallets(context.Background())
	require.EqualValues(t, 1, repo.configReads.Load())
}

func TestGatewayOperationsWalletReadContract(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		require.Equal(t, http.MethodGet, r.Method)
		if r.URL.Path == "/api/status" {
			require.Empty(t, r.Header.Get("Authorization"))
			require.Empty(t, r.Header.Get("New-Api-User"))
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"USD"}}`))
			return
		}
		require.Equal(t, "Bearer wallet-only-token", r.Header.Get("Authorization"))
		require.Equal(t, "17", r.Header.Get("New-Api-User"))
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":17,"quota":250000}}`))
	}))
	defer server.Close()
	wallet := &GatewayWallet{Provider: "newapi", BaseURL: server.URL, RemoteUserID: "17"}
	profile, err := gatewayWalletRead(context.Background(), server.Client(), wallet, "wallet-only-token", "/api/user/self")
	require.NoError(t, err)
	status, err := gatewayWalletRead(context.Background(), server.Client(), wallet, "wallet-only-token", "/api/status")
	require.NoError(t, err)
	balance, currency, err := gatewayWalletBalance("newapi", profile, status)
	require.NoError(t, err)
	require.Equal(t, 0.5, balance)
	require.Equal(t, "USD", currency)
	require.Equal(t, []string{"/api/user/self", "/api/status"}, paths)
}
func TestGatewayOperationsWalletRejectsNonfiniteStrings(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf"} {
		_, err := gatewayWalletNumber(map[string]json.RawMessage{"balance": json.RawMessage(`"` + value + `"`)}, "balance")
		require.Error(t, err)
	}
}

func TestGatewayOperationsWalletRejectsNullBalance(t *testing.T) {
	_, _, err := gatewayWalletBalance("sub2api", map[string]json.RawMessage{"balance": json.RawMessage(`null`)}, nil)
	require.ErrorContains(t, err, "invalid balance")
}

func TestGatewayOperationsIncompleteUsageRetainsUnknownCost(t *testing.T) {
	for _, protocol := range []string{"chat", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			for _, usage := range []string{`null`, `{}`, `{"prompt_tokens":10}`, `{"input_tokens":10}`, `{"prompt_tokens":10,"completion_tokens":null}`, `{"input_tokens":10,"output_tokens":null}`} {
				_, known := gatewayExperimentTokens(protocol, gjson.Parse(usage))
				require.False(t, known, usage)
			}
			response := `{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}],"usage":{}}`
			if protocol == "responses" {
				response = `{"status":"completed","output":[],"usage":null}`
			}
			repo := &gatewayOpsRepoStub{}
			transport := &gatewayOpsTransport{response: response}
			accounts := gatewayOpsAccountStub{account: &Account{ID: 7, Platform: "openai", Type: "apikey", Credentials: map[string]any{"api_key": "test-only", "base_url": "https://upstream.example"}}}
			svc := NewGatewayOperationsService(repo, gatewayOpsCipher{}, accounts, &AccountTestService{httpUpstream: transport, cfg: &config.Config{}}, NewBillingService(&config.Config{}, &PricingService{pricingData: map[string]*LiteLLMModelPricing{"gpt-4o": {InputCostPerToken: 0.000001, OutputCostPerToken: 0.000002, Mode: "chat", LiteLLMProvider: "openai"}}}), nil)
			run, err := svc.RunExperiment(context.Background(), 8, GatewayExperimentInput{SubmitID: uuid.NewString(), Kind: "pelican", AccountID: 7, Model: "gpt-4o", Protocol: protocol, MaxOutputTokens: 64, BudgetUSD: 10})
			require.NoError(t, err)
			require.Equal(t, "completed", run.State)
			require.Nil(t, run.CostUSD)
			require.Equal(t, 10.0, run.ReservedUSD)
			require.Nil(t, repo.run.CostUSD)
		})
	}
}

func TestGatewayOperationsWalletReadFailureDoesNotEchoRemoteBody(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"success":false,"message":"Bearer provider-secret"}`))
			}))
			defer server.Close()
			_, err := gatewayWalletRead(context.Background(), server.Client(), &GatewayWallet{BaseURL: server.URL}, "provider-secret", "/api/v1/user/profile")
			require.Error(t, err)
			require.NotContains(t, err.Error(), "provider-secret")
		})
	}
}

func TestGatewayOperationsCredentialUsesExistingAuditRedaction(t *testing.T) {
	redacted := RedactAuditBody([]byte(`{"credential_value":"provider-admin-token","name":"wallet"}`), "application/json")
	require.NotContains(t, redacted, "provider-admin-token")
	require.Contains(t, redacted, "wallet")
}
