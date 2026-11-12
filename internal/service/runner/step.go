package runner

import (
	"encoding/json"
	"time"
)

// Step 一个联调步骤。
type Step struct {
	Name    string            `json:"name"`
	Method  string            `json:"method"`            // GET/POST/PUT/DELETE...
	URL     string            `json:"url"`               // 支持 {{变量}} 模板
	Headers map[string]string `json:"headers,omitempty"` // 支持模板
	Query   map[string]string `json:"query,omitempty"`   // 支持模板
	Body    string            `json:"body,omitempty"`    // JSON 模板
	// Auth: inherit(默认，沿用任务上下文登录态) | none | login(先调登录接口) | bearer/basic
	Auth string `json:"auth,omitempty"`
	// AuthRef 当 Auth=login 时引用 auths 中的登录配置名
	AuthRef string `json:"auth_ref,omitempty"`
	// Extracts 从响应中提取变量：变量名 -> JSONPath 风格路径，如 data.list.0.id
	Extracts map[string]string `json:"extracts,omitempty"`
	// Asserts 断言列表
	Asserts    []Assert `json:"asserts,omitempty"`
	TimeoutSec int      `json:"timeout_sec,omitempty"`
	// SkipIf 模板渲染后为 "true" 时跳过该步骤
	SkipIf string `json:"skip_if,omitempty"`
}

// Assert 断言。Op 取值：
// eq ne gt gte lt lte contains not_contains exists not_exists
// len_eq status_in json_eq
type Assert struct {
	// Source: body(默认) / status / header:<name> / json:<path>
	Source  string `json:"source,omitempty"`
	Op      string `json:"op"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message,omitempty"`
}

// StepResult 单步执行结果。
type StepResult struct {
	Name       string            `json:"name"`
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Status     int               `json:"status"`
	Passed     bool              `json:"passed"`
	DurationMS int64             `json:"duration_ms"`
	Attempts   int               `json:"attempts"`
	Response   json.RawMessage   `json:"response,omitempty"`
	Extracted  map[string]string `json:"extracted,omitempty"`
	Failures   []string          `json:"failures,omitempty"`
	Error      string            `json:"error,omitempty"`
	StartedAt  time.Time         `json:"started_at"`
	Skipped    bool              `json:"skipped,omitempty"`
}

// TaskRunInput 由调用方（调度器 / 接口）传入的执行参数。
type TaskRunInput struct {
	// Trigger: schedule | manual | api
	Trigger string
	// TraceID 链路 ID，便于把联调请求与业务日志串联
	TraceID string
	// AuthHeaders 任务级登录态（例如已登录用户的 Bearer token）
	AuthHeaders map[string]string
}

// AuthConfig 登录配置，用于在联调中自动获取登录态。
type AuthConfig struct {
	// Type: wechat_miniprogram | oauth_password | custom
	Type     string `json:"type"`
	URL      string `json:"url,omitempty"`  // custom: 登录接口地址
	Body     string `json:"body,omitempty"` // custom: JSON 模板
	Code     string `json:"code,omitempty"` // wechat_miniprogram: wx code（mock 可为 openid:xxx）
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	// AccessPath 从登录响应提取 token 的路径，默认 data.access_token
	AccessPath  string `json:"access_path,omitempty"`
	RefreshPath string `json:"refresh_path,omitempty"`
}
