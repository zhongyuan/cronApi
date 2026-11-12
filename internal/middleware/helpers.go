package middleware

import "go.uber.org/zap"

func zapString(k, v string) zap.Field { return zap.String(k, v) }
