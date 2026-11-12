// Package handler 接入层：只做参数绑定、调用 service、返回统一响应。
package handler

import (
	"github.com/gin-gonic/gin"

	"cronApi/internal/auth"
	"cronApi/internal/errcode"
	"cronApi/internal/response"
	svc "cronApi/internal/service/auth"
)

var errInvalidID = errcode.New(errcode.InvalidParam).WithMessage("id 非法")

// AuthHandler 登录态相关接口。
type AuthHandler struct {
	svc *svc.Service
}

// NewAuthHandler 构造。
func NewAuthHandler(s *svc.Service) *AuthHandler { return &AuthHandler{svc: s} }

type loginReq struct {
	// Code 小程序 wx.login 返回的 code
	Code string `json:"code" binding:"required,max=256"`
}

// Login 小程序静默登录 POST /api/v1/auth/wechat/login
// @Summary 小程序登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.Wrap(errcode.BindError, err))
		return
	}
	tokens, err := h.svc.Login(c.Request.Context(), req.Code)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, tokens)
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Refresh 刷新 token POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.Wrap(errcode.BindError, err))
		return
	}
	tokens, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, tokens)
}

// Logout 注销当前会话 POST /api/v1/auth/logout（需登录）
func (h *AuthHandler) Logout(c *gin.Context) {
	id := auth.MustIdentity(c)
	if err := h.svc.Logout(c.Request.Context(), id.SessionID); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"logged_out": true})
}

// LogoutAll 注销用户全部会话 POST /api/v1/auth/logout-all（需登录）
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	n, err := h.svc.LogoutAll(c.Request.Context(), auth.MustUserID(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"revoked": n})
}

// Me 当前用户 GET /api/v1/auth/me（需登录）
func (h *AuthHandler) Me(c *gin.Context) {
	u, err := h.svc.Me(c.Request.Context(), auth.MustUserID(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, u)
}

type profileReq struct {
	Nickname  string `json:"nickname" binding:"max=32"`
	AvatarURL string `json:"avatar_url" binding:"max=512"`
}

// UpdateProfile 更新资料 PUT /api/v1/auth/profile（需登录）
func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	var req profileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.Wrap(errcode.BindError, err))
		return
	}
	u, err := h.svc.UpdateProfile(c.Request.Context(), auth.MustUserID(c), req.Nickname, req.AvatarURL)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, u)
}
