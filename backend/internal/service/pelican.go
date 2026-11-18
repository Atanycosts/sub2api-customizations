package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/google/uuid"
)

var (
	ErrPelicanInvalid     = infraerrors.BadRequest("PELICAN_INVALID", "invalid pelican configuration")
	ErrPelicanNotFound    = infraerrors.NotFound("PELICAN_NOT_FOUND", "pelican result or plan not found")
	ErrPelicanUnavailable = infraerrors.Conflict("PELICAN_UNAVAILABLE", "pelican is disabled, busy, or its budget is exhausted")
)

type PelicanConfig struct {
	Enabled        bool    `json:"enabled"`
	DailyBudgetUSD float64 `json:"daily_budget_usd"`
}
type PelicanPlanInput struct {
	GroupID         int64   `json:"group_id"`
	Model           string  `json:"model"`
	Effort          string  `json:"effort"`
	Protocol        string  `json:"protocol"`
	Enabled         bool    `json:"enabled"`
	Published       bool    `json:"published"`
	IntervalMinutes int     `json:"interval_minutes"`
	MaxOutputTokens int     `json:"max_output_tokens"`
	BudgetUSD       float64 `json:"budget_usd"`
}
type PelicanPlan struct {
	PelicanPlanInput
	ID           int64      `json:"id"`
	GroupName    string     `json:"group_name"`
	LastRunAt    *time.Time `json:"last_run_at"`
	NextRunAt    time.Time  `json:"next_run_at"`
	RunningUntil *time.Time `json:"running_until"`
}
type PelicanPublicRun struct {
	ID              string     `json:"id"`
	Source          string     `json:"source"`
	GroupID         int64      `json:"group_id"`
	GroupName       string     `json:"group_name"`
	Model           string     `json:"model"`
	Effort          string     `json:"effort"`
	Protocol        string     `json:"protocol"`
	PromptVersion   string     `json:"prompt_version"`
	MaxOutputTokens int        `json:"max_output_tokens"`
	State           string     `json:"state"`
	CreatedAt       time.Time  `json:"created_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	DurationMS      int64      `json:"duration_ms"`
	Terminal        string     `json:"terminal"`
	ErrorKind       string     `json:"error_kind"`
	HistoryCount    int        `json:"history_count,omitempty"`
	Output          string     `json:"output,omitempty"`
}
type PelicanRun struct {
	PelicanPublicRun
	Prompt        string               `json:"prompt"`
	PlanID        int64                `json:"plan_id"`
	RequestedBy   int64                `json:"requested_by"`
	Frozen        PelicanPlanInput     `json:"frozen"`
	Published     bool                 `json:"published"`
	AccountID     int64                `json:"account_id"`
	UpstreamModel string               `json:"upstream_model"`
	Endpoint      string               `json:"endpoint"`
	Transport     string               `json:"transport"`
	Error         string               `json:"error"`
	RawResponse   json.RawMessage      `json:"raw_response"`
	Usage         json.RawMessage      `json:"usage"`
	CostUSD       *float64             `json:"cost_usd"`
	ReservedUSD   float64              `json:"reserved_usd"`
	HTTPStatus    int                  `json:"http_status"`
	Timing        *requesttiming.Trace `json:"timing,omitempty"`
}
type PelicanFilter struct {
	GroupID, PlanID      int64
	Model, Effort, State string
	Page, PageSize       int
	LatestPerGroup       bool
}
type PelicanStats struct {
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Unknown   int `json:"unknown"`
	Running   int `json:"running"`
}
type PelicanGroup struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}
type PelicanPublicPage struct {
	Groups        []PelicanGroup     `json:"groups"`
	Items         []PelicanPublicRun `json:"items"`
	Total         int                `json:"total"`
	Page          int                `json:"page"`
	PageSize      int                `json:"page_size"`
	Pages         int                `json:"pages"`
	Stats         PelicanStats       `json:"stats"`
	PromptVersion string             `json:"prompt_version"`
	Prompt        string             `json:"prompt"`
}
type PelicanAdminPage struct {
	Items    []PelicanRun `json:"items"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Pages    int          `json:"pages"`
	Stats    PelicanStats `json:"stats"`
}
type PelicanRepository interface {
	Config(context.Context) (PelicanConfig, error)
	SaveConfig(context.Context, PelicanConfig) error
	Plans(context.Context) ([]PelicanPlan, error)
	SavePlan(context.Context, *PelicanPlan) error
	DeletePlan(context.Context, int64) error
	Submit(context.Context, int64, int64, string, bool) (*PelicanRun, error)
	Result(context.Context, string) (*PelicanRun, error)
	Results(context.Context, PelicanFilter, []int64, bool) ([]PelicanRun, int, PelicanStats, error)
	Next(context.Context) (*PelicanRun, error)
	Finish(context.Context, *PelicanRun) error
	Due(context.Context) ([]int64, error)
	Recover(context.Context, bool) error
	Prune(context.Context) error
}
type PelicanService struct {
	repo        PelicanRepository
	groups      GroupRepository
	keys        *APIKeyService
	gateway     *OpenAIGatewayService
	billing     *BillingService
	concurrency *ConcurrencyService
	mu          sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
}

func NewPelicanService(repo PelicanRepository, groups GroupRepository, keys *APIKeyService, gateway *OpenAIGatewayService, billing *BillingService, concurrency *ConcurrencyService) *PelicanService {
	return &PelicanService{repo: repo, groups: groups, keys: keys, gateway: gateway, billing: billing, concurrency: concurrency}
}
func (s *PelicanService) Config(ctx context.Context) (PelicanConfig, error) {
	return s.repo.Config(ctx)
}
func (s *PelicanService) SaveConfig(ctx context.Context, c PelicanConfig) error {
	if math.IsNaN(c.DailyBudgetUSD) || math.IsInf(c.DailyBudgetUSD, 0) || c.DailyBudgetUSD < 0 || c.DailyBudgetUSD > 10000 {
		return ErrPelicanInvalid
	}
	return s.repo.SaveConfig(ctx, c)
}
func (s *PelicanService) Plans(ctx context.Context) ([]PelicanPlan, error) { return s.repo.Plans(ctx) }
func (s *PelicanService) SavePlan(ctx context.Context, id int64, in PelicanPlanInput) (*PelicanPlan, error) {
	in.Model = strings.TrimSpace(in.Model)
	if in.GroupID <= 0 || in.Model == "" || len(in.Model) > 200 || in.IntervalMinutes < 30 || in.IntervalMinutes > 10080 || in.MaxOutputTokens < 64 || in.MaxOutputTokens > 16384 || math.IsNaN(in.BudgetUSD) || math.IsInf(in.BudgetUSD, 0) || in.BudgetUSD < 0.000001 || in.BudgetUSD > 100 {
		return nil, ErrPelicanInvalid
	}
	in.BudgetUSD = math.Round(in.BudgetUSD*1e6) / 1e6
	if in.Protocol != "chat" && in.Protocol != "responses" {
		return nil, ErrPelicanInvalid
	}
	switch in.Effort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return nil, ErrPelicanInvalid
	}
	g, err := s.groups.GetByID(ctx, in.GroupID)
	if err != nil {
		return nil, err
	}
	if g.Platform != PlatformOpenAI || g.Status != "active" || !g.ModelAllowlist.Allows(in.Model) {
		return nil, infraerrors.BadRequest("PELICAN_INVALID", "pelican currently supports active OpenAI groups and their allowed models")
	}
	body, _, err := ApplyOpenAIReasoningEffortPolicy(pelicanBody(in), g.MaxReasoningEffort, g.ReasoningEffortMappings, g.MaxReasoningEffortOverLimit)
	if err != nil {
		return nil, infraerrors.BadRequest("PELICAN_INVALID", err.Error())
	}
	if err = validateGPT61RawEffort(body, in.Model); err != nil {
		return nil, infraerrors.BadRequest("PELICAN_INVALID", err.Error())
	}
	p := &PelicanPlan{ID: id, PelicanPlanInput: in, GroupName: g.Name, NextRunAt: time.Now().UTC().Add(time.Duration(in.IntervalMinutes) * time.Minute)}
	err = s.repo.SavePlan(ctx, p)
	return p, err
}
func (s *PelicanService) DeletePlan(ctx context.Context, id int64) error {
	return s.repo.DeletePlan(ctx, id)
}
func (s *PelicanService) RunPlan(ctx context.Context, userID, planID int64, submitID string) (*PelicanRun, error) {
	if _, err := uuid.Parse(submitID); err != nil {
		return nil, infraerrors.BadRequest("PELICAN_INVALID", "valid submit_id UUID required")
	}
	return s.repo.Submit(ctx, planID, userID, submitID, false)
}
func (s *PelicanService) Results(ctx context.Context, f PelicanFilter) (*PelicanAdminPage, error) {
	f = pelicanPageFilter(f)
	items, total, stats, err := s.repo.Results(ctx, f, nil, false)
	return &PelicanAdminPage{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize, Pages: max(1, (total+f.PageSize-1)/f.PageSize), Stats: stats}, err
}
func (s *PelicanService) Result(ctx context.Context, id string) (*PelicanRun, error) {
	return s.repo.Result(ctx, id)
}
func pelicanPageFilter(f PelicanFilter) PelicanFilter {
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 {
		f.PageSize = 12
	}
	if f.PageSize > 48 {
		f.PageSize = 48
	}
	return f
}
func (s *PelicanService) publicGroups(ctx context.Context, userID int64) ([]PelicanGroup, []int64, error) {
	c, err := s.repo.Config(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := []PelicanGroup{}
	ids := []int64{}
	if !c.Enabled {
		return out, ids, nil
	}
	groups, err := s.keys.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	for _, g := range groups {
		if g.Platform == PlatformOpenAI {
			out = append(out, PelicanGroup{ID: g.ID, Name: g.Name, Platform: g.Platform})
			ids = append(ids, g.ID)
		}
	}
	return out, ids, nil
}
func (s *PelicanService) PublicResults(ctx context.Context, userID int64, f PelicanFilter) (*PelicanPublicPage, error) {
	f = pelicanPageFilter(f)
	groups, ids, err := s.publicGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	runs, total, stats, err := s.repo.Results(ctx, f, ids, true)
	if err != nil {
		return nil, err
	}
	items := make([]PelicanPublicRun, 0, len(runs))
	for _, r := range runs {
		p := pelicanPublic(r)
		p.Output = ""
		items = append(items, p)
	}
	return &PelicanPublicPage{Groups: groups, Items: items, Total: total, Page: f.Page, PageSize: f.PageSize, Pages: max(1, (total+f.PageSize-1)/f.PageSize), Stats: stats, Prompt: GatewayPelicanPrompt, PromptVersion: GatewayPelicanVersion}, nil
}
func (s *PelicanService) PublicResult(ctx context.Context, userID int64, id string) (*PelicanPublicRun, error) {
	_, ids, err := s.publicGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	r, err := s.repo.Result(ctx, id)
	if err != nil {
		return nil, err
	}
	allowed := false
	for _, gid := range ids {
		if gid == r.GroupID {
			allowed = true
		}
	}
	if !allowed || !r.Published {
		return nil, ErrPelicanNotFound
	}
	plans, err := s.repo.Plans(ctx)
	if err != nil {
		return nil, err
	}
	published := false
	for _, p := range plans {
		if p.ID == r.PlanID && p.Published {
			published = true
		}
	}
	if !published {
		return nil, ErrPelicanNotFound
	}
	p := pelicanPublic(*r)
	return &p, nil
}
func pelicanPublic(r PelicanRun) PelicanPublicRun {
	p := r.PelicanPublicRun
	p.Source = "group"
	switch p.ErrorKind {
	case "", "unavailable", "upstream_error", "incomplete", "timeout", "output_limit", "interrupted", "invalid_output":
	default:
		p.ErrorKind = "upstream_error"
	}
	switch p.Terminal {
	case "", "stop", "length", "completed", "failed", "incomplete", "response.completed", "response.failed", "response.incomplete", "error":
	default:
		p.Terminal = "other"
	}
	return p
}
func (s *PelicanService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go s.worker(ctx)
}
func (s *PelicanService) Stop() {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
		s.mu.Lock()
		s.cancel = nil
		s.mu.Unlock()
	}
}
func (s *PelicanService) worker(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.repo.Recover(ctx, true); err == nil {
			break
		} else if ctx.Err() == nil {
			logger.LegacyPrintf("service.pelican", "startup recovery failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
	var nextPrune time.Time
	for {
		if ctx.Err() != nil {
			return
		}
		if time.Now().After(nextPrune) {
			nextPrune = time.Now().Add(time.Hour)
			if err := s.repo.Prune(ctx); err != nil && ctx.Err() == nil {
				logger.LegacyPrintf("service.pelican", "history pruning failed: %v", err)
			}
		}
		if s.workOnce(ctx) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *PelicanService) workOnce(ctx context.Context) bool {
	if err := s.repo.Recover(ctx, false); err != nil {
		if ctx.Err() == nil {
			logger.LegacyPrintf("service.pelican", "lease recovery failed: %v", err)
		}
		return false
	}
	ids, err := s.repo.Due(ctx)
	if err != nil && ctx.Err() == nil {
		logger.LegacyPrintf("service.pelican", "due plans lookup failed: %v", err)
	}
	for _, id := range ids {
		_, err := s.repo.Submit(ctx, id, 0, uuid.NewString(), true)
		if err == nil {
			break
		}
		if errors.Is(err, ErrPelicanUnavailable) || errors.Is(err, ErrPelicanNotFound) {
			continue
		}
		if ctx.Err() == nil {
			logger.LegacyPrintf("service.pelican", "scheduled submit failed plan_id=%d: %v", id, err)
		}
		break
	}
	run, err := s.repo.Next(ctx)
	if err != nil && ctx.Err() == nil {
		logger.LegacyPrintf("service.pelican", "run claim failed: %v", err)
	}
	if err != nil || run == nil {
		return false
	}
	s.execute(ctx, run)
	return true
}
