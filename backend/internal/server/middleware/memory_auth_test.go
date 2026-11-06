//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMemoryControlAuthenticationRemainsAvailableWithoutBalance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "/v1/sub2api/memory"},
		{http.MethodGet, "/v1/sub2api/memory/events/12c6b35c-1cae-411f-bb7f-6d306ba5cb5d"},
		{http.MethodPost, "/v1/sub2api/memory/suspend"},
	} {
		t.Run(endpoint.path, func(t *testing.T) {
			user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 0}
			group := &service.Group{ID: 42, Status: service.StatusActive, Platform: service.PlatformOpenAI, SubscriptionType: service.SubscriptionTypeSubscription}
			expired := time.Now().Add(-time.Hour)
			key := &service.APIKey{ID: 100, UserID: 7, Key: "memory-control-test", Status: service.StatusAPIKeyQuotaExhausted, User: user, Group: group, GroupID: &group.ID, Quota: 1, QuotaUsed: 1, ExpiresAt: &expired}
			touchCalls, subscriptionCalls := 0, 0
			repo := &stubApiKeyRepo{getByKey: func(context.Context, string) (*service.APIKey, error) { clone := *key; return &clone, nil }, updateLastUsed: func(context.Context, int64, time.Time) error { touchCalls++; return nil }}
			subs := &stubUserSubscriptionRepo{getActive: func(context.Context, int64, int64) (*service.UserSubscription, error) {
				subscriptionCalls++
				return nil, service.ErrSubscriptionNotFound
			}}
			cfg := &config.Config{RunMode: config.RunModeStandard}
			subService := service.NewSubscriptionService(nil, subs, nil, nil, cfg)
			t.Cleanup(subService.Stop)
			r := gin.New()
			r.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg), subService, cfg)))
			r.Handle(endpoint.method, endpoint.path, func(c *gin.Context) {
				require.Equal(t, int64(7), c.Request.Context().Value(ctxkey.UserID))
				c.Status(http.StatusOK)
			})
			request := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			request.Header.Set("x-api-key", key.Key)
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
			require.Zero(t, subscriptionCalls)
			require.Zero(t, touchCalls)
			user.Status = service.StatusDisabled
			response = httptest.NewRecorder()
			r.ServeHTTP(response, request)
			require.Equal(t, http.StatusUnauthorized, response.Code, "an inactive user must not pass authentication")
		})
	}
}
