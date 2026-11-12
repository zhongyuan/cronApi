// Package errcode 定义业务错误码与 HTTP 状态映射。
//
// 错误码规则：采用 5 位数字，首段对应 HTTP 语义，后段为业务序号。
//
//	0            成功
//	4xxxx        客户端/参数类错误
//	10xxx        鉴权/登录态类错误
//	20xxx        业务领域错误
//	50xxx        服务端错误
package errcode

import "net/http"

// Code 业务错误码类型。
type Code int

// 通用
const (
	OK             Code = 0
	InvalidParam   Code = 40001
	BindError      Code = 40002
	NotFound       Code = 40401
	MethodNotAllow Code = 40501
	TooManyRequest Code = 42901
	InternalError  Code = 50001
	ServiceUnavail Code = 50301
)

// 鉴权 / 小程序登录态
const (
	Unauthorized    Code = 10001 // 未登录或 token 缺失
	TokenExpired    Code = 10002 // access_token 过期，需刷新
	TokenInvalid    Code = 10003 // token 非法/被篡改
	SessionRevoked  Code = 10004 // 会话已注销或失效
	WeChatCodeBad   Code = 10005 // wx code 无效或已使用
	WeChatError     Code = 10006 // 微信侧异常
	RefreshInvalid  Code = 10007 // refresh_token 无效/过期
	NoPermission    Code = 10008 // 无权限
	LoginRequired   Code = 10009 // 需要绑定用户信息
	UnsupportedType Code = 10010 // 不支持的登录类型
)

// 业务领域
const (
	TaskNotFound       Code = 20001
	TaskDisabled       Code = 20002
	TaskRunning        Code = 20003
	TaskValidateFailed Code = 20004
	CronExprInvalid    Code = 20005
	RunNotFound        Code = 20006
	RunNotFinished     Code = 20007
)

var messages = map[Code]string{
	OK:             "success",
	InvalidParam:   "参数错误",
	BindError:      "参数解析失败",
	NotFound:       "资源不存在",
	MethodNotAllow: "方法不允许",
	TooManyRequest: "请求过于频繁，请稍后再试",
	InternalError:  "服务内部错误",
	ServiceUnavail: "服务暂不可用",

	Unauthorized:    "未登录",
	TokenExpired:    "登录已过期，请刷新 token",
	TokenInvalid:    "无效的 token",
	SessionRevoked:  "登录态已失效，请重新登录",
	WeChatCodeBad:   "微信登录凭证无效或已过期",
	WeChatError:     "微信服务异常",
	RefreshInvalid:  "refresh_token 无效或已过期",
	NoPermission:    "无访问权限",
	LoginRequired:   "请先完善用户信息",
	UnsupportedType: "不支持的登录类型",

	TaskNotFound:       "任务不存在",
	TaskDisabled:       "任务已停用",
	TaskRunning:        "任务正在执行中",
	TaskValidateFailed: "任务校验失败",
	CronExprInvalid:    "cron 表达式非法",
	RunNotFound:        "执行记录不存在",
	RunNotFinished:     "执行尚未结束",
}

// Message 返回错误码对应的默认文案。
func (c Code) Message() string {
	if m, ok := messages[c]; ok {
		return m
	}
	return "未知错误"
}

// HTTPStatus 返回错误码应映射的 HTTP 状态码。
func (c Code) HTTPStatus() int {
	switch {
	case c == OK:
		return http.StatusOK
	case c >= 10001 && c <= 10010:
		return http.StatusUnauthorized
	case c == NotFound:
		return http.StatusNotFound
	case c >= 42901 && c < 43000:
		return http.StatusTooManyRequests
	case c >= 50001:
		return http.StatusInternalServerError
	case c >= 40000:
		return http.StatusBadRequest
	case c >= 20001 && c < 30000:
		return http.StatusBadRequest
	default:
		return http.StatusBadRequest
	}
}
