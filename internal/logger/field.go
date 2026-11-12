package logger

import (
	"errors"

	"go.uber.org/zap"

	"cronApi/internal/errcode"
)

// Err 把 error 转为结构化字段。
// 业务错误附带 code，并保留原始 cause，日志里一次性看到完整链路。
func Err(err error) zap.Field {
	var be *errcode.BizError
	if errors.As(err, &be) {
		fields := []zap.Field{
			zap.Int("code", int(be.Code())),
			zap.String("biz_msg", be.Message()),
		}
		if cause := be.Unwrap(); cause != nil {
			fields = append(fields, zap.NamedError("cause", cause))
		}
		return zap.Any("error", fields)
	}
	return zap.Error(err)
}
