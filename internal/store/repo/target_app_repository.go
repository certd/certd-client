package repo

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gorm.io/gorm"
)

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
			if errors.Is(err, gorm.ErrRecordNotFound) && runtime.GOOS == "windows" {
				var candidates []TargetApp
				if findErr := tx.Find(&candidates).Error; findErr != nil {
					return findErr
				}
				for _, candidate := range candidates {
					if sameWindowsPath(candidate.RootDir, app.RootDir) {
						existing = candidate
						err = nil
						break
					}
				}
			}
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
		Select("target_app.*, COALESCE(site_counts.site_count, 0) AS site_count, COALESCE(site_counts.https_site_count, 0) AS https_site_count, COALESCE(site_counts.synced_site_count, 0) AS synced_site_count, COALESCE(site_counts.failed_site_count, 0) AS failed_site_count").
		Joins("LEFT JOIN (SELECT app_id, SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END) AS site_count, SUM(CASE WHEN enabled = 1 AND https = 1 THEN 1 ELSE 0 END) AS https_site_count, SUM(CASE WHEN enabled = 1 AND sync_status = 'synced' THEN 1 ELSE 0 END) AS synced_site_count, SUM(CASE WHEN enabled = 1 AND sync_status = 'failed' THEN 1 ELSE 0 END) AS failed_site_count FROM app_site GROUP BY app_id) AS site_counts ON site_counts.app_id = target_app.id").
		Order("target_app.added_at DESC").
		Order("target_app.id DESC").
		Find(&apps).Error
	return apps, err
}

func sameWindowsPath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
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

// DeleteApp removes one application and all of its scanned site records.
func (r *TargetAppRepository) DeleteApp(appId uint) (int, error) {
	deletedSites := 0
	err := r.db.Transaction(func(tx *gorm.DB) error {
		siteRepository := NewAppSiteRepository(tx)
		count, err := siteRepository.deleteByApp(tx, appId)
		if err != nil {
			return err
		}
		appResult := tx.Where("id = ?", appId).Delete(&TargetApp{})
		if appResult.Error != nil {
			return appResult.Error
		}
		if appResult.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		deletedSites = int(count)
		return nil
	})
	return deletedSites, err
}
