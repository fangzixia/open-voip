package models

import "time"

// WebhookSubscription 事件订阅配置。
type WebhookSubscription struct {
	// ID 订阅 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:Webhook 订阅 ID"`
	// URL 投递目标 HTTPS 地址。
	URL string `gorm:"size:512;not null;comment:回调 URL"`
	// EventTypes 订阅事件类型 JSON 数组字符串。
	EventTypes string `gorm:"type:text;not null;comment:事件类型列表 JSON"`
	// Secret 签名密钥，可空。
	Secret string `gorm:"size:128;comment:HMAC 密钥"`
	// Enabled 是否启用。
	Enabled bool `gorm:"not null;default:true;comment:是否启用"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
}

// TableName 指定表名。
func (WebhookSubscription) TableName() string { return "os_webhook_subscriptions" }

// WebhookDelivery 单次 Webhook 投递记录。
type WebhookDelivery struct {
	// ID 投递 UUID。
	ID string `gorm:"type:uuid;primaryKey;comment:投递记录 ID"`
	// SubscriptionID 订阅 ID。
	SubscriptionID string `gorm:"type:uuid;index;not null;comment:订阅 ID"`
	// EventType 事件类型。
	EventType string `gorm:"size:64;not null;index;comment:事件类型"`
	// Payload 请求体 JSON。
	Payload string `gorm:"type:text;not null;comment:载荷 JSON"`
	// Status 投递状态：pending / success / failed。
	Status string `gorm:"size:16;not null;index;comment:投递状态"`
	// Attempts 已尝试次数。
	Attempts int `gorm:"not null;default:0;comment:尝试次数"`
	// LastError 最后一次错误信息。
	LastError string `gorm:"type:text;comment:最后错误"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `gorm:"comment:创建时间"`
	// UpdatedAt 更新时间。
	UpdatedAt time.Time `gorm:"comment:更新时间"`
}

// TableName 指定表名。
func (WebhookDelivery) TableName() string { return "os_webhook_deliveries" }
