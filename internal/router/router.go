// Package router 负责 Gin 引擎构建与路由注册。
// 分层约定：handler 不直接访问 DB，业务在 service，数据在 repository。
package router

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"cronApi/internal/auth"
	"cronApi/internal/config"
	"cronApi/internal/errcode"
	"cronApi/internal/handler"
	"cronApi/internal/middleware"
	"cronApi/internal/response"
)

// Deps 路由依赖。
type Deps struct {
	Config *config.Config
	Auth   *handler.AuthHandler
	Task   *handler.TaskHandler
	Parser auth.TokenParser
	RDB    *redis.Client // 为 nil 时限流使用内存实现
}

// New 构建 *gin.Engine。
func New(d Deps) *gin.Engine {
	cfg := d.Config
	if cfg.App.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	// 信任的代理：生产环境按实际网关配置
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})

	r.Use(middleware.Trace(), middleware.AccessLog(), middleware.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now()})
	})

	// 任务管理界面（静态单页，无需构建）
	serveConsole(r)

	v1 := r.Group("/api/v1")
	{
		// 登录态接口：登录/刷新需要限流防止撞库与刷接口
		var rlStore middleware.RateLimitStore
		if d.RDB != nil {
			rlStore = middleware.NewRedisRateLimitStore(d.RDB)
		} else {
			rlStore = middleware.NewMemoryRateLimitStore()
		}
		authGrp := v1.Group("/auth")
		authGrp.POST("/wechat/login", middleware.RateLimit(rlStore, 20, time.Minute, nil), d.Auth.Login)
		authGrp.POST("/refresh", middleware.RateLimit(rlStore, 60, time.Minute, nil), d.Auth.Refresh)

		// 需要登录态
		authed := authGrp.Group("", auth.Require(d.Parser))
		authed.POST("/logout", d.Auth.Logout)
		authed.POST("/logout-all", d.Auth.LogoutAll)
		authed.GET("/me", d.Auth.Me)
		authed.PUT("/profile", d.Auth.UpdateProfile)

		// 联调任务（全部需要登录）
		tasks := v1.Group("/tasks", auth.Require(d.Parser))
		{
			tasks.POST("", d.Task.Create)
			tasks.GET("", d.Task.List)
			tasks.GET("/:id", d.Task.Get)
			tasks.PUT("/:id", d.Task.Update)
			tasks.PATCH("/:id/enabled", d.Task.Toggle)
			tasks.DELETE("/:id", d.Task.Delete)
			tasks.POST("/:id/run", d.Task.Run)
			tasks.GET("/:id/runs", d.Task.ListRuns)
			tasks.GET("/:id/runs/:run_id", d.Task.GetRun)
		}
	}

	r.NoRoute(func(c *gin.Context) {
		response.Abort(c, errcode.NotFound, "接口不存在: "+c.Request.URL.Path)
	})
	r.NoMethod(func(c *gin.Context) {
		response.Abort(c, errcode.MethodNotAllow)
	})
	r.HandleMethodNotAllowed = true
	return r
}
