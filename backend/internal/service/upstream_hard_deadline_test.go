package service

import (
	"context"
	"testing"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/stretchr/testify/require"
)

func TestUpstreamHardDeadlinePreservesInternalExecutionBudget(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	ctx := WithUpstreamHardDeadline(parent)
	forward, constructionCleanup := detachUpstreamContext(ctx)
	streamForward, streamCleanup := detachStreamUpstreamContext(ctx, true)
	streamCleanup()
	require.Same(t, ctx, streamForward)
	constructionCleanup() // Existing forwarders release construction immediately.
	require.NoError(t, forward.Err())
	wantDeadline, _ := parent.Deadline()
	gotDeadline, ok := forward.Deadline()
	require.True(t, ok)
	require.Equal(t, wantDeadline, gotDeadline)
	attempt, release := pkghttputil.NewUpstreamAttemptContext(forward, time.Hour)
	defer release()
	select {
	case <-attempt.Done():
		require.ErrorIs(t, attempt.Err(), context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("internal attempt outlived the hard execution deadline")
	}
}

func TestUpstreamHardDeadlineDoesNotChangeOrdinaryDrain(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	forward, constructionCleanup := detachUpstreamContext(parent)
	constructionCleanup()
	attempt, release := pkghttputil.NewUpstreamAttemptContext(forward, time.Hour)
	defer release()
	cancel()
	require.NoError(t, attempt.Err(), "ordinary requests keep draining after client cancellation")
}
