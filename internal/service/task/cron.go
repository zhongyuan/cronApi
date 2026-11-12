package task

import (
	"time"

	"github.com/robfig/cron/v3"

	"cronApi/internal/errcode"
)

// 支持 5 段（分钟级）与 6 段（秒级）两种 cron 表达式，并内置 @every 等描述符。
var parser = cron.NewParser(
	cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// ValidateCron 校验表达式。
func validateCron(expr string) error {
	if expr == "" {
		return errcode.Newf(errcode.CronExprInvalid, "cron 表达式不能为空")
	}
	if _, err := parser.Parse(expr); err != nil {
		return errcode.Wrapf(errcode.CronExprInvalid, err, "cron 表达式非法: %s", expr)
	}
	return nil
}

// NextRun 返回下一次执行时间，用于前端展示。
func NextRun(expr, tz string, from time.Time) (time.Time, error) {
	loc := time.Local
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	sched, err := parser.Parse(expr)
	if err != nil {
		return time.Time{}, errcode.Wrap(errcode.CronExprInvalid, err)
	}
	return sched.Next(from.In(loc)), nil
}
