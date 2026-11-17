package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPelicanPublicBoundaryRejectsUnauthenticatedAndMalformedReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, path string
		auth       bool
		status     int
	}{
		{"authentication", "/pelican", false, 401},
		{"negative page", "/pelican?page=-1", true, 400},
		{"unbounded page", "/pelican?page=10001", true, 400},
		{"unbounded page size", "/pelican?page_size=49", true, 400},
		{"invalid group", "/pelican?group_id=private", true, 400},
		{"invalid effort", "/pelican?effort=turbo", true, 400},
		{"invalid state", "/pelican?state=deleted", true, 400},
		{"invalid detail", "/pelican/not-a-uuid", true, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			group := r.Group("")
			if tc.auth {
				group.Use(func(c *gin.Context) {
					c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 2})
					c.Next()
				})
			}
			NewPelicanHandler(nil).RegisterUserRoutes(group)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
		})
	}
}

func TestPelicanAdminBodyLimitPreventsOversizedPlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewPelicanHandler(nil).RegisterAdminRoutes(r.Group("/admin"))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/pelican/plans", strings.NewReader(`{"model":"`+strings.Repeat("x", 9<<10)+`"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
