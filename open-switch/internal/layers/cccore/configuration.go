// Package cccore 负责 DID 路由、不可变配置快照、队列、ACD 与坐席路由状态。
package cccore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"open-switch/internal/errs"
	"open-switch/internal/ports"
	"open-switch/internal/store/models"
)

// Options 非业务类的技术默认值（录音模式、保留天数等）。
type Options struct {
	RecordingMode string
	NotifyMessage string
	RetainDays    int
}

// Service 是单个 Switch 数据库上的呼叫中心权威运行时。
type Service struct {
	db      *gorm.DB
	events  ports.CallEventPublisher
	options Options
}

func New(db *gorm.DB, events ports.CallEventPublisher, options Options) *Service {
	if options.RecordingMode == "" {
		options.RecordingMode = "audio"
	}
	if options.RetainDays <= 0 {
		options.RetainDays = 90
	}
	return &Service{db: db, events: events, options: options}
}

var _ ports.CallCenterAdminPort = (*Service)(nil)
var _ ports.ConfigSnapshotPort = (*Service)(nil)
var _ ports.ACDDispatchPort = (*Service)(nil)
var _ ports.AgentDirectoryPort = (*Service)(nil)
var _ ports.RecordingPolicyPort = (*Service)(nil)
var _ ports.CDRRecorderPort = (*Service)(nil)
var _ ports.RecordingStorePort = (*Service)(nil)

// applicationID 兼容旧调用点；单租户下恒为空。
func applicationID(ctx context.Context) (string, error) {
	_ = ctx
	return "", nil
}

// StoreConfig 校验并存储一份不可变配置快照，但不激活。
func (s *Service) StoreConfig(ctx context.Context, bundle ports.ConfigBundle) (ports.ConfigVersionView, error) {
	appID, _ := applicationID(ctx)
	_ = appID
	normalizeBundle(&bundle)
	if err := validateBundle(bundle); err != nil {
		return ports.ConfigVersionView{}, err
	}
	hashInput := bundle
	hashInput.Version = 0
	rawForHash, _ := json.Marshal(hashInput)
	sum := sha256.Sum256(rawForHash)
	checksum := hex.EncodeToString(sum[:])

	var result models.ConfigVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "config").Error; err != nil {
			return err
		}
		if err := tx.Where("checksum = ?", checksum).First(&result).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		version := bundle.Version
		if version <= 0 {
			if err := tx.Raw("SELECT COALESCE(MAX(version), 0) + 1 FROM os_config_versions").Scan(&version).Error; err != nil {
				return err
			}
		}
		var exists int64
		if err := tx.Model(&models.ConfigVersion{}).Where("version = ?", version).Count(&exists).Error; err != nil {
			return err
		}
		if exists != 0 {
			return errs.Conflict("配置版本已存在", "")
		}
		bundle.Version = version
		payload, err := json.Marshal(bundle)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		result = models.ConfigVersion{Version: version, Status: "validated", Checksum: checksum, Payload: string(payload), CreatedAt: now}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		return insertBundle(tx, version, bundle, now)
	})
	if err != nil {
		return ports.ConfigVersionView{}, err
	}
	return configView(result), nil
}

// ActivateConfig 原子地将已校验快照设为新通话的权威配置。
func (s *Service) ActivateConfig(ctx context.Context, version int64) (ports.ConfigVersionView, error) {
	var result models.ConfigVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(67104232)").Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("version = ?", version).First(&result).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errs.NotFound("配置版本不存在")
			}
			return err
		}
		if err := validateActiveDIDConflicts(tx, "", version); err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&models.ConfigVersion{}).Where("status = ?", "active").Update("status", "superseded").Error; err != nil {
			return err
		}
		result.Status = "active"
		result.ActivatedAt = &now
		if err := tx.Model(&models.ConfigVersion{}).Where("version = ?", version).Updates(map[string]any{"status": "active", "activated_at": now}).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.Assignments(map[string]any{"version": version, "activated_at": now})}).Create(&models.ActiveConfig{ID: 1, Version: version, ActivatedAt: now}).Error
	})
	if err != nil {
		return ports.ConfigVersionView{}, err
	}
	return configView(result), nil
}

func (s *Service) GetConfigVersion(ctx context.Context, version int64) (ports.ConfigVersionView, error) {
	var row models.ConfigVersion
	if err := s.db.WithContext(ctx).Where("version = ?", version).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.ConfigVersionView{}, errs.NotFound("配置版本不存在")
		}
		return ports.ConfigVersionView{}, err
	}
	return configView(row), nil
}

func configView(row models.ConfigVersion) ports.ConfigVersionView {
	return ports.ConfigVersionView{Version: row.Version, Status: row.Status, Checksum: row.Checksum, CreatedAt: row.CreatedAt, ActivatedAt: row.ActivatedAt}
}

func normalizeBundle(bundle *ports.ConfigBundle) {
	for i := range bundle.Queues {
		q := &bundle.Queues[i]
		q.Name = strings.TrimSpace(q.Name)
		if q.MaxWaitSec <= 0 {
			q.MaxWaitSec = 300
		}
		if q.DispatchStrategy == "" {
			q.DispatchStrategy = "longest_idle"
		}
		if q.RecordingPolicy == "" {
			q.RecordingPolicy = "off"
		}
		if q.OverflowAction == "" {
			q.OverflowAction = "hangup"
		}
		if q.AfterHoursAction == "" {
			q.AfterHoursAction = "hangup"
		}
		if strings.TrimSpace(q.BusinessHoursJSON) == "" {
			q.BusinessHoursJSON = "always"
		}
	}
	for i := range bundle.Agents {
		a := &bundle.Agents[i]
		if a.TerminalType == "" {
			a.TerminalType = "webrtc"
		}
	}
	for i := range bundle.DIDs {
		d := &bundle.DIDs[i]
		if strings.TrimSpace(d.TrunkID) == "" {
			d.TrunkID = "*"
		}
		d.DID = NormalizeDID(d.DID)
		if d.TargetType == "" {
			d.TargetType = "queue"
		}
	}
}

func validateBundle(bundle ports.ConfigBundle) error {
	queues, skills, agents, ivrs := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, q := range bundle.Queues {
		if !validUUID(q.ID) || q.Name == "" {
			return errs.InvalidRequest("队列 id/name 无效")
		}
		if queues[q.ID] {
			return errs.InvalidRequest("队列 ID 重复")
		}
		queues[q.ID] = true
		if q.DispatchStrategy != "longest_idle" && q.DispatchStrategy != "round_robin" {
			return errs.InvalidRequest("队列分配策略无效")
		}
		if q.RecordingPolicy != "off" && q.RecordingPolicy != "audio" && q.RecordingPolicy != "video_composite" {
			return errs.InvalidRequest("录制策略无效")
		}
		if q.OverflowAction != "hangup" && q.OverflowAction != "voicemail" && q.OverflowAction != "queue" {
			return errs.InvalidRequest("溢出动作无效")
		}
		if q.AfterHoursAction != "hangup" && q.AfterHoursAction != "voicemail" && q.AfterHoursAction != "queue" {
			return errs.InvalidRequest("非工作时间动作无效")
		}
		if err := validateHours(q.BusinessHoursJSON); err != nil {
			return err
		}
	}
	for _, skill := range bundle.Skills {
		if !validUUID(skill.ID) || strings.TrimSpace(skill.Name) == "" {
			return errs.InvalidRequest("技能 id/name 无效")
		}
		if skills[skill.ID] {
			return errs.InvalidRequest("技能 ID 重复")
		}
		skills[skill.ID] = true
	}
	extensions, sipUsers := map[string]bool{}, map[string]bool{}
	for _, agent := range bundle.Agents {
		if !validUUID(agent.ID) || strings.TrimSpace(agent.UserRef) == "" || strings.TrimSpace(agent.Extension) == "" {
			return errs.InvalidRequest("坐席 id/user_ref/extension 无效")
		}
		if agents[agent.ID] {
			return errs.InvalidRequest("坐席 ID 重复")
		}
		agents[agent.ID] = true
		if extensions[agent.Extension] {
			return errs.InvalidRequest("分机号重复")
		}
		extensions[agent.Extension] = true
		if agent.SIPUsername != "" {
			if sipUsers[agent.SIPUsername] {
				return errs.InvalidRequest("SIP 账号重复")
			}
			sipUsers[agent.SIPUsername] = true
		}
		if agent.TerminalType != "webrtc" && agent.TerminalType != "sip" {
			return errs.InvalidRequest("坐席终端类型无效")
		}
		if agent.TerminalType == "sip" && strings.TrimSpace(agent.SIPUsername) == "" {
			return errs.InvalidRequest("SIP 坐席必须配置 sip_username")
		}
		for _, id := range agent.SkillIDs {
			if !skills[id] {
				return errs.InvalidRequest("坐席引用不存在的技能")
			}
		}
	}
	for _, ivr := range bundle.IVRs {
		if !validUUID(ivr.FlowID) || ivr.Version <= 0 || !json.Valid([]byte(ivr.PayloadJSON)) {
			return errs.InvalidRequest("IVR 快照无效")
		}
		if ivrs[ivr.FlowID] {
			return errs.InvalidRequest("IVR flow_id 重复")
		}
		if err := validateIVR(ivr.PayloadJSON, queues); err != nil {
			return err
		}
		ivrs[ivr.FlowID] = true
	}
	dids := map[string]bool{}
	for _, did := range bundle.DIDs {
		if !validUUID(did.ID) || did.DID == "" {
			return errs.InvalidRequest("DID id/号码无效")
		}
		key := did.TrunkID + "\x00" + did.DID
		if dids[key] {
			return errs.InvalidRequest("DID 路由重复")
		}
		dids[key] = true
		switch did.TargetType {
		case "queue":
			if !queues[did.TargetID] {
				return errs.InvalidRequest("DID 引用了不存在的队列")
			}
		case "ivr":
			if !ivrs[did.TargetID] {
				return errs.InvalidRequest("DID 引用了不存在的 IVR")
			}
		case "reject":
			if did.TargetID != "" {
				return errs.InvalidRequest("拒绝路由不能包含 target_id")
			}
		default:
			return errs.InvalidRequest("DID target_type 无效")
		}
	}
	for _, q := range bundle.Queues {
		if (q.OverflowAction == "queue" || q.AfterHoursAction == "queue") && q.OverflowQueueID == "" {
			return errs.InvalidRequest("队列跳转必须配置目标队列")
		}
		if q.OverflowQueueID != "" && (!queues[q.OverflowQueueID] || q.OverflowQueueID == q.ID) {
			return errs.InvalidRequest("队列溢出目标无效")
		}
		if q.IVRFlowID != "" && !ivrs[q.IVRFlowID] {
			return errs.InvalidRequest("队列引用不存在的 IVR")
		}
		for _, id := range q.SkillIDs {
			if !skills[id] {
				return errs.InvalidRequest("队列引用不存在的技能")
			}
		}
		for _, id := range q.AgentIDs {
			if !agents[id] {
				return errs.InvalidRequest("队列引用不存在的坐席")
			}
		}
	}
	return nil
}

func validUUID(value string) bool { _, err := uuid.Parse(value); return err == nil }

func insertBundle(tx *gorm.DB, version int64, bundle ports.ConfigBundle, now time.Time) error {
	for _, in := range bundle.Skills {
		if err := tx.Create(&models.Skill{ConfigVersion: version, ID: in.ID, Name: in.Name, CreatedAt: now}).Error; err != nil {
			return err
		}
	}
	for _, in := range bundle.Agents {
		row := models.Agent{ConfigVersion: version, ID: in.ID, UserID: in.UserRef, Extension: in.Extension, DisplayName: in.DisplayName, VideoCapable: in.VideoCapable, TerminalType: in.TerminalType, SIPUsername: in.SIPUsername, Enabled: in.Enabled, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for _, skillID := range in.SkillIDs {
			if err := tx.Create(&models.AgentSkill{ConfigVersion: version, AgentID: in.ID, SkillID: skillID}).Error; err != nil {
				return err
			}
		}
	}
	for _, in := range bundle.IVRs {
		if err := tx.Create(&models.IVRPublishedSnapshot{ConfigVersion: version, FlowID: in.FlowID, Version: in.Version, PayloadJSON: in.PayloadJSON, PublishedAt: now}).Error; err != nil {
			return err
		}
	}
	for _, in := range bundle.Queues {
		row := models.Queue{ConfigVersion: version, ID: in.ID, Name: in.Name, VideoEnabled: in.VideoEnabled, MaxWaitSec: in.MaxWaitSec, DispatchStrategy: in.DispatchStrategy, RecordingPolicy: in.RecordingPolicy, OverflowAction: in.OverflowAction, WaitPrompt: in.WaitPrompt, AnnounceRecording: in.AnnounceRecording, PriorityEnabled: in.PriorityEnabled, BusinessHoursJSON: in.BusinessHoursJSON, AfterHoursAction: in.AfterHoursAction, ForceHangupOnCheckout: in.ForceHangupOnCheckout, ListenAnnounce: in.ListenAnnounce, CreatedAt: now, UpdatedAt: now}
		if in.OverflowQueueID != "" {
			row.OverflowQueueID = &in.OverflowQueueID
		}
		if in.IVRFlowID != "" {
			row.IVRFlowID = &in.IVRFlowID
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for _, skillID := range in.SkillIDs {
			if err := tx.Create(&models.QueueSkill{ConfigVersion: version, QueueID: in.ID, SkillID: skillID}).Error; err != nil {
				return err
			}
		}
		for _, agentID := range in.AgentIDs {
			if err := tx.Create(&models.QueueAgent{ConfigVersion: version, QueueID: in.ID, AgentID: agentID}).Error; err != nil {
				return err
			}
		}
	}
	for _, in := range bundle.DIDs {
		var target *string
		if in.TargetID != "" {
			target = &in.TargetID
		}
		row := models.DIDRoute{ConfigVersion: version, ID: in.ID, TrunkID: in.TrunkID, DID: in.DID, TargetType: in.TargetType, TargetID: target, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func validateActiveDIDConflicts(tx *gorm.DB, _ string, version int64) error {
	// 单租户：仅校验候选版本内 DID 自洽即可，无跨应用冲突。
	_ = version
	_ = tx
	return nil
}

// NormalizeDID 生成确定性的被叫号码键，不推断本地国家/区号规则。
func NormalizeDID(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for i, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if r == '+' && i == 0 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func activeVersion(db *gorm.DB, _ string) (int64, error) {
	var active models.ActiveConfig
	if err := db.Where("id = ?", 1).First(&active).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, errs.Conflict("尚未激活呼叫中心配置", "")
		}
		return 0, err
	}
	return active.Version, nil
}

func uuidPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func wrapDB(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
