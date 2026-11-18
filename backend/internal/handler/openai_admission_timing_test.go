//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestOpenAIAccountSlotAcquisitionMarksImmediateAdmissionComplete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	traceCtx, collector := requesttiming.New(c.Request.Context(), "request", "/v1/chat/completions")
	c.Request = c.Request.WithContext(traceCtx)
	streamStarted := false
	releaseCalls := 0
	selection := &service.AccountSelectionResult{
		Account: &service.Account{
			ID:          101,
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Concurrency: 1,
		},
		Acquired:    true,
		ReleaseFunc: func() { releaseCalls++ },
	}
	h := &OpenAIGatewayHandler{gatewayService: &service.OpenAIGatewayService{}}

	release, result := h.acquireOpenAIAccountSlot(
		c,
		nil,
		"",
		selection,
		false,
		&streamStarted,
		zap.NewNop(),
		nil,
	)

	require.Equal(t, openAISlotAcquireOK, result)
	require.NotNil(t, release)
	release()
	require.Equal(t, 1, releaseCalls)
	trace := collector.Finish(http.StatusOK, false)
	require.True(t, trace.AdmissionComplete)
	require.Empty(t, trace.Admission, "an immediate slot acquisition must not be reported as queueing")
}
