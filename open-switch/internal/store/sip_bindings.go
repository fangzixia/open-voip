package store

import (
	"context"
	"time"

	"gorm.io/gorm"

	"open-switch/internal/ports"
)

type sipBindingRow struct {
	AOR        string    `gorm:"column:aor;primaryKey"`
	ContactURI string    `gorm:"column:contact_uri;primaryKey"`
	CallID     string    `gorm:"column:call_id"`
	CSeq       int64     `gorm:"column:cseq"`
	Addr       string    `gorm:"column:addr"`
	ExpiresAt  time.Time `gorm:"column:expires_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (sipBindingRow) TableName() string { return "os_sip_bindings" }

// SIPBindingStore 实现 ports.SIPBindingStore。
type SIPBindingStore struct {
	db *gorm.DB
}

// NewSIPBindingStore 创建注册绑定存储。
func NewSIPBindingStore(db *gorm.DB) *SIPBindingStore { return &SIPBindingStore{db: db} }

var _ ports.SIPBindingStore = (*SIPBindingStore)(nil)

// LoadSIPBindings 读取未过期绑定，并顺带清理已过期的行。
func (s *SIPBindingStore) LoadSIPBindings(ctx context.Context) ([]ports.SIPBinding, error) {
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Where("expires_at <= ?", now).Delete(&sipBindingRow{}).Error; err != nil {
		return nil, err
	}
	var rows []sipBindingRow
	if err := s.db.WithContext(ctx).Order("aor, updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ports.SIPBinding, 0, len(rows))
	for _, r := range rows {
		out = append(out, ports.SIPBinding{AOR: r.AOR, ContactURI: r.ContactURI, CallID: r.CallID, CSeq: uint32(r.CSeq), Addr: r.Addr, ExpiresAt: r.ExpiresAt})
	}
	return out, nil
}

// ReplaceSIPBindings 在事务内整体替换某 AOR 的绑定。
func (s *SIPBindingStore) ReplaceSIPBindings(ctx context.Context, aor string, bindings []ports.SIPBinding) error {
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("aor = ?", aor).Delete(&sipBindingRow{}).Error; err != nil {
			return err
		}
		for i, b := range bindings {
			// 越靠前的绑定越优先，用递减的 updated_at 保留顺序。
			row := sipBindingRow{AOR: aor, ContactURI: b.ContactURI, CallID: b.CallID, CSeq: int64(b.CSeq), Addr: b.Addr, ExpiresAt: b.ExpiresAt.UTC(), UpdatedAt: now.Add(-time.Duration(i) * time.Millisecond)}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
