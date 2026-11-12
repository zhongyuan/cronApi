// Package logger 提供基于 zap 的结构化日志，支持从 context 中自动带上 trace_id。
package logger

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ctxKey struct{}

var std = zap.NewNop()

// Init 初始化全局 logger。
func Init(level, format, output string) error {
	lv, err := zapcore.ParseLevel(level)
	if err != nil {
		lv = zapcore.InfoLevel
	}

	var encCfg zapcore.EncoderConfig
	if format == "json" {
		encCfg = zap.NewProductionEncoderConfig()
	} else {
		encCfg = zap.NewDevelopmentEncoderConfig()
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	encCfg.TimeKey = "ts"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	var encoder zapcore.Encoder
	if format == "json" {
		encoder = zapcore.NewJSONEncoder(encCfg)
	} else {
		encoder = zapcore.NewConsoleEncoder(encCfg)
	}

	ws := zapcore.Lock(os.Stdout)
	if output != "" && output != "stdout" {
		f, ferr := os.OpenFile(output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if ferr != nil {
			return ferr
		}
		ws = zapcore.Lock(f)
	}

	core := zapcore.NewCore(encoder, ws, lv)
	std = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))
	return nil
}

// L 返回全局 logger。
func L() *zap.Logger { return std }

// Sync 刷新缓冲。
func Sync() { _ = std.Sync() }

// WithContext 返回携带 trace_id 的 logger。
func WithContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return std
	}
	if tid, ok := ctx.Value(ctxKey{}).(string); ok && tid != "" {
		return std.With(zap.String("trace_id", tid))
	}
	return std
}

// WithTraceID 把 trace_id 注入 context。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, traceID)
}

// TraceIDFromContext 读取 context 中的 trace_id。
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	tid, _ := ctx.Value(ctxKey{}).(string)
	return tid
}

// Debug/Info/Warn/Error 便捷函数。
func Debug(msg string, fields ...zap.Field) { std.Debug(msg, fields...) }
func Info(msg string, fields ...zap.Field)  { std.Info(msg, fields...) }
func Warn(msg string, fields ...zap.Field)  { std.Warn(msg, fields...) }
func Error(msg string, fields ...zap.Field) { std.Error(msg, fields...) }
