package repo

import "time"

// AppSite records a website discovered from an application's configuration.
type AppSite struct {
	ID              uint      `gorm:"primaryKey"`
	PrimaryDomain   string    `gorm:"size:255;not null;uniqueIndex:idx_app_site_identity"`
	Domains         string    `gorm:"size:4096;not null;default:''"`
	SubdomainCount  int       `gorm:"not null"`
	ConfigPath      string    `gorm:"size:1024;not null;uniqueIndex:idx_app_site_identity"`
	CertificatePath string    `gorm:"size:1024;not null;default:''"`
	PrivateKeyPath  string    `gorm:"size:1024;not null;default:''"`
	DeploymentName  string    `gorm:"size:255;not null;default:''"`
	AppId           uint      `gorm:"column:app_id;not null;index;uniqueIndex:idx_app_site_identity"`
	Https           bool      `gorm:"column:https;not null"`
	Enabled         bool      `gorm:"not null;default:true;index"`
	ScannedAt       time.Time `gorm:"not null;index"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
	SyncStatus      string    `gorm:"size:32;not null;default:'';index"`
	SyncError       string    `gorm:"size:2048;not null;default:''"`
}

func (AppSite) TableName() string {
	return "app_site"
}
