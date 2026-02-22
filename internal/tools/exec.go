package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type ExecInput struct {
	Command string            `json:"command"`
	Workdir string            `json:"workdir,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Timeout int               `json:"timeout,omitempty"`
}

type ExecOutput struct {
	Command   string `json:"command"`
	Workdir   string `json:"workdir"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exitCode"`
	Duration  int64  `json:"durationMs"`
	TimedOut  bool   `json:"timedOut,omitempty"`
	Success   bool   `json:"success"`
}

func Exec(ctx context.Context, input map[string]any) (string, error) {
	var params ExecInput
	data, _ := json.Marshal(input)
	if err := json.Unmarshal(data, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if params.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	workdir := params.Workdir
	if workdir == "" {
		workdir, _ = os.Getwd()
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", params.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", params.Command)
	}

	cmd.Dir = workdir

	env := os.Environ()
	for k, v := range params.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	timeout := params.Timeout
	if timeout <= 0 {
		timeout = 300
	}

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	startTime := time.Now()

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	var stdout, stderr strings.Builder
	stdoutDone := make(chan struct{})
	stderrDone := make(chan struct{})

	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			stdout.WriteString(scanner.Text() + "\n")
		}
		close(stdoutDone)
	}()

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			stderr.WriteString(scanner.Text() + "\n")
		}
		close(stderrDone)
	}()

	err = cmd.Wait()
	<-stdoutDone
	<-stderrDone

	duration := time.Since(startTime).Milliseconds()

	exitCode := 0
	timedOut := false
	success := true

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			timedOut = true
			success = false
			exitCode = -1
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			success = false
		} else {
			return "", fmt.Errorf("command failed: %w", err)
		}
	}

	output := ExecOutput{
		Command:  params.Command,
		Workdir:  workdir,
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: exitCode,
		Duration: duration,
		TimedOut: timedOut,
		Success:  success,
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}

type ProcessListInput struct {
	Running bool `json:"running,omitempty"`
}

type ProcessInfo struct {
	ID        string `json:"id"`
	Command   string `json:"command"`
	Status    string `json:"status"`
	StartedAt int64  `json:"startedAt"`
	Pid       int    `json:"pid,omitempty"`
}

type ProcessListOutput struct {
	Processes []ProcessInfo `json:"processes"`
	Count     int           `json:"count"`
}

type ProcessSession struct {
	ID        string
	Command   string
	Status    string
	StartedAt time.Time
	Pid       int
	Process   *exec.Cmd
	Cancel    context.CancelFunc
}

var processSessions = make(map[string]*ProcessSession)
var processCounter = 0

func ExecBackground(ctx context.Context, input map[string]any) (string, error) {
	var params ExecInput
	data, _ := json.Marshal(input)
	if err := json.Unmarshal(data, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if params.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	workdir := params.Workdir
	if workdir == "" {
		workdir, _ = os.Getwd()
	}

	processCounter++
	sessionID := fmt.Sprintf("proc-%d", processCounter)

	ctx, cancel := context.WithCancel(context.Background())

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", params.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", params.Command)
	}

	cmd.Dir = workdir

	env := os.Environ()
	for k, v := range params.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	session := &ProcessSession{
		ID:        sessionID,
		Command:   params.Command,
		Status:    "running",
		StartedAt: time.Now(),
		Cancel:    cancel,
		Process:   cmd,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	session.Pid = cmd.Process.Pid
	processSessions[sessionID] = session

	go func() {
		cmd.Wait()
		if session.Status == "running" {
			session.Status = "completed"
		}
	}()

	output := ProcessInfo{
		ID:        sessionID,
		Command:   params.Command,
		Status:    "running",
		StartedAt: session.StartedAt.Unix(),
		Pid:       session.Pid,
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}

func ProcessList(ctx context.Context, input map[string]any) (string, error) {
	var params ProcessListInput
	data, _ := json.Marshal(input)
	if err := json.Unmarshal(data, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	var processes []ProcessInfo

	for _, session := range processSessions {
		if params.Running && session.Status != "running" {
			continue
		}

		processes = append(processes, ProcessInfo{
			ID:        session.ID,
			Command:   session.Command,
			Status:    session.Status,
			StartedAt: session.StartedAt.Unix(),
			Pid:       session.Pid,
		})
	}

	output := ProcessListOutput{
		Processes: processes,
		Count:     len(processes),
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}

type ProcessKillInput struct {
	ID string `json:"id"`
}

func ProcessKill(ctx context.Context, input map[string]any) (string, error) {
	var params ProcessKillInput
	data, _ := json.Marshal(input)
	if err := json.Unmarshal(data, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	if params.ID == "" {
		return "", fmt.Errorf("id is required")
	}

	session, ok := processSessions[params.ID]
	if !ok {
		return "", fmt.Errorf("process not found: %s", params.ID)
	}

	if session.Status != "running" {
		return "", fmt.Errorf("process is not running: %s", params.ID)
	}

	session.Cancel()
	if session.Process.Process != nil {
		session.Process.Process.Kill()
	}
	session.Status = "killed"

	output := map[string]any{
		"id":      params.ID,
		"killed":  true,
		"message": fmt.Sprintf("Process %s killed", params.ID),
	}

	outputJSON, _ := json.MarshalIndent(output, "", "  ")
	return string(outputJSON), nil
}
