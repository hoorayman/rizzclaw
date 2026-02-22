package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hoorayman/rizzclaw/internal/llm"
	"github.com/hoorayman/rizzclaw/internal/tools"
)

type Skill struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	Author      string                 `json:"author,omitempty"`
	Prompt      string                 `json:"prompt"`
	Tools       []string               `json:"tools,omitempty"`
	Config      map[string]any         `json:"config,omitempty"`
	Enabled     bool                   `json:"enabled"`
}

type SkillRegistry struct {
	skills   map[string]*Skill
	filePath string
	mu       sync.RWMutex
}

var globalRegistry *SkillRegistry
var registryOnce sync.Once

func GetSkillRegistry() *SkillRegistry {
	registryOnce.Do(func() {
		home, _ := os.UserHomeDir()
		skillsDir := filepath.Join(home, ".rizzclaw", "skills")
		os.MkdirAll(skillsDir, 0755)
		globalRegistry = &SkillRegistry{
			skills:   make(map[string]*Skill),
			filePath: filepath.Join(skillsDir, "skills.json"),
		}
		globalRegistry.load()
	})
	return globalRegistry
}

func (r *SkillRegistry) load() {
	data, err := os.ReadFile(r.filePath)
	if err != nil {
		return
	}

	var skills []*Skill
	if err := json.Unmarshal(data, &skills); err != nil {
		return
	}

	for _, skill := range skills {
		r.skills[skill.ID] = skill
	}
}

func (r *SkillRegistry) save() error {
	skills := make([]*Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		skills = append(skills, skill)
	}

	data, err := json.MarshalIndent(skills, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.filePath, data, 0644)
}

func (r *SkillRegistry) Register(skill *Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if skill.ID == "" {
		return fmt.Errorf("skill ID is required")
	}

	r.skills[skill.ID] = skill
	return r.save()
}

func (r *SkillRegistry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.skills, id)
	return r.save()
}

func (r *SkillRegistry) Get(id string) *Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.skills[id]
}

func (r *SkillRegistry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*Skill, 0, len(r.skills))
	for _, skill := range r.skills {
		skills = append(skills, skill)
	}
	return skills
}

func (r *SkillRegistry) ListEnabled() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*Skill, 0)
	for _, skill := range r.skills {
		if skill.Enabled {
			skills = append(skills, skill)
		}
	}
	return skills
}

func (r *SkillRegistry) Enable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	skill, ok := r.skills[id]
	if !ok {
		return fmt.Errorf("skill not found: %s", id)
	}

	skill.Enabled = true
	return r.save()
}

func (r *SkillRegistry) Disable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	skill, ok := r.skills[id]
	if !ok {
		return fmt.Errorf("skill not found: %s", id)
	}

	skill.Enabled = false
	return r.save()
}

func (s *Skill) GetPrompt() string {
	return s.Prompt
}

func (s *Skill) GetTools() []llm.Tool {
	var result []llm.Tool
	for _, toolName := range s.Tools {
		if tool := tools.GetTool(toolName); tool != nil {
			result = append(result, llm.Tool{
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: tool.InputSchema,
			})
		}
	}
	return result
}

func (s *Skill) Execute(ctx context.Context, input map[string]any) (string, error) {
	return "", fmt.Errorf("skill execution not implemented")
}

func GetEnabledSkillsPrompt() string {
	registry := GetSkillRegistry()
	skills := registry.ListEnabled()

	var prompts []string
	for _, skill := range skills {
		if skill.Prompt != "" {
			prompts = append(prompts, fmt.Sprintf("## Skill: %s\n%s", skill.Name, skill.Prompt))
		}
	}

	if len(prompts) > 0 {
		return "\n\n# Enabled Skills\n" + strings.Join(prompts, "\n\n")
	}
	return ""
}

func GetEnabledSkillsTools() []llm.Tool {
	registry := GetSkillRegistry()
	skills := registry.ListEnabled()

	toolMap := make(map[string]llm.Tool)
	for _, skill := range skills {
		for _, toolName := range skill.Tools {
			if tool := tools.GetTool(toolName); tool != nil {
				toolMap[toolName] = llm.Tool{
					Name:        tool.Name,
					Description: tool.Description,
					InputSchema: tool.InputSchema,
				}
			}
		}
	}

	result := make([]llm.Tool, 0, len(toolMap))
	for _, tool := range toolMap {
		result = append(result, tool)
	}
	return result
}
