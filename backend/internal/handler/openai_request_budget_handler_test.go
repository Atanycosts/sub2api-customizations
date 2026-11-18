//go:build unit

package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandleOpenAIRequestBudgetExceededUncommitted(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	budgetErr := &service.OpenAIRequestBudgetError{Phase: service.OpenAIRequestBudgetPreOutput}

	handled := h.handleOpenAIRequestBudgetExceeded(c, errors.New("ordinary failure"), false, false)
	require.False(t, handled)

	handled = h.handleOpenAIRequestBudgetExceeded(c, fmt.Errorf("wrapped: %w", budgetErr), false, false)
	require.True(t, handled)
	require.Equal(t, http.StatusGatewayTimeout, c.Writer.Status())
	require.Contains(t, recorder.Body.String(), "Execution state is unknown")
	require.NotContains(t, recorder.Body.String(), "response.failed", "an uncommitted response must use JSON HTTP 504")
}

func TestHandleOpenAIRequestBudgetExceededCommittedWritesOneSSETerminal(t *testing.T) {
	h := &OpenAIGatewayHandler{}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	_, err := c.Writer.Write([]byte(": keepalive\n\n"))
	require.NoError(t, err)
	budgetErr := &service.OpenAIRequestBudgetError{Phase: service.OpenAIRequestBudgetUploadTimeout}

	handled := h.handleOpenAIRequestBudgetExceeded(c, budgetErr, true, false)

	require.True(t, handled)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event:"), "budget failure must have one terminal event")
	require.Contains(t, recorder.Body.String(), "response.failed")
	require.NotContains(t, recorder.Body.String(), "event: error", "Responses requires a protocol terminal event")
}
