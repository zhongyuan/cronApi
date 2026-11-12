// Package response 统一 HTTP 响应格式。
//
// 成功：{"code":0,"msg":"success","data":{...},"trace_id":"..."}
// 失败：{"code":10002,"msg":"登录已过期","data":null,"trace_id":"..."}
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cronApi/internal/errcode"
	"cronApi/internal/logger"
)

// Body 统一响应体。
type Body struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Data    any    `json:"data"`
	TraceID string `json:"trace_id,omitempty"`
}

// Page 通用分页结构。
type Page struct {
	List     any   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// OK 返回成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: 0, Msg: "success", Data: data, TraceID: logger.TraceIDFromContext(c.Request.Context())})
}

// Fail 返回业务错误响应（由 *BizError 或任意 error 触发）。
func Fail(c *gin.Context, err error) {
	be := errcode.From(err)
	traceID := logger.TraceIDFromContext(c.Request.Context())

	// 5xx 才记 error 级别，4xx 属于预期内错误，避免日志噪音
	if be.Status() >= http.StatusInternalServerError {
		logger.WithContext(c.Request.Context()).Error("request failed", logger.Err(be))
	} else {
		logger.WithContext(c.Request.Context()).Warn("request rejected", logger.Err(be))
	}

	c.AbortWithStatusJSON(be.Status(), Body{Code: int(be.Code()), Msg: be.Message(), Data: nil, TraceID: traceID})
}

// Abort 在中间件中返回错误。
func Abort(c *gin.Context, code errcode.Code, msgOverride ...string) {
	be := errcode.New(code)
	if len(msgOverride) > 0 && msgOverride[0] != "" {
		be = be.WithMessage(msgOverride[0])
	}
	Fail(c, be)
}
