package store

import (
	"github.com/certd/certd-client/internal/store/repo"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

// OpenDatabase opens a SQLite database and migrates all store models.
func OpenDatabase(dsn string) (*gorm.DB, error) {
	// TUI 运行期间不能让 GORM 把多行 SQL 和错误诊断写入终端绘制流。
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormLogger.Default.LogMode(gormLogger.Silent)})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&repo.TargetApp{}, &repo.AppSite{}, &repo.Setting{}); err != nil {
		return nil, err
	}
	return db, nil
}
