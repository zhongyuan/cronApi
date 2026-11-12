package runner

import (
	"encoding/json"
	"testing"
)

func parseJSONAny(b []byte) (any, error) {
	var v any
	err := json.Unmarshal(b, &v)
	return v, err
}

func TestRender(t *testing.T) {
	vars := map[string]string{"token": "abc123", "user.id": "42"}
	cases := []struct{ in, want string }{
		{"Bearer {{token}}", "Bearer abc123"},
		{"{{ token }}", "abc123"},
		{"no var", "no var"},
		{"/u/{{user.id}}", "/u/42"},
		{"{{missing}}x", "x"},
	}
	for _, c := range cases {
		if got := Render(c.in, vars); got != c.want {
			t.Errorf("Render(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGetPath(t *testing.T) {
	root := map[string]any{
		"data": map[string]any{
			"access_token": "tok",
			"list":         []any{map[string]any{"id": float64(7)}},
		},
		"ok": true,
	}
	cases := []struct{ path, want string }{
		{"data.access_token", "tok"},
		{"$.data.access_token", "tok"},
		{"data.list.0.id", "7"},
		{"ok", "true"},
	}
	for _, c := range cases {
		got, ok := GetPathString(root, c.path)
		if !ok || got != c.want {
			t.Errorf("GetPathString(%q) = (%q,%v), want %q", c.path, got, ok, c.want)
		}
	}
	if _, ok := GetPathString(root, "data.nope"); ok {
		t.Error("expected missing path to return false")
	}
}

func TestRunAsserts(t *testing.T) {
	body := []byte(`{"code":0,"data":{"total":3},"msg":"ok"}`)
	cc := CheckContext{Status: 200, Header: map[string][]string{"Content-Type": {"application/json"}}}
	cc.Body = body
	var err error
	cc.BodyJSON, err = parseJSONAny(body)
	if err != nil {
		t.Fatal(err)
	}

	pass := []Assert{
		{Source: "status", Op: "eq", Value: "200"},
		{Source: "status", Op: "status_in", Value: "200,201"},
		{Source: "json:code", Op: "eq", Value: "0"},
		{Source: "json:data.total", Op: "gte", Value: "3"},
		{Source: "json:msg", Op: "contains", Value: "o"},
		{Source: "json:data.total", Op: "exists"},
		{Source: "json:data.missing", Op: "not_exists"},
		{Op: "contains", Value: "total"},
	}
	if fails := RunAsserts(pass, cc); len(fails) != 0 {
		t.Fatalf("expected all pass, got %v", fails)
	}

	fail := []Assert{
		{Source: "status", Op: "eq", Value: "500"},
		{Source: "json:code", Op: "eq", Value: "1"},
		{Source: "json:data.total", Op: "lt", Value: "2"},
	}
	if fails := RunAsserts(fail, cc); len(fails) != 3 {
		t.Fatalf("expected 3 failures, got %v", fails)
	}
}
