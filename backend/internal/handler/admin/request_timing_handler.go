package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RequestTimingHandler struct{ service *service.RequestTimingService }

func NewRequestTimingHandler(s *service.RequestTimingService) *RequestTimingHandler {
	return &RequestTimingHandler{s}
}
func (h *RequestTimingHandler) List(c *gin.Context) {
	f := service.RequestTimingFilter{RequestID: strings.TrimSpace(c.Query("request_id")), Outcome: strings.TrimSpace(c.Query("outcome"))}
	if len(f.RequestID) > 64 {
		response.Error(c, http.StatusBadRequest, "invalid request_id")
		return
	}
	if raw := c.Query("account_id"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 1 {
			response.Error(c, 400, "invalid account_id")
			return
		}
		f.AccountID = v
	}
	if raw := c.Query("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > 100 {
			response.Error(c, 400, "limit must be 1..100")
			return
		}
		f.Limit = v
	}
	if raw := c.Query("before"); raw != "" {
		v, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			response.Error(c, 400, "invalid before")
			return
		}
		f.Before = v
	}
	traces, err := h.service.List(c.Request.Context(), f)
	if err != nil {
		response.Error(c, 500, "failed to query request timings")
		return
	}
	response.Success(c, gin.H{"items": traces, "stats": h.service.Stats(), "retention_days": 7})
}
func (h *RequestTimingHandler) Get(c *gin.Context) {
	id := c.Param("trace_id")
	if _, err := uuid.Parse(id); err != nil {
		response.Error(c, 400, "invalid trace_id")
		return
	}
	trace, err := h.service.Get(c.Request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		response.Error(c, 404, "request timing not found")
		return
	}
	if err != nil {
		response.Error(c, 500, "failed to query request timing")
		return
	}
	response.Success(c, trace)
}
