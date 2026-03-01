package bus

import (
	"context"
	"sync"
)

// InboundMessage represents a message from user to agent
type InboundMessage struct {
	Channel   string            `json:"channel"`
	ChatID    string            `json:"chat_id"`
	UserID    string            `json:"user_id"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata"`
}

// OutboundMessage represents a message from agent to user
type OutboundMessage struct {
	Channel  string `json:"channel"`
	ChatID   string `json:"chat_id"`
	Content  string `json:"content"`
}

// MessageBus is a simple message queue for decoupling channels and agent
type MessageBus struct {
	inbound  chan InboundMessage
	outbound chan OutboundMessage
	handlers map[string]MessageHandler
	closed   bool
	mu       sync.RWMutex
}

// MessageHandler handles outbound messages for a channel
type MessageHandler func(msg OutboundMessage) error

// NewMessageBus creates a new message bus
func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:  make(chan InboundMessage, 100),
		outbound: make(chan OutboundMessage, 100),
		handlers: make(map[string]MessageHandler),
	}
}

// PublishInbound publishes an inbound message
func (mb *MessageBus) PublishInbound(msg InboundMessage) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	if mb.closed {
		return
	}
	select {
	case mb.inbound <- msg:
	default:
	}
}

// ConsumeInbound consumes an inbound message
func (mb *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, bool) {
	select {
	case msg := <-mb.inbound:
		return msg, true
	case <-ctx.Done():
		return InboundMessage{}, false
	}
}

// PublishOutbound publishes an outbound message
func (mb *MessageBus) PublishOutbound(msg OutboundMessage) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	if mb.closed {
		return
	}
	select {
	case mb.outbound <- msg:
	default:
	}
}

// SubscribeOutbound subscribes to outbound messages
func (mb *MessageBus) SubscribeOutbound(ctx context.Context) (OutboundMessage, bool) {
	select {
	case msg := <-mb.outbound:
		return msg, true
	case <-ctx.Done():
		return OutboundMessage{}, false
	}
}

// RegisterHandler registers a handler for a channel
func (mb *MessageBus) RegisterHandler(channel string, handler MessageHandler) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.handlers[channel] = handler
}

// GetHandler gets the handler for a channel
func (mb *MessageBus) GetHandler(channel string) (MessageHandler, bool) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	handler, ok := mb.handlers[channel]
	return handler, ok
}

// Close closes the message bus
func (mb *MessageBus) Close() {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.closed {
		return
	}
	mb.closed = true
	close(mb.inbound)
	close(mb.outbound)
}
