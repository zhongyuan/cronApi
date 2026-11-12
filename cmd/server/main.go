// cronApi 自动联调服务入口。
//
// 启动顺序：配置 → 日志 → 存储(MySQL/Redis) → 服务装配 → 调度器 → HTTP → 优雅退出
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"cronApi/internal/config"
	"cronApi/internal/handler"
	"cronApi/internal/logger"
	"cronApi/internal/repository"
	"cronApi/internal/router"
	"cronApi/internal/server"
	authsvc "cronApi/internal/service/auth"
	"cronApi/internal/service/runner"
	"cronApi/internal/service/scheduler"
	"cronApi/internal/service/task"
	"cronApi/internal/service/wechat"
	"cronApi/internal/store"
	"cronApi/pkg/cache"
	"cronApi/pkg/httpx"
	"cronApi/pkg/jwtutil"
)

// 通过 ldflags 注入：go build -ldflags "-X main.version=1.2.3"
var version = "dev"

func main() {
	configPath := flag.String("c", "configs/config.yaml", "配置文件路径")
	showVersion := flag.Bool("v", false, "打印版本号")
	flag.Parse()

	if *showVersion {
		fmt.Println("cronApi", version)
		return
	}

	if err := run(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, "[fatal]", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := logger.Init(cfg.Log.Level, cfg.Log.Format, cfg.Log.Output); err != nil {
		return err
	}
	defer logger.Sync()
	logger.Info("cronApi starting", zap.String("version", version), zap.String("env", cfg.App.Env))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ---------- 存储 ----------
	var (
		repos     *repository.Repos
		sessCache cache.Cache
		rdb       *redis.Client
	)

	if cfg.Redis.Enabled {
		rc, err := store.NewRedis(ctx, cfg)
		if err != nil {
			return err
		}
		sessCache = cache.NewRedis(rc)
		rdb = rc
		logger.Info("redis connected", zap.String("addr", cfg.Redis.Addr))
	} else {
		sessCache = cache.NewMemory()
		logger.Warn("redis disabled, using in-memory session store (单实例调试模式)")
	}

	var taskSvc *task.TaskService
	var authSvc *authsvc.Service
	var exec *runner.Executor

	if cfg.MySQL.Enabled {
		db, err := store.NewMySQL(cfg)
		if err != nil {
			return err
		}
		if err := store.AutoMigrate(db); err != nil {
			return fmt.Errorf("自动迁移失败: %w", err)
		}
		repos = repository.New(db)
		logger.Info("mysql connected", zap.String("database", cfg.MySQL.Database))
	} else {
		return fmt.Errorf("当前版本任务/用户数据依赖 MySQL，请设置 mysql.enabled=true（可先执行 make up 拉起 docker 依赖）")
	}

	// ---------- 服务装配 ----------
	jwtMgr := jwtutil.NewManager(cfg.JWT.Secret, cfg.JWT.Issuer)
	hc := httpx.New(httpx.Options{
		Timeout:            cfg.HTTPClient.Timeout,
		MaxIdleConns:       cfg.HTTPClient.MaxIdleConns,
		InsecureSkipVerify: cfg.HTTPClient.InsecureSkipVerify,
		DefaultHeaders:     map[string]string{"User-Agent": "cronApi/1.0"},
	})
	exec = runner.NewExecutor(hc)

	var wx wechat.Client = wechat.NewMockClient()
	if !cfg.WeChat.Mock {
		wx = wechat.NewHTTPClient(cfg.WeChat.AppID, cfg.WeChat.AppSecret, cfg.WeChat.Endpoint, cfg.WeChat.Timeout)
	}
	authSvc = authsvc.NewService(cfg, repos, wx, sessCache, jwtMgr)

	// ---------- 调度器 ----------
	// 注意装配顺序：调度器需要 taskSvc，而 taskSvc 需要「任务变更后重载调度」的回调，
	// 因此先用闭包捕获 sched 变量，二者完成后再 start。
	var sched *scheduler.Scheduler
	taskSvc = task.NewTaskService(repos, exec, func(ctx context.Context) error {
		if sched == nil {
			return nil
		}
		return sched.Reload(ctx)
	})

	var locker cache.Cache
	if cfg.Redis.Enabled {
		locker = sessCache
	}
	sched, err = scheduler.New(cfg, taskSvc, locker)
	if err != nil {
		return err
	}
	if err := sched.Start(ctx); err != nil {
		return fmt.Errorf("调度器启动失败: %w", err)
	}
	defer sched.Stop()

	// ---------- HTTP ----------
	engine := router.New(router.Deps{
		Config: cfg,
		Auth:   handler.NewAuthHandler(authSvc),
		Task:   handler.NewTaskHandler(taskSvc),
		Parser: authSvc,
		RDB:    rdb,
	})

	err = server.Run(ctx, cfg, engine)
	if rdb != nil {
		_ = rdb.Close()
	}
	if err != nil {
		return err
	}
	logger.Info("cronApi stopped", zap.String("at", time.Now().Format(time.RFC3339)))
	return nil
}
