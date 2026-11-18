package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/stretchr/testify/require"
)

func TestOpenAIRequestBudgetDisabled(t *testing.T) {
	budget := NewOpenAIRequestBudget(0, 0)
	ctx := InstallOpenAIRequestBudget(context.Background(), budget)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com/v1/responses", http.NoBody)
	require.NoError(t, err)

	guarded, guard := GuardOpenAIRequestBudget(req)
	require.Same(t, req, guarded)
	require.Nil(t, guard)
}

func TestOpenAIRequestBudgetPreOutputTimeout(t *testing.T) {
	budget := NewOpenAIRequestBudget(0, 20*time.Millisecond)
	ctx := InstallOpenAIRequestBudget(context.Background(), budget)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com/v1/responses", strings.NewReader("{}"))
	require.NoError(t, err)

	_, guard := GuardOpenAIRequestBudget(req)
	require.NotNil(t, guard)
	t.Cleanup(guard.Close)

	require.Eventually(t, func() bool {
		err := guard.Err()
		var budgetErr *OpenAIRequestBudgetError
		return errors.As(err, &budgetErr) && budgetErr.Phase == OpenAIRequestBudgetPreOutput
	}, time.Second, 5*time.Millisecond)
}

func TestOpenAIRequestBudgetHeartbeatDoesNotReleasePreOutput(t *testing.T) {
	budget := NewOpenAIRequestBudget(0, 20*time.Millisecond)
	ctx := InstallOpenAIRequestBudget(context.Background(), budget)
	requesttiming.Observe(ctx, false, false)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com/v1/responses", http.NoBody)
	require.NoError(t, err)

	_, guard := GuardOpenAIRequestBudget(req)
	require.NotNil(t, guard)
	t.Cleanup(guard.Close)

	require.Eventually(t, func() bool {
		var budgetErr *OpenAIRequestBudgetError
		return errors.As(guard.Err(), &budgetErr)
	}, time.Second, 5*time.Millisecond)
}

func TestOpenAIRequestBudgetSemanticContentReleasesPreOutput(t *testing.T) {
	budget := NewOpenAIRequestBudget(0, 20*time.Millisecond)
	ctx := InstallOpenAIRequestBudget(context.Background(), budget)
	requesttiming.Observe(ctx, true, true)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com/v1/responses", http.NoBody)
	require.NoError(t, err)

	_, guard := GuardOpenAIRequestBudget(req)
	require.NotNil(t, guard)
	t.Cleanup(guard.Close)

	time.Sleep(35 * time.Millisecond)
	require.NoError(t, guard.Err())
}

func TestOpenAIRequestBudgetUploadCompletesIndependently(t *testing.T) {
	budget := NewOpenAIRequestBudget(20*time.Millisecond, time.Minute)
	ctx := InstallOpenAIRequestBudget(context.Background(), budget)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com/v1/responses", strings.NewReader("{}"))
	require.NoError(t, err)

	_, guard := GuardOpenAIRequestBudget(req)
	require.NotNil(t, guard)
	t.Cleanup(guard.Close)
	guard.uploadComplete()

	time.Sleep(35 * time.Millisecond)
	require.NoError(t, guard.Err())
}

func TestOpenAIRequestBudgetTimeoutSurfacesFromBodyRead(t *testing.T) {
	budget := NewOpenAIRequestBudget(0, 15*time.Millisecond)
	ctx := InstallOpenAIRequestBudget(context.Background(), budget)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com/v1/responses", strings.NewReader("{}"))
	require.NoError(t, err)

	_, guard := GuardOpenAIRequestBudget(req)
	require.NotNil(t, guard)
	resp, finishErr := guard.Finish(&http.Response{
		StatusCode: http.StatusOK,
		Body:       delayedReadBody{delay: 30 * time.Millisecond},
	}, nil)
	require.NoError(t, finishErr)
	require.NotNil(t, resp)

	_, readErr := resp.Body.Read(make([]byte, 1))
	var budgetErr *OpenAIRequestBudgetError
	require.ErrorAs(t, readErr, &budgetErr)
	require.Equal(t, OpenAIRequestBudgetPreOutput, budgetErr.Phase)
	require.NoError(t, resp.Body.Close())
}

func TestOpenAIRequestBudgetParentCancellationIsNotBudgetFailure(t *testing.T) {
	budget := NewOpenAIRequestBudget(time.Minute, time.Minute)
	parent, cancel := context.WithCancel(context.Background())
	ctx := InstallOpenAIRequestBudget(parent, budget)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.com/v1/responses", http.NoBody)
	require.NoError(t, err)

	_, guard := GuardOpenAIRequestBudget(req)
	require.NotNil(t, guard)
	cancel()
	t.Cleanup(guard.Close)

	require.Eventually(t, func() bool {
		return guard.ctx.Err() != nil
	}, time.Second, 5*time.Millisecond)
	require.NoError(t, guard.Err())
}

type delayedReadBody struct {
	delay time.Duration
}

func (b delayedReadBody) Read([]byte) (int, error) {
	time.Sleep(b.delay)
	return 0, errors.New("upstream read stopped after budget cancellation")
}

func (delayedReadBody) Close() error {
	return nil
}
