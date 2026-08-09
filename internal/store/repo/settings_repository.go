package repo

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

type SettingsRepository struct {
	db *gorm.DB
}

func NewSettingsRepository(db *gorm.DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) SaveSetting(key, value string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing Setting
		err := tx.Where("key = ?", key).First(&existing).Error
		switch {
		case err == nil:
			return tx.Model(&existing).Updates(map[string]any{
				"setting": value,
			}).Error
		case errors.Is(err, gorm.ErrRecordNotFound):
			now := time.Now()
			return tx.Create(&Setting{Key: key, Setting: value, CreatedAt: now, UpdatedAt: now}).Error
		default:
			return err
		}
	})
}

func (r *SettingsRepository) GetSetting(key string) (string, error) {
	var setting Setting
	err := r.db.Where("key = ?", key).First(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return setting.Setting, nil
}
