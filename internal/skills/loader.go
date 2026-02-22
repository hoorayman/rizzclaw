package skills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
	}

	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".rizzclaw", "skills"))
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

	lines := bytes.Split(frontmatter, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		colonIdx := bytes.Index(line, []byte(":"))
		if colonIdx == -1 {
			continue
		}

		key := string(bytes.TrimSpace(line[:colonIdx]))
		value := bytes.TrimSpace(line[colonIdx+1:])

		value = bytes.Trim(value, "\"'")

		switch key {
		case "id":
			metadata.ID = string(value)
		case "name":
			metadata.Name = string(value)
		case "version":
			metadata.Version = string(value)
		case "author":
			metadata.Author = string(value)
		case "description":
			metadata.Description = string(value)
		case "priority":
			var p int
			fmt.Sscanf(string(value), "%d", &p)
			metadata.Priority = p
		case "enabled":
			metadata.Enabled = strings.ToLower(string(value)) == "true"
		case "tools":
			if len(value) > 0 && value[0] == '[' {
				toolsStr := strings.Trim(string(value), "[]")
				for _, t := range strings.Split(toolsStr, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						metadata.Tools = append(metadata.Tools, strings.Trim(t, "\"'"))
					}
				}
			}
		case "tags":
			if len(value) > 0 && value[0] == '[' {
				tagsStr := strings.Trim(string(value), "[]")
				for _, t := range strings.Split(tagsStr, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						metadata.Tags = append(metadata.Tags, strings.Trim(t, "\"'"))
					}
				}
			}
		}
	}

	return metadata, body, nil
}

func (l *SkillLoader) ToSkill(skillFile *SkillFile) *Skill {
	return &Skill{
		ID:          skillFile.Metadata.ID,
		Name:        skillFile.Metadata.Name,
		Description: skillFile.Metadata.Description,
		Version:     skillFile.Metadata.Version,
		Author:      skillFile.Metadata.Author,
		Prompt:      skillFile.Content,
		Tools:       skillFile.Metadata.Tools,
		Enabled:     skillFile.Metadata.Enabled,
	}
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

	return nil
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
