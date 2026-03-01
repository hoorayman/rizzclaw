package channels

import (
	"context"
	"sync"

	"github.com/hoorayman/rizzclaw/pkg/bus"
)

// Channel defines the interface for all communication channels
type Channel interface {
	// Name returns the channel name
	Name() string
	// Start starts the channel
	Start(ctx context.Context) error
	// Stop stops the channel
	Stop(ctx context.Context) error
	// IsRunning returns whether the channel is running
	IsRunning() bool
	// Send sends a message to the channel
	Send(ctx context.Context, msg bus.OutboundMessage) error
}

// BaseChannel provides common functionality for all channels
type BaseChannel struct {
	name      string
	bus       *bus.MessageBus
	running   bool
	mu        sync.RWMutex
	allowFrom []string
}

// NewBaseChannel creates a new base channel
func NewBaseChannel(name string, bus *bus.MessageBus, allowFrom []string) *BaseChannel {
	return &BaseChannel{
		name:      name,
		bus:       bus,
		allowFrom: allowFrom,
	}
}

// Name returns the channel name
func (c *BaseChannel) Name() string {
	return c.name
}

// IsRunning returns whether the channel is running
func (c *BaseChannel) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// setRunning sets the running state
func (c *BaseChannel) setRunning(running bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = running
}

// IsAllowed checks if a user is allowed to use this channel
func (c *BaseChannel) IsAllowed(userID string) bool {
	if len(c.allowFrom) == 0 {
		return true
	}
	for _, allowed := range c.allowFrom {
		if allowed == userID {
			return true
		}
	}
	return false
}

// HandleMessage handles an incoming message and publishes it to the bus
func (c *BaseChannel) HandleMessage(userID, chatID, content string, metadata map[string]string) {
	if !c.IsAllowed(userID) {
		return
	}

	if metadata == nil {
		metadata = make(map[string]string)
	}

	c.bus.PublishInbound(bus.InboundMessage{
		Channel:  c.name,
		ChatID:   chatID,
		UserID:   userID,
		Content:  content,
		Metadata: metadata,
	})
}
