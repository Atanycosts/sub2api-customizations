//go:build unit

package handler

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// These exercise the actual handler/forwarding path with fake external
// dependencies. They do not establish PostgreSQL or Respire system acceptance.
type memoryVerifierProvider struct {
	requests []service.MemoryRecallRequest
	items    []service.MemoryItem
	onRecall func(context.Context)
	err      error
}

func (p *memoryVerifierProvider) Recall(ctx context.Context, request service.MemoryRecallRequest) (service.MemoryRecallResult, error) {
	p.requests = append(p.requests, request)
	if p.onRecall != nil {
		p.onRecall(ctx)
	}
	return service.MemoryRecallResult{Items: p.items}, p.err
}

func (*memoryVerifierProvider) Archive(context.Context, service.MemoryTurn) error {
	panic("request handler must enqueue; it must not archive inline")
}

func (*memoryVerifierProvider) Delete(context.Context, service.MemoryScope, string) error {
	panic("not used")
}

type memoryVerifierRepository struct {
	turns      []service.MemoryTurn
	generation int64
	blocked    bool
	getUsers   []int64
	eventUsers []int64
	blockUsers []int64
	eventID    string
}

func (r *memoryVerifierRepository) Enqueue(ctx context.Context, turn service.MemoryTurn) (service.MemoryArchiveReceipt, error) {
	if err := ctx.Err(); err != nil {
		return service.MemoryArchiveReceipt{}, err
	}
	if r.blocked {
		return service.MemoryArchiveReceipt{}, service.ErrMemoryArchiveUserBlocked
	}
	r.turns = append(r.turns, turn)
	return service.MemoryArchiveReceipt{ID: int64(len(r.turns)), EventID: turn.EventID, State: service.MemoryArchivePending}, nil
}

func (r *memoryVerifierRepository) Get(_ context.Context, userID int64, eventID string) (service.MemoryArchiveReceipt, error) {
	r.eventUsers = append(r.eventUsers, userID)
	if userID != 100 || eventID != r.eventID {
		return service.MemoryArchiveReceipt{}, sql.ErrNoRows
	}
	return service.MemoryArchiveReceipt{ID: 1, EventID: eventID, State: service.MemoryArchivePending}, nil
}

func (*memoryVerifierRepository) Claim(context.Context, string, int, time.Duration, int, []int64) ([]service.MemoryArchiveEvent, error) {
	panic("worker must not start in these tests")
}

func (*memoryVerifierRepository) MarkSubmitted(context.Context, service.MemoryArchiveEvent) error {
	panic("not used")
}

func (*memoryVerifierRepository) Finish(context.Context, service.MemoryArchiveEvent, service.MemoryArchiveState, time.Time, string) error {
	panic("not used")
}

func (r *memoryVerifierRepository) ActiveGeneration(ctx context.Context, userID int64) (int64, error) {
	r.getUsers = append(r.getUsers, userID)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if r.blocked {
		return 0, service.ErrMemoryArchiveUserBlocked
	}
	if r.generation == 0 {
		return 1, nil
	}
	return r.generation, nil
}

func (r *memoryVerifierRepository) BlockUser(_ context.Context, userID int64) (int64, error) {
	r.blockUsers = append(r.blockUsers, userID)
	r.blocked = true
	r.generation++
	return r.generation, nil
}

func memoryVerifierInstall(h *OpenAIGatewayHandler, p *memoryVerifierProvider, repo *memoryVerifierRepository) {
	h.cfg.Memory = config.MemoryConfig{MaxItems: 8, MaxBytes: 4096, MaxTokens: 2048, CaptureMaxBytes: 64 * 1024, EnqueueTimeoutMS: 250}
	h.memoryGateway = service.NewMemoryGateway(p, repo, service.MemoryGatewayOptions{UserIDs: []int64{100}, RecallTimeout: 20 * time.Millisecond})
}

func TestMemoryPipelineAuthenticatesOnceAcrossActualFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := newAstraProCapturedUpstream(astra403(), astra200())
	h := newOpenAIResponsesFailoverTestHandler(t, upstream)
	p := &memoryVerifierProvider{items: []service.MemoryItem{{ID: "synthetic-memory", Text: "prefers concise replies"}}}
	repo := &memoryVerifierRepository{}
	memoryVerifierInstall(h, p, repo)
	original := `{"model":"gpt-6-astra","stream":false,"input":"current user statement","instructions":"original instruction","user_id":999,"metadata":{"user_id":999}}`
	c, rec := newAstraProFailoverContext(t, original)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.UserID, int64(100)))
	wantHash := h.gatewayService.GenerateSessionHash(c, []byte(original))
	h.Responses(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, p.requests, 1, "request-scoped recall must not repeat for each account")
	require.Equal(t, service.MemoryScope{UserID: 100}, p.requests[0].Scope)
	require.Equal(t, "current user statement", p.requests[0].Query)
	require.Equal(t, []int64{100, 100}, repo.getUsers)
	_, accountIDs, bodies := upstream.snapshot()
	assertAstraProAccountSwitch(t, accountIDs)
	for i, body := range bodies {
		require.Equal(t, 1, strings.Count(string(body), "prefers concise replies"), "attempt %d repeats an injection", i)
		require.Contains(t, string(body), "current user statement")
		require.Equal(t, "original instruction", gjson.GetBytes(body, "instructions").String())
		require.Equal(t, wantHash, h.gatewayService.GenerateSessionHash(c, body), "downstream session identity must use original content")
	}
	require.Len(t, repo.turns, 1)
	require.Equal(t, int64(100), repo.turns[0].Scope.UserID)
	require.Equal(t, "current user statement", repo.turns[0].UserText, "injected memories must not enter user transcript")
	require.Equal(t, "ok", repo.turns[0].AssistantText)
	require.Equal(t, rec.Header().Get("X-Sub2API-Memory-Event-Id"), repo.turns[0].EventID)
	require.NotEmpty(t, repo.turns[0].EventID)
	require.NotContains(t, repo.turns[0].UserText, "prefers concise replies")
}

func TestMemoryPipelineDoesNotAcceptBodyUserOrAuthSubjectAsTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		userID int64
	}{
		{name: "no authenticated context"},
		{name: "different product user", userID: 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := newAstraProCapturedUpstream(astra200())
			h := newOpenAIResponsesFailoverTestHandler(t, upstream)
			p, repo := &memoryVerifierProvider{}, &memoryVerifierRepository{}
			memoryVerifierInstall(h, p, repo)
			c, rec := newAstraProFailoverContext(t, `{"model":"gpt-6-astra","input":"hello","user_id":100}`)
			if tc.userID != 0 {
				c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.UserID, tc.userID))
			}
			h.Responses(c)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Empty(t, p.requests)
			require.Empty(t, repo.turns)
			require.Empty(t, rec.Header().Get("X-Sub2API-Memory-Event-Id"))
		})
	}
}

func TestMemoryPipelineRecallFailurePreservesForwardingAndCanEnqueue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := newAstraProCapturedUpstream(astra200())
	h := newOpenAIResponsesFailoverTestHandler(t, upstream)
	p := &memoryVerifierProvider{onRecall: func(ctx context.Context) { <-ctx.Done() }, err: context.DeadlineExceeded}
	repo := &memoryVerifierRepository{}
	memoryVerifierInstall(h, p, repo)
	c, rec := newAstraProFailoverContext(t, `{"model":"gpt-6-astra","input":"current question"}`)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.UserID, int64(100)))
	h.Responses(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, p.requests, 1)
	require.Len(t, repo.turns, 1)
	_, _, bodies := upstream.snapshot()
	require.Len(t, bodies, 1)
	require.NotContains(t, string(bodies[0]), "Saved memory references")
	require.Contains(t, string(bodies[0]), "current question")
}

func TestMemoryPipelineConcurrentBlockDiscardsRecallAndRejectsEnqueue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := newAstraProCapturedUpstream(astra200())
	h := newOpenAIResponsesFailoverTestHandler(t, upstream)
	repo := &memoryVerifierRepository{}
	p := &memoryVerifierProvider{items: []service.MemoryItem{{ID: "memory", Text: "must not leak after block"}}, onRecall: func(context.Context) { repo.blocked = true }}
	memoryVerifierInstall(h, p, repo)
	c, rec := newAstraProFailoverContext(t, `{"model":"gpt-6-astra","input":"hello"}`)
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.UserID, int64(100)))
	h.Responses(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, p.requests, 1)
	require.Empty(t, repo.turns)
	_, _, bodies := upstream.snapshot()
	require.Len(t, bodies, 1)
	require.NotContains(t, string(bodies[0]), "must not leak after block")
}

func TestMemoryPipelineSSECompletionControlsArchive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name        string
		terminal    string
		wantArchive bool
	}{
		{name: "completed", terminal: `{"type":"response.completed","response":{"id":"resp_mem","model":"gpt-6-astra","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"final answer"}]}],"usage":{"input_tokens":4,"output_tokens":2}}}`, wantArchive: true},
		{name: "incomplete", terminal: `{"type":"response.incomplete","response":{"id":"resp_mem","model":"gpt-6-astra","status":"incomplete","output":[],"usage":{"input_tokens":4,"output_tokens":2}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sse := "data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"final answer\"}\n\ndata: " + tc.terminal + "\n\n"
			upstream := newAstraProCapturedUpstream(&http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(bytes.NewBufferString(sse))})
			h := newOpenAIResponsesFailoverTestHandler(t, upstream)
			p, repo := &memoryVerifierProvider{}, &memoryVerifierRepository{}
			memoryVerifierInstall(h, p, repo)
			c, rec := newAstraProFailoverContext(t, `{"model":"gpt-6-astra","stream":true,"input":"hello"}`)
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.UserID, int64(100)))
			h.Responses(c)
			require.Equal(t, http.StatusOK, rec.Code)
			if tc.wantArchive {
				require.Len(t, repo.turns, 1)
				require.Equal(t, "final answer", repo.turns[0].AssistantText)
			} else {
				require.Empty(t, repo.turns)
			}
		})
	}
}

func TestMemoryPromptBudgetIncludesReferenceWrapper(t *testing.T) {
	original := []byte(`{"model":"test","input":"original","instructions":"keep"}`)
	before := append([]byte(nil), original...)
	injected, count, err := injectResponsesMemory(original, []service.MemoryItem{
		{ID: "first", Text: strings.Repeat("<&\"🧠", 5)},
		{ID: "second", Text: strings.Repeat("z", 1024)},
	}, service.MemoryBudget{MaxItems: 8, MaxBytes: 512, MaxTokens: 512})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, before, original)
	require.Equal(t, "keep", gjson.GetBytes(injected, "instructions").String())
	refs := gjson.GetBytes(injected, "input").Array()
	require.Len(t, refs, 2)
	var referenceText string
	for _, msg := range refs {
		text := msg.Get("content").String()
		if text != "original" {
			referenceText = text
		}
	}
	require.NotEmpty(t, referenceText)
	require.LessOrEqual(t, len(referenceText)+128, 512)
}

func TestMemoryInputRejectsUnsupportedSourcesInsteadOfExtractingToolData(t *testing.T) {
	for _, body := range []string{
		`{"input":"first parsed user","input":"different actual user"}`,
		`{"input":"hi","previous_response_id":"resp_hidden"}`,
		`{"input":"hi","tools":[{"type":"function","name":"run"}]}`,
		`{"input":[{"type":"function_call_output","call_id":"call_1","output":"secret tool text"}]}`,
		`{"input":[{"role":"user","content":[{"type":"input_text","text":"hi"},{"type":"input_image","image_url":"data:image/png;base64,anything"}]}]}`,
	} {
		text, ok := memoryResponsesUserText([]byte(body))
		require.False(t, ok, body)
		require.Empty(t, text)
	}
	text, ok := memoryResponsesUserText([]byte(`{"input":[{"role":"user","content":"older"},{"role":"assistant","content":"suggested"},{"role":"user","content":[{"type":"input_text","text":"latest"}]}]}`))
	require.True(t, ok)
	require.Equal(t, "latest", text)
}

func TestMemoryControlEndpointsRequireAuthenticatedContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &OpenAIGatewayHandler{}
	for _, handle := range []gin.HandlerFunc{h.MemoryStatus, h.MemoryEventStatus, h.SuspendMemory} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/sub2api/memory", strings.NewReader(`{"user_id":100}`))
		handle(c)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}
}

func TestMemoryControlEndpointsUseProductTenantAndExposeSuspensionHonestly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &memoryVerifierRepository{eventID: uuid.NewString()}
	h := &OpenAIGatewayHandler{memoryGateway: service.NewMemoryGateway(&memoryVerifierProvider{}, repo, service.MemoryGatewayOptions{UserIDs: []int64{100, 101}})}
	makeContext := func(userID int64) (*gin.Context, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/sub2api/memory/suspend", strings.NewReader(`{"user_id":101}`))
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.UserID, userID))
		c.Params = gin.Params{{Key: "event_id", Value: repo.eventID}}
		return c, rec
	}
	c, rec := makeContext(100)
	h.MemoryStatus(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "active", gjson.GetBytes(rec.Body.Bytes(), "state").String())
	require.False(t, gjson.GetBytes(rec.Body.Bytes(), "runtime_health_checked").Bool())

	c, rec = makeContext(101)
	h.MemoryEventStatus(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, []int64{101}, repo.eventUsers)
	require.NotContains(t, rec.Body.String(), repo.eventID)

	c, rec = makeContext(100)
	h.MemoryEventStatus(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "pending", gjson.GetBytes(rec.Body.Bytes(), "state").String())

	c, rec = makeContext(100)
	h.SuspendMemory(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []int64{100}, repo.blockUsers, "body-supplied target user must be ignored")
	require.Equal(t, "suspended", gjson.GetBytes(rec.Body.Bytes(), "state").String())
	confirmed := gjson.GetBytes(rec.Body.Bytes(), "physical_deletion_confirmed")
	require.True(t, confirmed.Exists())
	require.False(t, confirmed.Bool(), "fencing memory access must not claim engine data was physically erased")

	c, rec = makeContext(100)
	h.MemoryStatus(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "suspended", gjson.GetBytes(rec.Body.Bytes(), "state").String())
}

var _ service.MemoryProvider = (*memoryVerifierProvider)(nil)
var _ service.MemoryArchiveRepository = (*memoryVerifierRepository)(nil)
