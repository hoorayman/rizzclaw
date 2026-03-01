package agent

import (
	"fmt"
	"sync"
	"time"

	ctxmgr "github.com/hoorayman/rizzclaw/internal/context"
)

// SessionManager manages multiple sessions for different users/chats
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewSessionManager creates a new session manager
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// GetSession gets or creates a session for the given session key
func (sm *SessionManager) GetSession(sessionKey string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, ok := sm.sessions[sessionKey]; ok {
		return session
	}

	// Create new session
	session := &Session{
		ID:        sessionKey,
		Messages:  []Message{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	sm.sessions[sessionKey] = session
	return session
}

// GetOrLoadSession gets a session, loading from disk if not in memory
func (sm *SessionManager) GetOrLoadSession(sessionKey string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if session, ok := sm.sessions[sessionKey]; ok {
		return session
	}

	// Try to load from disk
	mgr := ctxmgr.GetSessionManager()
	ctxSession, err := mgr.LoadSession(sessionKey)
	if err == nil && ctxSession != nil {
		// Convert ctxmgr.Session to agent.Session
		session := &Session{
			ID:        ctxSession.ID,
			CreatedAt: ctxSession.CreatedAt,
			UpdatedAt: ctxSession.UpdatedAt,
			Messages:  make([]Message, len(ctxSession.Messages)),
		}
		for i, msg := range ctxSession.Messages {
			session.Messages[i] = Message{
				Role:      msg.Role,
				Content:   msg.Content,
				Timestamp: msg.Timestamp,
			}
		}
		sm.sessions[sessionKey] = session
		return session
	}

	// Create new session
	session := &Session{
		ID:        sessionKey,
		Messages:  []Message{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	sm.sessions[sessionKey] = session
	return session
}

// SaveSession saves a session to disk
func (sm *SessionManager) SaveSession(sessionKey string) error {
	sm.mu.RLock()
	session, ok := sm.sessions[sessionKey]
	sm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session not found: %s", sessionKey)
	}

	return SaveSessionToContext(session)
}

// SaveAllSessions saves all sessions to disk
func (sm *SessionManager) SaveAllSessions() error {
	sm.mu.RLock()
	sessions := make(map[string]*Session)
	for k, v := range sm.sessions {
		sessions[k] = v
	}
	sm.mu.RUnlock()

	var lastErr error
	for _, session := range sessions {
		if err := SaveSessionToContext(session); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// ListSessions returns all session keys
func (sm *SessionManager) ListSessions() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	keys := make([]string, 0, len(sm.sessions))
	for k := range sm.sessions {
		keys = append(keys, k)
	}
	return keys
}

// BuildSessionKey creates a session key for a channel/chat/user combination
func BuildSessionKey(channel, chatID, userID string) string {
	// Format: <channel>:<chat_id>:<user_id>
	// For group chats, userID is included to track individual users in the group
	// For direct messages, chatID and userID are often the same
	if userID != "" && userID != chatID {
		return fmt.Sprintf("%s:%s:%s", channel, chatID, userID)
	}
	return fmt.Sprintf("%s:%s", channel, chatID)
}
