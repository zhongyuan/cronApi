// Package jwtutil 轻量 HS256 JWT 实现（标准库，无第三方依赖），
// 适合小程序登录态这类「服务端签发 + 服务端校验」的场景。
// 每种 token 类型带 typ 声明，防止 access/refresh 混用。
package jwtutil

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrTokenInvalid token 非法/签名错误/格式错误/类型不匹配。
var ErrTokenInvalid = errors.New("jwt: token invalid")

// ErrTokenExpired token 已过期。
var ErrTokenExpired = errors.New("jwt: token expired")

// Claims 标准 + 自定义声明。
type Claims struct {
	Issuer    string `json:"iss,omitempty"`
	Subject   string `json:"sub,omitempty"` // 用户 ID
	Audience  string `json:"aud,omitempty"` // 应用/小程序 app_id
	IssuedAt  int64  `json:"iat"`
	NotBefore int64  `json:"nbf,omitempty"`
	ExpiresAt int64  `json:"exp"`
	JTI       string `json:"jti"`
	Type      string `json:"typ"`           // access | refresh
	SessionID string `json:"sid,omitempty"` // 会话 ID，用于服务端撤销
	OpenID    string `json:"oid,omitempty"`
	UnionID   string `json:"uid,omitempty"`
}

// TokenType 常量。
const (
	TypeAccess  = "access"
	TypeRefresh = "refresh"
)

// Manager 负责签发与校验。
type Manager struct {
	secret []byte
	issuer string
}

// NewManager 创建签发器。
func NewManager(secret, issuer string) *Manager {
	return &Manager{secret: []byte(secret), issuer: issuer}
}

// Sign 签发 token。
func (m *Manager) Sign(c Claims, ttl time.Duration) (string, error) {
	now := time.Now()
	c.Issuer = m.issuer
	if c.IssuedAt == 0 {
		c.IssuedAt = now.Unix()
	}
	if c.ExpiresAt == 0 {
		c.ExpiresAt = now.Add(ttl).Unix()
	}
	if c.JTI == "" {
		c.JTI = uuid.NewString()
	}

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	pb, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	signing := enc.EncodeToString(hb) + "." + enc.EncodeToString(pb)
	sig := m.signature(signing)
	return signing + "." + enc.EncodeToString(sig), nil
}

func (m *Manager) signature(signing string) []byte {
	h := hmac.New(sha256.New, m.secret)
	h.Write([]byte(signing))
	return h.Sum(nil)
}

// Parse 校验并解析 token；expectType 非空时校验 typ。
func (m *Manager) Parse(token string, expectType string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrTokenInvalid
	}
	enc := base64.RawURLEncoding
	hb, err := enc.DecodeString(parts[0])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(hb, &header); err != nil || header.Alg != "HS256" {
		return nil, fmt.Errorf("%w: unsupported alg", ErrTokenInvalid)
	}
	sig, err := enc.DecodeString(parts[2])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	if !hmac.Equal(sig, m.signature(parts[0]+"."+parts[1])) {
		return nil, ErrTokenInvalid
	}
	pb, err := enc.DecodeString(parts[1])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	var c Claims
	if err := json.Unmarshal(pb, &c); err != nil {
		return nil, ErrTokenInvalid
	}
	if m.issuer != "" && c.Issuer != m.issuer {
		return nil, ErrTokenInvalid
	}
	if expectType != "" && c.Type != expectType {
		return nil, fmt.Errorf("%w: token type mismatch", ErrTokenInvalid)
	}
	now := time.Now().Unix()
	if c.NotBefore > 0 && now < c.NotBefore {
		return nil, ErrTokenInvalid
	}
	if c.ExpiresAt > 0 && now >= c.ExpiresAt {
		return nil, ErrTokenExpired
	}
	return &c, nil
}
