package auth

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"cronApi/internal/errcode"
	"cronApi/internal/response"
)

const headerAuthorization = "Authorization"

// TokenParser 抽离 token 解析，避免 auth 包直接依赖业务服务。
// 由 internal/service/auth.Service 实现。
type TokenParser interface {
	ParseAccessToken(ctx context.Context, raw string) (*Identity, error)
}

// Optional 可选鉴权：带 token 就解析并注入身份，不带或不合法也放行。
func Optional(p TokenParser) gin.HandlerFunc {
	return func(c *gin.Context) {
		if raw := extractToken(c); raw != "" {
			if id, err := p.ParseAccessToken(c.Request.Context(), raw); err == nil {
				SetIdentity(c, id)
			}
		}
		c.Next()
	}
}

// Require 强制鉴权：登录态无效直接返回 401。
func Require(p TokenParser) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := extractToken(c)
		if raw == "" {
			response.Abort(c, errcode.Unauthorized)
			return
		}
		id, err := p.ParseAccessToken(c.Request.Context(), raw)
		if err != nil {
			response.Fail(c, err)
			return
		}
		SetIdentity(c, id)
		c.Next()
	}
}

// extractToken 支持 Authorization: Bearer xxx 与 ?token=xxx（调试用）。
func extractToken(c *gin.Context) string {
	h := c.GetHeader(headerAuthorization)
	if h != "" {
		if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
			return strings.TrimSpace(h[7:])
		}
		return strings.TrimSpace(h)
	}
	return c.Query("token")
}
