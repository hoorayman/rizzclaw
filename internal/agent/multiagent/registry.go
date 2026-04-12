package multiagent

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

const (
	DefaultMaxSpawnDepth       = 3
	DefaultMaxChildrenPerAgent = 5
)

var globalRegistry *Registry

type Registry struct {
	runs      map[string]*SubagentRun
	requester map[string][]string
	mu        sync.RWMutex
	callbacks []CompletionCallback
}

func GetRegistry() *Registry {
	if globalRegistry == nil {
		globalRegistry = NewRegistry()
	}
	return globalRegistry
}

func NewRegistry() *Registry {
	return &Registry{
		runs:      make(map[string]*SubagentRun),
		requester: make(map[string][]string),
		callbacks: make([]CompletionCallback, 0),
	}
}

func (r *Registry) RegisterCallback(cb CompletionCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, cb)
}

func (r *Registry) RegisterRun(run *SubagentRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.runs[run.RunID]; exists {
		return fmt.Errorf("run %s already registered", run.RunID)
	}

	r.runs[run.RunID] = run
	r.requester[run.RequesterSessionKey] = append(r.requester[run.RequesterSessionKey], run.RunID)

	return nil
}

func (r *Registry) GetRun(runID string) (*SubagentRun, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	run, exists := r.runs[runID]
	return run, exists
}

func (r *Registry) UpdateRunOutcome(runID string, outcome *SubagentOutcome) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	run, exists := r.runs[runID]
	if !exists {
		return fmt.Errorf("run %s not found", runID)
	}

	run.mu.Lock()
	run.Outcome = outcome
	now := time.Now()
	run.EndedAt = &now
	run.mu.Unlock()

	return nil
}

func (r *Registry) MarkNotified(runID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	run, exists := r.runs[runID]
	if !exists {
		return fmt.Errorf("run %s not found", runID)
	}

	run.mu.Lock()
	run.Notified = true
	run.mu.Unlock()

	return nil
}

func (r *Registry) GetActiveRunsForRequester(requesterSessionKey string) []*SubagentRun {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var activeRuns []*SubagentRun
	for _, runID := range r.requester[requesterSessionKey] {
		if run, exists := r.runs[runID]; exists {
			run.mu.RLock()
			if run.Outcome == nil {
				activeRuns = append(activeRuns, run)
			}
			run.mu.RUnlock()
		}
	}

	return activeRuns
}

func (r *Registry) CountActiveRunsForRequester(requesterSessionKey string) int {
	return len(r.GetActiveRunsForRequester(requesterSessionKey))
}

func (r *Registry) GetAllRuns() []*SubagentRun {
	r.mu.RLock()
	defer r.mu.RUnlock()

	runs := make([]*SubagentRun, 0, len(r.runs))
	for _, run := range r.runs {
		runs = append(runs, run)
	}

	return runs
}

func (r *Registry) GetUnnotifiedCompletedRuns() []*SubagentRun {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var runs []*SubagentRun
	for _, run := range r.runs {
		run.mu.RLock()
		if run.Outcome != nil && !run.Notified {
			runs = append(runs, run)
		}
		run.mu.RUnlock()
	}

	return runs
}

func (r *Registry) CleanupRun(runID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	run, exists := r.runs[runID]
	if !exists {
		return fmt.Errorf("run %s not found", runID)
	}

	delete(r.runs, runID)

	var updated []string
	for _, id := range r.requester[run.RequesterSessionKey] {
		if id != runID {
			updated = append(updated, id)
		}
	}
	r.requester[run.RequesterSessionKey] = updated

	return nil
}

func (r *Registry) GenerateRunID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("subagent-%x", b)
}

func (r *Registry) TriggerCallbacks(announce *AnnounceMessage) {
	r.mu.RLock()
	cbs := make([]CompletionCallback, len(r.callbacks))
	copy(cbs, r.callbacks)
	r.mu.RUnlock()

	for _, cb := range cbs {
		cb(announce)
	}
}
