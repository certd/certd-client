package repo

import "time"

// Setting stores a JSON document for one named client setting.
type Setting struct {
	Key       string    `gorm:"size:255;primaryKey"`
	Setting   string    `gorm:"type:longtext;not null"`
	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

func (Setting) TableName() string {
	return "settings"
}
