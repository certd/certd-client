package store

import (
	"github.com/certd/certd-client/internal/store/repo"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// OpenDatabase opens a SQLite database and migrates all store models.
func OpenDatabase(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&repo.TargetApp{}, &repo.AppSite{}, &repo.Setting{}); err != nil {
		return nil, err
	}
	return db, nil
}
