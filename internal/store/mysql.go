// Package store 负责第三方存储的连接与初始化（MySQL/Redis）。
package store

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"cronApi/internal/config"
	"cronApi/internal/logger"
)

// NewMySQL 建立 GORM 连接并配置连接池与慢日志。
func NewMySQL(cfg *config.Config) (*gorm.DB, error) {
	sqlCfg := gormlogger.Config{
		SlowThreshold:             cfg.MySQL.SlowThreshold,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	}
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN()), &gorm.Config{
		Logger:  gormlogger.New(gormWriter{}, sqlCfg),
		NowFunc: func() time.Time { return time.Now().Local() },
	})
	if err != nil {
		return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(cfg.MySQL.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MySQL.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.MySQL.ConnMaxLifetime)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("MySQL ping 失败: %w", err)
	}
	return db, nil
}

// gormWriter 把 GORM 日志桥接到 zap。
type gormWriter struct{}

func (gormWriter) Printf(format string, args ...any) {
	logger.L().Warn(fmt.Sprintf(format, args...), zap.String("component", "gorm"))
}
