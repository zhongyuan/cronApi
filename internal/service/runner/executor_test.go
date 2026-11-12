package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cronApi/pkg/httpx"
)

func newExecutor() *Executor {
	return NewExecutor(httpx.New(httpx.Options{Timeout: 3 * time.Second}))
}

// 模拟被测系统：登录 + 查询 + 需要鉴权的详情接口
func newMockServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/wechat/login", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["code"] == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{
			"code": 0,
			"data": map[string]any{"access_token": "tk-" + body["code"], "refresh_token": "rt"},
		})
	})
	mux.HandleFunc("/api/v1/orders", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer tk-") {
			w.WriteHeader(http.StatusUnauthorized)
			writeJSON(w, map[string]any{"code": 10001})
			return
		}
		writeJSON(w, map[string]any{
			"code": 0,
			"data": map[string]any{"list": []map[string]any{{"id": 101, "name": "订单A"}}},
		})
	})
	mux.HandleFunc("/api/v1/orders/101", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"id": 101}})
	})
	return httptest.NewServer(mux)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestExecutor_LoginAndChain(t *testing.T) {
	srv := newMockServer()
	defer srv.Close()

	res := newExecutor().Run(context.Background(), Request{
		TaskName: "下单链路",
		BaseURL:  srv.URL,
		Auths: map[string]AuthConfig{
			"wx": {Type: "wechat_miniprogram", Code: "code-1"},
		},
		Steps: []Step{
			{
				Name: "登录", Method: "POST", URL: "{{base}}/api/v1/auth/wechat/login",
				Auth: "none", Body: `{"code":"mock-001"}`,
				Extracts: map[string]string{"wx_token": "data.access_token"},
				Asserts:  []Assert{{Source: "json:code", Op: "eq", Value: "0"}},
			},
			{
				Name: "我的订单", Method: "GET", URL: "{{base}}/api/v1/orders",
				Auth: "login", AuthRef: "wx",
				Headers: map[string]string{"X-Case": "{{case}}"},
				Asserts: []Assert{
					{Source: "status", Op: "eq", Value: "200"},
					{Source: "json:data.list.0.name", Op: "eq", Value: "订单A"},
				},
				Extracts: map[string]string{"first_id": "data.list.0.id"},
			},
			{
				Name: "订单详情", Method: "GET", URL: "/api/v1/orders/{{first_id}}", Auth: "none",
				Asserts: []Assert{{Source: "json:data.id", Op: "eq", Value: "101"}},
			},
		},
		Vars:     map[string]string{"base": srv.URL, "case": "nightly"},
		MaxRetry: 0,
		Timeout:  5 * time.Second,
	})

	if !res.Passed {
		t.Fatalf("expected pass, err=%s, steps=%+v", res.Err, res.Steps)
	}
	if got := res.Variables["first_id"]; got != "101" {
		t.Fatalf("first_id = %q, want 101", got)
	}
	if got := res.Variables["wx_token"]; got != "tk-mock-001" {
		t.Fatalf("wx_token = %q", got)
	}
	if len(res.Steps) != 3 {
		t.Fatalf("steps = %d", len(res.Steps))
	}
}

func TestExecutor_FailFastAndSkip(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		writeJSON(w, map[string]any{"code": 1, "msg": "boom"})
	}))
	defer srv.Close()

	res := newExecutor().Run(context.Background(), Request{
		BaseURL: srv.URL,
		Steps: []Step{
			{Name: "A", URL: "/a", Asserts: []Assert{{Source: "json:code", Op: "eq", Value: "0"}}},
			{Name: "B", URL: "/b"},
		},
		MaxRetry: 2,
		Timeout:  5 * time.Second,
	})
	if res.Passed {
		t.Fatal("expected failure")
	}
	if res.Steps[0].Attempts != 3 {
		t.Fatalf("attempts = %d, want 3 (1 + retry 2)", res.Steps[0].Attempts)
	}
	if !res.Steps[1].Skipped {
		t.Fatal("second step should be skipped")
	}
	if n := atomic.LoadInt32(&hits); n != 3 {
		t.Fatalf("server hits = %d, want 3", n)
	}
}

func TestExecutor_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		writeJSON(w, map[string]any{"code": 0})
	}))
	defer srv.Close()

	res := newExecutor().Run(context.Background(), Request{
		BaseURL: srv.URL,
		Steps:   []Step{{Name: "slow", URL: "/slow"}},
		Timeout: 50 * time.Millisecond,
	})
	if res.Passed {
		t.Fatal("expected timeout failure")
	}
}
