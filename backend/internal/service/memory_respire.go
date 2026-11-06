package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const respireMemoryVersion = "1.0.20"

// RespireMemoryBinding maps one authenticated product user to an independently
// owned Runtime installation. DataDir is the absolute path reported by that
// server's health endpoint, not a path on the Sub2API client machine. A binding
// must never be reassigned to another tenant while requests are running: stable
// RPC v1 has no atomic account/instance admission fence.
type RespireMemoryBinding struct {
	UserID  int64
	BaseURL string
	DataDir string
}

type RespireMemoryOptions struct {
	Timeout          time.Duration
	QueueWait        time.Duration
	MaxResponseBytes int64
	HTTPClient       *http.Client
}

// RespireMemoryError reports protocol state without putting memory text or raw
// server diagnostics in logs. Unknown writes must be reconciled, not replayed.
type RespireMemoryError struct {
	Operation string
	Code      string
	unknown   bool
	retryable bool
	cause     error
}

func (e *RespireMemoryError) Error() string {
	return fmt.Sprintf("respire memory %s: %s", e.Operation, e.Code)
}
func (e *RespireMemoryError) Unwrap() error        { return e.cause }
func (e *RespireMemoryError) OutcomeUnknown() bool { return e.unknown }
func (e *RespireMemoryError) Retryable() bool      { return e.retryable && !e.unknown }

func respireMemoryFailure(op, code string, unknown, retryable bool, cause error) error {
	return &RespireMemoryError{Operation: op, Code: code, unknown: unknown, retryable: retryable, cause: cause}
}

type RespireMemoryProvider struct {
	bindings map[int64]RespireMemoryBinding
	client   *http.Client
	options  RespireMemoryOptions
}

var _ MemoryProvider = (*RespireMemoryProvider)(nil)
var _ MemoryArchiveReconciler = (*RespireMemoryProvider)(nil)

// NewRespireMemoryProvider only configures an adapter. It does not start a
// Runtime, create a library, log in, copy keys, or change a retrieval mode.
func NewRespireMemoryProvider(bindings []RespireMemoryBinding, options RespireMemoryOptions) (*RespireMemoryProvider, error) {
	if len(bindings) == 0 {
		return nil, respireMemoryFailure("configure", "bindings_required", false, false, nil)
	}
	if options.Timeout == 0 {
		options.Timeout = 5 * time.Second
	}
	if options.QueueWait == 0 {
		options.QueueWait = 250 * time.Millisecond
	}
	if options.MaxResponseBytes == 0 {
		options.MaxResponseBytes = 2 << 20
	}
	if options.Timeout < time.Millisecond || options.Timeout > 2*time.Minute || options.QueueWait < time.Millisecond || options.QueueWait > options.Timeout || options.MaxResponseBytes < 1024 || options.MaxResponseBytes > 8<<20 {
		return nil, respireMemoryFailure("configure", "invalid_limits", false, false, nil)
	}
	client := &http.Client{}
	if options.HTTPClient != nil {
		*client = *options.HTTPClient
	}
	// Enforce this even for a caller-supplied client. No write is replayed at a
	// redirect destination, and no request can escape the configured loopback.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if client.Timeout == 0 || client.Timeout > options.Timeout {
		client.Timeout = options.Timeout
	}
	p := &RespireMemoryProvider{bindings: make(map[int64]RespireMemoryBinding), client: client, options: options}
	endpoints := make(map[string]bool)
	libraries := make(map[string]bool)
	for _, binding := range bindings {
		u, err := url.Parse(binding.BaseURL)
		if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
			return nil, respireMemoryFailure("configure", "invalid_endpoint", false, false, nil)
		}
		ip := net.ParseIP(u.Hostname())
		if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, respireMemoryFailure("configure", "loopback_required", false, false, nil)
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, respireMemoryFailure("configure", "explicit_port_required", false, false, nil)
		}
		if binding.UserID <= 0 || !path.IsAbs(binding.DataDir) || path.Clean(binding.DataDir) != binding.DataDir {
			return nil, respireMemoryFailure("configure", "invalid_binding", false, false, nil)
		}
		// localhost/127.0.0.1/::1 aliases must not let one Runtime be shared by
		// two tenants. Conservatively reserve the whole loopback port per user.
		endpointKey := strconv.Itoa(port)
		if _, found := p.bindings[binding.UserID]; found || endpoints[endpointKey] || libraries[binding.DataDir] {
			return nil, respireMemoryFailure("configure", "duplicate_binding", false, false, nil)
		}
		binding.BaseURL = strings.TrimRight(binding.BaseURL, "/")
		p.bindings[binding.UserID] = binding
		endpoints[endpointKey] = true
		libraries[binding.DataDir] = true
	}
	return p, nil
}

type respireMemoryHealth struct {
	Server     string `json:"server"`
	Version    string `json:"bin"`
	Protocol   int    `json:"v"`
	PID        int    `json:"pid"`
	DataDir    string `json:"data_dir"`
	StoreReady bool   `json:"store_ready"`
}

type respireMemoryEnvelope struct {
	Command string          `json:"command"`
	Status  string          `json:"status"`
	Summary json.RawMessage `json:"summary"`
	Details json.RawMessage `json:"details"`
	Errors  []string        `json:"errors"`
}

type respireMemoryRPCResponse struct {
	Protocol int                    `json:"v"`
	ID       string                 `json:"id"`
	OK       bool                   `json:"ok"`
	Exit     int                    `json:"exit"`
	Version  string                 `json:"bin"`
	PID      int                    `json:"pid"`
	DataDir  string                 `json:"data_dir"`
	Code     string                 `json:"code"`
	Envelope *respireMemoryEnvelope `json:"envelope"`
}

type respireMemoryEntry struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	Kind         string `json:"kind"`
	Importance   string `json:"importance"`
	Project      string `json:"project"`
	SupersededBy string `json:"superseded_by"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func (p *RespireMemoryProvider) binding(scope MemoryScope, operation string) (RespireMemoryBinding, error) {
	if err := scope.Validate(); err != nil {
		return RespireMemoryBinding{}, err
	}
	if scope.ProjectID != "" || scope.ConversationID != "" {
		return RespireMemoryBinding{}, respireMemoryFailure(operation, "scoped_retrieval_unsupported", false, false, nil)
	}
	binding, found := p.bindings[scope.UserID]
	if !found {
		return RespireMemoryBinding{}, respireMemoryFailure(operation, "user_not_bound", false, false, nil)
	}
	return binding, nil
}

func (p *RespireMemoryProvider) health(ctx context.Context, binding RespireMemoryBinding, operation string) (respireMemoryHealth, error) {
	var health respireMemoryHealth
	err := p.httpJSON(ctx, http.MethodGet, binding.BaseURL+"/api/health", nil, &health)
	if err != nil {
		return health, respireMemoryFailure(operation, "health_unavailable", false, true, err)
	}
	if health.Server != "respire" || health.Protocol != 1 || health.Version != respireMemoryVersion || health.PID <= 0 || health.DataDir != binding.DataDir {
		return health, respireMemoryFailure(operation, "runtime_identity_mismatch", false, false, nil)
	}
	if !health.StoreReady {
		return health, respireMemoryFailure(operation, "store_not_ready", false, true, nil)
	}
	return health, nil
}

func (p *RespireMemoryProvider) httpJSON(ctx context.Context, method, endpoint string, body []byte, result any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	response, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, p.options.MaxResponseBytes+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > p.options.MaxResponseBytes {
		return errors.New("response body limit exceeded")
	}
	if !MemoryJSONUnambiguous(data) {
		return errors.New("response body is not unambiguous UTF-8 JSON")
	}
	return json.Unmarshal(data, result)
}

// rpc never retries. Once a write might have reached the Runtime, even a bad
// response or changed process identity has an unknown outcome.
func (p *RespireMemoryProvider) rpc(ctx context.Context, binding RespireMemoryBinding, health respireMemoryHealth, id, method string, args []string, write bool) (*respireMemoryEnvelope, error) {
	body, err := json.Marshal(struct {
		Protocol  int      `json:"v"`
		ID        string   `json:"id"`
		Method    string   `json:"method"`
		Args      []string `json:"args"`
		QueueWait int64    `json:"queue_wait_ms"`
	}{1, id, method, args, p.options.QueueWait.Milliseconds()})
	if err != nil {
		return nil, respireMemoryFailure(method, "request_encoding", false, false, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, respireMemoryFailure(method, "cancelled_before_submit", false, true, err)
	}
	var response respireMemoryRPCResponse
	if err := p.httpJSON(ctx, http.MethodPost, binding.BaseURL+"/api/rpc", body, &response); err != nil {
		return nil, respireMemoryFailure(method, "reply_unavailable", write, !write, err)
	}
	if response.Protocol != 1 || response.ID != id || response.Version != respireMemoryVersion || response.PID != health.PID {
		return nil, respireMemoryFailure(method, "response_identity_mismatch", write, false, nil)
	}
	if !response.OK || response.Exit != 0 {
		// Only these errors are emitted before command dispatch by the pinned
		// stable Runtime. Generic daemon/command failures may follow a commit.
		switch response.Code {
		case "protocol_version_mismatch", "model_switch_busy", "runtime_maintenance":
			return nil, respireMemoryFailure(method, response.Code, false, response.Code != "protocol_version_mismatch", nil)
		case "request_in_progress":
			return nil, respireMemoryFailure(method, response.Code, write, false, nil)
		default:
			return nil, respireMemoryFailure(method, "command_failed", write, false, nil)
		}
	}
	if response.DataDir != binding.DataDir {
		return nil, respireMemoryFailure(method, "response_library_mismatch", write, false, nil)
	}
	if response.Envelope == nil {
		return nil, respireMemoryFailure(method, "missing_envelope", write, false, nil)
	}
	return response.Envelope, nil
}

func checkRespireEnvelope(envelope *respireMemoryEnvelope, command string) error {
	if envelope.Command != command {
		return respireMemoryFailure(command, "unexpected_command", false, false, nil)
	}
	if envelope.Status == "pending" {
		return respireMemoryFailure(command, "pending", false, false, nil)
	}
	if envelope.Status != "ok" && envelope.Status != "skip" && envelope.Status != "warn" {
		return respireMemoryFailure(command, "unexpected_status", false, false, nil)
	}
	var summary struct {
		Mode    string `json:"mode"`
		Skipped bool   `json:"skipped"`
	}
	if err := json.Unmarshal(envelope.Summary, &summary); err != nil {
		return respireMemoryFailure(command, "invalid_summary", false, false, err)
	}
	if summary.Mode == "off" || summary.Skipped {
		return respireMemoryFailure(command, "runtime_memory_off", false, false, nil)
	}
	return nil
}

func (p *RespireMemoryProvider) Recall(ctx context.Context, request MemoryRecallRequest) (MemoryRecallResult, error) {
	if err := request.Validate(); err != nil {
		return MemoryRecallResult{}, err
	}
	binding, err := p.binding(request.Scope, "recall")
	if err != nil {
		return MemoryRecallResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, p.options.Timeout)
	defer cancel()
	health, err := p.health(ctx, binding, "recall")
	if err != nil {
		return MemoryRecallResult{}, err
	}
	budget := request.Budget.Normalize()
	limit := min(budget.MaxItems, 20)
	envelope, err := p.rpc(ctx, binding, health, uuid.NewString(), "cli.exec", []string{"--json", "recall", "--limit", strconv.Itoa(limit), "--", request.Query}, false)
	if err != nil {
		return MemoryRecallResult{}, err
	}
	if err := checkRespireEnvelope(envelope, "recall"); err != nil {
		return MemoryRecallResult{}, err
	}
	var main []struct {
		Entry        respireMemoryEntry `json:"entry"`
		SupersededBy string             `json:"superseded_by"`
	}
	if len(envelope.Details) > 0 && string(envelope.Details) != "null" {
		if err := json.Unmarshal(envelope.Details, &main); err != nil {
			return MemoryRecallResult{}, respireMemoryFailure("recall", "invalid_entries", false, false, err)
		}
	}
	var mainSummary struct {
		Count        *int `json:"count"`
		IndexPending int  `json:"index_pending"`
	}
	if json.Unmarshal(envelope.Summary, &mainSummary) != nil || mainSummary.Count == nil || *mainSummary.Count != len(main) {
		return MemoryRecallResult{}, respireMemoryFailure("recall", "entry_count_mismatch", false, false, nil)
	}
	result := MemoryRecallResult{}
	if mainSummary.IndexPending > 0 {
		markRespireIncomplete(&result, "main_index_pending")
	}
	seen := make(map[string]bool)
	for _, hit := range main {
		if hit.SupersededBy != "" || hit.Entry.SupersededBy != "" {
			continue
		}
		if hit.Entry.Project != "" {
			continue
		}
		if !validRespireMemoryID(hit.Entry.ID) || !utf8.ValidString(hit.Entry.Content) || hit.Entry.Content == "" {
			return MemoryRecallResult{}, respireMemoryFailure("recall", "invalid_entry", false, false, nil)
		}
		if !seen[hit.Entry.ID] {
			seen[hit.Entry.ID] = true
			result.Items = append(result.Items, MemoryItem{ID: hit.Entry.ID, Text: hit.Entry.Content,
				Provenance: MemoryProvenance{Source: "respire:reference"}, CreatedAt: respireMemoryTime(hit.Entry.CreatedAt), UpdatedAt: respireMemoryTime(hit.Entry.UpdatedAt)})
		}
	}
	// Explicit diary retrieval makes source diaries written by Archive reachable
	// without promoting model suggestions into main-memory facts.
	query := request.Query
	if utf8.RuneCountInString(query) > 500 {
		query = string([]rune(query)[:500])
		markRespireIncomplete(&result, "diary_query_truncated")
	}
	envelope, err = p.rpc(ctx, binding, health, uuid.NewString(), "cli.exec", []string{"--json", "diary-search", "--mode", "hybrid", "--limit", strconv.Itoa(limit), "--max-chars", "12000", "--", query}, false)
	if err != nil {
		return MemoryRecallResult{}, err
	}
	if err := checkRespireEnvelope(envelope, "diary-search"); err != nil {
		return MemoryRecallResult{}, err
	}
	var diary struct {
		Items []struct {
			SourceKind    string `json:"source_kind"`
			EvidenceState string `json:"evidence_state"`
			SourceID      string `json:"source_id"`
			FragmentID    string `json:"fragment_id"`
			SourceVersion string `json:"source_version"`
			Project       string `json:"project"`
			Start         int    `json:"start"`
			End           int    `json:"end"`
			Text          string `json:"text"`
			Truncated     bool   `json:"truncated"`
		} `json:"items"`
		SemanticReady bool   `json:"semantic_ready"`
		Truncated     bool   `json:"truncated"`
		OffsetUnit    string `json:"offset_unit"`
	}
	if err := json.Unmarshal(envelope.Details, &diary); err != nil || diary.OffsetUnit != "unicode_scalar" {
		return MemoryRecallResult{}, respireMemoryFailure("diary-search", "invalid_fragments", false, false, err)
	}
	var diarySummary struct {
		Count *int `json:"count"`
	}
	if json.Unmarshal(envelope.Summary, &diarySummary) != nil || diarySummary.Count == nil || *diarySummary.Count != len(diary.Items) {
		return MemoryRecallResult{}, respireMemoryFailure("diary-search", "fragment_count_mismatch", false, false, nil)
	}
	if !diary.SemanticReady {
		markRespireIncomplete(&result, "diary_semantic_index_pending")
	}
	if diary.Truncated {
		markRespireIncomplete(&result, "diary_results_truncated")
	}
	mainItems := result.Items
	result.Items = nil
	diaryItems := make([]MemoryItem, 0, len(diary.Items))
	for _, hit := range diary.Items {
		if hit.SourceKind != "diary" || hit.EvidenceState != "reference" || !validRespireMemoryID(hit.SourceID) || !validRespireFragmentLocator(hit.FragmentID, hit.Start, hit.End, hit.Truncated) || !validRespireDigest(hit.SourceVersion) || hit.Text == "" || !utf8.ValidString(hit.Text) || hit.End-hit.Start != utf8.RuneCountInString(hit.Text) {
			return MemoryRecallResult{}, respireMemoryFailure("diary-search", "invalid_fragment", false, false, nil)
		}
		if hit.Project != "" {
			continue
		}
		if hit.Truncated {
			markRespireIncomplete(&result, "diary_fragment_truncated")
		}
		key := hit.SourceID + ":" + hit.FragmentID
		if !seen[hit.SourceID] && !seen[key] {
			seen[key] = true
			diaryItems = append(diaryItems, MemoryItem{ID: hit.SourceID, Text: hit.Text, Provenance: MemoryProvenance{
				Source: "respire:diary:reference", SourceVersion: hit.SourceVersion, FragmentID: hit.FragmentID, Start: hit.Start, End: hit.End,
			}})
		}
	}
	// Preserve each source's ranking while reserving opportunities for both
	// main memories and newly archived source diaries. Scores from distinct
	// retrieval paths are not directly comparable confidence values.
	for i := 0; i < len(mainItems) || i < len(diaryItems); i++ {
		if i < len(mainItems) {
			result.Items = append(result.Items, mainItems[i])
		}
		if i < len(diaryItems) {
			result.Items = append(result.Items, diaryItems[i])
		}
	}
	return clampMemoryRecallResult(result, budget), nil
}

func markRespireIncomplete(result *MemoryRecallResult, reason string) {
	result.Incomplete = true
	if result.Reason != "" {
		result.Reason += ","
	}
	result.Reason += reason
}

func respireMemoryTime(value string) time.Time {
	stamp, _ := time.Parse(time.RFC3339Nano, value)
	return stamp
}

func validRespireMemoryID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

func validRespireDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && strings.ToLower(value) == value
}

func validRespireFragmentLocator(value string, start, end int, truncated bool) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 4 || parts[0] != "d1" || start < 0 || end <= start {
		return false
	}
	locatorStart, startErr := strconv.Atoi(parts[1])
	locatorEnd, endErr := strconv.Atoi(parts[2])
	if startErr != nil || endErr != nil || locatorStart < 0 || locatorEnd <= locatorStart || strconv.Itoa(locatorStart) != parts[1] || strconv.Itoa(locatorEnd) != parts[2] {
		return false
	}
	// Stable's render budget may return a window inside the immutable locator;
	// it updates start/end but deliberately keeps the original locator ID.
	if truncated {
		if start < locatorStart || end > locatorEnd {
			return false
		}
	} else if start != locatorStart || end != locatorEnd {
		return false
	}
	decoded, err := hex.DecodeString(parts[3])
	return err == nil && len(decoded) == 16 && strings.ToLower(parts[3]) == parts[3]
}

func respireMemoryArchiveIdentity(turn MemoryTurn) (string, string, string, error) {
	if err := turn.Validate(); err != nil {
		return "", "", "", err
	}
	if strings.TrimSpace(turn.EventID) == "" {
		return "", "", "", respireMemoryFailure("archive", "event_id_required", false, false, nil)
	}
	data, err := json.Marshal(struct {
		Tenant        string `json:"tenant"`
		EventID       string `json:"event_id"`
		UserStatement string `json:"user_statement"`
		ModelResponse string `json:"model_response_unverified"`
		Model         string `json:"model"`
		Source        string `json:"source"`
		CreatedAt     string `json:"created_at"`
	}{turn.Scope.Namespace(), turn.EventID, turn.UserText, turn.AssistantText, turn.Model, turn.Source, turn.CreatedAt.UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return "", "", "", err
	}
	eventHash := sha256.Sum256([]byte(turn.Scope.Namespace() + "\x00" + turn.EventID))
	digest := sha256.Sum256(data)
	id := "sub2api-archive-" + hex.EncodeToString(eventHash[:])
	title := "Sub2API recorded source conversation " + hex.EncodeToString(eventHash[:8])
	content := "【Cause】\nA Sub2API conversation produced source material, not independently verified facts.\n【Action】\n" + string(data) + "\n【Result】\nSource record SHA-256: " + hex.EncodeToString(digest[:]) + ". User statements and unverified model responses retain their distinct sources."
	return id, title, content, nil
}

func (p *RespireMemoryProvider) Archive(ctx context.Context, turn MemoryTurn) error {
	id, title, content, err := respireMemoryArchiveIdentity(turn)
	if err != nil {
		return err
	}
	binding, err := p.binding(turn.Scope, "archive")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, p.options.Timeout)
	defer cancel()
	health, err := p.health(ctx, binding, "archive")
	if err != nil {
		return err
	}
	envelope, err := p.rpc(ctx, binding, health, id, "cli.exec", []string{"--json", "remember", "--importance", "trivial", "--type", "context", "--title", title, "--", content}, true)
	if err != nil {
		return err
	}
	return p.verifyArchive(ctx, binding, health, envelope, title, content)
}

func (p *RespireMemoryProvider) verifyArchive(ctx context.Context, binding RespireMemoryBinding, health respireMemoryHealth, envelope *respireMemoryEnvelope, title, content string) error {
	if envelope.Command == "remember" && envelope.Status == "pending" {
		return respireMemoryFailure("archive", "write_pending", true, false, nil)
	}
	if err := checkRespireEnvelope(envelope, "remember"); err != nil {
		return respireMemoryFailure("archive", "write_unconfirmed", true, false, err)
	}
	var summary struct {
		ID     string `json:"id"`
		Action string `json:"action"`
	}
	var entry respireMemoryEntry
	if json.Unmarshal(envelope.Summary, &summary) != nil || summary.Action != "created" || !validRespireMemoryID(summary.ID) || json.Unmarshal(envelope.Details, &entry) != nil || entry.ID != summary.ID || !verifyRespireArchiveEntry(entry, title, content) {
		return respireMemoryFailure("archive", "write_receipt_mismatch", true, false, nil)
	}
	shown, err := p.rpc(ctx, binding, health, uuid.NewString(), "cli.exec", []string{"--json", "show", "--", summary.ID}, false)
	if err != nil {
		return respireMemoryFailure("archive", "write_readback_unavailable", true, false, err)
	}
	var details struct {
		Entry respireMemoryEntry `json:"entry"`
	}
	if checkRespireEnvelope(shown, "show") != nil || json.Unmarshal(shown.Details, &details) != nil || details.Entry.ID != entry.ID || details.Entry.Content != entry.Content || !verifyRespireArchiveEntry(details.Entry, title, content) {
		return respireMemoryFailure("archive", "write_readback_mismatch", true, false, nil)
	}
	return nil
}

func verifyRespireArchiveEntry(entry respireMemoryEntry, title, content string) bool {
	if entry.Title != title || !strings.EqualFold(entry.Kind, "context") || entry.Importance != "trivial" || entry.Project != "" {
		return false
	}
	// Stable v1.0.20 prepends exactly this timestamp to explicit-title diaries.
	prefix := strings.TrimSuffix(entry.Content, content)
	if prefix == entry.Content || !strings.HasPrefix(prefix, "【日记】") || !strings.HasSuffix(prefix, " ") {
		return false
	}
	_, err := time.Parse("2006-01-02 15:04", strings.TrimSuffix(strings.TrimPrefix(prefix, "【日记】"), " "))
	return err == nil
}

func (p *RespireMemoryProvider) ReconcileArchive(ctx context.Context, turn MemoryTurn) (MemoryArchiveReconciliation, error) {
	id, title, content, err := respireMemoryArchiveIdentity(turn)
	if err != nil {
		return MemoryArchiveReconciliationUnknown, err
	}
	binding, err := p.binding(turn.Scope, "reconcile")
	if err != nil {
		return MemoryArchiveReconciliationUnknown, err
	}
	ctx, cancel := context.WithTimeout(ctx, p.options.Timeout)
	defer cancel()
	health, err := p.health(ctx, binding, "reconcile")
	if err != nil {
		return MemoryArchiveReconciliationUnknown, err
	}
	envelope, err := p.rpc(ctx, binding, health, uuid.NewString(), "runtime.result", []string{id}, false)
	if err != nil {
		return MemoryArchiveReconciliationUnknown, err
	}
	if err := checkRespireEnvelope(envelope, "runtime.result"); err != nil {
		return MemoryArchiveReconciliationUnknown, err
	}
	var summary struct {
		RequestID string                    `json:"request_id"`
		State     string                    `json:"state"`
		Response  *respireMemoryRPCResponse `json:"response"`
	}
	if json.Unmarshal(envelope.Summary, &summary) != nil || summary.RequestID != id {
		return MemoryArchiveReconciliationUnknown, respireMemoryFailure("reconcile", "invalid_result", false, false, nil)
	}
	switch summary.State {
	case "pending":
		return MemoryArchiveReconciliationPending, nil
	case "unknown":
		return MemoryArchiveReconciliationUnknown, nil
	case "completed":
		response := summary.Response
		if response == nil || response.ID != id || response.Protocol != 1 || response.Version != respireMemoryVersion || response.PID != health.PID {
			return MemoryArchiveReconciliationUnknown, respireMemoryFailure("reconcile", "result_identity_mismatch", false, false, nil)
		}
		if !response.OK || response.Exit != 0 {
			if response.Code == "model_switch_busy" || response.Code == "runtime_maintenance" || response.Code == "protocol_version_mismatch" {
				return MemoryArchiveReconciliationRejected, nil
			}
			return MemoryArchiveReconciliationUnknown, nil
		}
		if response.DataDir != binding.DataDir || response.Envelope == nil {
			return MemoryArchiveReconciliationUnknown, respireMemoryFailure("reconcile", "result_library_mismatch", false, false, nil)
		}
		if err := p.verifyArchive(ctx, binding, health, response.Envelope, title, content); err != nil {
			return MemoryArchiveReconciliationUnknown, err
		}
		return MemoryArchiveReconciliationCompleted, nil
	default:
		return MemoryArchiveReconciliationUnknown, respireMemoryFailure("reconcile", "invalid_state", false, false, nil)
	}
}

func (p *RespireMemoryProvider) Delete(ctx context.Context, scope MemoryScope, memoryID string) error {
	binding, err := p.binding(scope, "delete")
	if err != nil {
		return err
	}
	if !validRespireMemoryID(memoryID) {
		return respireMemoryFailure("delete", "full_memory_id_required", false, false, nil)
	}
	ctx, cancel := context.WithTimeout(ctx, p.options.Timeout)
	defer cancel()
	health, err := p.health(ctx, binding, "delete")
	if err != nil {
		return err
	}
	// Delete has no durable event ID in the current interface, so it uses a
	// fresh transport identity and exposes uncertainty rather than retrying.
	envelope, err := p.rpc(ctx, binding, health, uuid.NewString(), "cli.exec", []string{"--json", "forget", "--", memoryID}, true)
	if err != nil {
		return err
	}
	var summary struct {
		ID      string `json:"id"`
		Deleted bool   `json:"deleted"`
	}
	if checkRespireEnvelope(envelope, "forget") != nil || envelope.Status != "ok" || json.Unmarshal(envelope.Summary, &summary) != nil || summary.ID != memoryID || !summary.Deleted {
		return respireMemoryFailure("delete", "delete_unconfirmed", true, false, nil)
	}
	return nil
}
