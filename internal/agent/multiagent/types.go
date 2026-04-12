package multiagent

import (
	"sync"
	"time"
)

type SubagentStatus string

const (
	SubagentStatusRunning   SubagentStatus = "running"
	SubagentStatusCompleted SubagentStatus = "completed"
	SubagentStatusFailed    SubagentStatus = "failed"
	SubagentStatusTimeout   SubagentStatus = "timeout"
)

type SubagentOutcome struct {
	Status    SubagentStatus `json:"status"`
	Error     string         `json:"error,omitempty"`
	Duration  time.Duration  `json:"duration"`
	Output    string         `json:"output"`
	Timestamp time.Time      `json:"timestamp"`
}

type SubagentRun struct {
	RunID               string           `json:"run_id"`
	ChildSessionKey     string           `json:"child_session_key"`
	RequesterSessionKey string           `json:"requester_session_key"`
	Channel             string           `json:"channel,omitempty"`
	ChatID              string           `json:"chat_id,omitempty"`
	Task                string           `json:"task"`
	Label               string           `json:"label,omitempty"`
	Model               string           `json:"model,omitempty"`
	Depth               int              `json:"depth"`
	SpawnMode           SpawnMode        `json:"spawn_mode"`
	Cleanup             CleanupMode      `json:"cleanup"`
	StartedAt           time.Time        `json:"started_at"`
	EndedAt             *time.Time       `json:"ended_at,omitempty"`
	Outcome             *SubagentOutcome `json:"outcome,omitempty"`
	Notified            bool             `json:"notified"`
	mu                  sync.RWMutex
}

type SpawnMode string

const (
	SpawnModeRun     SpawnMode = "run"
	SpawnModeSession SpawnMode = "session"
)

type CleanupMode string

const (
	CleanupModeDelete CleanupMode = "delete"
	CleanupModeKeep   CleanupMode = "keep"
)

type AnnounceMessage struct {
	RunID     string         `json:"run_id"`
	TaskLabel string         `json:"task_label"`
	Status    SubagentStatus `json:"status"`
	Output    string         `json:"output"`
	Duration  string         `json:"duration"`
	Timestamp time.Time      `json:"timestamp"`
	Message   string         `json:"message"`
	Channel   string         `json:"channel,omitempty"`
	ChatID    string         `json:"chat_id,omitempty"`
}

type CompletionCallback func(announce *AnnounceMessage)

func (r *SubagentRun) RLock() {
	r.mu.RLock()
}

func (r *SubagentRun) RUnlock() {
	r.mu.RUnlock()
}

func (r *SubagentRun) GetOutcome() *SubagentOutcome {
	return r.Outcome
}

func (r *SubagentRun) GetNotified() bool {
	return r.Notified
}
