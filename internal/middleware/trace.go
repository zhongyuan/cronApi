package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"cronApi/internal/logger"
)

const (
	// HeaderTraceID 客户端可透传的链路 ID 头
	HeaderTraceID = "X-Trace-Id"
	// CtxTraceID gin.Context 中的 key
	CtxTraceID = "trace_id"
)

// Trace 为每个请求生成/透传 trace_id，写入 context、响应头与日志字段。
func Trace() gin.HandlerFunc {
	return func(c *gin.Context) {
		tid := c.GetHeader(HeaderTraceID)
		if tid == "" || len(tid) > 64 {
			tid = newTraceID()
		}
		ctx := logger.WithTraceID(c.Request.Context(), tid)
		c.Request = c.Request.WithContext(ctx)
		c.Set(CtxTraceID, tid)
		c.Header(HeaderTraceID, tid)
		c.Next()
	}
}

// AccessLog 访问日志：方法、路径、状态、耗时、IP、trace_id。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		lg := logger.WithContext(c.Request.Context()).With(
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.ClientIP()),
		)
		if len(c.Errors) > 0 {
			lg = lg.With(zap.String("errors", c.Errors.String()))
		}

		switch {
		case c.Writer.Status() >= 500:
			lg.Error("access")
		case c.Writer.Status() >= 400:
			lg.Warn("access")
		default:
			lg.Info("access")
		}
	}
}

// TraceID 返回当前请求的 trace_id。
func TraceID(c *gin.Context) string {
	if v, ok := c.Get(CtxTraceID); ok {
		return v.(string)
	}
	return ""
}

func newTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return time.Now().Format("20060102150405.000000")
	}
	return hex.EncodeToString(b)
}
