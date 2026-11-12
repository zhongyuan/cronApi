// Package repository 数据访问层：定义接口，屏蔽 ORM 细节，方便替换与单测。
package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"cronApi/internal/model"
)

// ErrNotFound 统一「记录不存在」错误，service 层据此转换为业务错误码。
var ErrNotFound = errors.New("repository: record not found")

// UserRepository 用户仓储。
type UserRepository interface {
	FindByOpenID(ctx context.Context, openID string) (*model.User, error)
	FindByID(ctx context.Context, id int64) (*model.User, error)
	Create(ctx context.Context, u *model.User) error
	UpdateProfile(ctx context.Context, id int64, nickname, avatar string) error
	TouchLogin(ctx context.Context, id int64) error
}

// TaskRepository 任务仓储。
type TaskRepository interface {
	Create(ctx context.Context, t *model.Task) error
	Save(ctx context.Context, t *model.Task) error
	Update(ctx context.Context, t *model.Task) error
	Delete(ctx context.Context, id int64) error
	FindByID(ctx context.Context, id int64) (*model.Task, error)
	List(ctx context.Context, ownerID int64, page, size int) ([]*model.Task, int64, error)
	ListEnabled(ctx context.Context) ([]*model.Task, error)
	MarkRun(ctx context.Context, id int64, at any) error
}

// RunRepository 执行记录仓储。
type RunRepository interface {
	Create(ctx context.Context, r *model.Run) error
	Update(ctx context.Context, r *model.Run) error
	FindByID(ctx context.Context, id int64) (*model.Run, error)
	ListByTask(ctx context.Context, taskID int64, page, size int) ([]*model.Run, int64, error)
}

// Repos 聚合所有仓储。
type Repos struct {
	User UserRepository
	Task TaskRepository
	Run  RunRepository
}

// New 用同一 DB 句柄构造全部仓储。
func New(db *gorm.DB) *Repos {
	return &Repos{
		User: &userRepo{db: db},
		Task: &taskRepo{db: db},
		Run:  &runRepo{db: db},
	}
}

type userRepo struct{ db *gorm.DB }

func (r *userRepo) FindByOpenID(ctx context.Context, openID string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("open_id = ?", openID).First(&u).Error
	return &u, wrap(err)
}

func (r *userRepo) FindByID(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).First(&u, id).Error
	return &u, wrap(err)
}

func (r *userRepo) Create(ctx context.Context, u *model.User) error {
	return wrap(r.db.WithContext(ctx).Create(u).Error)
}

func (r *userRepo) UpdateProfile(ctx context.Context, id int64, nickname, avatar string) error {
	return wrap(r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
		Updates(map[string]any{"nickname": nickname, "avatar_url": avatar}).Error)
}

func (r *userRepo) TouchLogin(ctx context.Context, id int64) error {
	return wrap(r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
		UpdateColumn("last_login", gorm.Expr("NOW()")).Error)
}

type taskRepo struct{ db *gorm.DB }

func (r *taskRepo) Create(ctx context.Context, t *model.Task) error {
	return wrap(r.db.WithContext(ctx).Create(t).Error)
}

func (r *taskRepo) Save(ctx context.Context, t *model.Task) error {
	return wrap(r.db.WithContext(ctx).Save(t).Error)
}

func (r *taskRepo) Update(ctx context.Context, t *model.Task) error {
	return wrap(r.db.WithContext(ctx).Model(t).Select("*").Omit("created_at").Updates(t).Error)
}

func (r *taskRepo) Delete(ctx context.Context, id int64) error {
	return wrap(r.db.WithContext(ctx).Delete(&model.Task{}, id).Error)
}

func (r *taskRepo) FindByID(ctx context.Context, id int64) (*model.Task, error) {
	var t model.Task
	err := r.db.WithContext(ctx).First(&t, id).Error
	return &t, wrap(err)
}

func (r *taskRepo) List(ctx context.Context, ownerID int64, page, size int) ([]*model.Task, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Task{})
	if ownerID > 0 {
		q = q.Where("owner_id = ?", ownerID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, wrap(err)
	}
	var list []*model.Task
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, wrap(err)
}

func (r *taskRepo) ListEnabled(ctx context.Context) ([]*model.Task, error) {
	var list []*model.Task
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Find(&list).Error
	return list, wrap(err)
}

func (r *taskRepo) MarkRun(ctx context.Context, id int64, at any) error {
	return wrap(r.db.WithContext(ctx).Model(&model.Task{}).Where("id = ?", id).
		Update("last_run_at", at).Error)
}

type runRepo struct{ db *gorm.DB }

func (r *runRepo) Create(ctx context.Context, run *model.Run) error {
	return wrap(r.db.WithContext(ctx).Create(run).Error)
}

func (r *runRepo) Update(ctx context.Context, run *model.Run) error {
	return wrap(r.db.WithContext(ctx).Model(run).Select(
		"status", "passed_steps", "duration_ms", "error_msg", "steps", "variables", "finished_at",
	).Updates(run).Error)
}

func (r *runRepo) FindByID(ctx context.Context, id int64) (*model.Run, error) {
	var run model.Run
	err := r.db.WithContext(ctx).First(&run, id).Error
	return &run, wrap(err)
}

func (r *runRepo) ListByTask(ctx context.Context, taskID int64, page, size int) ([]*model.Run, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Run{}).Where("task_id = ?", taskID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, wrap(err)
	}
	var list []*model.Run
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, wrap(err)
}

func wrap(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
