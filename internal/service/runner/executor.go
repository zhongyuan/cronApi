package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	"cronApi/internal/logger"
	"cronApi/pkg/httpx"
)

// Request 一次待执行的联调（任务的一次运行）。
type Request struct {
	TaskName string
	BaseURL  string
	Steps    []Step
	Vars     map[string]string
	Auths    map[string]AuthConfig
	MaxRetry int
	Timeout  time.Duration
	TraceID  string
	// AuthHeaders 任务级默认登录态（例如通过小程序登录接口获取的 Bearer token）
	AuthHeaders map[string]string
}

// Result 一次联调结果。
type Result struct {
	Passed    bool
	Steps     []StepResult
	Variables map[string]string
	Err       string
	Duration  time.Duration
}

// Executor 执行联调任务。
type Executor struct {
	hc *httpx.Client
}

// NewExecutor 构造。
func NewExecutor(hc *httpx.Client) *Executor { return &Executor{hc: hc} }

// baseURL 与步骤 URL 解析。
func resolveURL(base, u string) (string, error) {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u, nil
	}
	if base == "" {
		return "", fmt.Errorf("步骤 URL 为相对路径，但未配置 base_url")
	}
	bu, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	ru, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	return bu.ResolveReference(ru).String(), nil
}

// Run 顺序执行所有步骤；任一步失败即终止（后续步骤标记 skipped）。
func (e *Executor) Run(ctx context.Context, req Request) *Result {
	start := time.Now()
	res := &Result{Variables: req.Vars}
	if res.Variables == nil {
		res.Variables = map[string]string{}
	}

	authHeaders := map[string]string{}
	for k, v := range req.AuthHeaders {
		authHeaders[k] = v
	}
	loginCache := map[string]map[string]string{}

	total := req.Timeout
	if total <= 0 {
		total = time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, total)
	defer cancel()

	lg := logger.WithContext(ctx).With(zap.String("task", req.TaskName))

	for i := range req.Steps {
		st := req.Steps[i]
		sr := StepResult{
			Name: st.Name, Method: strings.ToUpper(defaultStr(st.Method, http.MethodGet)),
			StartedAt: time.Now(),
		}
		if sr.Name == "" {
			sr.Name = fmt.Sprintf("step-%d", i+1)
		}

		// skip_if
		if st.SkipIf != "" && strings.EqualFold(Render(st.SkipIf, res.Variables), "true") {
			sr.Skipped, sr.Passed = true, true
			res.Steps = append(res.Steps, sr)
			continue
		}

		u, err := resolveURL(req.BaseURL, Render(st.URL, res.Variables))
		if err != nil {
			sr.Error = err.Error()
			res.Steps = append(res.Steps, sr)
			res.Err = fmt.Sprintf("步骤[%s] URL 解析失败: %v", sr.Name, err)
			e.skipRest(res, req.Steps[i+1:])
			break
		}
		sr.URL = u

		// 步骤级登录态
		mode := strings.ToLower(st.Auth)
		stepAuth := authHeaders
		switch mode {
		case "none":
			stepAuth = nil
		case "login":
			cfg, ok := req.Auths[st.AuthRef]
			if !ok {
				sr.Error = "未找到登录配置: " + st.AuthRef
				res.Steps = append(res.Steps, sr)
				res.Err = sr.Error
				e.skipRest(res, req.Steps[i+1:])
				return res
			}
			h, err := e.login(runCtx, req.BaseURL, cfg, loginCache)
			if err != nil {
				sr.Error = "获取登录态失败: " + err.Error()
				res.Steps = append(res.Steps, sr)
				res.Err = sr.Error
				e.skipRest(res, req.Steps[i+1:])
				return res
			}
			stepAuth = h
		}

		body := Render(st.Body, res.Variables)
		maxAttempts := req.MaxRetry + 1
		if maxAttempts < 1 {
			maxAttempts = 1
		}

		var lastFailure []string
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			sr.Attempts = attempt
			stepStart := time.Now()
			failures, status, raw, err := e.doStep(runCtx, st, u, body, stepAuth, sr.Method, res.Variables)
			sr.DurationMS = time.Since(stepStart).Milliseconds()
			sr.Status = status

			if err != nil {
				sr.Error = err.Error()
				lastFailure = []string{"请求失败: " + err.Error()}
			} else {
				sr.Error = ""
				lastFailure = failures
				parsed := parseJSON(raw)
				sr.Response = summarize(raw, parsed)
				if len(failures) == 0 {
					// 提取变量（仅成功时写入，避免污染后续步骤）
					if len(st.Extracts) > 0 && parsed != nil {
						sr.Extracted = map[string]string{}
						for name, p := range st.Extracts {
							if v, ok := GetPathString(parsed, Render(p, res.Variables)); ok {
								res.Variables[name] = v
								sr.Extracted[name] = v
							} else {
								lastFailure = append(lastFailure, fmt.Sprintf("提取变量 %s 失败(路径 %s)", name, p))
							}
						}
					}
				}
			}

			if len(lastFailure) == 0 {
				break
			}
			if attempt < maxAttempts {
				lg.Warn("step failed, retrying", zap.String("step", sr.Name),
					zap.Int("attempt", attempt), zap.Strings("failures", lastFailure))
				select {
				case <-runCtx.Done():
					lastFailure = append(lastFailure, "执行超时")
					attempt = maxAttempts
				case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
				}
			}
		}

		if len(lastFailure) > 0 {
			sr.Passed = false
			sr.Failures = lastFailure
			res.Steps = append(res.Steps, sr)
			res.Err = fmt.Sprintf("步骤[%s]失败: %s", sr.Name, strings.Join(lastFailure, "; "))
			e.skipRest(res, req.Steps[i+1:])
			break
		}
		sr.Passed = true
		res.Steps = append(res.Steps, sr)
	}

	res.Duration = time.Since(start)
	res.Passed = res.Err == ""
	if runCtx.Err() == context.DeadlineExceeded && res.Err == "" {
		res.Passed = false
		res.Err = "任务执行超时"
	}
	return res
}

func (e *Executor) skipRest(res *Result, rest []Step) {
	for _, st := range rest {
		name := st.Name
		if name == "" {
			name = "(unnamed)"
		}
		res.Steps = append(res.Steps, StepResult{Name: name, Passed: false, Skipped: true, Error: "前序步骤失败，已跳过"})
	}
}

func (e *Executor) doStep(ctx context.Context, st Step, u, body string, authH map[string]string, method string, vars map[string]string) (failures []string, status int, raw []byte, err error) {
	if q := RenderMap(st.Query, vars); len(q) > 0 {
		parsed, perr := url.Parse(u)
		if perr != nil {
			return nil, 0, nil, perr
		}
		vals := parsed.Query()
		for k, v := range q {
			vals.Set(k, v)
		}
		parsed.RawQuery = vals.Encode()
		u = parsed.String()
	}

	var reqBody io.Reader
	var bodyBytes []byte
	if body != "" {
		bodyBytes = []byte(body)
		reqBody = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, 0, nil, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range authH {
		req.Header.Set(k, v)
	}
	for k, v := range st.Headers {
		req.Header.Set(k, Render(v, vars))
	}

	resp, data, _, err := e.hc.Do(ctx, req, bodyBytes)
	if err != nil {
		return nil, 0, nil, err
	}
	cc := CheckContext{Status: resp.StatusCode, Header: resp.Header, Body: data, BodyJSON: parseJSON(data)}
	return RunAsserts(st.Asserts, cc), resp.StatusCode, data, nil
}

// login 获取并缓存登录态。
func (e *Executor) login(ctx context.Context, baseURL string, cfg AuthConfig, memo map[string]map[string]string) (map[string]string, error) {
	key := cfg.Type + "|" + cfg.URL + "|" + cfg.Code + "|" + cfg.Username
	if h, ok := memo[key]; ok {
		return h, nil
	}

	accessPath := defaultStr(cfg.AccessPath, "data.access_token")
	refreshPath := defaultStr(cfg.RefreshPath, "data.refresh_token")

	var (
		u, body string
		hdr     map[string]string
	)
	switch cfg.Type {
	case "wechat_miniprogram":
		u = defaultStr(cfg.URL, "/api/v1/auth/wechat/login")
		u = strings.TrimRight(baseURL, "/") + ensureLeadingSlash(u)
		code, _ := json.Marshal(map[string]string{"code": cfg.Code})
		body = string(code)
	case "custom":
		ru, err := resolveURL(baseURL, cfg.URL)
		if err != nil {
			return nil, err
		}
		u, body = ru, cfg.Body
	default:
		return nil, fmt.Errorf("不支持的登录类型: %s", cfg.Type)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, data, _, err := e.hc.Do(ctx, req, []byte(body))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("登录接口状态码 %d", resp.StatusCode)
	}
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("登录响应解析失败")
	}
	token, ok := GetPathString(parsed, accessPath)
	if !ok || token == "" {
		return nil, fmt.Errorf("登录响应中未找到 token(%s)", accessPath)
	}
	hdr = map[string]string{"Authorization": "Bearer " + token}
	if rt, ok := GetPathString(parsed, refreshPath); ok && rt != "" && refreshPath != accessPath {
		hdr["X-Refresh-Token"] = rt
	}
	memo[key] = hdr
	return hdr, nil
}

func parseJSON(b []byte) any {
	t := bytes.TrimSpace(b)
	if len(t) == 0 || (t[0] != '{' && t[0] != '[') {
		return nil
	}
	var v any
	if err := json.Unmarshal(t, &v); err != nil {
		return nil
	}
	return v
}

const maxStoreResp = 8 << 10

func summarize(raw []byte, parsed any) json.RawMessage {
	if parsed != nil && len(raw) <= maxStoreResp {
		return json.RawMessage(raw)
	}
	if parsed == nil {
		s := truncate(string(raw), 2000)
		b, _ := json.Marshal(s)
		return b
	}
	b, _ := json.Marshal(map[string]any{"_truncated": true, "_size": len(raw)})
	return b
}

func defaultStr(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

func ensureLeadingSlash(s string) string {
	if strings.HasPrefix(s, "/") {
		return s
	}
	return "/" + s
}
