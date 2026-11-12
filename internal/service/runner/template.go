package runner

import (
	"os"
	"regexp"
	"strings"
)

var tplRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)

// Render 用 vars 渲染 {{key}} 模板；未命中的变量替换为空串。
// 支持内置变量：{{env.XXX}} 读取系统环境变量。
func Render(tpl string, vars map[string]string) string {
	if tpl == "" || !strings.Contains(tpl, "{{") {
		return tpl
	}
	return tplRe.ReplaceAllStringFunc(tpl, func(m string) string {
		key := strings.TrimSpace(tplRe.FindStringSubmatch(m)[1])
		if strings.HasPrefix(key, "env.") {
			return os.Getenv(strings.TrimPrefix(key, "env."))
		}
		return vars[key]
	})
}

// RenderMap 渲染 map 的 value。
func RenderMap(m map[string]string, vars map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = Render(v, vars)
	}
	return out
}
