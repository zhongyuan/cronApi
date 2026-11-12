package task

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"cronApi/internal/errcode"
	"cronApi/internal/model"
	"cronApi/internal/repository"
	"cronApi/internal/service/runner"
)

// ---- 内存仓储 ----

type fakeTaskRepo struct {
	data map[int64]*model.Task
	seq  int64
}

func (f *fakeTaskRepo) Create(_ context.Context, t *model.Task) error {
	f.seq++
	t.ID = f.seq
	cp := *t
	f.data[t.ID] = &cp
	return nil
}
func (f *fakeTaskRepo) Save(_ context.Context, t *model.Task) error {
	cp := *t
	f.data[t.ID] = &cp
	return nil
}
func (f *fakeTaskRepo) Update(_ context.Context, t *model.Task) error { return f.Save(nil, t) }
func (f *fakeTaskRepo) Delete(_ context.Context, id int64) error      { delete(f.data, id); return nil }
func (f *fakeTaskRepo) FindByID(_ context.Context, id int64) (*model.Task, error) {
	if t, ok := f.data[id]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, repository.ErrNotFound
}
func (f *fakeTaskRepo) List(context.Context, int64, int, int) ([]*model.Task, int64, error) {
	return nil, int64(len(f.data)), nil
}
func (f *fakeTaskRepo) ListEnabled(context.Context) ([]*model.Task, error) {
	var out []*model.Task
	for _, t := range f.data {
		if t.Enabled {
			out = append(out, t)
		}
	}
	return out, nil
}
func (f *fakeTaskRepo) MarkRun(context.Context, int64, any) error { return nil }

func newSvc() (*TaskService, *fakeTaskRepo, *int) {
	repo := &fakeTaskRepo{data: map[int64]*model.Task{}}
	reloads := 0
	svc := NewTaskService(&repository.Repos{Task: repo, Run: nil}, nil, func(context.Context) error {
		reloads++
		return nil
	})
	return svc, repo, &reloads
}

func input(cron string, enabled bool) *CreateInput {
	return &CreateInput{
		Name:     "联调任务",
		BaseURL:  "http://127.0.0.1:8080",
		CronExpr: cron,
		Enabled:  &enabled,
		Steps:    []runner.Step{{Name: "s1", Method: "GET", URL: "/healthz"}},
	}
}

func TestSave_TriggersSchedulerReload(t *testing.T) {
	svc, _, reloads := newSvc()
	v, err := svc.Save(context.Background(), 7, input("*/5 * * * *", true))
	require.NoError(t, err)
	require.Equal(t, int64(7), v.OwnerID)
	require.Equal(t, 1, *reloads, "创建任务后应重载调度表")
	require.Equal(t, 60, v.TimeoutSec, "未指定超时应取默认 60s")
}

func TestUpdate_KeepsOwnerAndReloads(t *testing.T) {
	svc, _, reloads := newSvc()
	v, err := svc.Save(context.Background(), 7, input("*/5 * * * *", true))
	require.NoError(t, err)
	*reloads = 0

	// 前端不传 owner_id，更新后不应丢失
	in := input("0 9 * * *", false)
	updated, err := svc.Update(context.Background(), v.ID, 999, in)
	require.NoError(t, err)
	require.Equal(t, int64(7), updated.OwnerID, "更新时应保留原创建者")
	require.Equal(t, "0 9 * * *", updated.CronExpr)
	require.False(t, updated.Enabled)
	require.Equal(t, 1, *reloads)
}

func TestUpdate_NotFound(t *testing.T) {
	svc, _, _ := newSvc()
	_, err := svc.Update(context.Background(), 42, 1, input("* * * * *", true))
	var be *errcode.BizError
	require.ErrorAs(t, err, &be)
	require.Equal(t, errcode.TaskNotFound, be.Code())
}

func TestSetEnabled(t *testing.T) {
	svc, _, reloads := newSvc()
	v, _ := svc.Save(context.Background(), 1, input("*/5 * * * *", true))
	*reloads = 0

	v2, err := svc.SetEnabled(context.Background(), v.ID, false)
	require.NoError(t, err)
	require.False(t, v2.Enabled)
	require.Equal(t, 1, *reloads, "启停后应重载调度表，让停用立即生效")

	// 停用的任务不应出现在调度列表
	enabled, _ := svc.EnabledTasks(context.Background())
	require.Len(t, enabled, 0)
}

func TestSave_InvalidCronAndStep(t *testing.T) {
	svc, _, reloads := newSvc()
	_, err := svc.Save(context.Background(), 1, input("bad expr", true))
	var be *errcode.BizError
	require.ErrorAs(t, err, &be)
	require.Equal(t, errcode.CronExprInvalid, be.Code())

	in := input("* * * * *", true)
	in.Steps = []runner.Step{{Name: "no-url"}}
	_, err = svc.Save(context.Background(), 1, in)
	require.ErrorAs(t, err, &be)
	require.Equal(t, errcode.TaskValidateFailed, be.Code())
	require.Equal(t, 0, *reloads, "校验失败不应触发重载")
}

func TestSave_StepWithLoginAuthRef(t *testing.T) {
	svc, _, _ := newSvc()
	in := input("* * * * *", true)
	in.Steps = []runner.Step{{Name: "dbg", URL: "/api/v1/auth/me", Auth: "login", AuthRef: "wx"}}
	_, err := svc.Save(context.Background(), 1, in)
	var be *errcode.BizError
	require.ErrorAs(t, err, &be)
	require.Equal(t, errcode.TaskValidateFailed, be.Code(), "引用了不存在的登录配置应被拒绝")

	in.Auths = map[string]runner.AuthConfig{"wx": {Type: "wechat_miniprogram", Code: "openid:u1"}}
	_, err = svc.Save(context.Background(), 1, in)
	require.NoError(t, err)
}

func TestDetailDoc_RoundTrip(t *testing.T) {
	svc, repo, _ := newSvc()
	in := input("* * * * *", true)
	in.Auths = map[string]runner.AuthConfig{"wx": {Type: "wechat_miniprogram", Code: "openid:u1"}}
	in.Steps = []runner.Step{{
		Name: "查询", Method: "GET", URL: "/api/v1/orders", Auth: "login", AuthRef: "wx",
		Extracts: map[string]string{"id": "data.id"},
		Asserts:  []runner.Assert{{Source: "status", Op: "eq", Value: "200"}},
	}}
	v, err := svc.Save(context.Background(), 1, in)
	require.NoError(t, err)

	// 直接读库里的 JSON，确认定义完整落盘
	var doc detailDoc
	require.NoError(t, json.Unmarshal(repo.data[v.ID].Steps, &doc))
	require.Equal(t, "http://127.0.0.1:8080", doc.BaseURL)
	require.Len(t, doc.Steps, 1)
	require.Equal(t, "data.id", doc.Steps[0].Extracts["id"])
	require.Equal(t, "login", doc.Steps[0].Auth)
	require.Equal(t, "wechat_miniprogram", doc.Auths["wx"].Type)
}
