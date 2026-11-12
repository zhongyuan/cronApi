package middleware

import (
	"fmt"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"cronApi/internal/errcode"
	"cronApi/internal/logger"
	"cronApi/internal/response"
)

// Recovery 捕获 panic，返回统一的 500 响应并记录堆栈。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.WithContext(c.Request.Context()).Error("panic recovered",
					logger.Err(fmt.Errorf("panic: %v", r)),
					zapString("stack", string(debug.Stack())),
				)
				response.Abort(c, errcode.InternalError)
			}
		}()
		c.Next()
	}
}
