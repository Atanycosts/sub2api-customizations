package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMemoryOriginalBodyPreservesIdentitiesAcrossInjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	original := []byte(`{"model":"grok-4.5","input":"original user","instructions":"stable"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(original))
	svc := &OpenAIGatewayService{}
	want := svc.GenerateSessionHash(c, original)
	require.NotEmpty(t, want)
	mutated := []byte(`{"model":"grok-4.5","input":[{"role":"user","content":"dynamic memory"},{"role":"user","content":"original user"}],"instructions":"stable"}`)
	require.NotEqual(t, want, svc.GenerateSessionHash(c, mutated))
	c.Request = c.Request.WithContext(WithMemoryOriginalBody(c.Request.Context(), original))
	for i := range original {
		original[i] = 'x'
	}
	require.Equal(t, want, svc.GenerateSessionHash(c, mutated), "identity must own its input copy")
	c.Request = c.Request.WithContext(context.Background())
	require.NotEqual(t, want, svc.GenerateSessionHash(c, mutated), "override cannot escape the request")
}

func TestMemoryRecallBudgetSkipsOversizedHighRankedSnippet(t *testing.T) {
	provider := &recordingMemoryProvider{recall: MemoryRecallResult{Items: []MemoryItem{
		{ID: "large", Text: strings.Repeat("x", 5000)},
		{ID: "usable", Text: "small source fact"},
	}}}
	result, err := NewMemoryService(provider).Recall(context.Background(), MemoryRecallRequest{Scope: MemoryScope{UserID: 1}, Query: "fact", Budget: MemoryBudget{MaxItems: 1, MaxBytes: 100, MaxTokens: 100}})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "usable", result.Items[0].ID)
	require.True(t, result.Incomplete)
}

func TestMemoryGatewayShortArchiveTimeoutClampsRPCQueueWait(t *testing.T) {
	cfg := &config.Config{Memory: config.MemoryConfig{
		Enabled: true, Bindings: []config.MemoryRuntimeBinding{{UserID: 7, BaseURL: "http://127.0.0.1:15170", DataDir: "/srv/respire/u7"}},
		RecallTimeoutMS: 300, ArchiveTimeoutMS: 100, EnqueueTimeoutMS: 500,
		PollIntervalMS: 1000, LeaseSeconds: 180, MaxAttempts: 3,
		MaxItems: 8, MaxBytes: 8192, MaxTokens: 2048, CaptureMaxBytes: 262144, MaxResponseBytes: 1048576,
	}}
	require.NoError(t, cfg.Memory.Validate())
	repo := &memoryArchiveScopeRepository{}
	gateway, err := ProvideMemoryGateway(cfg, repo)
	require.NoError(t, err, "a configuration accepted by validation must construct the adapter")
	t.Cleanup(gateway.Stop)
	provider, ok := gateway.service.provider.(*RespireMemoryProvider)
	require.True(t, ok)
	require.Equal(t, 100*time.Millisecond, provider.options.QueueWait)
	require.LessOrEqual(t, provider.options.QueueWait, provider.options.Timeout)
	require.Equal(t, []int64{7}, gateway.worker.options.UserIDs)
	require.Zero(t, repo.claimCalls, "construction must not start a worker")
}
