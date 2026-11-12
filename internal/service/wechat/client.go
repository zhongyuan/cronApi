// Package wechat 封装微信小程序服务端接口。
package wechat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cronApi/pkg/httpx"
)

// SessionInfo 微信 code2session 结果。
type SessionInfo struct {
	OpenID     string
	UnionID    string
	SessionKey string
}

// Client 微信小程序服务端客户端。
type Client interface {
	// Code2Session 用 wx.login 得到的 code 换取 openid/session_key。
	Code2Session(ctx context.Context, code string) (*SessionInfo, error)
}

// Error 微信接口错误。
type Error struct {
	Errcode int    `json:"errcode"`
	Errmsg  string `json:"errmsg"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("wechat errcode=%d errmsg=%s", e.Errcode, e.Errmsg)
}

// HTTPClient 真实实现。
type HTTPClient struct {
	appID     string
	appSecret string
	endpoint  string
	hc        *httpx.Client
}

// NewHTTPClient 创建真实调用客户端。
func NewHTTPClient(appID, appSecret, endpoint string, timeout time.Duration) *HTTPClient {
	if endpoint == "" {
		endpoint = "https://api.weixin.qq.com"
	}
	return &HTTPClient{
		appID:     appID,
		appSecret: appSecret,
		endpoint:  strings.TrimRight(endpoint, "/"),
		hc:        httpx.New(httpx.Options{Timeout: timeout}),
	}
}

// Code2Session 调用 /sns/jscode2session。
// 注意：code 仅能用一次、5 分钟有效。
func (c *HTTPClient) Code2Session(ctx context.Context, code string) (*SessionInfo, error) {
	q := url.Values{}
	q.Set("appid", c.appID)
	q.Set("secret", c.appSecret)
	q.Set("js_code", code)
	q.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/sns/jscode2session?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, body, _, err := c.hc.Do(ctx, req, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wechat http status %d", resp.StatusCode)
	}
	var out struct {
		OpenID     string `json:"openid"`
		UnionID    string `json:"unionid"`
		SessionKey string `json:"session_key"`
		Error
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("wechat response decode: %w", err)
	}
	if out.Errcode != 0 {
		return nil, &out.Error
	}
	if out.OpenID == "" {
		return nil, fmt.Errorf("wechat response missing openid")
	}
	return &SessionInfo{OpenID: out.OpenID, UnionID: out.UnionID, SessionKey: out.SessionKey}, nil
}

// MockClient 本地联调实现：不请求微信服务器。
// 约定：code 以 "openid:" 开头时，其后直接作为 openid；否则对 code 做哈希稳定生成 openid，
// 保证同一 code 多次联调得到同一用户。
type MockClient struct{}

// NewMockClient 创建 mock 客户端。
func NewMockClient() *MockClient { return &MockClient{} }

// Code2Session 实现 Client。
func (m *MockClient) Code2Session(_ context.Context, code string) (*SessionInfo, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, &Error{Errcode: 40029, Errmsg: "invalid code"}
	}
	if strings.HasPrefix(code, "openid:") {
		v := strings.TrimPrefix(code, "openid:")
		if v == "" {
			return nil, &Error{Errcode: 40029, Errmsg: "invalid code"}
		}
		return &SessionInfo{OpenID: v, SessionKey: "mock-session-key"}, nil
	}
	sum := sha256.Sum256([]byte("mock:" + code))
	return &SessionInfo{OpenID: "mock_" + hex.EncodeToString(sum[:8]), SessionKey: "mock-session-key"}, nil
}
