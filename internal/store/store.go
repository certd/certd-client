package store

import (
	"errors"
	"os"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// OpenDatabase opens a SQLite database and creates the target_app table.
func OpenDatabase(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&TargetApp{}, &AppSite{}); err != nil {
		return nil, err
	}
	return db, nil
}

type TargetAppRepository struct {
	db *gorm.DB
}

func NewTargetAppRepository(db *gorm.DB) *TargetAppRepository {
	return &TargetAppRepository{db: db}
}

func (r *TargetAppRepository) Add(apps []TargetApp) error {
	if len(apps) == 0 {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, app := range apps {
			var existing TargetApp
			err := tx.Where("root_dir = ?", app.RootDir).First(&existing).Error
			switch {
			case err == nil:
				if err := tx.Model(&existing).Updates(map[string]any{
					"app_type":   app.AppType,
					"enabled":    true,
					"updated_at": time.Now(),
				}).Error; err != nil {
					return err
				}
			case errors.Is(err, gorm.ErrRecordNotFound):
				app.Enabled = true
				if app.AddedAt.IsZero() {
					app.AddedAt = time.Now()
				}
				if app.UpdatedAt.IsZero() {
					app.UpdatedAt = app.AddedAt
				}
				if err := tx.Create(&app).Error; err != nil {
					return err
				}
			default:
				return err
			}
		}
		return nil
	})
}

func (r *TargetAppRepository) List() ([]TargetApp, error) {
	var apps []TargetApp
	err := r.db.Model(&TargetApp{}).
		Select("target_app.*, COALESCE(site_counts.site_count, 0) AS site_count, COALESCE(site_counts.https_site_count, 0) AS https_site_count").
		Joins("LEFT JOIN (SELECT app_id, COUNT(*) AS site_count, SUM(CASE WHEN https THEN 1 ELSE 0 END) AS https_site_count FROM app_site GROUP BY app_id) AS site_counts ON site_counts.app_id = target_app.id").
		Order("target_app.added_at DESC").
		Order("target_app.id DESC").
		Find(&apps).Error
	return apps, err
}

// DisableMissingApps disables active applications whose root directories no
// longer exist or are no longer directories.
func (r *TargetAppRepository) DisableMissingApps() ([]TargetApp, error) {
	var activeApps []TargetApp
	if err := r.db.Where("enabled = ?", true).Find(&activeApps).Error; err != nil {
		return nil, err
	}
	disabled := make([]TargetApp, 0)
	for _, app := range activeApps {
		info, err := os.Stat(app.RootDir)
		if err == nil && info.IsDir() {
			continue
		}
		if err != nil && !os.IsNotExist(err) {
			continue
		}
		app.Enabled = false
		disabled = append(disabled, app)
	}
	if len(disabled) == 0 {
		return disabled, nil
	}
	if err := r.db.Transaction(func(tx *gorm.DB) error {
		for _, app := range disabled {
			if err := tx.Model(&TargetApp{}).Where("id = ?", app.ID).Updates(map[string]any{
				"enabled":    false,
				"updated_at": time.Now(),
			}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return disabled, nil
}

// SyncSites replaces the latest scan result for one application.
func (r *TargetAppRepository) SyncSites(appId uint, sites []AppSite) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("app_id = ?", appId).Delete(&AppSite{}).Error; err != nil {
			return err
		}
		if len(sites) == 0 {
			return nil
		}
		now := time.Now()
		for i := range sites {
			sites[i].AppId = appId
			sites[i].ScannedAt = now
			sites[i].CreatedAt = now
			sites[i].UpdatedAt = now
		}
		return tx.Create(&sites).Error
	})
}

func (r *TargetAppRepository) ListSites(appId uint) ([]AppSite, error) {
	var sites []AppSite
	err := r.db.Where("app_id = ?", appId).Order("primary_domain").Order("config_path").Find(&sites).Error
	return sites, err
}

// DeleteApp removes one application and all of its scanned site records.
func (r *TargetAppRepository) DeleteApp(appId uint) (int, error) {
	deletedSites := 0
	err := r.db.Transaction(func(tx *gorm.DB) error {
		siteResult := tx.Where("app_id = ?", appId).Delete(&AppSite{})
		if siteResult.Error != nil {
			return siteResult.Error
		}
		appResult := tx.Where("id = ?", appId).Delete(&TargetApp{})
		if appResult.Error != nil {
			return appResult.Error
		}
		if appResult.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		deletedSites = int(siteResult.RowsAffected)
		return nil
	})
	return deletedSites, err
}
