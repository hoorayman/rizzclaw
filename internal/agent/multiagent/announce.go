package multiagent

import (
	"fmt"
	"strings"
	"time"
)

func triggerAnnouncement(run *SubagentRun) {
	registry := GetRegistry()

	run.mu.RLock()
	outcome := run.Outcome
	run.mu.RUnlock()

	if outcome == nil {
		return
	}

	announce := buildAnnounceMessage(run, outcome)

	registry.TriggerCallbacks(announce)

	registry.MarkNotified(run.RunID)

	if run.Cleanup == CleanupModeDelete {
		go func() {
			time.Sleep(5 * time.Second)
			registry.CleanupRun(run.RunID)
		}()
	}
}

func buildAnnounceMessage(run *SubagentRun, outcome *SubagentOutcome) *AnnounceMessage {
	taskLabel := run.Label
	if taskLabel == "" {
		taskLabel = run.Task
		if len(taskLabel) > 50 {
			taskLabel = taskLabel[:50] + "..."
		}
	}

	statusEmoji := getStatusEmoji(outcome.Status)
	statusText := getStatusText(outcome.Status)

	header := fmt.Sprintf("%s Subagent %s %s",
		statusEmoji,
		taskLabel,
		statusText,
	)

	findings := strings.TrimSpace(outcome.Output)
	if findings == "" || findings == "(no output)" {
		return &AnnounceMessage{
			RunID:     run.RunID,
			TaskLabel: taskLabel,
			Status:    outcome.Status,
			Output:    "",
			Duration:  formatDuration(outcome.Duration),
			Timestamp: outcome.Timestamp,
			Message:   header,
			Channel:   run.Channel,
			ChatID:    run.ChatID,
		}
	}

	message := fmt.Sprintf("%s\n\n%s", header, findings)

	statsLine := buildStatsLine(run, outcome)
	if statsLine != "" {
		message += fmt.Sprintf("\n\n%s", statsLine)
	}

	return &AnnounceMessage{
		RunID:     run.RunID,
		TaskLabel: taskLabel,
		Status:    outcome.Status,
		Output:    findings,
		Duration:  formatDuration(outcome.Duration),
		Timestamp: outcome.Timestamp,
		Message:   message,
		Channel:   run.Channel,
		ChatID:    run.ChatID,
	}
}

func getStatusEmoji(status SubagentStatus) string {
	switch status {
	case SubagentStatusCompleted:
		return "✅"
	case SubagentStatusFailed:
		return "❌"
	case SubagentStatusTimeout:
		return "⏱️"
	default:
		return "❓"
	}
}

func getStatusText(status SubagentStatus) string {
	switch status {
	case SubagentStatusCompleted:
		return "completed successfully"
	case SubagentStatusFailed:
		return fmt.Sprintf("failed: %s", "unknown error")
	case SubagentStatusTimeout:
		return "timed out"
	default:
		return "finished with unknown status"
	}
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "n/a"
	}

	totalSeconds := int(d.Seconds())
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func buildStatsLine(run *SubagentRun, outcome *SubagentOutcome) string {
	parts := []string{
		fmt.Sprintf("runtime: %s", formatDuration(outcome.Duration)),
		fmt.Sprintf("status: %s", outcome.Status),
	}

	if outcome.Error != "" {
		parts = append(parts, fmt.Sprintf("error: %s", outcome.Error))
	}

	return fmt.Sprintf("Stats: %s", strings.Join(parts, " • "))
}

func BuildCompletionNotificationForUser(announces []*AnnounceMessage) string {
	if len(announces) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "\n📋 **Multi-Agent Task Completion Report**\n")

	for i, announce := range announces {
		lines = append(lines, fmt.Sprintf("--- Task %d ---", i+1))
		lines = append(lines, fmt.Sprintf("Task: %s", announce.TaskLabel))
		lines = append(lines, fmt.Sprintf("Status: %s %s", getStatusEmoji(announce.Status), announce.Status))
		lines = append(lines, fmt.Sprintf("Duration: %s", announce.Duration))
		lines = append(lines, fmt.Sprintf("Completed at: %s", announce.Timestamp.Format("2006-01-02 15:04:05")))

		if announce.Output != "" {
			outputPreview := announce.Output
			if len(outputPreview) > 500 {
				outputPreview = outputPreview[:500] + "... (truncated)"
			}
			lines = append(lines, fmt.Sprintf("\nResult:\n%s\n", outputPreview))
		}

		lines = append(lines, "")
	}

	activeRuns := GetRegistry().GetAllRuns()
	activeCount := 0
	for _, run := range activeRuns {
		run.mu.RLock()
		if run.Outcome == nil {
			activeCount++
		}
		run.mu.RUnlock()
	}

	if activeCount > 0 {
		lines = append(lines, fmt.Sprintf("\nℹ️  There are still %d active subagent(s) running...", activeCount))
	} else {
		lines = append(lines, "\n✨ All subagent tasks have been completed!")
	}

	return strings.Join(lines, "\n")
}
