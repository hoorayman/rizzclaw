package prompt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SystemPrompt struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Content     string    `json:"content"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type PromptManager struct {
	prompts  map[string]*SystemPrompt
	filePath string
	mu       sync.RWMutex
}

var globalManager *PromptManager
var managerOnce sync.Once

func GetPromptManager() *PromptManager {
	managerOnce.Do(func() {
		home, _ := os.UserHomeDir()
		promptsDir := filepath.Join(home, ".rizzclaw", "prompts")
		os.MkdirAll(promptsDir, 0755)
		globalManager = &PromptManager{
			prompts:  make(map[string]*SystemPrompt),
			filePath: filepath.Join(promptsDir, "prompts.json"),
		}
		globalManager.load()
	})
	return globalManager
}

func (m *PromptManager) load() {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return
	}

	var prompts []*SystemPrompt
	if err := json.Unmarshal(data, &prompts); err != nil {
		return
	}

	for _, prompt := range prompts {
		m.prompts[prompt.ID] = prompt
	}
}

func (m *PromptManager) save() error {
	prompts := make([]*SystemPrompt, 0, len(m.prompts))
	for _, prompt := range m.prompts {
		prompts = append(prompts, prompt)
	}

	data, err := json.MarshalIndent(prompts, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.filePath, data, 0644)
}

func (m *PromptManager) Create(id, name, content, description string) (*SystemPrompt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if id == "" {
		return nil, fmt.Errorf("prompt ID is required")
	}

	if _, exists := m.prompts[id]; exists {
		return nil, fmt.Errorf("prompt already exists: %s", id)
	}

	now := time.Now()
	prompt := &SystemPrompt{
		ID:          id,
		Name:        name,
		Content:     content,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	m.prompts[id] = prompt
	if err := m.save(); err != nil {
		delete(m.prompts, id)
		return nil, err
	}

	return prompt, nil
}

func (m *PromptManager) Update(id, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prompt, ok := m.prompts[id]
	if !ok {
		return fmt.Errorf("prompt not found: %s", id)
	}

	prompt.Content = content
	prompt.UpdatedAt = time.Now()
	return m.save()
}

func (m *PromptManager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.prompts[id]; !ok {
		return fmt.Errorf("prompt not found: %s", id)
	}

	delete(m.prompts, id)
	return m.save()
}

func (m *PromptManager) Get(id string) *SystemPrompt {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.prompts[id]
}

func (m *PromptManager) List() []*SystemPrompt {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prompts := make([]*SystemPrompt, 0, len(m.prompts))
	for _, prompt := range m.prompts {
		prompts = append(prompts, prompt)
	}
	return prompts
}

const DefaultSystemPrompt = `You are RizzClaw, an AI coding assistant powered by MiniMax. You help users with software development tasks.

## Capabilities
- Read, write, and edit files
- Execute shell commands
- Search for files and content
- Create and modify code

## Guidelines
- Be helpful, accurate, and concise
- Explain your reasoning when making changes
- Ask for clarification when needed
- Follow best practices for the language/framework being used

## Available Tools
- file_read: Read file contents
- file_write: Write content to a file
- file_list: List files in a directory
- file_search: Search for files or content
- file_delete: Delete a file or directory
- exec: Execute a shell command
- exec_background: Execute a command in background
- process_list: List running processes
- process_kill: Kill a background process
- edit: Edit a file by replacing text
- edit_regex: Edit using regex patterns
- apply_patch: Apply a unified diff patch
- create: Create a new file

Always use the appropriate tool for the task. Confirm before making destructive changes.`

func BuildSystemPrompt(customPrompt string, includeSkills bool) string {
	var parts []string

	if customPrompt != "" {
		parts = append(parts, customPrompt)
	} else {
		parts = append(parts, DefaultSystemPrompt)
	}

	return strings.Join(parts, "\n\n")
}

func GetEnabledSkillsPrompt() string {
	return ""
}
