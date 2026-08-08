package store

import "time"

// TargetApp records an application installation selected by the user.
type TargetApp struct {
	ID             uint      `gorm:"primaryKey"`
	RootDir        string    `gorm:"size:1024;not null;uniqueIndex:idx_target_app_root"`
	AppType        string    `gorm:"size:64;not null"`
	AddedAt        time.Time `gorm:"not null;index"`
	UpdatedAt      time.Time `gorm:"not null"`
	Enabled        bool      `gorm:"not null;default:true;index"`
	SiteCount      int       `gorm:"->;column:site_count;-:migration"`
	HttpsSiteCount int       `gorm:"->;column:https_site_count;-:migration"`
}

func (TargetApp) TableName() string {
	return "target_app"
}
