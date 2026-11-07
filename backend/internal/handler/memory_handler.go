package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// MemoryHandler is the JWT-authenticated account view of memory receipts.
type MemoryHandler struct{ gateway *service.MemoryGateway }

func NewMemoryHandler(gateway *service.MemoryGateway) *MemoryHandler {
	return &MemoryHandler{gateway: gateway}
}

type accountMemoryResponse struct {
	Configured           bool   `json:"configured"`
	State                string `json:"state"`
	SupportedEndpoint    string `json:"supported_endpoint,omitempty"`
	ArchiveMode          string `json:"archive_mode,omitempty"`
	RuntimeHealthChecked bool   `json:"runtime_health_checked"`
	service.MemoryArchiveOverview
}

// Overview never accepts a user ID from the request. The JWT subject is the
// same product account identity used by the gateway's API keys.
func (h *MemoryHandler) Overview(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	result := accountMemoryResponse{
		State:                 "unconfigured",
		MemoryArchiveOverview: service.MemoryArchiveOverview{Recent: []service.MemoryArchiveActivity{}},
	}
	if !h.gateway.EnabledFor(subject.UserID) {
		response.Success(c, result)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	_, err := h.gateway.ActiveGeneration(ctx, subject.UserID)
	result.Configured = true
	if errors.Is(err, service.ErrMemoryArchiveUserBlocked) {
		result.State = "suspended"
		response.Success(c, result)
		return
	}
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Memory status is unavailable")
		return
	}
	result.MemoryArchiveOverview, err = h.gateway.Overview(ctx, subject.UserID)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Memory activity is unavailable")
		return
	}
	result.State = "active"
	result.SupportedEndpoint = "responses_http_text"
	result.ArchiveMode = "source_diary"
	response.Success(c, result)
}
