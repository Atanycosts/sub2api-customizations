//go:build memorycomponent && !integration

package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMemoryOverviewPostgresScopesCountsAndRecentReceipts(t *testing.T) {
	db := memoryComponentDB(t)
	repo := NewMemoryArchiveRepository(db)
	ctx := context.Background()
	for i := 0; i < 23; i++ {
		_, err := repo.Enqueue(ctx, memoryTestTurn(1, fmt.Sprintf("own-%02d", i)))
		require.NoError(t, err)
	}
	_, err := repo.Enqueue(ctx, memoryTestTurn(2, "other-account"))
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE memory_archive_queue SET state = CASE event_id
		WHEN 'own-00' THEN 'completed' WHEN 'own-01' THEN 'failed'
		WHEN 'own-02' THEN 'needs_reconciliation' WHEN 'own-03' THEN 'cancelled'
		ELSE 'pending' END WHERE user_id = 1`)
	require.NoError(t, err)
	data, err := repo.Overview(ctx, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, data.Archived)
	require.EqualValues(t, 19, data.Pending)
	require.EqualValues(t, 2, data.Attention)
	require.Len(t, data.Recent, 20)
	require.Equal(t, "own-22", data.Recent[0].EventID)
	require.Equal(t, "own-03", data.Recent[19].EventID)
	other, err := repo.Overview(ctx, 2)
	require.NoError(t, err)
	require.EqualValues(t, 1, other.Pending)
	require.Len(t, other.Recent, 1)
	require.Equal(t, "other-account", other.Recent[0].EventID)
	_, err = repo.Overview(ctx, 0)
	require.ErrorIs(t, err, service.ErrMemoryUserRequired)
}
