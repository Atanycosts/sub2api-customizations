package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// MemoryScope identifies the product-owned tenant whose memories may be read
// or written. Upstream provider, account, and model identifiers deliberately
// do not appear here: a user's memory follows the product user across keys,
// models, and upstream accounts.
type MemoryScope struct {
	UserID         int64
	ProjectID      string
	ConversationID string
}

var (
	ErrMemoryUserRequired     = errors.New("memory scope requires a product user")
	ErrMemoryScopeIdentifier  = errors.New("memory scope identifier is invalid")
	ErrMemoryQueryRequired    = errors.New("memory recall query is required")
	ErrMemoryTurnContentEmpty = errors.New("memory turn must contain user or assistant text")
	ErrMemoryItemIDRequired   = errors.New("memory item id is required")
)

const (
	defaultMemoryRecallItems  = 8
	defaultMemoryRecallBytes  = 16 * 1024
	defaultMemoryRecallTokens = 2048

	maxMemoryRecallItems  = 32
	maxMemoryRecallBytes  = 64 * 1024
	maxMemoryRecallTokens = 4096

	maxMemoryScopeIdentifierBytes = 256
	maxMemoryQueryBytes           = 32 * 1024
	maxMemoryTurnTextBytes        = 256 * 1024
)

// Validate checks the identity and optional product-owned scope identifiers.
// It intentionally accepts arbitrary UTF-8 identifiers so existing project and
// conversation IDs do not need to be reformatted by the memory layer.
func (s MemoryScope) Validate() error {
	if s.UserID <= 0 {
		return ErrMemoryUserRequired
	}
	if err := validateMemoryIdentifier(s.ProjectID); err != nil {
		return fmt.Errorf("project id: %w", err)
	}
	if err := validateMemoryIdentifier(s.ConversationID); err != nil {
		return fmt.Errorf("conversation id: %w", err)
	}
	return nil
}

func validateMemoryIdentifier(value string) error {
	if value == "" {
		return nil
	}
	if len(value) > maxMemoryScopeIdentifierBytes || !utf8.ValidString(value) {
		return ErrMemoryScopeIdentifier
	}
	for _, r := range value {
		if r == '\u0000' || r == '\n' || r == '\r' || r == '\t' {
			return ErrMemoryScopeIdentifier
		}
	}
	return nil
}

// Namespace returns the stable product-user tenant namespace. ProjectID and
// ConversationID are filtering metadata within that tenant, not separate
// libraries: a user must still be able to recall global memories in a new
// conversation. Providers must enforce tenant isolation independently of tags.
func (s MemoryScope) Namespace() string {
	if s.UserID <= 0 {
		return ""
	}
	return fmt.Sprintf("sub2api_user:%d", s.UserID)
}

// MemoryScopeFromContext obtains the authenticated Sub2API product user. The
// value is populated by API-key authentication middleware and is preferred to
// any user-like field supplied in a request body.
func MemoryScopeFromContext(ctx context.Context) (MemoryScope, bool) {
	if ctx == nil {
		return MemoryScope{}, false
	}
	userID, ok := ctx.Value(ctxkey.UserID).(int64)
	if !ok || userID <= 0 {
		return MemoryScope{}, false
	}
	return MemoryScope{UserID: userID}, true
}

// MemoryBudget bounds recalled snippet payloads, before prompt assembly.
// MaxTokens reserves one unit per UTF-8 byte, conservatively bounding text
// tokens for byte-level BPE tokenizers such as cl100k_base and o200k_base. It
// is not an exact model token count. The prompt assembler must also budget
// wrappers and protocol overhead with the target model's counting rules.
// Providers may apply a stricter limit than these normalized values.
type MemoryBudget struct {
	MaxItems  int
	MaxBytes  int
	MaxTokens int
}

func (b MemoryBudget) Normalize() MemoryBudget {
	if b.MaxItems <= 0 {
		b.MaxItems = defaultMemoryRecallItems
	}
	if b.MaxBytes <= 0 {
		b.MaxBytes = defaultMemoryRecallBytes
	}
	if b.MaxTokens <= 0 {
		b.MaxTokens = defaultMemoryRecallTokens
	}
	if b.MaxItems > maxMemoryRecallItems {
		b.MaxItems = maxMemoryRecallItems
	}
	if b.MaxBytes > maxMemoryRecallBytes {
		b.MaxBytes = maxMemoryRecallBytes
	}
	if b.MaxTokens > maxMemoryRecallTokens {
		b.MaxTokens = maxMemoryRecallTokens
	}
	return b
}

// MemoryRecallRequest is the single request-scoped lookup operation. Query is
// usually the latest user turn or a compact conversation summary; callers must
// not send the complete unbounded prompt as a lookup query.
type MemoryRecallRequest struct {
	Scope  MemoryScope
	Query  string
	Budget MemoryBudget
}

func (r MemoryRecallRequest) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.Query) == "" {
		return ErrMemoryQueryRequired
	}
	if len(r.Query) > maxMemoryQueryBytes || !utf8.ValidString(r.Query) {
		return fmt.Errorf("memory query exceeds %d bytes or is not valid UTF-8", maxMemoryQueryBytes)
	}
	return nil
}

// MemoryProvenance describes where a memory came from without making the
// upstream account part of the tenant key. RequestID and Model are useful for
// debugging and confidence review; providers should redact them from user
// visible text when appropriate.
type MemoryProvenance struct {
	Source        string
	RequestID     string
	Model         string
	CreatedAt     time.Time
	FragmentID    string
	SourceVersion string
	// Start and End locate a half-open range of Unicode scalars in the source
	// revision, not UTF-8 byte offsets. A source edit invalidates this locator.
	Start int
	End   int
}

// MemoryItem is a recalled, already-decrypted memory snippet ready for prompt
// assembly. Text is bounded by the provider/service before it reaches a model.
type MemoryItem struct {
	ID         string
	Text       string
	Confidence float64
	Provenance MemoryProvenance
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// MemoryRecallResult contains the snippets selected for one request.
type MemoryRecallResult struct {
	Items      []MemoryItem
	Incomplete bool
	Reason     string
}

// MemoryTurn is the unit archived after a successful product request. It has
// no upstream account field by design, so failover or model changes cannot
// create separate long-term memory tenants.
type MemoryTurn struct {
	// EventID identifies one product request across upstream attempts and queue
	// recovery. Queue implementations require it; it is not an upstream ID.
	EventID        string
	Scope          MemoryScope
	UserText       string
	AssistantText  string
	ConversationID string
	Model          string
	RequestID      string
	Source         string
	CreatedAt      time.Time
}

func (t MemoryTurn) Validate() error {
	if err := t.Scope.Validate(); err != nil {
		return err
	}
	if err := validateMemoryIdentifier(t.EventID); err != nil {
		return fmt.Errorf("event id: %w", err)
	}
	if t.ConversationID != "" && t.ConversationID != t.Scope.ConversationID {
		return fmt.Errorf("conversation id must match memory scope")
	}
	if len(t.UserText) > maxMemoryTurnTextBytes || len(t.AssistantText) > maxMemoryTurnTextBytes {
		return fmt.Errorf("memory turn text exceeds %d bytes", maxMemoryTurnTextBytes)
	}
	if !utf8.ValidString(t.UserText) || !utf8.ValidString(t.AssistantText) {
		return fmt.Errorf("memory turn text is not valid UTF-8")
	}
	if strings.TrimSpace(t.UserText) == "" && strings.TrimSpace(t.AssistantText) == "" {
		return ErrMemoryTurnContentEmpty
	}
	return nil
}

// MemoryProvider is the boundary between Sub2API and a memory implementation
// such as a Respire worker, a local store, or a Redis-backed short-term cache.
// Implementations must scope every operation using MemoryScope and must treat
// failures as non-fatal to the upstream LLM request.
type MemoryProvider interface {
	Recall(ctx context.Context, request MemoryRecallRequest) (MemoryRecallResult, error)
	Archive(ctx context.Context, turn MemoryTurn) error
	Delete(ctx context.Context, scope MemoryScope, memoryID string) error
}

// NoopMemoryProvider keeps memory integration disabled while preserving the
// call contract. It is also useful in tests and as the startup default until a
// worker with an authorized Respire vault is configured.
type NoopMemoryProvider struct{}

func (NoopMemoryProvider) Recall(context.Context, MemoryRecallRequest) (MemoryRecallResult, error) {
	return MemoryRecallResult{}, nil
}

func (NoopMemoryProvider) Archive(context.Context, MemoryTurn) error { return nil }

func (NoopMemoryProvider) Delete(context.Context, MemoryScope, string) error { return nil }

// MemoryService validates the contract and bounds recalled snippet payloads.
// It intentionally does not perform any network, vault, or prompt assembly
// operations by itself.
type MemoryService struct {
	provider MemoryProvider
}

func NewMemoryService(provider MemoryProvider) *MemoryService {
	if provider == nil {
		provider = NoopMemoryProvider{}
	}
	return &MemoryService{provider: provider}
}

func (s *MemoryService) Recall(ctx context.Context, request MemoryRecallRequest) (MemoryRecallResult, error) {
	if err := request.Validate(); err != nil {
		return MemoryRecallResult{}, err
	}
	request.Budget = request.Budget.Normalize()
	result, err := s.provider.Recall(ctx, request)
	if err != nil {
		return MemoryRecallResult{}, err
	}
	return clampMemoryRecallResult(result, request.Budget), nil
}

func (s *MemoryService) Archive(ctx context.Context, turn MemoryTurn) error {
	if err := turn.Validate(); err != nil {
		return err
	}
	return s.provider.Archive(ctx, turn)
}

func (s *MemoryService) Delete(ctx context.Context, scope MemoryScope, memoryID string) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(memoryID) == "" {
		return ErrMemoryItemIDRequired
	}
	return s.provider.Delete(ctx, scope, memoryID)
}

func clampMemoryRecallResult(result MemoryRecallResult, budget MemoryBudget) MemoryRecallResult {
	if budget.MaxBytes <= 0 || budget.MaxTokens <= 0 || budget.MaxItems <= 0 {
		result.Items = nil
		return result
	}
	used := 0
	usedTokens := 0
	kept := make([]MemoryItem, 0, min(len(result.Items), budget.MaxItems))
	for _, item := range result.Items {
		if len(kept) == budget.MaxItems {
			break
		}
		if item.Text == "" || !utf8.ValidString(item.Text) {
			continue
		}
		itemTokens := estimateMemoryTokens(item.Text)
		if used+len(item.Text) > budget.MaxBytes || usedTokens+itemTokens > budget.MaxTokens {
			result.Incomplete = true
			if !strings.Contains(result.Reason, "snippet_budget_exhausted") {
				if result.Reason != "" {
					result.Reason += ","
				}
				result.Reason += "snippet_budget_exhausted"
			}
			continue
		}
		used += len(item.Text)
		usedTokens += itemTokens
		kept = append(kept, item)
	}
	result.Items = kept
	return result
}

// estimateMemoryTokens reserves at most one byte-level BPE token per UTF-8
// byte. Code-point counts and bytes/4 can undercount CJK, emoji and rare Unicode
// characters. This reserve applies only to snippet text, not to wrappers or
// arbitrary tokenizers; actual upstream usage must be measured separately.
func estimateMemoryTokens(text string) int {
	return len(text)
}
