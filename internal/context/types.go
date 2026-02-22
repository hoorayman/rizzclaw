package context

import (
	"time"
)

type ContextFile struct {
	Path        string    `json:"path"`
	Content     string    `json:"content"`
	ModTime     time.Time `json:"modTime"`
	Size        int64     `json:"size"`
	Truncated   bool      `json:"truncated"`
	OriginalLen int       `json:"originalLen,omitempty"`
}

type BootstrapFile struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Content     string `json:"content"`
	MaxChars    int    `json:"maxChars"`
	Truncated   bool   `json:"truncated"`
	Priority    int    `json:"priority"`
}

type AgentIdentity struct {
	Name     string `json:"name,omitempty"`
	Emoji    string `json:"emoji,omitempty"`
	Theme    string `json:"theme,omitempty"`
	Creature string `json:"creature,omitempty"`
	Vibe     string `json:"vibe,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	Version  string `json:"version,omitempty"`
}

type UserPreference struct {
	Language       string   `json:"language,omitempty"`
	CodeStyle      string   `json:"codeStyle,omitempty"`
	EditorPrefs    string   `json:"editorPrefs,omitempty"`
	Frameworks     []string `json:"frameworks,omitempty"`
	CustomPrompts  string   `json:"customPrompts,omitempty"`
}

type SoulConfig struct {
	Personality string `json:"personality,omitempty"`
	Tone        string `json:"tone,omitempty"`
	Style       string `json:"style,omitempty"`
	Values      string `json:"values,omitempty"`
	Custom      string `json:"custom,omitempty"`
}

type AgentBehavior struct {
	DefaultMode      string            `json:"defaultMode,omitempty"`
	AutoSave         bool              `json:"autoSave"`
	MaxTokens        int               `json:"maxTokens"`
	Temperature      float64           `json:"temperature"`
	ToolPermissions  map[string]bool   `json:"toolPermissions,omitempty"`
	CustomRules      []string          `json:"customRules,omitempty"`
}

type SessionMessage struct {
	ID        string                 `json:"id"`
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	Timestamp time.Time              `json:"timestamp"`
	Tokens    int                    `json:"tokens"`
	Metadata  map[string]any         `json:"metadata,omitempty"`
}

type SessionSummary struct {
	ID          string    `json:"id"`
	StartTime   time.Time `json:"startTime"`
	EndTime     time.Time `json:"endTime,omitempty"`
	MessageCount int      `json:"messageCount"`
	TokenCount  int       `json:"tokenCount"`
	Summary     string    `json:"summary"`
	KeyTopics   []string `json:"keyTopics,omitempty"`
}

type Session struct {
	ID           string            `json:"id"`
	Topic        string            `json:"topic,omitempty"`
	Messages     []SessionMessage  `json:"messages"`
	Summaries    []SessionSummary  `json:"summaries,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
	TotalTokens  int               `json:"totalTokens"`
	Compressed   bool              `json:"compressed"`
}

type MemoryEntry struct {
	ID          string                 `json:"id"`
	Content     string                 `json:"content"`
	Embedding   []float32              `json:"embedding,omitempty"`
	Keywords    []string               `json:"keywords,omitempty"`
	Source      string                 `json:"source"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
	Metadata    map[string]any         `json:"metadata,omitempty"`
	IsEvergreen bool                   `json:"isEvergreen"`
}

type SearchResult struct {
	Entry      *MemoryEntry `json:"entry"`
	Score      float64      `json:"score"`
	VectorScore float64     `json:"vectorScore"`
	KeywordScore float64    `json:"keywordScore"`
	DecayMultiplier float64 `json:"decayMultiplier"`
}

type SearchOptions struct {
	Query          string    `json:"query"`
	MaxResults     int       `json:"maxResults"`
	MinScore       float64   `json:"minScore"`
	VectorWeight   float64   `json:"vectorWeight"`
	KeywordWeight  float64   `json:"keywordWeight"`
	UseMMR         bool      `json:"useMMR"`
	MMRLambda      float64   `json:"mmrLambda"`
	TemporalDecay  bool      `json:"temporalDecay"`
	HalfLifeDays   float64   `json:"halfLifeDays"`
}

type CompactionConfig struct {
	MaxTokens         int     `json:"maxTokens"`
	MaxHistoryShare   float64 `json:"maxHistoryShare"`
	ChunkRatio        float64 `json:"chunkRatio"`
	SafetyMargin      float64 `json:"safetyMargin"`
	MinMessagesToKeep int     `json:"minMessagesToKeep"`
}

type ContextConfig struct {
	BootstrapMaxChars      int     `json:"bootstrapMaxChars"`
	BootstrapTotalMaxChars int     `json:"bootstrapTotalMaxChars"`
	SessionDeltaBytes      int     `json:"sessionDeltaBytes"`
	SessionDeltaMessages   int     `json:"sessionDeltaMessages"`
	ChunkTokens            int     `json:"chunkTokens"`
	ChunkOverlap           int     `json:"chunkOverlap"`
	MaxResults             int     `json:"maxResults"`
	MinScore               float64 `json:"minScore"`
	VectorWeight           float64 `json:"vectorWeight"`
	KeywordWeight          float64 `json:"keywordWeight"`
	MMRLambda              float64 `json:"mmrLambda"`
	TemporalDecayHalfLife  float64 `json:"temporalDecayHalfLife"`
}

func DefaultContextConfig() *ContextConfig {
	return &ContextConfig{
		BootstrapMaxChars:      20000,
		BootstrapTotalMaxChars: 150000,
		SessionDeltaBytes:      100000,
		SessionDeltaMessages:   50,
		ChunkTokens:            400,
		ChunkOverlap:           80,
		MaxResults:             6,
		MinScore:               0.35,
		VectorWeight:           0.7,
		KeywordWeight:          0.3,
		MMRLambda:              0.7,
		TemporalDecayHalfLife:  30,
	}
}

func DefaultCompactionConfig() *CompactionConfig {
	return &CompactionConfig{
		MaxTokens:         128000,
		MaxHistoryShare:   0.5,
		ChunkRatio:        0.4,
		SafetyMargin:      1.2,
		MinMessagesToKeep: 10,
	}
}

func DefaultSearchOptions() *SearchOptions {
	return &SearchOptions{
		MaxResults:      6,
		MinScore:        0.35,
		VectorWeight:    0.7,
		KeywordWeight:   0.3,
		UseMMR:          false,
		MMRLambda:       0.7,
		TemporalDecay:   true,
		HalfLifeDays:    30,
	}
}

const (
	AgentsFilename    = "AGENTS.md"
	SoulFilename      = "SOUL.md"
	UserFilename      = "USER.md"
	IdentityFilename  = "IDENTITY.md"
	MemoryFilename    = "MEMORY.md"
	SessionFilename   = "SESSION.md"
)

var BootstrapFilenames = []string{
	AgentsFilename,
	SoulFilename,
	UserFilename,
	IdentityFilename,
	MemoryFilename,
}
