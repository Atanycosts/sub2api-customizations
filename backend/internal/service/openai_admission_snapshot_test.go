//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIAdmissionSnapshotCache struct {
	SchedulerCache
	accounts []Account
	hit      bool
	written  []Account
}

func (c *openAIAdmissionSnapshotCache) GetSnapshotValues(context.Context, SchedulerBucket) ([]Account, bool, error) {
	return append([]Account(nil), c.accounts...), c.hit, nil
}

func (c *openAIAdmissionSnapshotCache) CaptureBucketWriteToken(_ context.Context, bucket SchedulerBucket) (SchedulerBucketWriteToken, error) {
	return SchedulerBucketWriteToken{Bucket: bucket, Epoch: 1}, nil
}

func (c *openAIAdmissionSnapshotCache) SetSnapshot(_ context.Context, _ SchedulerBucket, _ SchedulerBucketWriteToken, accounts []Account) error {
	c.written = append([]Account(nil), accounts...)
	return nil
}

type openAIAdmissionFallbackRepo struct {
	AccountRepository
	accounts []Account
	calls    int
}

func (r *openAIAdmissionFallbackRepo) ListModelAvailabilityCandidates(context.Context, *int64, []string, bool) ([]Account, error) {
	r.calls++
	return append([]Account(nil), r.accounts...), nil
}

func (r *openAIAdmissionFallbackRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			account := r.accounts[i]
			return &account, nil
		}
	}
	return nil, ErrAccountNotFound
}

func TestOpenAIAdmissionSnapshotMixedTemporaryExclusionsDoNotBecome429(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	retryAt := time.Now().Add(45 * time.Second)
	overloadUntil := time.Now().Add(time.Minute)
	tempUnschedulableUntil := time.Now().Add(30 * time.Second)
	expiredAt := time.Now().Add(-time.Minute)
	accounts := []Account{
		{
			ID:               38151,
			Platform:         PlatformOpenAI,
			Type:             AccountTypeAPIKey,
			Status:           StatusActive,
			Schedulable:      true,
			Concurrency:      1,
			RateLimitResetAt: &retryAt,
		},
		{
			ID:            38152,
			Platform:      PlatformOpenAI,
			Type:          AccountTypeAPIKey,
			Status:        StatusActive,
			Schedulable:   true,
			Concurrency:   1,
			OverloadUntil: &overloadUntil,
		},
		{
			ID:                     38153,
			Platform:               PlatformOpenAI,
			Type:                   AccountTypeAPIKey,
			Status:                 StatusActive,
			Schedulable:            true,
			Concurrency:            1,
			TempUnschedulableUntil: &tempUnschedulableUntil,
		},
		{
			ID:          38154,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: false,
			Concurrency: 1,
		},
		{
			ID:                 38155,
			Platform:           PlatformOpenAI,
			Type:               AccountTypeAPIKey,
			Status:             StatusActive,
			Schedulable:        true,
			Concurrency:        1,
			AutoPauseOnExpired: true,
			ExpiresAt:          &expiredAt,
		},
	}

	tests := []struct {
		name          string
		cache         *openAIAdmissionSnapshotCache
		useSnapshot   bool
		wantRepoCalls int
	}{
		{
			name:        "cache hit",
			cache:       &openAIAdmissionSnapshotCache{accounts: accounts, hit: true},
			useSnapshot: true,
		},
		{
			name:          "snapshot fallback",
			cache:         &openAIAdmissionSnapshotCache{},
			useSnapshot:   true,
			wantRepoCalls: 1,
		},
		{
			name:          "selector fallback without snapshot",
			cache:         &openAIAdmissionSnapshotCache{},
			wantRepoCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			repo := &openAIAdmissionFallbackRepo{accounts: accounts}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			cfg.Gateway.Scheduling.DbFallbackEnabled = true
			svc := &OpenAIGatewayService{
				accountRepo:        repo,
				cache:              &schedulerTestGatewayCache{},
				cfg:                cfg,
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
			}
			if tc.useSnapshot {
				svc.schedulerSnapshot = NewSchedulerSnapshotService(tc.cache, nil, repo, nil, cfg)
			}

			selection, _, err := svc.SelectAccountWithScheduler(
				context.Background(), nil, "", "", "deepseek-v4-pro", nil, OpenAIUpstreamTransportAny, false,
			)

			require.Nil(t, selection)
			require.ErrorIs(t, err, ErrNoAvailableAccounts)
			var rateLimited *OpenAIRateLimitedAdmissionError
			require.False(t, errors.As(err, &rateLimited), "overload and temporary failures must keep the 503 classification")
			require.Contains(t, err.Error(), "not_schedulable=3")
			require.Equal(t, tc.wantRepoCalls, repo.calls)
			if tc.useSnapshot && tc.wantRepoCalls == 1 {
				require.Len(t, tc.cache.written, len(accounts), "fallback must publish the configured pool once")
			}
		})
	}
}

func TestOpenAIAdmissionSnapshotExclusiveTemporary429AcrossCacheAndFallback(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	retryAt := time.Now().Add(45 * time.Second)
	accounts := []Account{
		{
			ID:               38251,
			Platform:         PlatformOpenAI,
			Type:             AccountTypeAPIKey,
			Status:           StatusActive,
			Schedulable:      true,
			Concurrency:      1,
			RateLimitResetAt: &retryAt,
		},
	}

	tests := []struct {
		name          string
		cache         *openAIAdmissionSnapshotCache
		useSnapshot   bool
		wantRepoCalls int
	}{
		{
			name:        "cache hit",
			cache:       &openAIAdmissionSnapshotCache{accounts: accounts, hit: true},
			useSnapshot: true,
		},
		{
			name:          "snapshot fallback",
			cache:         &openAIAdmissionSnapshotCache{},
			useSnapshot:   true,
			wantRepoCalls: 1,
		},
		{
			name:          "selector fallback without snapshot",
			cache:         &openAIAdmissionSnapshotCache{},
			wantRepoCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			repo := &openAIAdmissionFallbackRepo{accounts: accounts}
			cfg := &config.Config{RunMode: config.RunModeSimple}
			cfg.Gateway.Scheduling.DbFallbackEnabled = true
			svc := &OpenAIGatewayService{
				accountRepo:        repo,
				cache:              &schedulerTestGatewayCache{},
				cfg:                cfg,
				rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
			}
			if tc.useSnapshot {
				svc.schedulerSnapshot = NewSchedulerSnapshotService(tc.cache, nil, repo, nil, cfg)
			}

			selection, _, err := svc.SelectAccountWithScheduler(
				context.Background(), nil, "", "", "deepseek-v4-pro", nil, OpenAIUpstreamTransportAny, false,
			)

			require.Nil(t, selection)
			require.ErrorIs(t, err, ErrNoAvailableAccounts)
			var rateLimited *OpenAIRateLimitedAdmissionError
			require.ErrorAs(t, err, &rateLimited)
			require.Equal(t, 1, rateLimited.TemporaryRateLimited)
			require.Equal(t, map[string]int{"not_schedulable": 1}, rateLimited.ReasonCounts)
			require.WithinDuration(t, retryAt, rateLimited.EarliestRetryAt, time.Millisecond)
			require.Equal(t, tc.wantRepoCalls, repo.calls)
		})
	}
}
