package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	serveConsole(r)
	return r
}

func TestConsoleRoutes(t *testing.T) {
	r := newTestEngine()

	cases := []struct {
		path       string
		wantStatus int
		wantInBody string
	}{
		{"/", http.StatusFound, ""},
		{"/console/", http.StatusOK, "cronApi"},
		{"/console/index.html", http.StatusOK, "<html"},
		{"/console/app.js", http.StatusOK, "describeCron"},
		{"/console/style.css", http.StatusOK, "--brand"},
		{"/console/nope.js", http.StatusNotFound, ""},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, c.path, nil))
		if w.Code != c.wantStatus {
			t.Errorf("GET %s => %d, want %d", c.path, w.Code, c.wantStatus)
			continue
		}
		if c.wantInBody != "" && !strings.Contains(w.Body.String(), c.wantInBody) {
			t.Errorf("GET %s 响应体缺少 %q", c.path, c.wantInBody)
		}
	}
}

func TestConsoleRejectsTraversal(t *testing.T) {
	r := newTestEngine()
	for _, p := range []string{"/console/../configs/config.yaml", "/console/%2e%2e/style.css"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "mysql") {
			t.Fatalf("路径穿越未被拦截: %s", p)
		}
	}
}
