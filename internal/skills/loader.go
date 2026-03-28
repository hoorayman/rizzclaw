package skills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type SkillMetadata struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Version     string   `yaml:"version" json:"version"`
	Author      string   `yaml:"author" json:"author"`
	Description string   `yaml:"description" json:"description"`
	Tools       []string `yaml:"tools" json:"tools"`
	Priority    int      `yaml:"priority" json:"priority"`
	Enabled     bool     `yaml:"enabled" json:"enabled"`
	Tags        []string `yaml:"tags" json:"tags"`
	When        string   `yaml:"when" json:"when"`
	Always      bool     `yaml:"always" json:"always"`
	Emoji       string   `yaml:"emoji" json:"emoji"`
	Homepage    string   `yaml:"homepage" json:"homepage"`
	OS          []string `yaml:"os" json:"os"`
	Requires    struct {
		Bins    []string `yaml:"bins" json:"bins"`
		AnyBins []string `yaml:"anyBins" json:"anyBins"`
		Env     []string `yaml:"env" json:"env"`
		Config  []string `yaml:"config" json:"config"`
	} `yaml:"requires" json:"requires"`
	Install []SkillInstallSpec `yaml:"install" json:"install"`
}

type SkillInstallSpec struct {
	ID      string `yaml:"id" json:"id"`
	Kind    string `yaml:"kind" json:"kind"`
	Formula string `yaml:"formula" json:"formula"`
	DocURL  string `yaml:"docUrl" json:"docUrl"`
}

type SkillFile struct {
	Metadata *SkillMetadata
	Content  string
	Path     string
	ModTime  time.Time
}

type SkillLoader struct {
	searchPaths []string
	cache       map[string]*SkillFile
	mu          sync.RWMutex
}

var globalLoader *SkillLoader
var loaderOnce sync.Once

func GetSkillLoader() *SkillLoader {
	loaderOnce.Do(func() {
		globalLoader = &SkillLoader{
			searchPaths: getDefaultSearchPaths(),
			cache:       make(map[string]*SkillFile),
		}
	})
	return globalLoader
}

func getDefaultSearchPaths() []string {
	paths := make([]string, 0)

	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, ".rizzclaw", "skills"))
		paths = append(paths, filepath.Join(cwd, "skills"))
		paths = append(paths, filepath.Join(cwd, ".agents", "skills"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".rizzclaw", "skills"))
		paths = append(paths, filepath.Join(home, ".agents", "skills"))
	}

	return paths
}

func (l *SkillLoader) AddSearchPath(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return
	}

	for _, p := range l.searchPaths {
		if p == absPath {
			return
		}
	}

	l.searchPaths = append(l.searchPaths, absPath)
}

func (l *SkillLoader) GetSearchPaths() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	paths := make([]string, len(l.searchPaths))
	copy(paths, l.searchPaths)
	return paths
}

func (l *SkillLoader) LoadAll() ([]*SkillFile, error) {
	l.mu.RLock()
	paths := make([]string, len(l.searchPaths))
	copy(paths, l.searchPaths)
	l.mu.RUnlock()

	var skills []*SkillFile
	seen := make(map[string]bool)

	for _, searchPath := range paths {
		if _, err := os.Stat(searchPath); os.IsNotExist(err) {
			continue
		}

		filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			if info.IsDir() {
				return nil
			}

			base := filepath.Base(path)
			if base == "SKILL.md" || strings.HasSuffix(base, ".skill.md") {
				skill, err := l.LoadFile(path)
				if err != nil {
					return nil
				}

				if skill.Metadata != nil && skill.Metadata.ID != "" {
					if seen[skill.Metadata.ID] {
						return nil
					}
					seen[skill.Metadata.ID] = true
				}

				skills = append(skills, skill)
			}

			return nil
		})
	}

	return skills, nil
}

func (l *SkillLoader) LoadFile(path string) (*SkillFile, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill file: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat skill file: %w", err)
	}

	metadata, body, err := parseFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	if metadata.ID == "" {
		base := filepath.Base(filepath.Dir(absPath))
		metadata.ID = strings.ToLower(strings.ReplaceAll(base, " ", "-"))
	}

	if metadata.Name == "" {
		metadata.Name = metadata.ID
	}

	if metadata.Priority == 0 {
		metadata.Priority = 100
	}

	skill := &SkillFile{
		Metadata: metadata,
		Content:  string(body),
		Path:     absPath,
		ModTime:  info.ModTime(),
	}

	l.mu.Lock()
	l.cache[absPath] = skill
	l.mu.Unlock()

	return skill, nil
}

func (l *SkillLoader) GetCached(path string) *SkillFile {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.cache[path]
}

func (l *SkillLoader) ClearCache() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache = make(map[string]*SkillFile)
}

func parseFrontmatter(content []byte) (*SkillMetadata, []byte, error) {
	metadata := &SkillMetadata{
		Enabled: true,
	}

	if !bytes.HasPrefix(content, []byte("---")) {
		return metadata, content, nil
	}

	parts := bytes.SplitN(content, []byte("---"), 3)
	if len(parts) < 3 {
		return metadata, content, nil
	}

	frontmatter := bytes.TrimSpace(parts[1])
	body := bytes.TrimSpace(parts[2])

	var yamlData map[string]interface{}
	if err := yaml.Unmarshal(frontmatter, &yamlData); err != nil {
		return metadata, body, nil
	}

	if id, ok := yamlData["id"].(string); ok {
		metadata.ID = id
	}
	if name, ok := yamlData["name"].(string); ok {
		metadata.Name = name
	}
	if version, ok := yamlData["version"].(string); ok {
		metadata.Version = version
	}
	if author, ok := yamlData["author"].(string); ok {
		metadata.Author = author
	}
	if description, ok := yamlData["description"].(string); ok {
		metadata.Description = description
	}
	if description, ok := yamlData["when"].(string); ok {
		metadata.When = description
	}
	if always, ok := yamlData["always"].(bool); ok {
		metadata.Always = always
	}
	if emoji, ok := yamlData["emoji"].(string); ok {
		metadata.Emoji = emoji
	}
	if homepage, ok := yamlData["homepage"].(string); ok {
		metadata.Homepage = homepage
	}
	if priority, ok := yamlData["priority"].(int); ok {
		metadata.Priority = priority
	}
	if enabled, ok := yamlData["enabled"].(bool); ok {
		metadata.Enabled = enabled
	}

	if osList, ok := yamlData["os"].([]interface{}); ok {
		for _, o := range osList {
			if s, ok := o.(string); ok {
				metadata.OS = append(metadata.OS, s)
			}
		}
	}

	if tools, ok := yamlData["tools"].([]interface{}); ok {
		for _, t := range tools {
			if s, ok := t.(string); ok {
				metadata.Tools = append(metadata.Tools, s)
			}
		}
	}

	if tags, ok := yamlData["tags"].([]interface{}); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				metadata.Tags = append(metadata.Tags, s)
			}
		}
	}

	if requires, ok := yamlData["requires"].(map[string]interface{}); ok {
		if bins, ok := requires["bins"].([]interface{}); ok {
			for _, b := range bins {
				if s, ok := b.(string); ok {
					metadata.Requires.Bins = append(metadata.Requires.Bins, s)
				}
			}
		}
		if anyBins, ok := requires["anyBins"].([]interface{}); ok {
			for _, b := range anyBins {
				if s, ok := b.(string); ok {
					metadata.Requires.AnyBins = append(metadata.Requires.AnyBins, s)
				}
			}
		}
		if env, ok := requires["env"].([]interface{}); ok {
			for _, e := range env {
				if s, ok := e.(string); ok {
					metadata.Requires.Env = append(metadata.Requires.Env, s)
				}
			}
		}
		if config, ok := requires["config"].([]interface{}); ok {
			for _, c := range config {
				if s, ok := c.(string); ok {
					metadata.Requires.Config = append(metadata.Requires.Config, s)
				}
			}
		}
	}

	if install, ok := yamlData["install"].([]interface{}); ok {
		for _, i := range install {
			if instMap, ok := i.(map[string]interface{}); ok {
				spec := SkillInstallSpec{}
				if id, ok := instMap["id"].(string); ok {
					spec.ID = id
				}
				if kind, ok := instMap["kind"].(string); ok {
					spec.Kind = kind
				}
				if formula, ok := instMap["formula"].(string); ok {
					spec.Formula = formula
				}
				if docURL, ok := instMap["docUrl"].(string); ok {
					spec.DocURL = docURL
				}
				metadata.Install = append(metadata.Install, spec)
			}
		}
	}

	return metadata, body, nil
}

func (l *SkillLoader) ToSkill(skillFile *SkillFile) *Skill {
	skill := &Skill{
		ID:          skillFile.Metadata.ID,
		Name:        skillFile.Metadata.Name,
		Description: skillFile.Metadata.Description,
		Version:     skillFile.Metadata.Version,
		Author:      skillFile.Metadata.Author,
		Prompt:      skillFile.Content,
		Tools:       skillFile.Metadata.Tools,
		Enabled:     skillFile.Metadata.Enabled,
		When:        skillFile.Metadata.When,
		Always:      skillFile.Metadata.Always,
		Emoji:       skillFile.Metadata.Emoji,
		Homepage:    skillFile.Metadata.Homepage,
		OS:          skillFile.Metadata.OS,
		Tags:        skillFile.Metadata.Tags,
		Priority:    skillFile.Metadata.Priority,
		SourcePath:  skillFile.Path,
	}

	skill.Requires.Bins = skillFile.Metadata.Requires.Bins
	skill.Requires.AnyBins = skillFile.Metadata.Requires.AnyBins
	skill.Requires.Env = skillFile.Metadata.Requires.Env
	skill.Requires.Config = skillFile.Metadata.Requires.Config

	for _, inst := range skillFile.Metadata.Install {
		skill.Install = append(skill.Install, SkillInstall{
			ID:      inst.ID,
			Kind:    inst.Kind,
			Formula: inst.Formula,
			DocURL:  inst.DocURL,
		})
	}

	return skill
}

func (l *SkillLoader) LoadAndRegister() error {
	skills, err := l.LoadAll()
	if err != nil {
		return err
	}

	registry := GetSkillRegistry()
	for _, sf := range skills {
		skill := l.ToSkill(sf)
		if err := registry.Register(skill); err != nil {
			return fmt.Errorf("failed to register skill %s: %w", sf.Metadata.ID, err)
		}
	}

	return registry.Save()
}

func (l *SkillLoader) Watch(interval time.Duration, onChange func(skill *SkillFile)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastModTimes := make(map[string]time.Time)

	for range ticker.C {
		skills, err := l.LoadAll()
		if err != nil {
			continue
		}

		for _, skill := range skills {
			lastMod, exists := lastModTimes[skill.Path]
			if !exists || skill.ModTime.After(lastMod) {
				lastModTimes[skill.Path] = skill.ModTime
				if onChange != nil {
					onChange(skill)
				}
			}
		}
	}
}

func LoadSkillsFromDirectory(dir string) ([]*Skill, error) {
	loader := GetSkillLoader()
	loader.AddSearchPath(dir)

	files, err := loader.LoadAll()
	if err != nil {
		return nil, err
	}

	skills := make([]*Skill, 0, len(files))
	for _, f := range files {
		skills = append(skills, loader.ToSkill(f))
	}

	return skills, nil
}
