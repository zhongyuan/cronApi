package store

import (
	"gorm.io/gorm"

	"cronApi/internal/model"
)

// AutoMigrate 自动建表（生产建议改用版本化 SQL 迁移）。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&model.User{}, &model.Task{}, &model.Run{})
}
