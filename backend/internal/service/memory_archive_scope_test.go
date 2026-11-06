package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type memoryArchiveScopeRepository struct {
	MemoryArchiveRepository
	claimCalls int
	userIDs    []int64
}

func (r *memoryArchiveScopeRepository) Claim(_ context.Context, _ string, _ int, _ time.Duration, _ int, userIDs []int64) ([]MemoryArchiveEvent, error) {
	r.claimCalls++
	r.userIDs = append([]int64(nil), userIDs...)
	return nil, nil
}

func TestMemoryArchiveWorkerRequiresExplicitTenantScope(t *testing.T) {
	for _, ids := range [][]int64{nil, {}, {0}, {-1}, {1, 0}, {1, -2}} {
		t.Run("invalid_scope", func(t *testing.T) {
			repo := &memoryArchiveScopeRepository{}
			worker := NewMemoryArchiveWorker(repo, &recordingMemoryProvider{}, MemoryArchiveWorkerOptions{UserIDs: ids})
			t.Cleanup(worker.Stop)
			require.Error(t, worker.RunOnce(context.Background()))
			require.Zero(t, repo.claimCalls, "invalid scope must never reach the shared queue")
		})
	}
}

func TestMemoryArchiveWorkerOwnsDeduplicatedTenantScope(t *testing.T) {
	repo := &memoryArchiveScopeRepository{}
	ids := []int64{7, 7, 9}
	worker := NewMemoryArchiveWorker(repo, &recordingMemoryProvider{}, MemoryArchiveWorkerOptions{UserIDs: ids})
	t.Cleanup(worker.Stop)
	ids[0], ids[1], ids[2] = 100, 101, 102
	require.NoError(t, worker.RunOnce(context.Background()))
	require.Equal(t, 1, repo.claimCalls)
	require.Equal(t, []int64{7, 9}, repo.userIDs, "caller mutation must not grant admission to another tenant")
}
