package jwtutil

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSignAndParse(t *testing.T) {
	m := NewManager("secret", "cronApi")
	tok, err := m.Sign(Claims{Subject: "1", Type: TypeAccess, SessionID: "s1"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	c, err := m.Parse(tok, TypeAccess)
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "1" || c.SessionID != "s1" || c.Issuer != "cronApi" || c.JTI == "" {
		t.Fatalf("unexpected claims: %+v", c)
	}
}

func TestParse_WrongType(t *testing.T) {
	m := NewManager("secret", "cronApi")
	tok, _ := m.Sign(Claims{Subject: "1", Type: TypeRefresh}, time.Minute)
	if _, err := m.Parse(tok, TypeAccess); err == nil {
		t.Fatal("refresh token must not be accepted as access")
	}
}

func TestParse_Tampered(t *testing.T) {
	m := NewManager("secret", "cronApi")
	tok, _ := m.Sign(Claims{Subject: "1", Type: TypeAccess}, time.Minute)
	parts := strings.Split(tok, ".")
	// 篡改 payload
	other, _ := m.Sign(Claims{Subject: "2", Type: TypeAccess}, time.Minute)
	parts[1] = strings.Split(other, ".")[1]
	if _, err := m.Parse(strings.Join(parts, "."), TypeAccess); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected invalid, got %v", err)
	}
	// 换密钥
	if _, err := NewManager("other", "cronApi").Parse(tok, TypeAccess); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("expected invalid signature, got %v", err)
	}
}

func TestParse_Expired(t *testing.T) {
	m := NewManager("secret", "cronApi")
	tok, _ := m.Sign(Claims{Subject: "1", Type: TypeAccess, ExpiresAt: time.Now().Add(-time.Second).Unix()}, 0)
	if _, err := m.Parse(tok, TypeAccess); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected expired, got %v", err)
	}
}
