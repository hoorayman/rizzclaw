package channels

import (
	"context"
	"fmt"
	"sync"

	"github.com/hoorayman/rizzclaw/internal/config"
	"github.com/hoorayman/rizzclaw/pkg/bus"
)

// Manager manages all channels
type Manager struct {
	channels map[string]Channel
	bus      *bus.MessageBus
	config   *config.Config
	mu       sync.RWMutex
}

// NewManager creates a new channel manager
func NewManager(cfg *config.Config, messageBus *bus.MessageBus) (*Manager, error) {
	m := &Manager{
		channels: make(map[string]Channel),
		bus:      messageBus,
		config:   cfg,
	}

	if err := m.initChannels(); err != nil {
		return nil, err
	}

	return m, nil
}

// initChannels initializes all enabled channels
func (m *Manager) initChannels() error {
	// Always initialize console channel for CLI mode
	console := NewConsoleChannel(m.bus)
	m.channels["console"] = console

	// TODO: Initialize other channels (Feishu, Telegram, etc.) based on config

	return nil
}

// StartAll starts all channels
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, channel := range m.channels {
		if err := channel.Start(ctx); err != nil {
			return fmt.Errorf("failed to start channel %s: %w", name, err)
		}
	}

	// Register outbound message handler
	go m.handleOutbound(ctx)

	return nil
}

// StopAll stops all channels
func (m *Manager) StopAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, channel := range m.channels {
		if err := channel.Stop(ctx); err != nil {
			return fmt.Errorf("failed to stop channel %s: %w", name, err)
		}
	}

	return nil
}

// GetChannel gets a channel by name
func (m *Manager) GetChannel(name string) (Channel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	channel, ok := m.channels[name]
	return channel, ok
}

// GetEnabledChannels returns a list of enabled channel names
func (m *Manager) GetEnabledChannels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var names []string
	for name, channel := range m.channels {
		if channel.IsRunning() {
			names = append(names, name)
		}
	}
	return names
}

// handleOutbound handles outbound messages from the bus
func (m *Manager) handleOutbound(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, ok := m.bus.SubscribeOutbound(ctx)
		if !ok {
			return
		}

		channel, ok := m.GetChannel(msg.Channel)
		if !ok {
			continue
		}

		if err := channel.Send(ctx, msg); err != nil {
			// Log error but continue
			fmt.Printf("Failed to send message to channel %s: %v\n", msg.Channel, err)
		}
	}
}
