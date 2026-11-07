package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRespireProvisionedBindingsAreAdditiveAndIsolated(t *testing.T) {
	first := newRespireMemoryFixture(t)
	second := newRespireMemoryFixture(t)
	second.dataDir = "/srv/respire/u8"
	p := first.provider(t, RespireMemoryOptions{})
	binding := RespireMemoryBinding{UserID: 8, BaseURL: second.server.URL, DataDir: second.dataDir}
	require.NoError(t, p.AddBindings([]RespireMemoryBinding{binding}))
	require.Equal(t, []int64{7, 8}, p.UserIDs())
	turn := MemoryTurn{EventID: "new-user-event", Scope: MemoryScope{UserID: 8}, UserText: "only second account", CreatedAt: time.Now()}
	require.NoError(t, p.Archive(context.Background(), turn))
	require.Empty(t, first.requests, "admitting a new account must not route its write to the existing library")
	require.Equal(t, 1, second.writeCount)
	require.NoError(t, p.AddBindings([]RespireMemoryBinding{binding}), "same manifest is idempotent")
	require.NoError(t, p.AddBindings(nil), "manifest shrink must not remove existing tenant admission")
	require.True(t, p.HasUser(8))
	require.Error(t, p.AddBindings([]RespireMemoryBinding{{UserID: 8, BaseURL: first.server.URL, DataDir: first.dataDir}}))
	require.Error(t, p.AddBindings([]RespireMemoryBinding{{UserID: 9, BaseURL: second.server.URL, DataDir: "/srv/respire/u9"}}))
	require.Equal(t, []int64{7, 8}, p.UserIDs(), "invalid update must preserve all existing bindings")
}

func TestMemoryGatewayRefreshAdmitsOnlyValidatedManifestUsers(t *testing.T) {
	first := newRespireMemoryFixture(t)
	second := newRespireMemoryFixture(t)
	second.dataDir = "/srv/respire/u8"
	provider := first.provider(t, RespireMemoryOptions{})
	file := filepath.Join(t.TempDir(), "bindings.json")
	repo := &memoryArchiveScopeRepository{}
	g := NewMemoryGateway(provider, repo, MemoryGatewayOptions{UserIDs: []int64{7}})
	g.runtime, g.bindingsFile = provider, file
	worker := NewMemoryArchiveWorker(repo, provider, MemoryArchiveWorkerOptions{UserScope: provider.UserIDs})
	t.Cleanup(worker.Stop)
	bindings := []config.MemoryRuntimeBinding{{UserID: 8, BaseURL: second.server.URL, DataDir: second.dataDir}}
	encoded, err := json.Marshal(bindings)
	require.NoError(t, err)
	require.False(t, g.EnabledFor(8))
	require.NoError(t, os.WriteFile(file, encoded, 0600))
	require.NoError(t, g.reloadBindings())
	require.True(t, g.EnabledFor(8))
	require.NoError(t, worker.RunOnce(context.Background()))
	require.Equal(t, []int64{7, 8}, repo.userIDs, "worker admission follows the same validated provider snapshot")
	require.NoError(t, os.WriteFile(file, []byte(`[{"user_id":9`), 0600))
	require.Error(t, g.reloadBindings())
	require.True(t, g.EnabledFor(8))
	require.False(t, g.EnabledFor(9))
}

// These fixtures mirror fixed v1.0.20 output structs and command envelopes.
// They prove the adapter contract, not a live Core model or deployment.
type respireMemoryFixture struct {
	mu            sync.Mutex
	server        *httptest.Server
	dataDir       string
	version       string
	storeReady    bool
	requests      []respireMemoryFixtureRequest
	entries       map[string]respireMemoryEntry
	results       map[string]respireMemoryRPCResponse
	semanticReady bool
	unknownResult bool
	changeReplyID bool
	pendingWrite  bool
	dropReadback  bool
	writeCount    int
	mainHits      []map[string]any
}

type respireMemoryFixtureRequest struct {
	Protocol  int      `json:"v"`
	ID        string   `json:"id"`
	Method    string   `json:"method"`
	Args      []string `json:"args"`
	QueueWait int64    `json:"queue_wait_ms"`
}

func newRespireMemoryFixture(t *testing.T) *respireMemoryFixture {
	t.Helper()
	f := &respireMemoryFixture{dataDir: "/srv/respire/u7", version: respireMemoryVersion, storeReady: true,
		entries: make(map[string]respireMemoryEntry), results: make(map[string]respireMemoryRPCResponse), semanticReady: true}
	f.server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.server.Close)
	return f
}

func (f *respireMemoryFixture) provider(t *testing.T, options RespireMemoryOptions) *RespireMemoryProvider {
	t.Helper()
	p, err := NewRespireMemoryProvider([]RespireMemoryBinding{{UserID: 7, BaseURL: f.server.URL, DataDir: f.dataDir}}, options)
	require.NoError(t, err)
	return p
}

func (f *respireMemoryFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/api/health" {
		_ = json.NewEncoder(w).Encode(respireMemoryHealth{Server: "respire", Version: f.version, Protocol: 1, PID: 123, DataDir: f.dataDir, StoreReady: f.storeReady})
		return
	}
	if r.URL.Path != "/api/rpc" || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var request respireMemoryFixtureRequest
	if json.NewDecoder(r.Body).Decode(&request) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.requests = append(f.requests, request)
	response := respireMemoryRPCResponse{Protocol: 1, ID: request.ID, OK: true, Exit: 0, Version: f.version, PID: 123, DataDir: f.dataDir}
	if request.Method == "runtime.result" {
		original, found := f.results[request.Args[0]]
		state := "unknown"
		var originalJSON any
		if found && !f.unknownResult {
			state = "completed"
			originalJSON = original
		}
		response.Envelope = respireFixtureEnvelope("runtime.result", "ok", map[string]any{"request_id": request.Args[0], "state": state, "response": originalJSON}, nil)
	} else if request.Method == "cli.exec" {
		if original, found := f.results[request.ID]; found {
			response = original
		} else {
			command := request.Args[1]
			value := request.Args[len(request.Args)-1]
			switch command {
			case "remember":
				if f.pendingWrite {
					response.Envelope = respireFixtureEnvelope("remember", "pending", map[string]any{"state": "blocked", "written": false}, nil)
					break
				}
				f.writeCount++
				id := uuid.NewString()
				title := request.Args[7]
				entry := respireMemoryEntry{ID: id, Title: title, Content: "【日记】2026-11-06 12:00 " + value, Kind: "Context", Importance: "trivial"}
				f.entries[id] = entry
				response.Envelope = respireFixtureEnvelope("remember", "ok", map[string]any{"action": "created", "index_state": "ready", "id": id}, entry)
			case "show":
				if f.dropReadback {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				entry, found := f.entries[value]
				if !found {
					response.Envelope = respireFixtureEnvelope("show", "fail", map[string]any{"id": value}, nil)
					break
				}
				response.Envelope = respireFixtureEnvelope("show", "ok", map[string]any{"id": value}, map[string]any{"entry": entry})
			case "recall":
				status := "skip"
				if len(f.mainHits) > 0 {
					status = "ok"
				}
				response.Envelope = respireFixtureEnvelope("recall", status, map[string]any{"count": len(f.mainHits)}, f.mainHits)
			case "diary-search":
				items := make([]map[string]any, 0)
				for _, entry := range f.entries {
					if strings.Contains(entry.Content, value) {
						end := utf8.RuneCountInString(entry.Content)
						items = append(items, map[string]any{"source_kind": "diary", "evidence_state": "reference", "source_id": entry.ID,
							"fragment_id": fmt.Sprintf("d1.0.%d.%s", end, strings.Repeat("a", 32)), "source_version": strings.Repeat("b", 64),
							"text": entry.Content, "project": entry.Project, "start": 0, "end": end, "truncated": false})
					}
				}
				response.Envelope = respireFixtureEnvelope("diary-search", "ok", map[string]any{"count": len(items)}, map[string]any{
					"items": items, "semantic_ready": f.semanticReady, "truncated": false, "offset_unit": "unicode_scalar",
				})
			case "forget":
				_, found := f.entries[value]
				delete(f.entries, value)
				response.Envelope = respireFixtureEnvelope("forget", "ok", map[string]any{"deleted": found, "id": value}, nil)
			default:
				response.OK = false
				response.Exit = 1
				response.Code = "protocol_version_mismatch"
			}
			f.results[request.ID] = response
		}
	}
	if f.changeReplyID {
		response.ID = "unexpected"
	}
	_ = json.NewEncoder(w).Encode(response)
}

func respireFixtureEnvelope(command, status string, summary, details any) *respireMemoryEnvelope {
	summaryJSON, _ := json.Marshal(summary)
	detailsJSON, _ := json.Marshal(details)
	return &respireMemoryEnvelope{Command: command, Status: status, Summary: summaryJSON, Details: detailsJSON}
}

func respireFixtureTurn() MemoryTurn {
	return MemoryTurn{EventID: "logical-event-123", Scope: MemoryScope{UserID: 7}, UserText: "My synthetic test project is Amber Fox.",
		AssistantText: "A model suggestion, not a confirmed user fact.", Model: "test-model", Source: "responses-http", CreatedAt: time.Date(2026, 11, 6, 4, 0, 0, 0, time.UTC)}
}

func TestRespireMemoryProviderSourceArchiveRecallDeleteContract(t *testing.T) {
	fixture := newRespireMemoryFixture(t)
	provider := fixture.provider(t, RespireMemoryOptions{})
	turn := respireFixtureTurn()
	require.NoError(t, provider.Archive(context.Background(), turn))
	require.Equal(t, 1, fixture.writeCount)
	request := fixture.requests[0]
	require.Equal(t, 1, request.Protocol)
	require.Equal(t, "cli.exec", request.Method)
	require.Equal(t, []string{"--json", "remember", "--importance", "trivial", "--type", "context", "--title"}, request.Args[:7])
	require.NotContains(t, request.Args, "--force")
	require.Contains(t, request.Args[len(request.Args)-1], `"user_statement":"My synthetic test project is Amber Fox."`)
	require.Contains(t, request.Args[len(request.Args)-1], `"model_response_unverified":`)
	// Runtime bounded deduplication is not a durable exactly-once guarantee.
	require.NoError(t, provider.Archive(context.Background(), turn))
	require.Equal(t, 1, fixture.writeCount)
	require.Equal(t, request.ID, fixture.requests[2].ID)

	result, err := provider.Recall(context.Background(), MemoryRecallRequest{Scope: turn.Scope, Query: "Amber Fox", Budget: MemoryBudget{MaxTokens: 4096}})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Contains(t, result.Items[0].Text, "Amber Fox")
	require.Equal(t, "respire:diary:reference", result.Items[0].Provenance.Source)
	require.Equal(t, strings.Repeat("b", 64), result.Items[0].Provenance.SourceVersion)
	require.NotEmpty(t, result.Items[0].Provenance.FragmentID)
	require.Equal(t, utf8.RuneCountInString(result.Items[0].Text), result.Items[0].Provenance.End-result.Items[0].Provenance.Start)
	require.False(t, result.Incomplete)
	require.Zero(t, result.Items[0].Confidence)
	require.NoError(t, provider.Delete(context.Background(), turn.Scope, result.Items[0].ID))
	result, err = provider.Recall(context.Background(), MemoryRecallRequest{Scope: turn.Scope, Query: "Amber Fox"})
	require.NoError(t, err)
	require.Empty(t, result.Items)
}

func TestRespireMemoryProviderBindingsRejectTenantSharing(t *testing.T) {
	for _, tc := range []struct {
		name     string
		bindings []RespireMemoryBinding
	}{
		{"same endpoint", []RespireMemoryBinding{{7, "http://127.0.0.1:15169", "/srv/u7"}, {8, "http://127.0.0.1:15169", "/srv/u8"}}},
		{"loopback alias", []RespireMemoryBinding{{7, "http://localhost:15169", "/srv/u7"}, {8, "http://127.0.0.1:15169", "/srv/u8"}}},
		{"same library", []RespireMemoryBinding{{7, "http://127.0.0.1:15169", "/srv/shared"}, {8, "http://127.0.0.1:15170", "/srv/shared"}}},
		{"same user", []RespireMemoryBinding{{7, "http://127.0.0.1:15169", "/srv/u7"}, {7, "http://127.0.0.1:15170", "/srv/u7-copy"}}},
		{"public endpoint", []RespireMemoryBinding{{7, "http://example.com:15169", "/srv/u7"}}},
		{"path endpoint", []RespireMemoryBinding{{7, "http://127.0.0.1:15169/path", "/srv/u7"}}},
		{"relative library", []RespireMemoryBinding{{7, "http://127.0.0.1:15169", "relative"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRespireMemoryProvider(tc.bindings, RespireMemoryOptions{})
			require.Error(t, err)
		})
	}
}

func TestRespireMemoryProviderRejectsUnboundUserAndScopedAccessBeforeHTTP(t *testing.T) {
	fixture := newRespireMemoryFixture(t)
	provider := fixture.provider(t, RespireMemoryOptions{})
	for _, scope := range []MemoryScope{{UserID: 8}, {UserID: 7, ProjectID: "other"}, {UserID: 7, ConversationID: "conv"}} {
		_, err := provider.Recall(context.Background(), MemoryRecallRequest{Scope: scope, Query: "Amber Fox"})
		require.Error(t, err)
	}
	require.Empty(t, fixture.requests)
}

func TestRespireMemoryProviderChecksCurrentLibraryAndVersionBeforeWrite(t *testing.T) {
	for _, kind := range []string{"version", "library", "not ready"} {
		t.Run(kind, func(t *testing.T) {
			fixture := newRespireMemoryFixture(t)
			provider := fixture.provider(t, RespireMemoryOptions{})
			switch kind {
			case "version":
				fixture.version = "1.0.21-dev.6"
			case "library":
				fixture.dataDir = "/srv/respire/another-user"
			case "not ready":
				fixture.storeReady = false
			}
			err := provider.Archive(context.Background(), respireFixtureTurn())
			require.Error(t, err)
			var outcome MemoryArchiveOutcomeUnknown
			require.ErrorAs(t, err, &outcome)
			require.False(t, outcome.OutcomeUnknown())
			require.Empty(t, fixture.requests)
		})
	}
}

func TestRespireMemoryProviderUnknownWriteReconcilesWithoutReplay(t *testing.T) {
	fixture := newRespireMemoryFixture(t)
	provider := fixture.provider(t, RespireMemoryOptions{})
	fixture.dropReadback = true
	err := provider.Archive(context.Background(), respireFixtureTurn())
	requireRespireUnknown(t, err)
	require.Equal(t, 1, fixture.writeCount)
	fixture.dropReadback = false
	state, err := provider.ReconcileArchive(context.Background(), respireFixtureTurn())
	require.NoError(t, err)
	require.Equal(t, MemoryArchiveReconciliationCompleted, state)
	require.Equal(t, 1, fixture.writeCount)
	fixture.unknownResult = true
	state, err = provider.ReconcileArchive(context.Background(), respireFixtureTurn())
	require.NoError(t, err)
	require.Equal(t, MemoryArchiveReconciliationUnknown, state)
	require.Equal(t, 1, fixture.writeCount)
}

func TestRespireMemoryProviderWrongReplyIDAfterCommitIsUnknown(t *testing.T) {
	fixture := newRespireMemoryFixture(t)
	provider := fixture.provider(t, RespireMemoryOptions{})
	fixture.changeReplyID = true
	requireRespireUnknown(t, provider.Archive(context.Background(), respireFixtureTurn()))
	require.Equal(t, 1, fixture.writeCount)
}

func TestRespireMemoryProviderSameEventDifferentContentCannotConfirmOldWrite(t *testing.T) {
	fixture := newRespireMemoryFixture(t)
	provider := fixture.provider(t, RespireMemoryOptions{})
	turn := respireFixtureTurn()
	require.NoError(t, provider.Archive(context.Background(), turn))
	turn.UserText = "Conflicting contents under an existing event identity"
	requireRespireUnknown(t, provider.Archive(context.Background(), turn))
	state, err := provider.ReconcileArchive(context.Background(), turn)
	require.Error(t, err)
	require.Equal(t, MemoryArchiveReconciliationUnknown, state)
	require.Equal(t, 1, fixture.writeCount)
}

func TestRespireMemoryProviderGlobalRecallExcludesProjectDiaries(t *testing.T) {
	fixture := newRespireMemoryFixture(t)
	provider := fixture.provider(t, RespireMemoryOptions{})
	id := uuid.NewString()
	fixture.entries[id] = respireMemoryEntry{ID: id, Content: "Secret Amber Fox", Project: "private-project"}
	result, err := provider.Recall(context.Background(), MemoryRecallRequest{Scope: MemoryScope{UserID: 7}, Query: "Amber Fox"})
	require.NoError(t, err)
	require.Empty(t, result.Items)
}

func TestRespireMemoryProviderMainRecallParsesBodiesAndExcludesSupersededOrProjectEntries(t *testing.T) {
	fixture := newRespireMemoryFixture(t)
	provider := fixture.provider(t, RespireMemoryOptions{})
	currentID, replacedID, projectID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	fixture.mainHits = []map[string]any{
		{"score": 0.99, "entry": respireMemoryEntry{ID: replacedID, Content: "outdated statement"}, "superseded_by": currentID},
		{"score": 0.90, "entry": respireMemoryEntry{ID: projectID, Content: "project-specific statement", Project: "separate-project"}},
		{"score": 0.80, "entry": respireMemoryEntry{ID: currentID, Title: "current preference", Content: "source-supported preference"}},
	}
	result, err := provider.Recall(context.Background(), MemoryRecallRequest{Scope: MemoryScope{UserID: 7}, Query: "preference"})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, currentID, result.Items[0].ID)
	require.Equal(t, "source-supported preference", result.Items[0].Text)
	require.Equal(t, "respire:reference", result.Items[0].Provenance.Source)
	require.Zero(t, result.Items[0].Confidence)
	require.NotContains(t, fixture.requests[0].Args, "--no-related", "stable recall needs related metadata to surface supersession")
}

func TestRespireMemoryProviderCandidateReceiptIsNotSaved(t *testing.T) {
	fixture := newRespireMemoryFixture(t)
	provider := fixture.provider(t, RespireMemoryOptions{})
	fixture.pendingWrite = true
	err := provider.Archive(context.Background(), respireFixtureTurn())
	require.Error(t, err)
	var outcome MemoryArchiveOutcomeUnknown
	require.ErrorAs(t, err, &outcome)
	require.True(t, outcome.OutcomeUnknown())
	require.Zero(t, fixture.writeCount)
}

func TestRespireMemoryProviderIncompleteSemanticCoverageIsExplicit(t *testing.T) {
	fixture := newRespireMemoryFixture(t)
	provider := fixture.provider(t, RespireMemoryOptions{})
	fixture.semanticReady = false
	result, err := provider.Recall(context.Background(), MemoryRecallRequest{Scope: MemoryScope{UserID: 7}, Query: strings.Repeat("中", 501)})
	require.NoError(t, err)
	require.True(t, result.Incomplete)
	require.Contains(t, result.Reason, "diary_query_truncated")
	require.Contains(t, result.Reason, "diary_semantic_index_pending")
	diaryRequest := fixture.requests[len(fixture.requests)-1]
	require.Len(t, []rune(diaryRequest.Args[len(diaryRequest.Args)-1]), 500)
}

func TestRespireMemoryProviderTreatsExternalTextAsLiteralArguments(t *testing.T) {
	fixture := newRespireMemoryFixture(t)
	provider := fixture.provider(t, RespireMemoryOptions{})
	query := "--mode quality; $(synthetic-command)"
	_, err := provider.Recall(context.Background(), MemoryRecallRequest{Scope: MemoryScope{UserID: 7}, Query: query})
	require.NoError(t, err)
	for _, request := range fixture.requests {
		require.Equal(t, query, request.Args[len(request.Args)-1])
		require.Equal(t, "--", request.Args[len(request.Args)-2])
	}
	turn := respireFixtureTurn()
	turn.UserText = query
	require.NoError(t, provider.Archive(context.Background(), turn))
	write := fixture.requests[len(fixture.requests)-2]
	require.Equal(t, "remember", write.Args[1])
	require.Equal(t, "--", write.Args[len(write.Args)-2])
	require.Contains(t, write.Args[len(write.Args)-1], query)
}

func TestRespireMemoryProviderDoesNotFollowRedirectsFromInjectedClient(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++; w.WriteHeader(http.StatusOK) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}
	provider, err := NewRespireMemoryProvider([]RespireMemoryBinding{{7, source.URL, "/srv/u7"}}, RespireMemoryOptions{HTTPClient: client})
	require.NoError(t, err)
	_, err = provider.Recall(context.Background(), MemoryRecallRequest{Scope: MemoryScope{UserID: 7}, Query: "test"})
	require.Error(t, err)
	require.Zero(t, targetCalls)
}

func TestRespireMemoryProviderBoundsResponseAndDeadline(t *testing.T) {
	for _, kind := range []string{"body", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "body" {
					_, _ = fmt.Fprint(w, strings.Repeat("x", 2048))
					return
				}
				<-r.Context().Done()
			}))
			defer server.Close()
			provider, err := NewRespireMemoryProvider([]RespireMemoryBinding{{7, server.URL, "/srv/u7"}}, RespireMemoryOptions{Timeout: 50 * time.Millisecond, QueueWait: 10 * time.Millisecond, MaxResponseBytes: 1024})
			require.NoError(t, err)
			_, err = provider.Recall(context.Background(), MemoryRecallRequest{Scope: MemoryScope{UserID: 7}, Query: "test"})
			require.Error(t, err)
			if kind == "deadline" {
				require.True(t, errors.Is(err, context.DeadlineExceeded))
			}
		})
	}
}

func TestRespireMemoryFragmentLocatorPreservesBudgetedSourceRange(t *testing.T) {
	locator := "d1.100.900." + strings.Repeat("a", 32)
	require.True(t, validRespireFragmentLocator(locator, 100, 900, false))
	require.True(t, validRespireFragmentLocator(locator, 200, 600, true))
	require.False(t, validRespireFragmentLocator(locator, 200, 600, false))
	require.False(t, validRespireFragmentLocator(locator, 90, 600, true))
	require.False(t, validRespireFragmentLocator(locator, 200, 901, true))
}

func requireRespireUnknown(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	var outcome MemoryArchiveOutcomeUnknown
	require.ErrorAs(t, err, &outcome)
	require.True(t, outcome.OutcomeUnknown())
	var retry MemoryArchiveRetryable
	require.ErrorAs(t, err, &retry)
	require.False(t, retry.Retryable())
}
