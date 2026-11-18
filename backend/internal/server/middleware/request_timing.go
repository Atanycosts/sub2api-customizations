package middleware

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// RequestTiming must run before gateway authentication and admission. Only the
// supported HTTP Chat/Responses routes are collected; WS turns need their own clock.
func RequestTiming(s *service.RequestTimingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if c.Request.Method != "POST" || (!strings.HasSuffix(path, "/chat/completions") && !strings.HasSuffix(path, "/responses") && !strings.HasSuffix(path, "/responses/compact")) {
			c.Next()
			return
		}
		requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string)
		canonicalPath := "/v1/responses"
		if strings.HasSuffix(path, "/chat/completions") {
			canonicalPath = "/v1/chat/completions"
		} else if strings.HasSuffix(path, "/responses/compact") {
			canonicalPath = "/v1/responses/compact"
		}
		ctx, collector := requesttiming.New(c.Request.Context(), requestID, canonicalPath)
		collector.SetUsageSink(s.AttachUsage)
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Gateway-Trace-ID", requesttiming.TraceID(ctx))
		c.Next()
		if key, ok := GetAPIKeyFromContext(c); ok {
			collector.Identity(key.UserID, key.ID)
		}
		s.Submit(collector.Finish(c.Writer.Status(), c.Request.Context().Err() != nil))
	}
}
