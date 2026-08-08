package store

import "time"

// AppSite records a website discovered from an application's configuration.
type AppSite struct {
	ID             uint      `gorm:"primaryKey"`
	PrimaryDomain  string    `gorm:"size:255;not null;uniqueIndex:idx_app_site_identity"`
	SubdomainCount int       `gorm:"not null"`
	ConfigPath     string    `gorm:"size:1024;not null;uniqueIndex:idx_app_site_identity"`
	AppId          uint      `gorm:"column:app_id;not null;index;uniqueIndex:idx_app_site_identity"`
	Https          bool      `gorm:"column:https;not null"`
	ScannedAt      time.Time `gorm:"not null;index"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (AppSite) TableName() string {
	return "app_site"
}
