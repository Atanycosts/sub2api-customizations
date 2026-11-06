//go:build memorycomponent && !integration

package repository

// These tests compose PostgreSQL, the real queue worker, MemoryGateway, and
// RespireMemoryProvider against a pinned-protocol HTTP fixture. The fixture is
// not a real Respire Runtime or model, and no personal memory library is used.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type memoryRoundtripFixture struct {
	mu                 sync.Mutex
	dataDir            string
	entries            map[string]map[string]any
	results            map[string]map[string]any
	writeCount         int
	dropNextWriteReply bool
	recallCalls        int
}

func (f *memoryRoundtripFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/api/health" {
		_ = json.NewEncoder(w).Encode(map[string]any{"server": "respire", "bin": "1.0.20", "v": 1, "pid": 321, "data_dir": f.dataDir, "store_ready": true})
		return
	}
	if r.URL.Path != "/api/rpc" || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var req struct {
		V      int      `json:"v"`
		ID     string   `json:"id"`
		Method string   `json:"method"`
		Args   []string `json:"args"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	response := map[string]any{"v": 1, "id": req.ID, "ok": true, "exit": 0, "bin": "1.0.20", "pid": 321, "data_dir": f.dataDir}
	envelope := func(command string, summary, details any) map[string]any {
		return map[string]any{"command": command, "status": "ok", "summary": summary, "details": details}
	}
	if req.Method == "runtime.result" {
		state := "unknown"
		var original any
		if hit, found := f.results[req.Args[0]]; found {
			state = "completed"
			original = hit
		}
		response["envelope"] = envelope("runtime.result", map[string]any{"request_id": req.Args[0], "state": state, "response": original}, nil)
	} else if req.Method == "cli.exec" && len(req.Args) >= 2 {
		if hit, found := f.results[req.ID]; found {
			response = hit
		} else {
			command := req.Args[1]
			value := req.Args[len(req.Args)-1]
			switch command {
			case "remember":
				f.writeCount++
				id := uuid.NewString()
				entry := map[string]any{"id": id, "title": req.Args[7], "content": "【日记】2026-11-06 12:00 " + value, "kind": "Context", "importance": "trivial", "project": ""}
				f.entries[id] = entry
				response["envelope"] = envelope("remember", map[string]any{"action": "created", "id": id, "index_state": "ready"}, entry)
				f.results[req.ID] = response
				if f.dropNextWriteReply {
					f.dropNextWriteReply = false
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
			case "show":
				response["envelope"] = envelope("show", map[string]any{"id": value}, map[string]any{"entry": f.entries[value]})
			case "recall":
				f.recallCalls++
				response["envelope"] = envelope("recall", map[string]any{"count": 0}, nil)
			case "diary-search":
				items := []map[string]any{}
				for _, entry := range f.entries {
					text := entry["content"].(string)
					if strings.Contains(text, value) {
						end := utf8.RuneCountInString(text)
						items = append(items, map[string]any{"source_kind": "diary", "evidence_state": "reference", "source_id": entry["id"], "fragment_id": fmt.Sprintf("d1.0.%d.%s", end, strings.Repeat("a", 32)), "source_version": strings.Repeat("b", 64), "text": text, "project": "", "start": 0, "end": end, "truncated": false})
					}
				}
				response["envelope"] = envelope("diary-search", map[string]any{"count": len(items)}, map[string]any{"items": items, "semantic_ready": true, "truncated": false, "offset_unit": "unicode_scalar"})
			default:
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.results[req.ID] = response
		}
	} else {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(response)
}
func newMemoryRoundtrip(t *testing.T) (*memoryRoundtripFixture, *service.RespireMemoryProvider) {
	t.Helper()
	fixture := &memoryRoundtripFixture{dataDir: "/srv/respire/component/u1", entries: map[string]map[string]any{}, results: map[string]map[string]any{}}
	server := httptest.NewServer(fixture)
	t.Cleanup(server.Close)
	provider, err := service.NewRespireMemoryProvider([]service.RespireMemoryBinding{{UserID: 1, BaseURL: server.URL, DataDir: fixture.dataDir}}, service.RespireMemoryOptions{Timeout: time.Second})
	require.NoError(t, err)
	return fixture, provider
}

func TestMemoryRoundtripPostgresHTTPFixtureArchivesRecallsAndIsolates(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	fixture, provider := newMemoryRoundtrip(t)
	gateway := service.NewMemoryGateway(provider, repo, service.MemoryGatewayOptions{UserIDs: []int64{1}, RecallTimeout: time.Second})
	ctx := context.Background()
	turn := memoryTestTurn(1, "full-roundtrip")
	turn.UserText = "Synthetic project: Amber Fox"
	receipt, err := gateway.Enqueue(ctx, turn)
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchivePending, receipt.State)
	worker := service.NewMemoryArchiveWorker(repo, provider, service.MemoryArchiveWorkerOptions{UserIDs: []int64{1}, BatchSize: 1})
	require.NoError(t, worker.RunOnce(ctx))
	receipt, err = gateway.Event(ctx, 1, turn.EventID)
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveCompleted, receipt.State)
	var payload sql.NullString
	require.NoError(t, db.QueryRow(`SELECT payload::text FROM memory_archive_queue WHERE id=$1`, receipt.ID).Scan(&payload))
	require.False(t, payload.Valid)
	result, err := gateway.Recall(ctx, service.MemoryRecallRequest{Scope: service.MemoryScope{UserID: 1}, Query: "Amber Fox", Budget: service.MemoryBudget{MaxTokens: 4096}})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Contains(t, result.Items[0].Text, `"user_statement":"Synthetic project: Amber Fox"`)
	require.Contains(t, result.Items[0].Text, "model_response_unverified")
	require.Equal(t, "respire:diary:reference", result.Items[0].Provenance.Source)
	// A different API Key/model is not represented in the tenant lookup. A new
	// logical event still sees the same library via the product user's scope.
	next := memoryTestTurn(1, "new-client-new-model")
	next.Model = "another-model"
	_, err = gateway.Enqueue(ctx, next)
	require.NoError(t, err)
	_, err = gateway.Recall(ctx, service.MemoryRecallRequest{Scope: service.MemoryScope{UserID: 2}, Query: "Amber Fox"})
	require.ErrorIs(t, err, service.ErrMemoryNotConfigured)
	fixture.mu.Lock()
	require.Equal(t, 1, fixture.recallCalls)
	require.Equal(t, 1, fixture.writeCount)
	fixture.mu.Unlock()
	_, err = gateway.Suspend(ctx, 1)
	require.NoError(t, err)
	_, err = gateway.Recall(ctx, service.MemoryRecallRequest{Scope: service.MemoryScope{UserID: 1}, Query: "Amber Fox"})
	require.ErrorIs(t, err, service.ErrMemoryArchiveUserBlocked)
	fixture.mu.Lock()
	require.Equal(t, 1, fixture.recallCalls, "blocked user must never reach provider")
	fixture.mu.Unlock()
}

func TestMemoryRoundtripPostgresHTTPFixtureLostReplyReconcilesWithoutSecondWrite(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	fixture, provider := newMemoryRoundtrip(t)
	ctx := context.Background()
	fixture.dropNextWriteReply = true
	turn := memoryTestTurn(1, "lost-write-reply")
	_, err := repo.Enqueue(ctx, turn)
	require.NoError(t, err)
	worker := service.NewMemoryArchiveWorker(repo, provider, service.MemoryArchiveWorkerOptions{UserIDs: []int64{1}, BatchSize: 1})
	require.NoError(t, worker.RunOnce(ctx))
	receipt, err := repo.Get(ctx, 1, turn.EventID)
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveNeedsReconciliation, receipt.State)
	_, err = db.Exec(`UPDATE memory_archive_queue SET available_at=NOW()-INTERVAL '1 second' WHERE id=$1`, receipt.ID)
	require.NoError(t, err)
	require.NoError(t, worker.RunOnce(ctx))
	receipt, err = repo.Get(ctx, 1, turn.EventID)
	require.NoError(t, err)
	require.Equal(t, service.MemoryArchiveCompleted, receipt.State)
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	require.Equal(t, 1, fixture.writeCount)
	require.Equal(t, 1, receipt.Attempts)
}
