package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hoorayman/rizzclaw/internal/agent/multiagent"
	"github.com/hoorayman/rizzclaw/internal/llm"
)

func init() {
	RegisterTool(&ToolDefinition{
		Name:        "spawn_subagent",
		Description: "Spawn a subagent to handle a specific task in parallel. The subagent will run independently and automatically notify you when complete. Use this for parallel work, complex tasks that can be delegated, or when you need to work on multiple things at once.",
		Handler:     SpawnSubagent,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"task": {
					Type:        "string",
					Description: "The task description for the subagent to complete. Be specific and detailed about what you want it to do.",
				},
				"label": {
					Type:        "string",
					Description: "A short label/name for this task (optional, helps identify the task in completion notifications).",
				},
				"model": {
					Type:        "string",
					Description: "Optional model override for this subagent (uses default if not specified).",
				},
				"timeout_seconds": {
					Type:        "number",
					Description: "Timeout in seconds for this task (default: 300, max: 3600).",
				},
				"cleanup": {
					Type:        "string",
					Description: "Cleanup mode: 'delete' to remove after completion, 'keep' to retain session (default: 'keep').",
					Enum:        []string{"delete", "keep"},
				},
			},
			Required: []string{"task"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "list_subagents",
		Description: "List all active and completed subagents. Shows status, task labels, duration, and outcomes. Use this to check on spawned subagents.",
		Handler:     ListSubagents,
		InputSchema: llm.InputSchema{
			Type:       "object",
			Properties: map[string]llm.ToolParameterProperty{},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "get_subagent_result",
		Description: "Get the full result/output of a completed subagent by its run ID. Use this when you need the detailed output from a completed task.",
		Handler:     GetSubagentResult,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"run_id": {
					Type:        "string",
					Description: "The run ID of the subagent (from spawn_subagent response or list_subagents output).",
				},
			},
			Required: []string{"run_id"},
		},
	})
}

func SpawnSubagent(ctx context.Context, input map[string]any) (string, error) {
	task, ok := input["task"].(string)
	if !ok || strings.TrimSpace(task) == "" {
		return "", fmt.Errorf("task is required and must be a non-empty string")
	}

	label, _ := input["label"].(string)
	model, _ := input["model"].(string)

	timeoutSeconds := 300
	if ts, ok := input["timeout_seconds"].(float64); ok {
		timeoutSeconds = int(ts)
		if timeoutSeconds > 3600 {
			timeoutSeconds = 3600
		}
		if timeoutSeconds <= 0 {
			timeoutSeconds = 300
		}
	}

	cleanup := multiagent.CleanupModeKeep
	if c, ok := input["cleanup"].(string); ok {
		switch c {
		case "delete":
			cleanup = multiagent.CleanupModeDelete
		case "keep":
			cleanup = multiagent.CleanupModeKeep
		}
	}

	params := &multiagent.SpawnParams{
		Task:              task,
		Label:             label,
		Model:             model,
		RunTimeoutSeconds: timeoutSeconds,
		Mode:              multiagent.SpawnModeRun,
		Cleanup:           cleanup,
		Channel:           multiagent.GetChannelFromContext(ctx),
		ChatID:            multiagent.GetChatIDFromContext(ctx),
	}

	result, err := multiagent.SpawnSubagent(ctx, "main", params)
	if err != nil {
		return "", fmt.Errorf("failed to spawn subagent: %w", err)
	}

	response := map[string]interface{}{
		"status":            result.Status,
		"message":           "Subagent spawned successfully! It will auto-notify you when complete.",
		"run_id":            result.RunID,
		"child_session_key": result.ChildSessionKey,
		"mode":              string(result.Mode),
		"note":              "✅ Subagent is now running. Results will auto-announce on completion - no need to poll.",
	}

	jsonBytes, _ := json.MarshalIndent(response, "", "  ")
	return string(jsonBytes), nil
}

type SubagentInfo struct {
	RunID     string                      `json:"run_id"`
	Task      string                      `json:"task"`
	Label     string                      `json:"label"`
	Status    string                      `json:"status"`
	Duration  string                      `json:"duration"`
	StartedAt string                      `json:"started_at"`
	Outcome   *multiagent.SubagentOutcome `json:"outcome,omitempty"`
}

func ListSubagents(ctx context.Context, input map[string]any) (string, error) {
	registry := multiagent.GetRegistry()
	allRuns := registry.GetAllRuns()

	if len(allRuns) == 0 {
		return "No subagents have been spawned yet.", nil
	}

	var infos []SubagentInfo

	for _, run := range allRuns {
		run.RLock()
		info := SubagentInfo{
			RunID:     run.RunID,
			Task:      run.Task,
			Label:     run.Label,
			StartedAt: run.StartedAt.Format("2006-01-02 15:04:05"),
		}

		if run.GetOutcome() != nil {
			info.Status = string(run.GetOutcome().Status)
			info.Duration = fmt.Sprintf("%v", run.GetOutcome().Duration)
			info.Outcome = run.GetOutcome()
		} else {
			info.Status = "running"
			info.Duration = fmt.Sprintf("%v", timeSince(run.StartedAt))
		}
		run.RUnlock()

		infos = append(infos, info)
	}

	activeCount := 0
	completedCount := 0
	for _, info := range infos {
		if info.Status == "running" {
			activeCount++
		} else {
			completedCount++
		}
	}

	summary := fmt.Sprintf("Subagent Summary: %d total (%d active, %d completed)\n\n",
		len(infos), activeCount, completedCount)

	var lines []string
	lines = append(lines, summary)

	for i, info := range infos {
		statusEmoji := "🔄"
		switch info.Status {
		case "completed":
			statusEmoji = "✅"
		case "failed":
			statusEmoji = "❌"
		case "timeout":
			statusEmoji = "⏱️"
		}

		line := fmt.Sprintf("%d. %s [%s] Task: %s", i+1, statusEmoji, info.RunID[:12], info.TaskLabel())
		if info.Label != "" {
			line += fmt.Sprintf(" (Label: %s)", info.Label)
		}
		line += fmt.Sprintf("\n   Status: %s | Duration: %s | Started: %s",
			info.Status, info.Duration, info.StartedAt)

		lines = append(lines, line)
		lines = append(lines, "")
	}

	result := map[string]interface{}{
		"summary":   summary,
		"subagents": infos,
		"total":     len(infos),
		"active":    activeCount,
		"completed": completedCount,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return strings.Join(lines, "\n") + "\n\n" + string(jsonBytes), nil
}

func GetSubagentResult(ctx context.Context, input map[string]any) (string, error) {
	runID, ok := input["run_id"].(string)
	if !ok || strings.TrimSpace(runID) == "" {
		return "", fmt.Errorf("run_id is required")
	}

	registry := multiagent.GetRegistry()
	run, exists := registry.GetRun(runID)
	if !exists {
		return "", fmt.Errorf("subagent with run_id %s not found", runID)
	}

	run.RLock()
	defer run.RUnlock()

	if run.GetOutcome() == nil {
		return fmt.Sprintf("Subagent %s is still running. It will auto-notify when complete.", runID), nil
	}

	result := map[string]interface{}{
		"run_id":    run.RunID,
		"task":      run.Task,
		"label":     run.Label,
		"status":    run.GetOutcome().Status,
		"duration":  fmt.Sprintf("%v", run.GetOutcome().Duration),
		"output":    run.GetOutcome().Output,
		"timestamp": run.GetOutcome().Timestamp.Format("2006-01-02 15:04:05"),
		"notified":  run.GetNotified(),
	}

	if run.GetOutcome().Error != "" {
		result["error"] = run.GetOutcome().Error
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return string(jsonBytes), nil
}

func timeSince(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

func (i *SubagentInfo) TaskLabel() string {
	if i.Label != "" {
		return i.Label
	}
	if len(i.Task) > 50 {
		return i.Task[:50] + "..."
	}
	return i.Task
}
