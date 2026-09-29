package http

import "open-switch/internal/store"

func testApplicationRegistry(secret string) *store.ApplicationRegistry {
	reg := store.NewApplicationRegistry(nil)
	reg.SetTestRecord(store.ApplicationRecord{
		ID: "crm", Secret: secret, EventRetentionDays: 14, MaxConcurrentCalls: 100, Enabled: true,
	})
	return reg
}
