package multiagent

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

type AgentRunner interface {
	RunSilent(ctx context.Context, input string) (string, error)
	SetModel(model string)
}

type AgentCreator func(id string, opts ...AgentOption) (AgentRunner, error)

type AgentOption func(interface{})

var globalAgentCreator AgentCreator

func SetAgentCreator(creator AgentCreator) {
	globalAgentCreator = creator
}

type SpawnParams struct {
	Task              string
	Label             string
	Model             string
	RunTimeoutSeconds int
	Mode              SpawnMode
	Cleanup           CleanupMode
	Channel           string
	ChatID            string
}

type SpawnResult struct {
	Status          string    `json:"status"`
	ChildSessionKey string    `json:"child_session_key,omitempty"`
	RunID           string    `json:"run_id,omitempty"`
	Mode            SpawnMode `json:"mode,omitempty"`
	Error           string    `json:"error,omitempty"`
}

func buildSubagentSystemPrompt(params *SpawnParams, depth int, maxDepth int) string {
	taskText := strings.TrimSpace(params.Task)
	if taskText == "" {
		taskText = "{{TASK_DESCRIPTION}}"
	}

	canSpawn := depth < maxDepth
	parentLabel := "main agent"
	if depth >= 2 {
		parentLabel = "parent orchestrator"
	}

	lines := []string{
		"# Subagent Context",
		"",
		fmt.Sprintf("You are a **subagent** spawned by the %s for a specific task.", parentLabel),
		"",
		"## Your Role",
		fmt.Sprintf("- You were created to handle: %s", taskText),
		"- Complete this task. That's your entire purpose.",
		fmt.Sprintf("- You are NOT the %s. Don't try to be.", parentLabel),
		"",
		"## Rules",
		"1. **Stay focused** - Do your assigned task, nothing else",
		fmt.Sprintf("2. **Complete the task** - Your final message will be automatically reported to the %s", parentLabel),
		"3. **Don't initiate** - No heartbeats, no proactive actions, no side quests",
		"4. **Be ephemeral** - You may be terminated after task completion. That's fine.",
		"5. **Trust push-based completion** - Results are auto-announced back; do not busy-poll for status.",
		"",
		"## Output Format",
		"When complete, your final response should include:",
		"- What you accomplished or found",
		"- Any relevant details the parent agent should know",
		"- Keep it concise but informative",
		"",
		"## What You DON'T Do",
		fmt.Sprintf("- NO user conversations (that's %s's job)", parentLabel),
		"- NO external messages unless explicitly tasked",
		"- NO cron jobs or persistent state",
		fmt.Sprintf("- NO pretending to be the %s", parentLabel),
		"",
	}

	if canSpawn {
		lines = append(lines,
			"## Sub-Agent Spawning",
			"You CAN spawn your own sub-agents for parallel or complex work.",
			"Your sub-agents will announce their results back to you automatically.",
			"Default workflow: spawn work, continue orchestrating, and wait for auto-announced completions.",
			"",
		)
	} else if depth >= 2 {
		lines = append(lines,
			"## Sub-Agent Spawning",
			"You are a leaf worker and CANNOT spawn further sub-agents. Focus on your assigned task.",
			"",
		)
	}

	lines = append(lines,
		"## Session Context",
		fmt.Sprintf("- Depth: %d/%d", depth, maxDepth),
		fmt.Sprintf("- Task: %s", taskText),
		"",
	)

	return strings.Join(lines, "\n")
}

func SpawnSubagent(ctx context.Context, requesterSessionKey string, params *SpawnParams) (*SpawnResult, error) {
	if globalAgentCreator == nil {
		return &SpawnResult{
			Status: "error",
			Error:  "agent creator not initialized",
		}, nil
	}

	registry := GetRegistry()

	activeChildren := registry.CountActiveRunsForRequester(requesterSessionKey)
	if activeChildren >= DefaultMaxChildrenPerAgent {
		return &SpawnResult{
			Status: "forbidden",
			Error:  fmt.Sprintf("max active children reached (%d/%d)", activeChildren, DefaultMaxChildrenPerAgent),
		}, nil
	}

	childSessionKey := fmt.Sprintf("agent:subagent:%s", generateUUID())
	runID := registry.GenerateRunID()

	run := &SubagentRun{
		RunID:               runID,
		ChildSessionKey:     childSessionKey,
		RequesterSessionKey: requesterSessionKey,
		Channel:             params.Channel,
		ChatID:              params.ChatID,
		Task:                params.Task,
		Label:               params.Label,
		Model:               params.Model,
		Depth:               1,
		SpawnMode:           params.Mode,
		Cleanup:             params.Cleanup,
		StartedAt:           time.Now(),
		Notified:            false,
	}

	if err := registry.RegisterRun(run); err != nil {
		return &SpawnResult{
			Status: "error",
			Error:  fmt.Errorf("failed to register run: %w", err).Error(),
		}, nil
	}

	systemPrompt := buildSubagentSystemPrompt(params, run.Depth, DefaultMaxSpawnDepth)

	go func() {
		runSubagentAsync(ctx, run, systemPrompt, params)
	}()

	return &SpawnResult{
		Status:          "accepted",
		RunID:           runID,
		ChildSessionKey: childSessionKey,
		Mode:            params.Mode,
	}, nil
}

func runSubagentAsync(ctx context.Context, run *SubagentRun, systemPrompt string, params *SpawnParams) {
	registry := GetRegistry()
	startTime := time.Now()

	subagent, err := globalAgentCreator(run.ChildSessionKey,
		func(a interface{}) {
			if agent, ok := a.(interface{ SetSystemPrompt(string) }); ok {
				agent.SetSystemPrompt(systemPrompt)
			}
		},
		func(a interface{}) {
			if agent, ok := a.(interface{ SetName(string) }); ok {
				agent.SetName(run.Label)
			}
		},
	)
	if err != nil {
		outcome := &SubagentOutcome{
			Status:    SubagentStatusFailed,
			Error:     fmt.Sprintf("failed to create subagent: %v", err),
			Duration:  time.Since(startTime),
			Output:    "",
			Timestamp: time.Now(),
		}
		registry.UpdateRunOutcome(run.RunID, outcome)
		triggerAnnouncement(run)
		return
	}

	if params.Model != "" {
		subagent.SetModel(params.Model)
	}

	timeout := time.Duration(params.RunTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, err := subagent.RunSilent(ctxWithTimeout, params.Task)

	outcome := &SubagentOutcome{
		Duration:  time.Since(startTime),
		Output:    output,
		Timestamp: time.Now(),
	}

	if err != nil {
		if ctxWithTimeout.Err() == context.DeadlineExceeded {
			outcome.Status = SubagentStatusTimeout
			outcome.Error = "task timed out"
		} else {
			outcome.Status = SubagentStatusFailed
			outcome.Error = err.Error()
		}
	} else {
		outcome.Status = SubagentStatusCompleted
	}

	registry.UpdateRunOutcome(run.RunID, outcome)
	triggerAnnouncement(run)
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
