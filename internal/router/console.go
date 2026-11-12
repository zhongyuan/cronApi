package router

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cronApi/internal/errcode"
	"cronApi/internal/response"
)

// webFS 嵌入式托管前端静态资源（web/ 下的原生 HTML/CSS/JS，无需 node 构建）。
//
//go:embed web
var webFS embed.FS

// serveConsole 注册控制台路由：
//
//	GET /            → 302 到 /console/
//	GET /console/*   → 静态资源，目录请求回落到 index.html
//
// 不用 http.FileServer：它对 embed FS 的路径会触发 301 重定向链，
// 这里直接按路径读文件并写响应，行为完全可控。
func serveConsole(r *gin.Engine) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}

	r.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/console/") })

	r.GET("/console/*filepath", func(c *gin.Context) {
		p := path.Clean("/" + c.Param("filepath"))
		if strings.Contains(p, "..") {
			response.Abort(c, errcode.NotFound, "资源不存在")
			return
		}
		if p == "/" {
			p = "/index.html"
		}

		data, err := fs.ReadFile(sub, strings.TrimPrefix(p, "/"))
		if err != nil {
			response.Abort(c, errcode.NotFound, "资源不存在: "+p)
			return
		}

		ctype := mime.TypeByExtension(path.Ext(p))
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		c.Header("Cache-Control", "no-cache")
		http.ServeContent(c.Writer, c.Request, path.Base(p), time.Time{}, bytes.NewReader(data))
		c.Header("Content-Type", ctype)
	})
}
