// Package task 联调任务的增删改查与「立即执行」。
package task

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"go.uber.org/zap"

	"cronApi/internal/errcode"
	"cronApi/internal/logger"
	"cronApi/internal/model"
	"cronApi/internal/repository"
	"cronApi/internal/service/runner"
)

// TaskService 任务服务。
type TaskService struct {
	repos *repository.Repos
	exec  *runner.Executor
	// reload 任务变更后重载调度表（由 main 注入 scheduler.Reload，避免包循环依赖）
	reload func(ctx context.Context) error
}

// NewTaskService 构造。reload 可为 nil（未启用调度器时）。
func NewTaskService(repos *repository.Repos, exec *runner.Executor, reload func(ctx context.Context) error) *TaskService {
	return &TaskService{repos: repos, exec: exec, reload: reload}
}

// CreateInput 创建任务入参（含联调定义字段，简单起见与 Task 字段贴近）。
type CreateInput struct {
	Name        string                       `json:"name" binding:"required,max=128"`
	Description string                       `json:"description" binding:"max=512"`
	BaseURL     string                       `json:"base_url" binding:"required,url"`
	CronExpr    string                       `json:"cron_expr" binding:"required"`
	Timezone    string                       `json:"timezone"`
	Enabled     *bool                        `json:"enabled"`
	Steps       []runner.Step                `json:"steps" binding:"required,min=1,dive"`
	Auths       map[string]runner.AuthConfig `json:"auths"`
	Variables   map[string]string            `json:"variables"`
	MaxRetry    int                          `json:"max_retry" binding:"gte=0,lte=5"`
	TimeoutSec  int                          `json:"timeout_sec" binding:"omitempty,gte=1,lte=3600"` // 省略时默认 60s
	NotifyURL   string                       `json:"notify_url"`
}

// TaskView 对外返回结构。
type TaskView struct {
	*model.Task
	BaseURL string                       `json:"base_url"`
	Auths   map[string]runner.AuthConfig `json:"auths"`
}

// Detail 任务完整定义（DB 中 task 表按 steps/json 存储，base_url 与 auths 一并放在 steps 文档中）。
type detailDoc struct {
	BaseURL string                       `json:"base_url"`
	Auths   map[string]runner.AuthConfig `json:"auths,omitempty"`
	Steps   []runner.Step                `json:"steps"`
}

// Save 创建任务。
func (s *TaskService) Save(ctx context.Context, ownerID int64, in *CreateInput) (*TaskView, error) {
	return s.save(ctx, 0, ownerID, in)
}

// Update 全量更新任务；变更定时规则后触发调度器热加载。
func (s *TaskService) Update(ctx context.Context, id, ownerID int64, in *CreateInput) (*TaskView, error) {
	if _, err := s.repos.Task.FindByID(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errcode.New(errcode.TaskNotFound)
		}
		return nil, err
	}
	return s.save(ctx, id, ownerID, in)
}

// SetEnabled 启用/停用任务（列表页开关用，无需传完整定义）。
func (s *TaskService) SetEnabled(ctx context.Context, id int64, enabled bool) (*TaskView, error) {
	t, err := s.repos.Task.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errcode.New(errcode.TaskNotFound)
		}
		return nil, err
	}
	t.Enabled = enabled
	if err := s.repos.Task.Save(ctx, t); err != nil {
		return nil, err
	}
	s.reloadScheduler(ctx)
	return toView(t)
}

// save 内部实现：id=0 创建，否则更新。
func (s *TaskService) save(ctx context.Context, id, ownerID int64, in *CreateInput) (*TaskView, error) {
	if err := validateCron(in.CronExpr); err != nil {
		return nil, err
	}
	for i, st := range in.Steps {
		if st.URL == "" {
			return nil, errcode.Newf(errcode.TaskValidateFailed, "第 %d 步缺少 url", i+1)
		}
		if st.Auth == "login" && in.Auths[st.AuthRef].Type == "" {
			return nil, errcode.Newf(errcode.TaskValidateFailed, "第 %d 步引用登录配置 %q 不存在", i+1, st.AuthRef)
		}
	}
	doc, err := json.Marshal(detailDoc{BaseURL: in.BaseURL, Auths: in.Auths, Steps: in.Steps})
	if err != nil {
		return nil, err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	t := &model.Task{
		ID:          id,
		Name:        in.Name,
		OwnerID:     ownerID,
		Description: in.Description,
		CronExpr:    in.CronExpr,
		Timezone:    in.Timezone,
		Enabled:     enabled,
		Steps:       doc,
		MaxRetry:    in.MaxRetry,
		TimeoutSec:  in.TimeoutSec,
		NotifyURL:   in.NotifyURL,
	}
	if t.Timezone == "" {
		t.Timezone = "Asia/Shanghai"
	}
	if t.TimeoutSec == 0 {
		t.TimeoutSec = 60
	}
	if in.Variables != nil {
		b, _ := json.Marshal(in.Variables)
		_ = json.Unmarshal(b, &t.Variables)
	}
	// 更新时保留库中既有字段：创建者、创建时间、上次执行时间。
	// gorm.Save 会写全部列，若不回填 created_at 会被写成零值而报 0000-00-00。
	if id > 0 {
		if old, err := s.repos.Task.FindByID(ctx, id); err == nil {
			t.OwnerID = old.OwnerID
			t.CreatedAt = old.CreatedAt
			t.LastRunAt = old.LastRunAt
		}
	}
	if id > 0 {
		if err := s.repos.Task.Save(ctx, t); err != nil {
			return nil, err
		}
	} else {
		if err := s.repos.Task.Create(ctx, t); err != nil {
			return nil, err
		}
	}
	s.reloadScheduler(ctx)
	return toView(t)
}

// reloadScheduler 让定时规则变更立即生效，无需重启进程。
func (s *TaskService) reloadScheduler(ctx context.Context) {
	if s.reload == nil {
		return
	}
	if err := s.reload(ctx); err != nil {
		logger.WithContext(ctx).Warn("scheduler reload failed", zap.Error(err))
	}
}

// List 分页列出任务。
func (s *TaskService) List(ctx context.Context, ownerID int64, page, size int) ([]*TaskView, int64, error) {
	list, total, err := s.repos.Task.List(ctx, ownerID, page, size)
	if err != nil {
		return nil, 0, err
	}
	views := make([]*TaskView, 0, len(list))
	for _, t := range list {
		v, err := toView(t)
		if err != nil {
			continue
		}
		views = append(views, v)
	}
	return views, total, nil
}

// Get 查询任务。
func (s *TaskService) Get(ctx context.Context, id int64) (*TaskView, error) {
	t, err := s.repos.Task.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errcode.New(errcode.TaskNotFound)
		}
		return nil, err
	}
	return toView(t)
}

// Delete 删除任务。
func (s *TaskService) Delete(ctx context.Context, id int64) error {
	if _, err := s.repos.Task.FindByID(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errcode.New(errcode.TaskNotFound)
		}
		return err
	}
	return s.repos.Task.Delete(ctx, id)
}

// RunNow 立即执行任务并落库执行记录，返回完整报告。
func (s *TaskService) RunNow(ctx context.Context, taskID int64, in runner.TaskRunInput) (*model.Run, error) {
	t, err := s.repos.Task.FindByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errcode.New(errcode.TaskNotFound)
		}
		return nil, err
	}
	trigger := in.Trigger
	if trigger == "" {
		trigger = "manual"
	}
	return s.execute(ctx, t, in, trigger)
}

// execute 内部执行：构建 Request → 执行 → 持久化。
func (s *TaskService) execute(ctx context.Context, t *model.Task, in runner.TaskRunInput, trigger string) (*model.Run, error) {
	var doc detailDoc
	if len(t.Steps) > 0 {
		if err := json.Unmarshal(t.Steps, &doc); err != nil {
			return nil, errcode.Wrap(errcode.TaskValidateFailed, err)
		}
	}
	vars := map[string]string{}
	for k, v := range t.Variables {
		vars[k] = v
	}
	req := runner.Request{
		TaskName:    t.Name,
		BaseURL:     doc.BaseURL,
		Steps:       doc.Steps,
		Vars:        vars,
		Auths:       doc.Auths,
		MaxRetry:    t.MaxRetry,
		Timeout:     time.Duration(t.TimeoutSec) * time.Second,
		TraceID:     in.TraceID,
		AuthHeaders: in.AuthHeaders,
	}

	run := &model.Run{
		TaskID:     t.ID,
		TraceID:    in.TraceID,
		Status:     model.RunStatusRunning,
		Trigger:    trigger,
		TotalSteps: len(doc.Steps),
		StartedAt:  time.Now(),
	}
	if err := s.repos.Run.Create(ctx, run); err != nil {
		return nil, err
	}

	res := s.exec.Run(ctx, req)

	finished := time.Now()
	run.PassedSteps = 0
	for _, sr := range res.Steps {
		if sr.Passed && !sr.Skipped {
			run.PassedSteps++
		}
	}
	run.DurationMS = res.Duration.Milliseconds()
	run.FinishedAt = &finished
	run.ErrorMsg = truncateStr(res.Err, 1000)
	run.Status = model.RunStatusFailed
	switch {
	case res.Passed:
		run.Status = model.RunStatusSuccess
	case ctx.Err() == context.DeadlineExceeded:
		run.Status = model.RunStatusTimeout
	}
	if b, err := json.Marshal(res.Steps); err == nil {
		run.Steps = b
	}
	if vm, err := json.Marshal(res.Variables); err == nil {
		_ = json.Unmarshal(vm, &run.Variables)
	}

	if err := s.repos.Run.Update(ctx, run); err != nil {
		logger.WithContext(ctx).Error("save run failed", zap.Error(err))
	}
	_ = s.repos.Task.MarkRun(ctx, t.ID, run.StartedAt)
	return run, nil
}

// ListRuns 查询任务执行记录。
func (s *TaskService) ListRuns(ctx context.Context, taskID int64, page, size int) ([]*model.Run, int64, error) {
	if _, err := s.repos.Task.FindByID(ctx, taskID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, 0, errcode.New(errcode.TaskNotFound)
		}
		return nil, 0, err
	}
	return s.repos.Run.ListByTask(ctx, taskID, page, size)
}

// GetRun 查询单次执行。
func (s *TaskService) GetRun(ctx context.Context, taskID, runID int64) (*model.Run, error) {
	run, err := s.repos.Run.FindByID(ctx, runID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errcode.New(errcode.RunNotFound)
		}
		return nil, err
	}
	if run.TaskID != taskID {
		return nil, errcode.New(errcode.RunNotFound)
	}
	return run, nil
}

// EnabledTasks 供调度器加载。
func (s *TaskService) EnabledTasks(ctx context.Context) ([]*model.Task, error) {
	return s.repos.Task.ListEnabled(ctx)
}

func toView(t *model.Task) (*TaskView, error) {
	var doc detailDoc
	if len(t.Steps) > 0 {
		if err := json.Unmarshal(t.Steps, &doc); err != nil {
			return nil, err
		}
	}
	return &TaskView{Task: t, BaseURL: doc.BaseURL, Auths: doc.Auths}, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// SortedStepNames 调试辅助。
func SortedStepNames(steps []runner.Step) []string {
	names := make([]string, 0, len(steps))
	for _, s := range steps {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return names
}
