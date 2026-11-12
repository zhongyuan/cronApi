package model

import "time"

// User 业务用户。小程序用户以 OpenID 为唯一身份。
type User struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	OpenID    string    `gorm:"size:64;uniqueIndex:uk_openid;not null;comment:小程序 openid" json:"open_id"`
	UnionID   string    `gorm:"size:64;index:idx_unionid;default:'';comment:开放平台 unionid" json:"union_id,omitempty"`
	Nickname  string    `gorm:"size:64;default:''" json:"nickname"`
	AvatarURL string    `gorm:"size:512;default:''" json:"avatar_url"`
	Status    int16     `gorm:"type:smallint;not null;default:1;comment:1 正常 0 禁用" json:"status"`
	LastLogin time.Time `gorm:"comment:最近登录时间" json:"last_login_at"`
	CreatedAt time.Time `gorm:"comment:创建时间" json:"created_at"`
	UpdatedAt time.Time `gorm:"comment:更新时间" json:"updated_at"`
}

// TableName 表名。
func (User) TableName() string { return "sys_user" }

// 用户状态
const (
	UserStatusDisabled = 0
	UserStatusActive   = 1
)
