package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPelicanRegisteredRoutesRequireExistingAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &handler.Handlers{Admin: &handler.AdminHandlers{}, Pelican: handler.NewPelicanHandler(nil)}
	r := gin.New()
	adminAuth := middleware.AdminAuthMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.AbortWithStatus(http.StatusForbidden)
	})
	jwtAuth := middleware.JWTAuthMiddleware(func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	audit := middleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(r.Group("/api/v1"), h, adminAuth, audit, middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() }), nil, nil)
	RegisterUserRoutes(r.Group("/api/v1"), h, jwtAuth, audit, nil, nil)
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/pelican"},
		{"GET", "/api/v1/pelican/4bd10e16-f1a1-49ab-8c56-20fd98c4fc41"},
		{"GET", "/api/v1/admin/pelican/config"},
		{"PUT", "/api/v1/admin/pelican/config"},
		{"GET", "/api/v1/admin/pelican/plans"},
		{"POST", "/api/v1/admin/pelican/plans"},
		{"PUT", "/api/v1/admin/pelican/plans/1"},
		{"DELETE", "/api/v1/admin/pelican/plans/1"},
		{"POST", "/api/v1/admin/pelican/plans/1/run"},
		{"GET", "/api/v1/admin/pelican/results"},
		{"GET", "/api/v1/admin/pelican/results/4bd10e16-f1a1-49ab-8c56-20fd98c4fc41"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		require.Equal(t, http.StatusUnauthorized, w.Code, tc.path)
		if len(tc.path) >= len("/api/v1/admin/") && tc.path[:len("/api/v1/admin/")] == "/api/v1/admin/" {
			w = httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Authorization", "Bearer regular-user")
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code, tc.path)
		}
	}
}
