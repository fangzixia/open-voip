package external

import (
	"context"
	"open-switch/internal/ports"
	"open-switch/internal/store"
)

// EventPublisher persists an event before optionally notifying a business adapter.
// Consumers can always resume from the durable numeric cursor.
type EventPublisher struct {
	Store   store.CallEvents
	Forward ports.CallEventPublisher
}

func (p EventPublisher) PublishCallEvent(ctx context.Context, ev ports.CallEvent) error {
	if err := p.Store.Append(ctx, &ev); err != nil {
		return err
	}
	if p.Forward != nil {
		return p.Forward.PublishCallEvent(ctx, ev)
	}
	return nil
}
