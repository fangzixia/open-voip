package cccore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"gorm.io/gorm"

	"open-switch/internal/ports"
)

// configMutationLockID 覆盖 load→store→activate 整段，与 Store/Activate 同锁，避免并发 CRUD 丢更新。
const configMutationLockID int64 = 67104232

// ApplyConfigMutation 读取当前激活配置（或空配置）、应用变更、校验并激活新版本。
func (s *Service) ApplyConfigMutation(ctx context.Context, mutate func(*ports.ConfigBundle) error) (ports.ConfigVersionView, error) {
	bundle, err := s.loadActiveBundleOrEmpty(ctx)
	if err != nil {
		return ports.ConfigVersionView{}, err
	}
	if mutate != nil {
		if err := mutate(&bundle); err != nil {
			return ports.ConfigVersionView{}, err
		}
	}
	bundle.Version = 0
	var stored ports.ConfigVersionView
	var activated ports.ConfigVersionView
	normalizeBundle(&bundle)
	if err := validateBundle(bundle); err != nil {
		return ports.ConfigVersionView{}, err
	}
	hashInput := bundle
	hashInput.Version = 0
	rawForHash, _ := json.Marshal(hashInput)
	sum := sha256.Sum256(rawForHash)
	checksum := hex.EncodeToString(sum[:])

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", configMutationLockID).Error; err != nil {
			return err
		}
		row, err := s.storeConfigTx(ctx, tx, bundle, checksum)
		if err != nil {
			return err
		}
		stored = configView(row)
		row, err = s.activateConfigTx(ctx, tx, stored.Version)
		if err != nil {
			return err
		}
		activated = configView(row)
		return nil
	})
	if err != nil {
		return ports.ConfigVersionView{}, err
	}
	return activated, nil
}

func findQueueIndex(bundle *ports.ConfigBundle, id string) int {
	for i, q := range bundle.Queues {
		if q.ID == id {
			return i
		}
	}
	return -1
}

func findSkillIndex(bundle *ports.ConfigBundle, id string) int {
	for i, sk := range bundle.Skills {
		if sk.ID == id {
			return i
		}
	}
	return -1
}

func findAgentIndex(bundle *ports.ConfigBundle, id string) int {
	for i, a := range bundle.Agents {
		if a.ID == id {
			return i
		}
	}
	return -1
}

func findDIDIndex(bundle *ports.ConfigBundle, id string) int {
	for i, d := range bundle.DIDs {
		if d.ID == id {
			return i
		}
	}
	return -1
}

func removeQueue(bundle *ports.ConfigBundle, id string) {
	out := bundle.Queues[:0]
	for _, q := range bundle.Queues {
		if q.ID != id {
			out = append(out, q)
		}
	}
	bundle.Queues = out
}

func removeSkill(bundle *ports.ConfigBundle, id string) {
	out := bundle.Skills[:0]
	for _, sk := range bundle.Skills {
		if sk.ID != id {
			out = append(out, sk)
		}
	}
	bundle.Skills = out
}

func removeAgent(bundle *ports.ConfigBundle, id string) {
	out := bundle.Agents[:0]
	for _, a := range bundle.Agents {
		if a.ID != id {
			out = append(out, a)
		}
	}
	bundle.Agents = out
}

func removeDID(bundle *ports.ConfigBundle, id string) {
	out := bundle.DIDs[:0]
	for _, d := range bundle.DIDs {
		if d.ID != id {
			out = append(out, d)
		}
	}
	bundle.DIDs = out
}
