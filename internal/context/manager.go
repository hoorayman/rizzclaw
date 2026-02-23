package context

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	workspaceDir string
	config       *ContextConfig
	cache        map[string]*ContextFile
	mu           sync.RWMutex
}

var globalManager *Manager
var managerOnce sync.Once

func GetManager() *Manager {
	managerOnce.Do(func() {
		home, _ := os.UserHomeDir()
		workspaceDir := filepath.Join(home, ".rizzclaw", "workspace")
		os.MkdirAll(workspaceDir, 0755)
		
		globalManager = &Manager{
			workspaceDir: workspaceDir,
			config:       DefaultContextConfig(),
			cache:        make(map[string]*ContextFile),
		}
		globalManager.loadAllFiles()
		
		if globalManager.needsInitialization() {
			InitializeWorkspace(workspaceDir)
			globalManager.loadAllFiles()
		}
	})
	return globalManager
}

func (m *Manager) needsInitialization() bool {
	for _, filename := range BootstrapFilenames {
		cf := m.cache[filename]
		if cf == nil || cf.Content == "" {
			return true
		}
	}
	return false
}

func NewManager(workspaceDir string, config *ContextConfig) *Manager {
	if config == nil {
		config = DefaultContextConfig()
	}
	os.MkdirAll(workspaceDir, 0755)
	
	m := &Manager{
		workspaceDir: workspaceDir,
		config:       config,
		cache:        make(map[string]*ContextFile),
	}
	m.loadAllFiles()
	return m
}

func (m *Manager) loadAllFiles() {
	for _, filename := range BootstrapFilenames {
		m.loadFile(filename)
	}
}

func (m *Manager) loadFile(filename string) *ContextFile {
	path := filepath.Join(m.workspaceDir, filename)
	
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		m.mu.Lock()
		m.cache[filename] = &ContextFile{
			Path:    path,
			Content: "",
			ModTime: time.Now(),
		}
		m.mu.Unlock()
		return m.cache[filename]
	}
	
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	
	contentStr := string(content)
	truncated := false
	originalLen := len(contentStr)
	
	if len(contentStr) > m.config.BootstrapMaxChars {
		contentStr = contentStr[:m.config.BootstrapMaxChars]
		truncated = true
	}
	
	cf := &ContextFile{
		Path:        path,
		Content:     contentStr,
		ModTime:     info.ModTime(),
		Size:        info.Size(),
		Truncated:   truncated,
		OriginalLen: originalLen,
	}
	
	m.mu.Lock()
	m.cache[filename] = cf
	m.mu.Unlock()
	
	return cf
}

func (m *Manager) GetFile(filename string) *ContextFile {
	m.mu.RLock()
	cf, exists := m.cache[filename]
	m.mu.RUnlock()
	
	if !exists {
		return m.loadFile(filename)
	}
	
	path := filepath.Join(m.workspaceDir, filename)
	info, err := os.Stat(path)
	if err == nil && info.ModTime().After(cf.ModTime) {
		return m.loadFile(filename)
	}
	
	return cf
}

func (m *Manager) SaveFile(filename, content string) error {
	path := filepath.Join(m.workspaceDir, filename)
	
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to save %s: %w", filename, err)
	}
	
	m.loadFile(filename)
	return nil
}

func (m *Manager) AppendToFile(filename, content string) error {
	path := filepath.Join(m.workspaceDir, filename)
	
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", filename, err)
	}
	defer f.Close()
	
	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("failed to append to %s: %w", filename, err)
	}
	
	m.loadFile(filename)
	return nil
}

func (m *Manager) GetAgentsContent() string {
	return m.GetFile(AgentsFilename).Content
}

func (m *Manager) GetSoulContent() string {
	return m.GetFile(SoulFilename).Content
}

func (m *Manager) GetUserContent() string {
	return m.GetFile(UserFilename).Content
}

func (m *Manager) GetIdentityContent() string {
	return m.GetFile(IdentityFilename).Content
}

func (m *Manager) GetMemoryContent() string {
	return m.GetFile(MemoryFilename).Content
}

func (m *Manager) LoadBootstrapFiles() []*BootstrapFile {
	var files []*BootstrapFile
	
	priorities := map[string]int{
		AgentsFilename:   100,
		SoulFilename:     90,
		IdentityFilename: 80,
		UserFilename:     70,
		MemoryFilename:   60,
	}
	
	for _, filename := range BootstrapFilenames {
		cf := m.GetFile(filename)
		if cf.Content == "" {
			continue
		}
		
		priority := priorities[filename]
		if priority == 0 {
			priority = 50
		}
		
		files = append(files, &BootstrapFile{
			Name:      filename,
			Path:      cf.Path,
			Content:   cf.Content,
			MaxChars:  m.config.BootstrapMaxChars,
			Truncated: cf.Truncated,
			Priority:  priority,
		})
	}
	
	return files
}

func (m *Manager) BuildSystemPrompt(basePrompt string) string {
	var buf bytes.Buffer
	
	identity := m.GetFile(IdentityFilename)
	if identity.Content != "" {
		parsed := ParseIdentityMarkdown(identity.Content)
		if parsed.Name != "" {
			buf.WriteString(fmt.Sprintf("Your name is %s. ", parsed.Name))
		}
		if parsed.Emoji != "" {
			buf.WriteString(fmt.Sprintf("Your emoji is %s. ", parsed.Emoji))
		}
		if parsed.Vibe != "" {
			buf.WriteString(fmt.Sprintf("Your style is: %s. ", parsed.Vibe))
		}
		buf.WriteString("\n\n")
	}
	
	buf.WriteString(basePrompt)
	buf.WriteString("\n\n")
	
	files := m.LoadBootstrapFiles()
	if len(files) == 0 {
		return buf.String()
	}
	
	buf.WriteString("# Project Context\n\n")
	buf.WriteString("The following project context files have been loaded:\n\n")
	
	for _, file := range files {
		cf := m.GetFile(file.Name)
		if cf.Content == "" {
			continue
		}
		buf.WriteString(fmt.Sprintf("## %s\n\n", file.Name))
		buf.WriteString(cf.Content)
		if cf.Truncated {
			buf.WriteString(fmt.Sprintf("\n\n[Content truncated, original length: %d chars]", cf.OriginalLen))
		}
		buf.WriteString("\n\n")
	}
	
	soulContent := m.GetFile(SoulFilename).Content
	if soulContent != "" {
		buf.WriteString("If SOUL.md is present, embody its persona and tone. ")
		buf.WriteString("Avoid stiff, generic replies; follow its guidance unless higher-priority instructions override it.\n\n")
	}
	
	return buf.String()
}

func (m *Manager) GetWorkspaceDir() string {
	return m.workspaceDir
}

func (m *Manager) SetWorkspaceDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workspaceDir = dir
	m.cache = make(map[string]*ContextFile)
	m.loadAllFiles()
}

func (m *Manager) GetConfig() *ContextConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

func (m *Manager) SetConfig(config *ContextConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

func (m *Manager) ClearCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = make(map[string]*ContextFile)
}

var placeholderValues = map[string]bool{
	"pick something you like":                                  true,
	"ai? robot? familiar? ghost in the machine? something weirder?": true,
	"how do you come across? sharp? warm? chaotic? calm?":       true,
	"your name":                                                true,
	"your emoji":                                               true,
	"your theme":                                               true,
}

func ParseIdentityMarkdown(content string) *AgentIdentity {
	identity := &AgentIdentity{}
	
	re := regexp.MustCompile(`(?m)^(\w+)\s*[:：]\s*(.+)$`)
	matches := re.FindAllStringSubmatch(content, -1)
	
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		
		key := strings.ToLower(strings.TrimSpace(match[1]))
		value := strings.TrimSpace(match[2])
		
		if placeholderValues[strings.ToLower(value)] {
			continue
		}
		
		switch key {
		case "name":
			identity.Name = value
		case "emoji":
			identity.Emoji = value
		case "theme":
			identity.Theme = value
		case "creature":
			identity.Creature = value
		case "vibe":
			identity.Vibe = value
		case "avatar":
			identity.Avatar = value
		case "version":
			identity.Version = value
		}
	}
	
	return identity
}

func ParseUserMarkdown(content string) *UserPreference {
	pref := &UserPreference{}
	
	re := regexp.MustCompile(`(?m)^(\w+)\s*[:：]\s*(.+)$`)
	matches := re.FindAllStringSubmatch(content, -1)
	
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		
		key := strings.ToLower(strings.TrimSpace(match[1]))
		value := strings.TrimSpace(match[2])
		
		switch key {
		case "language":
			pref.Language = value
		case "codestyle", "code_style":
			pref.CodeStyle = value
		case "editorprefs", "editor_prefs":
			pref.EditorPrefs = value
		case "frameworks":
			pref.Frameworks = strings.Split(value, ",")
			for i, f := range pref.Frameworks {
				pref.Frameworks[i] = strings.TrimSpace(f)
			}
		case "customprompts", "custom_prompts":
			pref.CustomPrompts = value
		}
	}
	
	return pref
}

func ParseSoulMarkdown(content string) *SoulConfig {
	soul := &SoulConfig{}
	
	re := regexp.MustCompile(`(?m)^(\w+)\s*[:：]\s*(.+)$`)
	matches := re.FindAllStringSubmatch(content, -1)
	
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		
		key := strings.ToLower(strings.TrimSpace(match[1]))
		value := strings.TrimSpace(match[2])
		
		switch key {
		case "personality":
			soul.Personality = value
		case "tone":
			soul.Tone = value
		case "style":
			soul.Style = value
		case "values":
			soul.Values = value
		case "custom":
			soul.Custom = value
		}
	}
	
	return soul
}

func ParseAgentsMarkdown(content string) *AgentBehavior {
	behavior := &AgentBehavior{
		AutoSave: true,
	}
	
	re := regexp.MustCompile(`(?m)^(\w+)\s*[:：]\s*(.+)$`)
	matches := re.FindAllStringSubmatch(content, -1)
	
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		
		key := strings.ToLower(strings.TrimSpace(match[1]))
		value := strings.TrimSpace(match[2])
		
		switch key {
		case "defaultmode", "default_mode":
			behavior.DefaultMode = value
		case "autosave", "auto_save":
			behavior.AutoSave = strings.ToLower(value) == "true"
		case "maxtokens", "max_tokens":
			fmt.Sscanf(value, "%d", &behavior.MaxTokens)
		case "temperature":
			fmt.Sscanf(value, "%f", &behavior.Temperature)
		case "customrules", "custom_rules":
			behavior.CustomRules = strings.Split(value, "\n")
		}
	}
	
	return behavior
}

func FormatIdentityMarkdown(identity *AgentIdentity) string {
	var buf bytes.Buffer
	
	if identity.Name != "" {
		buf.WriteString(fmt.Sprintf("name: %s\n", identity.Name))
	}
	if identity.Emoji != "" {
		buf.WriteString(fmt.Sprintf("emoji: %s\n", identity.Emoji))
	}
	if identity.Theme != "" {
		buf.WriteString(fmt.Sprintf("theme: %s\n", identity.Theme))
	}
	if identity.Creature != "" {
		buf.WriteString(fmt.Sprintf("creature: %s\n", identity.Creature))
	}
	if identity.Vibe != "" {
		buf.WriteString(fmt.Sprintf("vibe: %s\n", identity.Vibe))
	}
	if identity.Avatar != "" {
		buf.WriteString(fmt.Sprintf("avatar: %s\n", identity.Avatar))
	}
	if identity.Version != "" {
		buf.WriteString(fmt.Sprintf("version: %s\n", identity.Version))
	}
	
	return buf.String()
}

func FormatUserMarkdown(pref *UserPreference) string {
	var buf bytes.Buffer
	
	if pref.Language != "" {
		buf.WriteString(fmt.Sprintf("language: %s\n", pref.Language))
	}
	if pref.CodeStyle != "" {
		buf.WriteString(fmt.Sprintf("codeStyle: %s\n", pref.CodeStyle))
	}
	if pref.EditorPrefs != "" {
		buf.WriteString(fmt.Sprintf("editorPrefs: %s\n", pref.EditorPrefs))
	}
	if len(pref.Frameworks) > 0 {
		buf.WriteString(fmt.Sprintf("frameworks: %s\n", strings.Join(pref.Frameworks, ", ")))
	}
	if pref.CustomPrompts != "" {
		buf.WriteString(fmt.Sprintf("customPrompts: %s\n", pref.CustomPrompts))
	}
	
	return buf.String()
}

func FormatSoulMarkdown(soul *SoulConfig) string {
	var buf bytes.Buffer
	
	if soul.Personality != "" {
		buf.WriteString(fmt.Sprintf("personality: %s\n", soul.Personality))
	}
	if soul.Tone != "" {
		buf.WriteString(fmt.Sprintf("tone: %s\n", soul.Tone))
	}
	if soul.Style != "" {
		buf.WriteString(fmt.Sprintf("style: %s\n", soul.Style))
	}
	if soul.Values != "" {
		buf.WriteString(fmt.Sprintf("values: %s\n", soul.Values))
	}
	if soul.Custom != "" {
		buf.WriteString(fmt.Sprintf("custom: %s\n", soul.Custom))
	}
	
	return buf.String()
}
