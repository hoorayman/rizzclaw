package agent

import (
	"context"
	"fmt"
	"time"

	ctxmgr "github.com/hoorayman/rizzclaw/internal/context"
)

type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
}

type Session struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Messages  []Message
	Metadata  map[string]any
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

func (s *Session) AddMessage(role, content string) {
	s.Messages = append(s.Messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	})
	s.UpdatedAt = time.Now()
}

func SaveSessionToContext(session *Session) error {
	mgr := ctxmgr.GetSessionManager()

	ctxSession := &ctxmgr.Session{
		ID:        session.ID,
		Messages:  make([]ctxmgr.SessionMessage, len(session.Messages)),
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.UpdatedAt,
	}

	for i, msg := range session.Messages {
		ctxSession.Messages[i] = ctxmgr.SessionMessage{
			ID:        generateMsgID(),
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
			Tokens:    estimateMsgTokens(msg.Content),
		}
	}

	return mgr.SaveSession(ctxSession)
}

func LoadSessionFromContext(sessionID string) (*Session, error) {
	mgr := ctxmgr.GetSessionManager()

	ctxSession, err := mgr.LoadSession(sessionID)
	if err != nil {
		return nil, err
	}

	session := &Session{
		ID:        ctxSession.ID,
		CreatedAt: ctxSession.CreatedAt,
		UpdatedAt: ctxSession.UpdatedAt,
		Messages:  make([]Message, len(ctxSession.Messages)),
		Metadata:  make(map[string]any),
	}

	for i, msg := range ctxSession.Messages {
		session.Messages[i] = Message{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		}
	}

	return session, nil
}

func SaveMemory(content string, isEvergreen bool) error {
	mgr := ctxmgr.GetSessionManager()
	return mgr.SaveImportantMemory(content, isEvergreen)
}

func SearchMemory(ctx context.Context, query string) ([]*ctxmgr.SearchResult, error) {
	store := ctxmgr.GetMemoryStore()
	return store.Search(ctx, &ctxmgr.SearchOptions{
		Query:      query,
		MaxResults: 6,
	})
}

func generateMsgID() string {
	return time.Now().Format("20060102150405")
}

func estimateMsgTokens(text string) int {
	return len(text) / 4
}

const (
	MaxSessionTokens    = 128000
	MinMessagesToKeep   = 10
	CompactionThreshold = 0.5
	CompactionRatio     = 0.4
)

func ShouldCompactSession(session *Session) bool {
	if session == nil || len(session.Messages) < MinMessagesToKeep*2 {
		return false
	}

	totalTokens := 0
	for _, msg := range session.Messages {
		totalTokens += estimateMsgTokens(msg.Content)
	}

	return totalTokens > int(float64(MaxSessionTokens)*CompactionThreshold)
}

func CompactSession(session *Session) bool {
	if !ShouldCompactSession(session) {
		return false
	}

	totalMessages := len(session.Messages)
	compactCount := int(float64(totalMessages-MinMessagesToKeep) * CompactionRatio)

	if compactCount <= 0 {
		return false
	}

	var summaryContent string
	for i := 0; i < compactCount; i++ {
		msg := session.Messages[i]
		summaryContent += fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content)
	}

	summaryMsg := Message{
		Role:      "system",
		Content:   fmt.Sprintf("[Summary of earlier conversation]\n%s", summaryContent),
		Timestamp: time.Now(),
	}

	session.Messages = append([]Message{summaryMsg}, session.Messages[compactCount:]...)
	session.UpdatedAt = time.Now()

	return true
}
