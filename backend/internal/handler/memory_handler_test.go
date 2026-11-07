package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type accountMemoryRepository struct {
	service.MemoryArchiveRepository
	users    []int64
	blocked  bool
	err      error
	overview service.MemoryArchiveOverview
}

func (r *accountMemoryRepository) ActiveGeneration(_ context.Context, userID int64) (int64, error) {
	r.users = append(r.users, userID)
	if r.blocked {
		return 0, service.ErrMemoryArchiveUserBlocked
	}
	return 1, r.err
}

func (r *accountMemoryRepository) Overview(_ context.Context, userID int64) (service.MemoryArchiveOverview, error) {
	r.users = append(r.users, userID)
	return r.overview, r.err
}

func accountMemoryRequest(t *testing.T, h *MemoryHandler, userID int64) (*httptest.ResponseRecorder, accountMemoryResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/memory?user_id=999", nil)
	if userID > 0 {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID})
	}
	h.Overview(c)
	var envelope struct {
		Data accountMemoryResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return rec, envelope.Data
}

func TestAccountMemoryUsesJWTSubjectAndExposesOnlyReceipts(t *testing.T) {
	repo := &accountMemoryRepository{overview: service.MemoryArchiveOverview{
		Archived: 8, Pending: 2, Attention: 1,
		Recent: []service.MemoryArchiveActivity{{EventID: "own-event", State: service.MemoryArchiveCompleted}},
	}}
	h := NewMemoryHandler(service.NewMemoryGateway(nil, repo, service.MemoryGatewayOptions{UserIDs: []int64{7}}))
	rec, data := accountMemoryRequest(t, h, 7)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []int64{7, 7}, repo.users, "request parameters must not select another account")
	require.Equal(t, "active", data.State)
	require.False(t, data.RuntimeHealthChecked)
	require.Equal(t, "responses_http_text", data.SupportedEndpoint)
	require.Equal(t, "source_diary", data.ArchiveMode)
	require.EqualValues(t, 8, data.Archived)
	require.Equal(t, "own-event", data.Recent[0].EventID)
	require.NotContains(t, rec.Body.String(), "payload")
}

func TestAccountMemoryUnboundAndSuspendedDoNotReadActivity(t *testing.T) {
	repo := &accountMemoryRepository{}
	h := NewMemoryHandler(service.NewMemoryGateway(nil, repo, service.MemoryGatewayOptions{UserIDs: []int64{7}}))
	rec, data := accountMemoryRequest(t, h, 8)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "unconfigured", data.State)
	require.False(t, data.Configured)
	require.Empty(t, repo.users)
	require.NotNil(t, data.Recent)
	repo.blocked = true
	rec, data = accountMemoryRequest(t, h, 7)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "suspended", data.State)
	require.True(t, data.Configured)
	require.Equal(t, []int64{7}, repo.users)
	require.Empty(t, data.Recent)
}

func TestAccountMemoryRequiresAuthenticationAndDoesNotLeakErrors(t *testing.T) {
	repo := &accountMemoryRepository{err: errors.New("private provider path /srv/private and credentials")}
	h := NewMemoryHandler(service.NewMemoryGateway(nil, repo, service.MemoryGatewayOptions{UserIDs: []int64{7}}))
	rec, _ := accountMemoryRequest(t, h, 0)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Empty(t, repo.users)
	rec, _ = accountMemoryRequest(t, h, 7)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.NotContains(t, rec.Body.String(), "/srv/private")
	require.NotContains(t, rec.Body.String(), "credentials")
}
