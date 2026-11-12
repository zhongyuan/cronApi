// Package scheduler 基于 robfig/cron 的定时调度器。
//
// 特性：
//   - 任务变更后热加载（Reload），无需重启；
//   - 单任务防重入：上一轮未结束时本轮标记为 skipped；
//   - 全局并发上限，避免同时打爆被测服务。
package scheduler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"cronApi/internal/config"
	"cronApi/internal/logger"
	"cronApi/internal/model"
	"cronApi/internal/service/runner"
	"cronApi/pkg/cache"
	"cronApi/pkg/httpx"
)

// TaskExecutor 调度器依赖的任务执行能力（由 task.TaskService 实现）。
type TaskExecutor interface {
	EnabledTasks(ctx context.Context) ([]*model.Task, error)
	RunNow(ctx context.Context, taskID int64, in runner.TaskRunInput) (*model.Run, error)
}

// Scheduler 调度器。
type Scheduler struct {
	cfg     *config.Config
	exec    TaskExecutor
	c       *cron.Cron
	hc      *httpx.Client
	locker  cache.Cache // 分布式锁；为 nil 时退化为进程内锁
	mu      sync.Mutex
	sem     chan struct{}
	running sync.Map // taskID -> struct{}
}

// New 创建调度器。locker 为 Redis 缓存时启用分布式锁，传 nil 使用进程内锁。
func New(cfg *config.Config, exec TaskExecutor, locker cache.Cache) (*Scheduler, error) {
	loc := time.Local
	if cfg.Scheduler.Timezone != "" {
		l, err := time.LoadLocation(cfg.Scheduler.Timezone)
		if err != nil {
			return nil, err
		}
		loc = l
	}
	return &Scheduler{
		cfg:  cfg,
		exec: exec,
		c: cron.New(
			cron.WithLocation(loc),
			cron.WithParser(cron.NewParser(cron.SecondOptional|cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow|cron.Descriptor)),
			cron.WithChain(cron.Recover(cron.DefaultLogger)),
		),
		hc:     httpx.New(httpx.Options{Timeout: 10 * time.Second}),
		locker: locker,
		sem:    make(chan struct{}, cfg.Scheduler.MaxConcurrency),
	}, nil
}

// Start 加载任务并启动。
func (s *Scheduler) Start(ctx context.Context) error {
	if !s.cfg.Scheduler.Enabled {
		logger.Info("scheduler disabled")
		return nil
	}
	if err := s.Reload(ctx); err != nil {
		return err
	}
	s.c.Start()
	logger.Info("scheduler started", zap.String("entries", "loaded"))
	return nil
}

// Stop 优雅停止（等待运行中的任务完成）。
func (s *Scheduler) Stop() {
	if s.c == nil {
		return
	}
	ctx := s.c.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(s.cfg.App.ShutdownTimeout):
	}
}

// Reload 清空并重新加载所有启用任务。
func (s *Scheduler) Reload(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range s.c.Entries() {
		s.c.Remove(e.ID)
	}
	tasks, err := s.exec.EnabledTasks(ctx)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		t := t
		_, err := s.c.AddFunc(t.CronExpr, func() { s.trigger(t.ID) })
		if err != nil {
			logger.Warn("skip invalid cron task", zap.Int64("task_id", t.ID), zap.String("cron", t.CronExpr), zap.Error(err))
			continue
		}
	}
	logger.Info("scheduler reloaded", zap.Int("tasks", len(tasks)))
	return nil
}

func (s *Scheduler) trigger(taskID int64) {
	// 防重入：分布式锁（Redis）保证多实例下同一任务只跑一份
	lockKey := "sched:lock:" + itoa(taskID)
	ok, err := s.lock(lockKey, 10*time.Minute)
	if err != nil || !ok {
		logger.Warn("task locked or lock error, skip", zap.Int64("task_id", taskID), zap.Error(err))
		return
	}
	defer s.unlock(lockKey)

	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	default:
		logger.Warn("scheduler concurrency limit reached, skip", zap.Int64("task_id", taskID))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	ctx = logger.WithTraceID(ctx, "sched-"+itoa(taskID)+"-"+itoa(time.Now().Unix()))

	run, err := s.exec.RunNow(ctx, taskID, runner.TaskRunInput{Trigger: "schedule", TraceID: logger.TraceIDFromContext(ctx)})
	if err != nil {
		logger.WithContext(ctx).Error("scheduled task failed", zap.Int64("task_id", taskID), zap.Error(err))
		return
	}
	if run.Status != model.RunStatusSuccess {
		s.notify(ctx, taskID, run)
	}
}

func (s *Scheduler) notify(ctx context.Context, taskID int64, run *model.Run) {
	tasks, err := s.exec.EnabledTasks(ctx)
	if err != nil {
		return
	}
	var notifyURL string
	for _, t := range tasks {
		if t.ID == taskID {
			notifyURL = t.NotifyURL
		}
	}
	if notifyURL == "" {
		return
	}
	body := map[string]any{"task_id": taskID, "run_id": run.ID, "status": run.Status, "error": run.ErrorMsg}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, notifyURL, strings.NewReader(string(b)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if _, _, _, err := s.hc.Do(ctx, req, b); err != nil {
		logger.WithContext(ctx).Warn("notify webhook failed", zap.String("url", notifyURL), zap.Error(err))
	}
}

// lock 获取任务级分布式锁（Redis SetNX），保证多实例部署时同一任务同一时刻只执行一份。
// 未启用 Redis 时退化为进程内锁。
func (s *Scheduler) lock(key string, ttl time.Duration) (bool, error) {
	if s.locker != nil {
		return s.locker.SetNX(context.Background(), key, "1", ttl)
	}
	_, loaded := s.running.LoadOrStore(key, time.Now())
	return !loaded, nil
}

func (s *Scheduler) unlock(key string) {
	if s.locker != nil {
		_ = s.locker.Del(context.Background(), key)
		return
	}
	s.running.Delete(key)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
