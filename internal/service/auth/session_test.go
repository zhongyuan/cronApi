package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appauth "cronApi/internal/auth"
	"cronApi/internal/config"
	"cronApi/internal/errcode"
	"cronApi/internal/model"
	"cronApi/internal/repository"
	svcauth "cronApi/internal/service/auth"
	"cronApi/internal/service/wechat"

	"cronApi/pkg/cache"
	"cronApi/pkg/jwtutil"
)

// ---- 内存版仓储 ----

type fakeUsers struct {
	data map[string]*model.User
	seq  int64
}

func (f *fakeUsers) FindByOpenID(_ context.Context, openID string) (*model.User, error) {
	if u, ok := f.data[openID]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, repository.ErrNotFound
}
func (f *fakeUsers) FindByID(_ context.Context, id int64) (*model.User, error) {
	for _, u := range f.data {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, repository.ErrNotFound
}
func (f *fakeUsers) Create(_ context.Context, u *model.User) error {
	f.seq++
	u.ID = f.seq
	cp := *u
	f.data[u.OpenID] = &cp
	return nil
}
func (f *fakeUsers) UpdateProfile(_ context.Context, id int64, nickname, avatar string) error {
	for _, u := range f.data {
		if u.ID == id {
			u.Nickname, u.AvatarURL = nickname, avatar
		}
	}
	return nil
}
func (f *fakeUsers) TouchLogin(context.Context, int64) error { return nil }

func newService(t *testing.T) *svcauth.Service {
	t.Helper()
	cfg := &config.Config{}
	cfg.App.Env = "test"
	cfg.JWT.Secret = "test-secret"
	cfg.JWT.Issuer = "cronApi"
	cfg.JWT.AccessTTL = 2 * time.Second
	cfg.JWT.RefreshTTL = time.Hour
	cfg.WeChat.Mock = true

	repos := &repository.Repos{User: &fakeUsers{data: map[string]*model.User{}}}
	return svcauth.NewService(cfg, repos, wechat.NewMockClient(), cache.NewMemory(),
		jwtutil.NewManager(cfg.JWT.Secret, cfg.JWT.Issuer))
}

func TestLoginAndParseToken(t *testing.T) {
	svc := newService(t)
	tokens, err := svc.Login(context.Background(), "openid:u001")
	require.NoError(t, err)
	require.NotEmpty(t, tokens.AccessToken)
	require.NotEmpty(t, tokens.RefreshToken)
	require.Equal(t, "u001", tokens.User.OpenID)
	require.True(t, tokens.User.IsNew)

	id, err := svc.ParseAccessToken(context.Background(), tokens.AccessToken)
	require.NoError(t, err)
	require.Equal(t, "u001", id.OpenID)
	require.NotEmpty(t, id.SessionID)

	// 同 code 再登录：不是新用户，会话独立
	again, err := svc.Login(context.Background(), "openid:u001")
	require.NoError(t, err)
	require.False(t, again.User.IsNew)
	id2, err := svc.ParseAccessToken(context.Background(), again.AccessToken)
	require.NoError(t, err)
	require.NotEqual(t, id.SessionID, id2.SessionID)
}

func TestRefreshRotation(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	tokens, err := svc.Login(ctx, "openid:u002")
	require.NoError(t, err)

	newTokens, err := svc.Refresh(ctx, tokens.RefreshToken)
	require.NoError(t, err)
	require.NotEqual(t, tokens.AccessToken, newTokens.AccessToken)

	// 旧 refresh_token 不可重放
	_, err = svc.Refresh(ctx, tokens.RefreshToken)
	require.Error(t, err)
	var be *errcode.BizError
	require.ErrorAs(t, err, &be)
	require.Equal(t, errcode.RefreshInvalid, be.Code())

	// 旧 access_token 对应的 session 已被轮换掉 → 失效
	_, err = svc.ParseAccessToken(ctx, tokens.AccessToken)
	require.Error(t, err)

	// 新 access_token 可用
	_, err = svc.ParseAccessToken(ctx, newTokens.AccessToken)
	require.NoError(t, err)
}

func TestLogout(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	tokens, err := svc.Login(ctx, "openid:u003")
	require.NoError(t, err)

	id, err := svc.ParseAccessToken(ctx, tokens.AccessToken)
	require.NoError(t, err)

	// 另一个会话
	other, _ := svc.Login(ctx, "openid:u003")

	require.NoError(t, svc.Logout(ctx, id.SessionID))
	_, err = svc.ParseAccessToken(ctx, tokens.AccessToken)
	require.Error(t, err)

	// 该用户其余会话仍有效
	_, err = svc.ParseAccessToken(ctx, other.AccessToken)
	require.NoError(t, err)

	// 全端下线
	n, err := svc.LogoutAll(ctx, id.UserID)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	_, err = svc.ParseAccessToken(ctx, other.AccessToken)
	require.Error(t, err)
}

func TestEmptyCode(t *testing.T) {
	svc := newService(t)
	_, err := svc.Login(context.Background(), "")
	var be *errcode.BizError
	require.ErrorAs(t, err, &be)
	require.Equal(t, errcode.InvalidParam, be.Code())
}

func TestMockClientStable(t *testing.T) {
	c := wechat.NewMockClient()
	a, err := c.Code2Session(context.Background(), "same-code")
	require.NoError(t, err)
	b, _ := c.Code2Session(context.Background(), "same-code")
	require.Equal(t, a.OpenID, b.OpenID)
	require.NotEmpty(t, a.OpenID)
}

var _ appauth.TokenParser = (*svcauth.Service)(nil)
