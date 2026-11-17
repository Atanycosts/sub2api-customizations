package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PelicanHandler exposes shared observations to users and keeps test controls
// behind the existing administrator authentication boundary.
type PelicanHandler struct{ pelican *service.PelicanService }

func NewPelicanHandler(pelican *service.PelicanService) *PelicanHandler {
	return &PelicanHandler{pelican: pelican}
}

func (h *PelicanHandler) RegisterUserRoutes(authenticated *gin.RouterGroup) {
	routes := authenticated.Group("/pelican")
	routes.Use(pelicanPrivateResponse)
	routes.GET("", h.publicResults)
	routes.GET("/:id", h.publicResult)
}

func (h *PelicanHandler) RegisterAdminRoutes(admin *gin.RouterGroup) {
	routes := admin.Group("/pelican")
	routes.Use(pelicanPrivateResponse)
	routes.Use(func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
		}
		c.Next()
	})
	routes.GET("/config", h.config)
	routes.PUT("/config", h.saveConfig)
	routes.GET("/plans", h.plans)
	routes.POST("/plans", h.savePlan)
	routes.PUT("/plans/:id", h.savePlan)
	routes.DELETE("/plans/:id", h.deletePlan)
	routes.POST("/plans/:id/run", h.runPlan)
	routes.GET("/results", h.results)
	routes.GET("/results/:id", h.result)
}

func pelicanPrivateResponse(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Next()
}

func pelicanSubject(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "user authentication required")
		return 0, false
	}
	return subject.UserID, true
}

func pelicanFilter(c *gin.Context, admin bool) (service.PelicanFilter, bool) {
	f := service.PelicanFilter{Page: 1, PageSize: 12}
	for name, dest := range map[string]*int{"page": &f.Page, "page_size": &f.PageSize} {
		if raw, supplied := c.GetQuery(name); supplied {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || (name == "page_size" && value > 48) || value > 10000 {
				response.BadRequest(c, "page must be 1–10000 and page_size 1–48")
				return f, false
			}
			*dest = value
		}
	}
	ids := map[string]*int64{"group_id": &f.GroupID}
	if admin {
		ids["plan_id"] = &f.PlanID
	}
	for name, dest := range ids {
		if raw, supplied := c.GetQuery(name); supplied {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value < 1 {
				response.BadRequest(c, "invalid "+name)
				return f, false
			}
			*dest = value
		}
	}
	f.Model = strings.TrimSpace(c.Query("model"))
	if len(f.Model) > 200 {
		response.BadRequest(c, "model must be at most 200 bytes")
		return f, false
	}
	f.Effort = c.Query("effort")
	switch f.Effort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		response.BadRequest(c, "unsupported reasoning effort")
		return f, false
	}
	f.State = c.Query("state")
	switch f.State {
	case "", "completed", "failed", "unknown", "running":
	default:
		response.BadRequest(c, "invalid result state")
		return f, false
	}
	return f, true
}

func pelicanPlanID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid test plan ID")
		return 0, false
	}
	return id, true
}

func pelicanRunID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		response.BadRequest(c, "invalid test result ID")
		return "", false
	}
	return id, true
}

func pelicanResponse(c *gin.Context, value any, err error) {
	if !response.ErrorFrom(c, err) {
		response.Success(c, value)
	}
}

func (h *PelicanHandler) publicResults(c *gin.Context) {
	userID, ok := pelicanSubject(c)
	if !ok {
		return
	}
	f, ok := pelicanFilter(c, false)
	if !ok {
		return
	}
	page, err := h.pelican.PublicResults(c.Request.Context(), userID, f)
	pelicanResponse(c, page, err)
}

func (h *PelicanHandler) publicResult(c *gin.Context) {
	userID, ok := pelicanSubject(c)
	if !ok {
		return
	}
	id, ok := pelicanRunID(c)
	if !ok {
		return
	}
	run, err := h.pelican.PublicResult(c.Request.Context(), userID, id)
	pelicanResponse(c, run, err)
}

func (h *PelicanHandler) config(c *gin.Context) {
	config, err := h.pelican.Config(c.Request.Context())
	pelicanResponse(c, config, err)
}

func (h *PelicanHandler) saveConfig(c *gin.Context) {
	var config service.PelicanConfig
	if c.ShouldBindJSON(&config) != nil {
		response.BadRequest(c, "invalid pelican config")
		return
	}
	pelicanResponse(c, config, h.pelican.SaveConfig(c.Request.Context(), config))
}

func (h *PelicanHandler) plans(c *gin.Context) {
	plans, err := h.pelican.Plans(c.Request.Context())
	pelicanResponse(c, plans, err)
}

func (h *PelicanHandler) savePlan(c *gin.Context) {
	var id int64
	if c.Param("id") != "" {
		var ok bool
		id, ok = pelicanPlanID(c)
		if !ok {
			return
		}
	}
	var in service.PelicanPlanInput
	if c.ShouldBindJSON(&in) != nil {
		response.BadRequest(c, "invalid test plan")
		return
	}
	plan, err := h.pelican.SavePlan(c.Request.Context(), id, in)
	pelicanResponse(c, plan, err)
}

func (h *PelicanHandler) deletePlan(c *gin.Context) {
	id, ok := pelicanPlanID(c)
	if !ok {
		return
	}
	pelicanResponse(c, gin.H{"deleted": true}, h.pelican.DeletePlan(c.Request.Context(), id))
}

func (h *PelicanHandler) runPlan(c *gin.Context) {
	userID, ok := pelicanSubject(c)
	if !ok {
		return
	}
	id, ok := pelicanPlanID(c)
	if !ok {
		return
	}
	var in struct {
		SubmitID string `json:"submit_id"`
	}
	if c.ShouldBindJSON(&in) != nil {
		response.BadRequest(c, "submit_id UUID required")
		return
	}
	run, err := h.pelican.RunPlan(c.Request.Context(), userID, id, in.SubmitID)
	pelicanResponse(c, run, err)
}

func (h *PelicanHandler) results(c *gin.Context) {
	f, ok := pelicanFilter(c, true)
	if !ok {
		return
	}
	page, err := h.pelican.Results(c.Request.Context(), f)
	pelicanResponse(c, page, err)
}

func (h *PelicanHandler) result(c *gin.Context) {
	id, ok := pelicanRunID(c)
	if !ok {
		return
	}
	run, err := h.pelican.Result(c.Request.Context(), id)
	pelicanResponse(c, run, err)
}
