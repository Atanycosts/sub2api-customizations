package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type GatewayOperationsHandler struct {
	operations *service.GatewayOperationsService
}

func NewGatewayOperationsHandler(operations *service.GatewayOperationsService) *GatewayOperationsHandler {
	return &GatewayOperationsHandler{operations: operations}
}

// RegisterRoutes must receive the existing authenticated admin route group.
func (h *GatewayOperationsHandler) RegisterRoutes(admin *gin.RouterGroup) {
	routes := admin.Group("/gateway-operations")
	routes.Use(func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 96<<10)
		}
		c.Next()
	})
	routes.GET("/config", h.config)
	routes.PUT("/config", h.saveConfig)
	routes.GET("/wallets", h.wallets)
	routes.POST("/wallets", h.saveWallet)
	routes.PUT("/wallets/:id", h.saveWallet)
	routes.DELETE("/wallets/:id", h.deleteWallet)
	routes.POST("/wallets/:id/refresh", h.refreshWallet)
	routes.GET("/experiments", h.experiments)
	routes.POST("/experiments", h.runExperiment)
	routes.POST("/cache-usage", h.cacheUsage)
}
func gatewayOperationsPage(c *gin.Context) (int, int, bool) {
	limit := 50
	offset := 0
	var err error
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			response.BadRequest(c, "invalid limit")
			return 0, 0, false
		}
	}
	if raw := c.Query("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil {
			response.BadRequest(c, "invalid offset")
			return 0, 0, false
		}
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		response.BadRequest(c, "limit must be 1–100; offset 0–100000")
		return 0, 0, false
	}
	return limit, offset, true
}
func gatewayOperationsID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid wallet ID")
		return 0, false
	}
	return id, true
}
func gatewayOperationsResult(c *gin.Context, data any, err error) {
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, data)
}
func (h *GatewayOperationsHandler) config(c *gin.Context) {
	v, e := h.operations.Config(c.Request.Context())
	gatewayOperationsResult(c, v, e)
}
func (h *GatewayOperationsHandler) saveConfig(c *gin.Context) {
	var in service.GatewayOperationsConfig
	if c.ShouldBindJSON(&in) != nil {
		response.BadRequest(c, "invalid operations config")
		return
	}
	gatewayOperationsResult(c, in, h.operations.SaveConfig(c.Request.Context(), in))
}
func (h *GatewayOperationsHandler) wallets(c *gin.Context) {
	l, o, ok := gatewayOperationsPage(c)
	if !ok {
		return
	}
	v, e := h.operations.Wallets(c.Request.Context(), l, o)
	gatewayOperationsResult(c, v, e)
}
func (h *GatewayOperationsHandler) saveWallet(c *gin.Context) {
	var id int64
	if c.Param("id") != "" {
		var ok bool
		id, ok = gatewayOperationsID(c)
		if !ok {
			return
		}
	}
	var in service.GatewayWalletInput
	if c.ShouldBindJSON(&in) != nil {
		response.BadRequest(c, "invalid wallet input")
		return
	}
	v, e := h.operations.SaveWallet(c.Request.Context(), id, in)
	gatewayOperationsResult(c, v, e)
}
func (h *GatewayOperationsHandler) deleteWallet(c *gin.Context) {
	id, ok := gatewayOperationsID(c)
	if !ok {
		return
	}
	gatewayOperationsResult(c, gin.H{"deleted": true}, h.operations.DeleteWallet(c.Request.Context(), id))
}
func (h *GatewayOperationsHandler) refreshWallet(c *gin.Context) {
	id, ok := gatewayOperationsID(c)
	if !ok {
		return
	}
	v, e := h.operations.RefreshWallet(c.Request.Context(), id)
	gatewayOperationsResult(c, v, e)
}
func (h *GatewayOperationsHandler) experiments(c *gin.Context) {
	l, o, ok := gatewayOperationsPage(c)
	if !ok {
		return
	}
	v, e := h.operations.Experiments(c.Request.Context(), l, o)
	gatewayOperationsResult(c, v, e)
}
func (h *GatewayOperationsHandler) runExperiment(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "user authentication required")
		return
	}
	var in service.GatewayExperimentInput
	if c.ShouldBindJSON(&in) != nil {
		response.BadRequest(c, "invalid experiment input")
		return
	}
	v, e := h.operations.RunExperiment(c.Request.Context(), subject.UserID, in)
	gatewayOperationsResult(c, v, e)
}
func (h *GatewayOperationsHandler) cacheUsage(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "user authentication required")
		return
	}
	var in struct {
		KeyID      int64    `json:"key_id"`
		RequestIDs []string `json:"request_ids"`
	}
	if c.ShouldBindJSON(&in) != nil {
		response.BadRequest(c, "invalid correlation input")
		return
	}
	v, e := h.operations.CacheUsage(c.Request.Context(), subject.UserID, in.KeyID, in.RequestIDs)
	gatewayOperationsResult(c, v, e)
}
