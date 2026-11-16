package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayOperationsRejectsOversizedWalletBeforeService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewGatewayOperationsHandler(nil).RegisterRoutes(router.Group("/admin"))
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/gateway-operations/wallets", strings.NewReader(`{"credential_value":"`+strings.Repeat("x", 100<<10)+`"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "invalid wallet input")
}
