package context

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type SessionManager struct {
	sessionsDir string
	config      *CompactionConfig
	active      *Session
	mu          sync.RWMutex
}

var globalSessionManager *SessionManager
var sessionManagerOnce sync.Once

func GetSessionManager() *SessionManager {
	sessionManagerOnce.Do(func() {
		home, _ := os.UserHomeDir()
		sessionsDir := filepath.Join(home, ".rizzclaw", "sessions")
		os.MkdirAll(sessionsDir, 0755)

		globalSessionManager = &SessionManager{
			sessionsDir: sessionsDir,
			config:      DefaultCompactionConfig(),
		}
	})
	return globalSessionManager
}

func NewSessionManager(sessionsDir string, config *CompactionConfig) *SessionManager {
	if config == nil {
		config = DefaultCompactionConfig()
	}
	os.MkdirAll(sessionsDir, 0755)

	return &SessionManager{
		sessionsDir: sessionsDir,
		config:      config,
	}
}

func (sm *SessionManager) NewSession(topic string) *Session {
	session := &Session{
		ID:         generateSessionID(),
		Topic:      topic,
		Messages:   make([]SessionMessage, 0),
		Summaries:  make([]SessionSummary, 0),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Compressed: false,
	}

	sm.mu.Lock()
	sm.active = session
	sm.mu.Unlock()

	return session
}

func (sm *SessionManager) GetActiveSession() *Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.active
}

func (sm *SessionManager) SetActiveSession(session *Session) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.active = session
}

func (sm *SessionManager) AddMessage(role, content string) *SessionMessage {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.active == nil {
		sm.active = sm.NewSession("")
	}

	msg := SessionMessage{
		ID:        generateMessageID(),
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
		Tokens:    estimateTokens(content),
	}

	sm.active.Messages = append(sm.active.Messages, msg)
	sm.active.TotalTokens += msg.Tokens
	sm.active.UpdatedAt = time.Now()

	return &msg
}

func (sm *SessionManager) LoadSession(sessionID string) (*Session, error) {
	path := filepath.Join(sm.sessionsDir, sessionID+".jsonl")

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open session file: %w", err)
	}
	defer file.Close()

	session := &Session{
		ID:        sessionID,
		Messages:  make([]SessionMessage, 0),
		Summaries: make([]SessionSummary, 0),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entryType, ok := entry["type"].(string); ok {
			switch entryType {
			case "session":
				if data, err := json.Marshal(entry); err == nil {
					json.Unmarshal(data, session)
				}
			case "message":
				var msg SessionMessage
				if data, err := json.Marshal(entry); err == nil {
					json.Unmarshal(data, &msg)
					session.Messages = append(session.Messages, msg)
				}
			case "summary":
				var summary SessionSummary
				if data, err := json.Marshal(entry); err == nil {
					json.Unmarshal(data, &summary)
					session.Summaries = append(session.Summaries, summary)
				}
			}
		}
	}

	session.TotalTokens = calculateTotalTokens(session.Messages)
	return session, nil
}

func (sm *SessionManager) SaveSession(session *Session) error {
	path := filepath.Join(sm.sessionsDir, session.ID+".jsonl")

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create session file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	sessionEntry := map[string]any{
		"type":        "session",
		"id":          session.ID,
		"topic":       session.Topic,
		"createdAt":   session.CreatedAt,
		"updatedAt":   session.UpdatedAt,
		"totalTokens": session.TotalTokens,
		"compressed":  session.Compressed,
	}
	sessionJSON, _ := json.Marshal(sessionEntry)
	writer.WriteString(string(sessionJSON) + "\n")

	for _, msg := range session.Messages {
		msgEntry := map[string]any{
			"type":      "message",
			"id":        msg.ID,
			"role":      msg.Role,
			"content":   msg.Content,
			"timestamp": msg.Timestamp,
			"tokens":    msg.Tokens,
			"metadata":  msg.Metadata,
		}
		msgJSON, _ := json.Marshal(msgEntry)
		writer.WriteString(string(msgJSON) + "\n")
	}

	for _, summary := range session.Summaries {
		summaryEntry := map[string]any{
			"type":         "summary",
			"id":           summary.ID,
			"startTime":    summary.StartTime,
			"endTime":      summary.EndTime,
			"messageCount": summary.MessageCount,
			"tokenCount":   summary.TokenCount,
			"summary":      summary.Summary,
			"keyTopics":    summary.KeyTopics,
		}
		summaryJSON, _ := json.Marshal(summaryEntry)
		writer.WriteString(string(summaryJSON) + "\n")
	}

	return writer.Flush()
}

func (sm *SessionManager) ListSessions() ([]string, error) {
	entries, err := os.ReadDir(sm.sessionsDir)
	if err != nil {
		return nil, err
	}

	var sessions []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			sessions = append(sessions, strings.TrimSuffix(entry.Name(), ".jsonl"))
		}
	}

	return sessions, nil
}

func (sm *SessionManager) DeleteSession(sessionID string) error {
	path := filepath.Join(sm.sessionsDir, sessionID+".jsonl")
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (sm *SessionManager) CleanupOldSessions(maxSessions int) error {
	sessions, err := sm.ListSessions()
	if err != nil {
		return err
	}

	if len(sessions) <= maxSessions {
		return nil
	}

	type sessionInfo struct {
		id      string
		modTime time.Time
	}

	var sessionInfos []sessionInfo
	for _, id := range sessions {
		path := filepath.Join(sm.sessionsDir, id+".jsonl")
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		sessionInfos = append(sessionInfos, sessionInfo{id: id, modTime: info.ModTime()})
	}

	sort.Slice(sessionInfos, func(i, j int) bool {
		return sessionInfos[i].modTime.After(sessionInfos[j].modTime)
	})

	for i := maxSessions; i < len(sessionInfos); i++ {
		sm.DeleteSession(sessionInfos[i].id)
	}

	return nil
}

func (sm *SessionManager) ShouldCompact(session *Session) bool {
	if session == nil {
		return false
	}

	if len(session.Messages) < sm.config.MinMessagesToKeep*2 {
		return false
	}

	return session.TotalTokens > int(float64(sm.config.MaxTokens)*sm.config.MaxHistoryShare)
}

func (sm *SessionManager) Compact(session *Session, summaryGenerator func(messages []SessionMessage) (string, error)) (*SessionSummary, error) {
	if !sm.ShouldCompact(session) {
		return nil, nil
	}

	totalMessages := len(session.Messages)
	keepMessages := sm.config.MinMessagesToKeep
	compactRatio := sm.config.ChunkRatio

	compactCount := int(float64(totalMessages-keepMessages) * compactRatio)
	if compactCount < 1 {
		compactCount = totalMessages - keepMessages
	}

	if compactCount <= 0 {
		return nil, nil
	}

	messagesToCompact := session.Messages[:compactCount]

	summary, err := summaryGenerator(messagesToCompact)
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}

	sessionSummary := SessionSummary{
		ID:           generateSummaryID(),
		StartTime:    messagesToCompact[0].Timestamp,
		EndTime:      messagesToCompact[len(messagesToCompact)-1].Timestamp,
		MessageCount: len(messagesToCompact),
		TokenCount:   calculateTotalTokens(messagesToCompact),
		Summary:      summary,
	}

	session.Messages = session.Messages[compactCount:]
	session.Summaries = append(session.Summaries, sessionSummary)
	session.TotalTokens = calculateTotalTokens(session.Messages)
	session.Compressed = true
	session.UpdatedAt = time.Now()

	return &sessionSummary, nil
}

func (sm *SessionManager) PruneHistory(session *Session, maxTokens int) []SessionMessage {
	if session == nil || len(session.Messages) == 0 {
		return nil
	}

	effectiveMax := int(float64(maxTokens) / sm.config.SafetyMargin)

	var keptMessages []SessionMessage
	var tokenCount int

	for i := len(session.Messages) - 1; i >= 0; i-- {
		msg := session.Messages[i]
		if tokenCount+msg.Tokens > effectiveMax {
			break
		}
		keptMessages = append([]SessionMessage{msg}, keptMessages...)
		tokenCount += msg.Tokens
	}

	return keptMessages
}

func (sm *SessionManager) GetMessagesWithSummaries(session *Session) []string {
	var result []string

	for _, summary := range session.Summaries {
		result = append(result, fmt.Sprintf("[Summary of %d earlier messages]: %s", summary.MessageCount, summary.Summary))
	}

	for _, msg := range session.Messages {
		result = append(result, fmt.Sprintf("[%s]: %s", msg.Role, msg.Content))
	}

	return result
}

func (sm *SessionManager) BuildContextFromSession(session *Session, maxTokens int) string {
	var buf strings.Builder

	buf.WriteString("# Session History\n\n")

	for _, summary := range session.Summaries {
		buf.WriteString(fmt.Sprintf("## Summary (%s - %s)\n", summary.StartTime.Format("2006-01-02 15:04"), summary.EndTime.Format("15:04")))
		buf.WriteString(fmt.Sprintf("%s\n\n", summary.Summary))
	}

	buf.WriteString("## Recent Messages\n\n")

	var tokenCount int
	messages := sm.PruneHistory(session, maxTokens)

	for _, msg := range messages {
		if tokenCount+msg.Tokens > maxTokens {
			break
		}
		buf.WriteString(fmt.Sprintf("**%s** (%s):\n%s\n\n", msg.Role, msg.Timestamp.Format("15:04:05"), msg.Content))
		tokenCount += msg.Tokens
	}

	return buf.String()
}

func estimateTokens(text string) int {
	chars := len(text)
	words := len(strings.Fields(text))

	return (chars + words*4) / 4
}

func calculateTotalTokens(messages []SessionMessage) int {
	total := 0
	for _, msg := range messages {
		total += msg.Tokens
	}
	return total
}

func generateSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}

func generateMessageID() string {
	return fmt.Sprintf("msg-%d", time.Now().UnixNano())
}

func generateSummaryID() string {
	return fmt.Sprintf("summary-%d", time.Now().UnixNano())
}

func ChunkMessagesByTokens(messages []SessionMessage, maxTokens int) [][]SessionMessage {
	effectiveMax := int(float64(maxTokens) / 1.2)

	var chunks [][]SessionMessage
	var currentChunk []SessionMessage
	var currentTokens int

	for _, msg := range messages {
		if currentTokens+msg.Tokens > effectiveMax && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = nil
			currentTokens = 0
		}

		currentChunk = append(currentChunk, msg)
		currentTokens += msg.Tokens
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

func LimitHistoryTurns(messages []SessionMessage, limit int) []SessionMessage {
	if limit <= 0 || len(messages) <= limit*2 {
		return messages
	}

	userTurns := 0
	var result []SessionMessage

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		result = append([]SessionMessage{msg}, result...)

		if msg.Role == "user" {
			userTurns++
			if userTurns >= limit {
				break
			}
		}
	}

	return result
}

func (sm *SessionManager) EstimateSessionTokens(session *Session) int {
	if session == nil {
		return 0
	}
	return session.TotalTokens
}

func (sm *SessionManager) GetConfig() *CompactionConfig {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.config
}

func (sm *SessionManager) SetConfig(config *CompactionConfig) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.config = config
}

type SessionStats struct {
	TotalSessions  int       `json:"totalSessions"`
	TotalMessages  int       `json:"totalMessages"`
	TotalTokens    int       `json:"totalTokens"`
	OldestSession  time.Time `json:"oldestSession"`
	NewestSession  time.Time `json:"newestSession"`
	AvgMessagesPer float64   `json:"avgMessagesPer"`
	AvgTokensPer   float64   `json:"avgTokensPer"`
}

func (sm *SessionManager) GetStats() (*SessionStats, error) {
	sessions, err := sm.ListSessions()
	if err != nil {
		return nil, err
	}

	stats := &SessionStats{
		TotalSessions: len(sessions),
		OldestSession: time.Now(),
	}

	for _, sessionID := range sessions {
		session, err := sm.LoadSession(sessionID)
		if err != nil {
			continue
		}

		stats.TotalMessages += len(session.Messages)
		stats.TotalTokens += session.TotalTokens

		if session.CreatedAt.Before(stats.OldestSession) {
			stats.OldestSession = session.CreatedAt
		}
		if session.CreatedAt.After(stats.NewestSession) {
			stats.NewestSession = session.CreatedAt
		}
	}

	if stats.TotalSessions > 0 {
		stats.AvgMessagesPer = float64(stats.TotalMessages) / float64(stats.TotalSessions)
		stats.AvgTokensPer = float64(stats.TotalTokens) / float64(stats.TotalSessions)
	}

	return stats, nil
}

func (sm *SessionManager) AppendToMemory(content string) error {
	ctxMgr := GetManager()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("\n## [%s]\n%s\n", timestamp, content)

	if err := ctxMgr.AppendToFile(MemoryFilename, entry); err != nil {
		return err
	}

	sm.checkMemoryFileSize()

	return nil
}

func (sm *SessionManager) SaveImportantMemory(content string, isEvergreen bool) error {
	ctxMgr := GetManager()

	var entry string
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	if isEvergreen {
		entry = fmt.Sprintf("\n## [EVERGREEN - %s]\n%s\n", timestamp, content)
	} else {
		entry = fmt.Sprintf("\n## [%s]\n%s\n", timestamp, content)
	}

	if err := ctxMgr.AppendToFile(MemoryFilename, entry); err != nil {
		return err
	}

	sm.checkMemoryFileSize()

	return nil
}

const MaxMemoryFileSize = 100000

func (sm *SessionManager) checkMemoryFileSize() {
	ctxMgr := GetManager()
	cf := ctxMgr.GetFile(MemoryFilename)

	if cf == nil || cf.Size < MaxMemoryFileSize {
		return
	}

	sm.archiveMemoryFile()
}

func (sm *SessionManager) archiveMemoryFile() error {
	ctxMgr := GetManager()
	cf := ctxMgr.GetFile(MemoryFilename)

	if cf == nil || cf.Content == "" {
		return nil
	}

	store := GetMemoryStore()

	sections := strings.Split(cf.Content, "\n## ")

	for _, section := range sections {
		if strings.TrimSpace(section) == "" {
			continue
		}

		lines := strings.SplitN(section, "\n", 2)
		if len(lines) < 2 {
			continue
		}

		header := strings.TrimSpace(lines[0])
		body := strings.TrimSpace(lines[1])

		isEvergreen := strings.Contains(header, "EVERGREEN")

		entry := &MemoryEntry{
			ID:          generateMemoryID(),
			Content:     body,
			Source:      "MEMORY.md",
			CreatedAt:   time.Now(),
			IsEvergreen: isEvergreen,
		}

		store.AddMemory(nil, entry)
	}

	evergreenContent := ""
	for _, section := range sections {
		if strings.Contains(section, "EVERGREEN") {
			evergreenContent += "## " + section + "\n"
		}
	}

	if evergreenContent != "" {
		ctxMgr.SaveFile(MemoryFilename, "# MEMORY.md - Long-term Memory\n\n## Evergreen Memories\n\n"+evergreenContent)
	} else {
		ctxMgr.SaveFile(MemoryFilename, "# MEMORY.md - Long-term Memory\n\n(Memories archived to database)\n")
	}

	return nil
}

func CalculateTemporalDecay(ageInDays, halfLifeDays float64) float64 {
	lambda := math.Ln2 / halfLifeDays
	return math.Exp(-lambda * ageInDays)
}

func IsEvergreenMemory(content string) bool {
	return strings.Contains(content, "[EVERGREEN")
}
