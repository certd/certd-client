package repo

import (
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gorm.io/gorm"
)

type AppSiteRepository struct {
	db *gorm.DB
}

func NewAppSiteRepository(db *gorm.DB) *AppSiteRepository {
	return &AppSiteRepository{db: db}
}

// SyncSites replaces the latest scan result for one application.
func (r *AppSiteRepository) SyncSites(appId uint, sites []AppSite) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing []AppSite
		if err := tx.Where("app_id = ?", appId).Find(&existing).Error; err != nil {
			return err
		}
		existingEnabled := make(map[string]bool, len(existing))
		for _, site := range existing {
			existingEnabled[appSiteIdentity(site.PrimaryDomain, site.ConfigPath)] = site.Enabled
		}
		if _, err := r.deleteByApp(tx, appId); err != nil {
			return err
		}
		sites = deduplicateAppSites(sites)
		if len(sites) == 0 {
			return nil
		}
		now := time.Now()
		disabledIndexes := make([]int, 0)
		for i := range sites {
			sites[i].AppId = appId
			sites[i].Enabled = true
			if enabled, found := existingEnabled[appSiteIdentity(sites[i].PrimaryDomain, sites[i].ConfigPath)]; found {
				sites[i].Enabled = enabled
			}
			if !sites[i].Enabled {
				disabledIndexes = append(disabledIndexes, i)
			}
			sites[i].ScannedAt = now
			sites[i].CreatedAt = now
			sites[i].UpdatedAt = now
		}
		if err := tx.Select("*").Create(&sites).Error; err != nil {
			return err
		}
		for _, index := range disabledIndexes {
			if err := tx.Model(&AppSite{}).Where("id = ?", sites[index].ID).Update("enabled", false).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *AppSiteRepository) ListEnabledSites(appId uint) ([]AppSite, error) {
	var sites []AppSite
	err := r.db.Where("app_id = ? AND enabled = ?", appId, true).Order("primary_domain").Order("config_path").Find(&sites).Error
	return sites, err
}

func (r *AppSiteRepository) ListSites(appId uint) ([]AppSite, error) {
	var sites []AppSite
	err := r.db.Where("app_id = ?", appId).Order("primary_domain").Order("config_path").Find(&sites).Error
	return sites, err
}

func (r *AppSiteRepository) UpdateSyncStatus(siteId uint, status, syncError string) error {
	return r.db.Model(&AppSite{}).Where("id = ?", siteId).Updates(map[string]any{
		"sync_status": status,
		"sync_error":  syncError,
		"updated_at":  time.Now(),
	}).Error
}

func (r *AppSiteRepository) SetEnabled(siteId uint, enabled bool) error {
	result := r.db.Model(&AppSite{}).Where("id = ?", siteId).Updates(map[string]any{
		"enabled":    enabled,
		"updated_at": time.Now(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func appSiteIdentity(primaryDomain, configPath string) string {
	if runtime.GOOS == "windows" {
		configPath = strings.ToLower(filepath.Clean(configPath))
	}
	return primaryDomain + "\x00" + configPath
}

func deduplicateAppSites(sites []AppSite) []AppSite {
	result := make([]AppSite, 0, len(sites))
	indexes := make(map[string]int, len(sites))
	for _, site := range sites {
		key := appSiteIdentity(site.PrimaryDomain, site.ConfigPath)
		if index, found := indexes[key]; found {
			result[index] = mergeAppSite(result[index], site)
			continue
		}
		indexes[key] = len(result)
		result = append(result, site)
	}
	return result
}

func mergeAppSite(current, incoming AppSite) AppSite {
	if incoming.Https {
		current.Https = true
	}
	if incoming.Domains != "" {
		current.Domains = incoming.Domains
	}
	if incoming.SubdomainCount > current.SubdomainCount {
		current.SubdomainCount = incoming.SubdomainCount
	}
	if incoming.CertificatePath != "" {
		current.CertificatePath = incoming.CertificatePath
	}
	if incoming.PrivateKeyPath != "" {
		current.PrivateKeyPath = incoming.PrivateKeyPath
	}
	if incoming.DeploymentName != "" {
		current.DeploymentName = incoming.DeploymentName
	}
	return current
}

func (r *AppSiteRepository) deleteByApp(tx *gorm.DB, appId uint) (int64, error) {
	result := tx.Where("app_id = ?", appId).Delete(&AppSite{})
	return result.RowsAffected, result.Error
}
