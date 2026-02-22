package agent

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Session struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Messages  []Message
	Metadata  map[string]any
}

type SessionStore struct {
	Sessions map[string]*Session
	mu       sync.RWMutex
	filePath string
}

func NewSessionStore(filePath string) *SessionStore {
	return &SessionStore{
		Sessions: make(map[string]*Session),
		filePath: filePath,
	}
}

func (s *SessionStore) Get(id string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Sessions[id]
}

func (s *SessionStore) Set(id string, session *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session.UpdatedAt = time.Now()
	s.Sessions[id] = session
}

func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Sessions, id)
}

func (s *SessionStore) List() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sessions := make([]*Session, 0, len(s.Sessions))
	for _, session := range s.Sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

func GetSessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".rizzclaw", "sessions")
}

func GetSessionFilePath(sessionID string) string {
	return filepath.Join(GetSessionDir(), sessionID+".json")
}

func NewSession(id string) *Session {
	now := time.Now()
	return &Session{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  make([]Message, 0),
		Metadata:  make(map[string]any),
	}
}
