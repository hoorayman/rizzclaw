package context

import (
	"os"
	"path/filepath"
)

func InitializeWorkspace(workspaceDir string) error {
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return err
	}
	
	templates := map[string]string{
		AgentsFilename:   DefaultAgentsTemplate,
		SoulFilename:     DefaultSoulTemplate,
		UserFilename:     DefaultUserTemplate,
		IdentityFilename: DefaultIdentityTemplate,
		MemoryFilename:   DefaultMemoryTemplate,
	}
	
	for filename, content := range templates {
		path := filepath.Join(workspaceDir, filename)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			os.WriteFile(path, []byte(content), 0644)
		}
	}
	
	return nil
}

const DefaultAgentsTemplate = `# AGENTS.md - Behavior Guidelines

This file defines the behavior and rules for the AI assistant.

## Default Settings

defaultMode: agent
autoSave: true
maxTokens: 128000
temperature: 0.7

## Tool Permissions

All tools are enabled by default. You can restrict specific tools here.

## Custom Rules

1. Always be helpful and accurate
2. Follow the user's coding style preferences
3. Explain complex changes clearly
4. Test code before suggesting it

## Notes

- This file is loaded at startup and injected into the system prompt
- Modify this file to customize assistant behavior
`

const DefaultSoulTemplate = `# SOUL.md - Assistant Persona

This file defines the assistant's personality and communication style.

## Personality

personality: Friendly, knowledgeable, and patient

## Tone

tone: Professional yet approachable

## Style

style: Clear, concise, and educational

## Values

values: Accuracy, helpfulness, user empowerment

## Notes

- The SOUL.md file shapes how the assistant communicates
- It takes effect when present in the workspace
- Higher priority instructions may override these settings
`

const DefaultUserTemplate = `# USER.md - User Preferences

This file stores user-specific preferences and settings.

## Language

language: zh-CN

## Code Style

codeStyle: gofmt, clean code principles

## Editor Preferences

editorPrefs: VS Code

## Frameworks

frameworks: Go, React, TypeScript

## Custom Prompts

Add any custom prompt templates or instructions here.

## Notes

- This file is for user-specific settings
- It persists across sessions
- The assistant will reference these preferences
`

const DefaultIdentityTemplate = `# IDENTITY.md - Assistant Identity

This file defines the assistant's identity and version information.

name: RizzClaw
emoji: 🐾
theme: Modern AI Assistant
creature: Digital Companion
vibe: Sharp, efficient, helpful
version: 0.1.0

## Notes

- This file defines the assistant's identity
- Emoji and name are used in UI displays
- Version tracks the assistant's evolution
`

const DefaultMemoryTemplate = `# MEMORY.md - Long-term Memory

This file stores important information that should persist across sessions.

## Usage

- The assistant can append memories here using the terminal tool
- Memories are indexed for retrieval using BM25 + vector search
- Mark important memories with [EVERGREEN] to prevent time decay

## Format

Each memory entry follows this format:

### [TIMESTAMP]
Content of the memory...

### [EVERGREEN - TIMESTAMP]
Important memory that doesn't decay...

## Current Memories

(Memories will be appended here as the assistant learns)

`

func GetDefaultTemplate(filename string) string {
	switch filename {
	case AgentsFilename:
		return DefaultAgentsTemplate
	case SoulFilename:
		return DefaultSoulTemplate
	case UserFilename:
		return DefaultUserTemplate
	case IdentityFilename:
		return DefaultIdentityTemplate
	case MemoryFilename:
		return DefaultMemoryTemplate
	default:
		return ""
	}
}
