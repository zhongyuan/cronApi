package model

import "time"

// RunStatus 执行状态。
type RunStatus string

const (
	RunStatusRunning RunStatus = "running"
	RunStatusSuccess RunStatus = "success"
	RunStatusFailed  RunStatus = "failed"
	RunStatusTimeout RunStatus = "timeout"
	RunStatusSkipped RunStatus = "skipped" // 上一轮仍在执行
)

// Run 一次任务执行记录（联调报告）。
type Run struct {
	ID          int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskID      int64      `gorm:"not null;index:idx_task_time,priority:1;comment:所属任务" json:"task_id"`
	TraceID     string     `gorm:"size:64;default:'';index:idx_trace_id;comment:链路 ID" json:"trace_id"`
	Status      RunStatus  `gorm:"size:16;not null;index:idx_status;comment:running|success|failed|timeout" json:"status"`
	Trigger     string     `gorm:"size:16;not null;default:manual;comment:schedule|manual|api" json:"trigger"`
	TotalSteps  int        `gorm:"not null;default:0;comment:总步骤数" json:"total_steps"`
	PassedSteps int        `gorm:"not null;default:0;comment:通过步骤数" json:"passed_steps"`
	DurationMS  int64      `gorm:"not null;default:0;comment:总耗时(毫秒)" json:"duration_ms"`
	ErrorMsg    string     `gorm:"size:1024;default:'';comment:失败原因" json:"error_msg"`
	Steps       JSONSteps  `gorm:"type:json;comment:逐步结果明细" json:"steps"`
	Variables   JSONMap    `gorm:"type:json;comment:执行时的变量快照" json:"variables"`
	StartedAt   time.Time  `gorm:"not null;index:idx_task_time,priority:2;comment:开始时间" json:"started_at"`
	FinishedAt  *time.Time `gorm:"comment:结束时间" json:"finished_at"`
}

// TableName 表名。
func (Run) TableName() string { return "cron_run" }
