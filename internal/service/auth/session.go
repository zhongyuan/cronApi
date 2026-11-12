// Package auth 登录态服务：小程序 code 换 openid → 建服务端会话 → 签发 JWT。
//
// 设计要点（小程序登录态常见坑）：
//  1. wx code 一次性、5 分钟有效，必须服务端换取 openid，不能复用前端传的 openid；
//  2. session_key 只存服务端（Redis），绝不能下发到前端；
//  3. 同时下发 access_token（短期 JWT）与 refresh_token，前端静默续期；
//  4. 引入 session_id 并在服务端存储，可支持「踢下线 / 改密失效 / 多端登录」；
//  5. 刷新时轮换 refresh_token（一次性使用），防止重放。
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"cronApi/internal/auth"
	"cronApi/internal/config"
	"cronApi/internal/errcode"
	"cronApi/internal/logger"
	"cronApi/internal/model"
	"cronApi/internal/repository"
	"cronApi/internal/service/wechat"
	"cronApi/pkg/cache"
	"cronApi/pkg/jwtutil"

	"go.uber.org/zap"
)

const (
	sessionKeyPrefix  = "session:"
	refreshUsedPrefix = "refresh:used:"
)

// Session 服务端会话（存 Redis，TTL = refresh_ttl）。
type Session struct {
	SessionID  string    `json:"session_id"`
	UserID     int64     `json:"user_id"`
	OpenID     string    `json:"open_id"`
	UnionID    string    `json:"union_id,omitempty"`
	SessionKey string    `json:"session_key"` // 敏感，仅服务端持有
	AppID      string    `json:"app_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	RenewedAt  time.Time `json:"renewed_at"`
}

// Tokens 登录结果。
type Tokens struct {
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
	TokenType    string     `json:"token_type"`
	ExpiresIn    int64      `json:"expires_in"` // access_token 有效期（秒）
	User         *UserBrief `json:"user"`
}

// UserBrief 用户简要信息。
type UserBrief struct {
	ID        int64  `json:"id"`
	OpenID    string `json:"open_id"`
	Nickname  string `json:"nickname,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	IsNew     bool   `json:"is_new"`
}

// Service 登录态服务。
type Service struct {
	cfg   *config.Config
	repos *repository.Repos
	wx    wechat.Client
	sess  cache.Cache
	jwt   *jwtutil.Manager
}

// NewService 构造。
func NewService(cfg *config.Config, repos *repository.Repos, wx wechat.Client, sess cache.Cache, jwtMgr *jwtutil.Manager) *Service {
	return &Service{cfg: cfg, repos: repos, wx: wx, sess: sess, jwt: jwtMgr}
}

// Login 小程序登录：code → wx session → 查/建用户 → 会话 → token。
func (s *Service) Login(ctx context.Context, code string) (*Tokens, error) {
	if code == "" {
		return nil, errcode.New(errcode.InvalidParam).WithMessage("code 不能为空")
	}
	info, err := s.wx.Code2Session(ctx, code)
	if err != nil {
		var wxErr *wechat.Error
		if errors.As(err, &wxErr) {
			switch wxErr.Errcode {
			case 40029, 40163, 45011:
				return nil, errcode.Wrapf(errcode.WeChatCodeBad, err, "微信登录凭证无效或已使用(%d)", wxErr.Errcode)
			}
			return nil, errcode.Wrap(errcode.WeChatError, err)
		}
		return nil, errcode.Wrap(errcode.WeChatError, err)
	}

	user, isNew, err := s.upsertUser(ctx, info)
	if err != nil {
		return nil, err
	}
	if user.Status == model.UserStatusDisabled {
		return nil, errcode.New(errcode.NoPermission).WithMessage("账号已被禁用")
	}

	tokens, err := s.issueSession(ctx, user, info)
	if err != nil {
		return nil, err
	}
	tokens.User.IsNew = isNew
	return tokens, nil
}

func (s *Service) upsertUser(ctx context.Context, info *wechat.SessionInfo) (*model.User, bool, error) {
	user, err := s.repos.User.FindByOpenID(ctx, info.OpenID)
	if err == nil {
		return user, false, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return nil, false, err
	}
	u := &model.User{
		OpenID:    info.OpenID,
		UnionID:   info.UnionID,
		Nickname:  "微信用户",
		Status:    model.UserStatusActive,
		LastLogin: time.Now(),
	}
	if err := s.repos.User.Create(ctx, u); err != nil {
		// 并发首登可能撞唯一键：回查一次
		if again, e2 := s.repos.User.FindByOpenID(ctx, info.OpenID); e2 == nil {
			return again, false, nil
		}
		return nil, false, err
	}
	return u, true, nil
}

func (s *Service) issueSession(ctx context.Context, user *model.User, info *wechat.SessionInfo) (*Tokens, error) {
	now := time.Now()
	sess := &Session{
		SessionID:  uuid.NewString(),
		UserID:     user.ID,
		OpenID:     user.OpenID,
		UnionID:    user.UnionID,
		SessionKey: info.SessionKey,
		AppID:      s.cfg.WeChat.AppID,
		CreatedAt:  now,
		RenewedAt:  now,
	}
	raw, err := json.Marshal(sess)
	if err != nil {
		return nil, err
	}
	if err := s.sess.Set(ctx, sessionKeyPrefix+sess.SessionID, string(raw), s.cfg.JWT.RefreshTTL); err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}
	_ = s.repos.User.TouchLogin(ctx, user.ID)

	access, err := s.jwt.Sign(jwtutil.Claims{
		Subject:   fmt.Sprint(user.ID),
		Type:      jwtutil.TypeAccess,
		SessionID: sess.SessionID,
		OpenID:    user.OpenID,
		UnionID:   user.UnionID,
	}, s.cfg.JWT.AccessTTL)
	if err != nil {
		return nil, err
	}
	refresh, err := s.jwt.Sign(jwtutil.Claims{
		Subject:   fmt.Sprint(user.ID),
		Type:      jwtutil.TypeRefresh,
		SessionID: sess.SessionID,
	}, s.cfg.JWT.RefreshTTL)
	if err != nil {
		return nil, err
	}
	return &Tokens{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.cfg.JWT.AccessTTL.Seconds()),
		User: &UserBrief{
			ID: user.ID, OpenID: user.OpenID, Nickname: user.Nickname, AvatarURL: user.AvatarURL,
		},
	}, nil
}

// Refresh 用 refresh_token 换新的一对 token（refresh_token 轮换，旧的立即失效）。
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*Tokens, error) {
	if refreshToken == "" {
		return nil, errcode.New(errcode.RefreshInvalid)
	}
	claims, err := s.jwt.Parse(refreshToken, jwtutil.TypeRefresh)
	if err != nil {
		if errors.Is(err, jwtutil.ErrTokenExpired) {
			return nil, errcode.Wrap(errcode.RefreshInvalid, err).WithMessage("refresh_token 已过期，请重新登录")
		}
		return nil, errcode.Wrap(errcode.RefreshInvalid, err)
	}
	// 先用 SetNX 原子消费该 refresh_token（JTI 一次性），再校验会话：
	// 这样重放旧 token 得到的是「refresh 无效」，而不是难以定位的「会话失效」。
	ok, err := s.sess.SetNX(ctx, refreshUsedPrefix+claims.JTI, "1", s.cfg.JWT.RefreshTTL)
	if err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err)
	}
	if !ok {
		return nil, errcode.New(errcode.RefreshInvalid).WithMessage("refresh_token 已被使用")
	}

	sess, err := s.loadSession(ctx, claims.SessionID)
	if err != nil {
		return nil, err
	}

	user, err := s.repos.User.FindByID(ctx, sess.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errcode.New(errcode.SessionRevoked)
		}
		return nil, err
	}
	if user.Status == model.UserStatusDisabled {
		return nil, errcode.New(errcode.NoPermission).WithMessage("账号已被禁用")
	}

	if err := s.sess.Del(ctx, sessionKeyPrefix+sess.SessionID); err != nil {
		return nil, err
	}
	return s.issueSession(ctx, user, &wechat.SessionInfo{OpenID: sess.OpenID, UnionID: sess.UnionID, SessionKey: sess.SessionKey})
}

// Logout 注销当前会话。
func (s *Service) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return s.sess.Del(ctx, sessionKeyPrefix+sessionID)
}

// LogoutAll 注销某用户所有会话（安全事件、改绑手机号等场景）。
func (s *Service) LogoutAll(ctx context.Context, userID int64) (int, error) {
	keys, err := s.sess.ScanKeys(ctx, sessionKeyPrefix, 1000)
	if err != nil {
		return 0, err
	}
	var hit []string
	for _, k := range keys {
		raw, err := s.sess.Get(ctx, k)
		if err != nil {
			continue
		}
		var sess Session
		if json.Unmarshal([]byte(raw), &sess) == nil && sess.UserID == userID {
			hit = append(hit, k)
		}
	}
	if len(hit) > 0 {
		if err := s.sess.Del(ctx, hit...); err != nil {
			return 0, err
		}
	}
	return len(hit), nil
}

// UpdateProfile 更新昵称头像（wx.getUserProfile 授权后回传）。
func (s *Service) UpdateProfile(ctx context.Context, userID int64, nickname, avatar string) (*UserBrief, error) {
	if len([]rune(nickname)) > 32 {
		return nil, errcode.New(errcode.InvalidParam).WithMessage("昵称过长")
	}
	if err := s.repos.User.UpdateProfile(ctx, userID, nickname, avatar); err != nil {
		return nil, err
	}
	u, err := s.repos.User.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &UserBrief{ID: u.ID, OpenID: u.OpenID, Nickname: u.Nickname, AvatarURL: u.AvatarURL}, nil
}

// Me 查询当前用户。
func (s *Service) Me(ctx context.Context, userID int64) (*UserBrief, error) {
	u, err := s.repos.User.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errcode.New(errcode.SessionRevoked)
		}
		return nil, err
	}
	return &UserBrief{ID: u.ID, OpenID: u.OpenID, Nickname: u.Nickname, AvatarURL: u.AvatarURL}, nil
}

// ParseAccessToken 实现 auth.TokenParser：校验 JWT + 校验服务端会话存在。
func (s *Service) ParseAccessToken(ctx context.Context, raw string) (*auth.Identity, error) {
	claims, err := s.jwt.Parse(raw, jwtutil.TypeAccess)
	if err != nil {
		if errors.Is(err, jwtutil.ErrTokenExpired) {
			return nil, errcode.Wrap(errcode.TokenExpired, err)
		}
		return nil, errcode.Wrap(errcode.TokenInvalid, err)
	}
	sess, err := s.loadSession(ctx, claims.SessionID)
	if err != nil {
		return nil, err
	}
	return &auth.Identity{
		UserID:    sess.UserID,
		OpenID:    sess.OpenID,
		UnionID:   sess.UnionID,
		SessionID: sess.SessionID,
	}, nil
}

func (s *Service) loadSession(ctx context.Context, sessionID string) (*Session, error) {
	if sessionID == "" {
		return nil, errcode.New(errcode.TokenInvalid)
	}
	raw, err := s.sess.Get(ctx, sessionKeyPrefix+sessionID)
	if err != nil {
		if errors.Is(err, cache.ErrNotFound) {
			return nil, errcode.New(errcode.SessionRevoked)
		}
		logger.WithContext(ctx).Error("load session", zap.Error(err))
		return nil, errcode.Wrap(errcode.InternalError, err)
	}
	var sess Session
	if err := json.Unmarshal([]byte(raw), &sess); err != nil {
		return nil, errcode.Wrap(errcode.InternalError, err)
	}
	return &sess, nil
}
