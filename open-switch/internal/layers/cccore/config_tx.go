package cccore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/store/models"
)

func (s *Service) storeConfigTx(ctx context.Context, tx *gorm.DB, bundle ports.ConfigBundle, checksum string) (models.ConfigVersion, error) {
	var result models.ConfigVersion
	if err := tx.Where("checksum = ?", checksum).First(&result).Error; err == nil {
		return result, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ConfigVersion{}, err
	}
	version := bundle.Version
	if version <= 0 {
		if err := tx.Raw("SELECT COALESCE(MAX(version), 0) + 1 FROM os_config_versions").Scan(&version).Error; err != nil {
			return models.ConfigVersion{}, err
		}
	}
	var exists int64
	if err := tx.Model(&models.ConfigVersion{}).Where("version = ?", version).Count(&exists).Error; err != nil {
		return models.ConfigVersion{}, err
	}
	if exists != 0 {
		return models.ConfigVersion{}, errs.Conflict("配置版本已存在", "")
	}
	bundle.Version = version
	payload, err := json.Marshal(bundle)
	if err != nil {
		return models.ConfigVersion{}, err
	}
	now := time.Now().UTC()
	result = models.ConfigVersion{Version: version, Status: "validated", Checksum: checksum, Payload: string(payload), CreatedAt: now}
	if err := tx.Create(&result).Error; err != nil {
		return models.ConfigVersion{}, err
	}
	if err := insertBundle(tx, version, bundle, now); err != nil {
		return models.ConfigVersion{}, err
	}
	return result, nil
}

func (s *Service) activateConfigTx(ctx context.Context, tx *gorm.DB, version int64) (models.ConfigVersion, error) {
	var result models.ConfigVersion
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("version = ?", version).First(&result).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.ConfigVersion{}, errs.NotFound("配置版本不存在")
		}
		return models.ConfigVersion{}, err
	}
	now := time.Now().UTC()
	if err := tx.Model(&models.ConfigVersion{}).Where("status = ?", "active").Update("status", "superseded").Error; err != nil {
		return models.ConfigVersion{}, err
	}
	result.Status = "active"
	result.ActivatedAt = &now
	if err := tx.Model(&models.ConfigVersion{}).Where("version = ?", version).Updates(map[string]any{"status": "active", "activated_at": now}).Error; err != nil {
		return models.ConfigVersion{}, err
	}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.Assignments(map[string]any{"version": version, "activated_at": now})}).Create(&models.ActiveConfig{ID: 1, Version: version, ActivatedAt: now}).Error; err != nil {
		return models.ConfigVersion{}, err
	}
	return result, nil
}
