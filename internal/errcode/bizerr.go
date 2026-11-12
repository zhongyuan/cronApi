package errcode

import (
	"errors"
	"fmt"
)

// BizError 业务错误：携带错误码、对外文案、HTTP 状态，并可用 Unwrap 追溯底层 error。
// 底层 error 只写日志，不返回给客户端，避免泄露内部细节。
type BizError struct {
	code    Code
	message string
	status  int
	err     error
}

// New 创建业务错误。
func New(code Code) *BizError {
	return &BizError{code: code, message: code.Message(), status: code.HTTPStatus()}
}

// Newf 创建带格式化文案的业务错误。
func Newf(code Code, format string, args ...any) *BizError {
	return &BizError{code: code, message: fmt.Sprintf(format, args...), status: code.HTTPStatus()}
}

// Wrap 包装底层错误（仅用于日志链）。
func Wrap(code Code, err error) *BizError {
	return &BizError{code: code, message: code.Message(), status: code.HTTPStatus(), err: err}
}

// Wrapf 包装底层错误并自定义文案。
func Wrapf(code Code, err error, format string, args ...any) *BizError {
	return &BizError{code: code, message: fmt.Sprintf(format, args...), status: code.HTTPStatus(), err: err}
}

func (e *BizError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.code, e.message, e.err)
	}
	return fmt.Sprintf("[%d] %s", e.code, e.message)
}

// Unwrap 支持 errors.Is/As。
func (e *BizError) Unwrap() error { return e.err }

// Code 返回业务码。
func (e *BizError) Code() Code { return e.code }

// Message 返回对外文案。
func (e *BizError) Message() string { return e.message }

// Status 返回 HTTP 状态码。
func (e *BizError) Status() int { return e.status }

// WithMessage 覆盖对外文案，返回新的错误副本。
func (e *BizError) WithMessage(msg string) *BizError {
	cp := *e
	cp.message = msg
	return &cp
}

// From 将任意 error 转换为 *BizError；非业务错误统一归为 InternalError。
func From(err error) *BizError {
	var be *BizError
	if errors.As(err, &be) {
		return be
	}
	return &BizError{code: InternalError, message: InternalError.Message(), status: InternalError.HTTPStatus(), err: err}
}
