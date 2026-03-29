package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/hoorayman/rizzclaw/internal/llm"
	"github.com/hoorayman/rizzclaw/internal/tools"
)

type Skill struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Version     string         `json:"version"`
	Author      string         `json:"author,omitempty"`
	Prompt      string         `json:"prompt"`
	Tools       []string       `json:"tools,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	Enabled     bool           `json:"enabled"`
	When        string         `json:"when,omitempty"`
	Always      bool           `json:"always,omitempty"`
	Emoji       string         `json:"emoji,omitempty"`
	Homepage    string         `json:"homepage,omitempty"`
	OS          []string       `json:"os,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	Requires    SkillRequires  `json:"requires,omitempty"`
	Install     []SkillInstall `json:"install,omitempty"`
	SourcePath  string         `json:"sourcePath,omitempty"`
	Priority    int            `json:"priority,omitempty"`
}

type SkillRequires struct {
	Bins    []string `json:"bins,omitempty"`
	AnyBins []string `json:"anyBins,omitempty"`
	Env     []string `json:"env,omitempty"`
	Config  []string `json:"config,omitempty"`
}

type SkillInstall struct {
	ID      string `json:"id,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Formula string `json:"formula,omitempty"`
	DocURL  string `json:"docUrl,omitempty"`
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
		var skillsDir, filePath string

		home, homeErr := os.UserHomeDir()
		if homeErr == nil {
			skillsDir = filepath.Join(home, ".rizzclaw", "skills")
			filePath = filepath.Join(skillsDir, "skills.json")

			if err := os.MkdirAll(skillsDir, 0755); err == nil {
				if _, err := os.Stat(filePath); os.IsNotExist(err) {
					if err := os.WriteFile(filePath, []byte("[]"), 0644); err != nil {
						homeErr = err
					}
				}
			} else {
				homeErr = err
			}
		}

		if homeErr != nil {
			cwd, _ := os.Getwd()
			skillsDir = filepath.Join(cwd, ".rizzclaw", "skills")
			filePath = filepath.Join(skillsDir, "skills.json")
			os.MkdirAll(skillsDir, 0755)

			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				os.WriteFile(filePath, []byte("[]"), 0644)
			}
		}

		globalRegistry = &SkillRegistry{
			skills:   make(map[string]*Skill),
			filePath: filePath,
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

	dir := filepath.Dir(r.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		cwd, _ := os.Getwd()
		fallbackDir := filepath.Join(cwd, ".rizzclaw", "skills")
		if err := os.MkdirAll(fallbackDir, 0755); err != nil {
			return fmt.Errorf("failed to create skills directory: %w", err)
		}
		r.filePath = filepath.Join(fallbackDir, "skills.json")
		dir = fallbackDir
	}

	if err := os.WriteFile(r.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write skills file %s: %w", r.filePath, err)
	}
	return nil
}

func (r *SkillRegistry) Register(skill *Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if skill.ID == "" {
		return fmt.Errorf("skill ID is required")
	}

	r.skills[skill.ID] = skill
	return nil
}

func (r *SkillRegistry) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.save()
}

func (r *SkillRegistry) RegisterAndSave(skill *Skill) error {
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

func (s *Skill) IsEligible() bool {
	if !s.Enabled {
		return false
	}

	if len(s.OS) > 0 {
		currentOS := runtime.GOOS
		found := false
		for _, os := range s.OS {
			if os == currentOS || os == "all" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(s.Requires.Bins) > 0 {
		for _, bin := range s.Requires.Bins {
			if _, err := exec.LookPath(bin); err != nil {
				return false
			}
		}
	}

	if len(s.Requires.AnyBins) > 0 {
		found := false
		for _, bin := range s.Requires.AnyBins {
			if _, err := exec.LookPath(bin); err == nil {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(s.Requires.Env) > 0 {
		for _, env := range s.Requires.Env {
			if os.Getenv(env) == "" {
				return false
			}
		}
	}

	return true
}

func (s *Skill) CheckDependencies() []string {
	var missing []string

	for _, bin := range s.Requires.Bins {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, fmt.Sprintf("binary: %s", bin))
		}
	}

	if len(s.Requires.AnyBins) > 0 {
		found := false
		for _, bin := range s.Requires.AnyBins {
			if _, err := exec.LookPath(bin); err == nil {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, fmt.Sprintf("any binary: %s", strings.Join(s.Requires.AnyBins, " or ")))
		}
	}

	for _, env := range s.Requires.Env {
		if os.Getenv(env) == "" {
			missing = append(missing, fmt.Sprintf("env: %s", env))
		}
	}

	return missing
}

func GetEligibleSkills() []*Skill {
	registry := GetSkillRegistry()
	skills := registry.ListEnabled()

	var eligible []*Skill
	for _, skill := range skills {
		if skill.IsEligible() {
			eligible = append(eligible, skill)
		}
	}
	return eligible
}

func GetEligibleSkillsPrompt() string {
	skills := GetEligibleSkills()

	var prompts []string
	for _, skill := range skills {
		if skill.Prompt != "" {
			header := skill.Name
			if skill.Emoji != "" {
				header = skill.Emoji + " " + header
			}
			
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("## Skill: %s\n", header))
			sb.WriteString(fmt.Sprintf("Source: %s\n\n", skill.SourcePath))
			sb.WriteString(skill.Prompt)
			
			prompts = append(prompts, sb.String())
		}
	}

	if len(prompts) > 0 {
		return "\n\n# Enabled Skills\n" + strings.Join(prompts, "\n\n")
	}
	return ""
}

func GetEligibleSkillsTools() []llm.Tool {
	skills := GetEligibleSkills()

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

func LoadAllSkillsFromDisk() error {
	loader := GetSkillLoader()
	return loader.LoadAndRegister()
}
