package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
	"github.com/tiktoken-go/tokenizer"
)

func TestMemoryScopeNamespaceIsProductUserScoped(t *testing.T) {
	base := MemoryScope{UserID: 42}
	project := MemoryScope{UserID: 42, ProjectID: "项目/alpha"}
	conversation := MemoryScope{UserID: 42, ProjectID: "项目/alpha", ConversationID: "conv:1"}

	require.Equal(t, "sub2api_user:42", base.Namespace())
	require.Equal(t, base.Namespace(), project.Namespace())
	require.Equal(t, base.Namespace(), conversation.Namespace())
	require.NotEqual(t, base.Namespace(), (MemoryScope{UserID: 43}).Namespace())
	require.NoError(t, base.Validate())
	require.NoError(t, project.Validate())
	require.NoError(t, conversation.Validate())

	for _, scope := range []MemoryScope{
		{},
		{UserID: 0},
		{UserID: 42, ProjectID: "bad\nvalue"},
		{UserID: 42, ConversationID: string(make([]byte, maxMemoryScopeIdentifierBytes+1))},
	} {
		require.Error(t, scope.Validate())
	}
}

func TestMemoryScopeFromContextUsesAuthenticatedProductUser(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.UserID, int64(91))
	scope, ok := MemoryScopeFromContext(ctx)
	require.True(t, ok)
	require.Equal(t, MemoryScope{UserID: 91}, scope)

	_, ok = MemoryScopeFromContext(context.Background())
	require.False(t, ok)
	ctx = context.WithValue(context.Background(), ctxkey.UserID, int64(0))
	_, ok = MemoryScopeFromContext(ctx)
	require.False(t, ok)
}

func TestMemoryServiceNormalizesBudgetAndClampsRecall(t *testing.T) {
	provider := &recordingMemoryProvider{
		recall: MemoryRecallResult{Items: []MemoryItem{
			{ID: "one", Text: "12345"},
			{ID: "two", Text: "67890"},
			{ID: "three", Text: "discarded"},
		}},
	}
	svc := NewMemoryService(provider)
	result, err := svc.Recall(context.Background(), MemoryRecallRequest{
		Scope:  MemoryScope{UserID: 7},
		Query:  "latest turn",
		Budget: MemoryBudget{MaxItems: 99, MaxBytes: 10, MaxTokens: 9999},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"one", "two"}, []string{result.Items[0].ID, result.Items[1].ID})
	require.Equal(t, 32, provider.lastRequest.Budget.MaxItems)
	require.Equal(t, 10, provider.lastRequest.Budget.MaxBytes)
	require.Equal(t, maxMemoryRecallTokens, provider.lastRequest.Budget.MaxTokens)
}

func TestMemoryServiceClampsRecallByTokenBudget(t *testing.T) {
	for _, tc := range []struct {
		name      string
		text      string
		maxTokens int
		wantItems int
	}{
		{name: "CJK exceeds budget", text: "中文记忆", maxTokens: 4},
		{name: "emoji exceeds budget", text: "🧠", maxTokens: 2},
		{name: "rare Unicode exceeds budget", text: "𠮷", maxTokens: 3},
		{name: "exact reserve keeps first item", text: "中文记忆", maxTokens: 12, wantItems: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &recordingMemoryProvider{recall: MemoryRecallResult{Items: []MemoryItem{
				{ID: "first", Text: tc.text},
				{ID: "second", Text: "another memory"},
			}}}

			result, err := NewMemoryService(provider).Recall(context.Background(), MemoryRecallRequest{
				Scope:  MemoryScope{UserID: 7},
				Query:  "hello",
				Budget: MemoryBudget{MaxItems: 8, MaxBytes: 1024, MaxTokens: tc.maxTokens},
			})
			require.NoError(t, err)
			require.Len(t, result.Items, tc.wantItems)
			if tc.wantItems > 0 {
				require.Equal(t, "first", result.Items[0].ID)
			}
		})
	}
}

func TestEstimateMemoryTokensBoundsSupportedBytePairEncodings(t *testing.T) {
	for _, encoding := range []tokenizer.Encoding{tokenizer.Cl100kBase, tokenizer.O200kBase} {
		t.Run(string(encoding), func(t *testing.T) {
			codec, err := tokenizer.Get(encoding)
			require.NoError(t, err)
			for _, text := range []string{"", "hello world", "中文记忆", "🧠", "𠮷", "e\u0301", strings.Repeat("记忆🧠abc!", 100)} {
				count, err := codec.Count(text)
				require.NoError(t, err)
				require.GreaterOrEqual(t, estimateMemoryTokens(text), count, "text=%q", text)
			}
		})
	}
}

func TestMemoryServiceSkipsInvalidUTF8FromProvider(t *testing.T) {
	provider := &recordingMemoryProvider{recall: MemoryRecallResult{Items: []MemoryItem{
		{ID: "invalid", Text: string([]byte{0xff})},
		{ID: "valid", Text: "hello"},
	}}}
	result, err := NewMemoryService(provider).Recall(context.Background(), MemoryRecallRequest{
		Scope: MemoryScope{UserID: 7}, Query: "hello",
	})
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, "valid", result.Items[0].ID)
}

func TestMemoryServiceTreatsProviderFailuresAsObservable(t *testing.T) {
	provider := &recordingMemoryProvider{recallErr: errors.New("respire unavailable")}
	svc := NewMemoryService(provider)
	_, err := svc.Recall(context.Background(), MemoryRecallRequest{Scope: MemoryScope{UserID: 7}, Query: "hello"})
	require.EqualError(t, err, "respire unavailable")

	// The no-op implementation is safe to use before a vault worker is configured.
	result, err := NewMemoryService(nil).Recall(context.Background(), MemoryRecallRequest{Scope: MemoryScope{UserID: 7}, Query: "hello"})
	require.NoError(t, err)
	require.Empty(t, result.Items)
}

func TestMemoryTurnRejectsScopeConversationMismatch(t *testing.T) {
	err := (MemoryTurn{
		Scope:          MemoryScope{UserID: 7, ConversationID: "scope-conv"},
		ConversationID: "other-conv",
		UserText:       "hello",
	}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "conversation id must match memory scope")
}

type recordingMemoryProvider struct {
	recall       MemoryRecallResult
	lastRequest  MemoryRecallRequest
	recallErr    error
	archivedTurn MemoryTurn
}

func (p *recordingMemoryProvider) Recall(_ context.Context, request MemoryRecallRequest) (MemoryRecallResult, error) {
	p.lastRequest = request
	return p.recall, p.recallErr
}

func (p *recordingMemoryProvider) Archive(_ context.Context, turn MemoryTurn) error {
	p.archivedTurn = turn
	return nil
}

func (*recordingMemoryProvider) Delete(context.Context, MemoryScope, string) error { return nil }
