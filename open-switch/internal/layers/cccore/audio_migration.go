package cccore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"open-switch/internal/ports"
	"open-switch/internal/store/models"
)

// The shared configuration lock keeps concurrent startup instances from
// publishing stale snapshots. History and in-flight calls remain immutable.
func (s *Service) MigrateNarrowbandConfiguration(ctx context.Context) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(?)", configMutationLockID).Error; e != nil {
			return e
		}
		var active models.ActiveConfig
		if e := tx.Where("id = ?", 1).First(&active).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return nil
			}
			return e
		}
		var row models.ConfigVersion
		if e := tx.Where("version = ?", active.Version).First(&row).Error; e != nil {
			return e
		}
		var bundle ports.ConfigBundle
		if e := json.Unmarshal([]byte(row.Payload), &bundle); e != nil {
			return e
		}
		changed := false
		for i := range bundle.Queues {
			q := &bundle.Queues[i]
			if q.AudioProfile == ports.AudioProfileWideband || q.AudioProfile == ports.AudioProfileHDWebRTC {
				q.AudioProfile = ports.AudioProfileNarrowband
				changed = true
			}
		}
		if !changed {
			return nil
		}
		bundle.Version = 0
		normalizeBundle(&bundle)
		if e := validateBundle(bundle); e != nil {
			return e
		}
		raw, e := json.Marshal(bundle)
		if e != nil {
			return e
		}
		sum := sha256.Sum256(raw)
		version, e := s.storeConfigTx(ctx, tx, bundle, hex.EncodeToString(sum[:]))
		if e != nil {
			return e
		}
		_, e = s.activateConfigTx(ctx, tx, version.Version)
		return e
	})
}
