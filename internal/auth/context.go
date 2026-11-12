// Package auth 集中管理登录态相关的 context key 与鉴权中间件。
package auth

import (
	"github.com/gin-gonic/gin"
)

const (
	ctxKeyClaims  = "auth.claims"
	ctxKeySession = "auth.session"
)

// Identity 当前登录身份，登录态校验通过后写入 gin.Context。
type Identity struct {
	UserID    int64  `json:"user_id"`
	OpenID    string `json:"open_id"`
	UnionID   string `json:"union_id,omitempty"`
	SessionID string `json:"session_id"`
	Nickname  string `json:"nickname,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// SetIdentity 写入当前请求身份。
func SetIdentity(c *gin.Context, id *Identity) { c.Set(ctxKeySession, id) }

// GetIdentity 读取当前请求身份；未登录返回 nil。
func GetIdentity(c *gin.Context) *Identity {
	if v, ok := c.Get(ctxKeySession); ok {
		if id, ok2 := v.(*Identity); ok2 {
			return id
		}
	}
	return nil
}

// MustIdentity 在已挂鉴权中间件的路由中读取身份，未登录直接 panic（属于编码错误）。
func MustIdentity(c *gin.Context) *Identity {
	id := GetIdentity(c)
	if id == nil {
		panic("auth: MustIdentity called on unauthenticated route")
	}
	return id
}

// MustUserID 便捷读取当前用户 ID。
func MustUserID(c *gin.Context) int64 { return MustIdentity(c).UserID }
