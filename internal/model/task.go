package model

import "time"

// TaskStatus 任务状态。
type TaskStatus int

const (
	TaskStatusDisabled TaskStatus = 0
	TaskStatusEnabled  TaskStatus = 1
)

// Task 一个「自动联调」任务：按 cron 定时或手动触发，按步骤调用目标接口并断言。
type Task struct {
	ID          int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string     `gorm:"size:128;not null" json:"name"`
	OwnerID     int64      `gorm:"index:idx_owner" json:"owner_id"`
	Description string     `gorm:"size:512" json:"description"`
	CronExpr    string     `gorm:"size:64;not null;comment:robfig cron 表达式(秒级可选)" json:"cron_expr"`
	Timezone    string     `gorm:"size:64;not null;default:Asia/Shanghai" json:"timezone"`
	Enabled     bool       `gorm:"not null;default:true;index:idx_enabled;comment:是否启用定时执行" json:"enabled"`
	Steps       JSONSteps  `gorm:"type:json;not null;comment:联调步骤(JSON)" json:"steps"`
	Variables   JSONMap    `gorm:"type:json;comment:默认变量(JSON)" json:"variables"`
	MaxRetry    int        `gorm:"not null;default:0;comment:步骤级重试次数" json:"max_retry"`
	TimeoutSec  int        `gorm:"not null;default:60;comment:整任务超时秒数" json:"timeout_sec"`
	NotifyURL   string     `gorm:"size:512;default:'';comment:失败回调 webhook" json:"notify_url"`
	LastRunAt   *time.Time `gorm:"comment:上次执行时间" json:"last_run_at"`
	CreatedAt   time.Time  `gorm:"comment:创建时间" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"comment:更新时间" json:"updated_at"`
}

// TableName 表名。
func (Task) TableName() string { return "cron_task" }
