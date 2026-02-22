package tools

import (
	"context"

	"github.com/hoorayman/rizzclaw/internal/llm"
)

type ToolHandler func(ctx context.Context, input map[string]any) (string, error)

type ToolDefinition struct {
	Name        string
	Description string
	Handler     ToolHandler
	InputSchema llm.InputSchema
}

var registeredTools = make(map[string]*ToolDefinition)

func RegisterTool(tool *ToolDefinition) {
	registeredTools[tool.Name] = tool
}

func GetTool(name string) *ToolDefinition {
	return registeredTools[name]
}

func ListTools() []*ToolDefinition {
	tools := make([]*ToolDefinition, 0, len(registeredTools))
	for _, tool := range registeredTools {
		tools = append(tools, tool)
	}
	return tools
}

func ToLLMTools() []llm.Tool {
	tools := ListTools()
	result := make([]llm.Tool, len(tools))
	for i, t := range tools {
		schema := t.InputSchema
		if schema.Properties == nil {
			schema.Properties = make(map[string]llm.ToolParameterProperty)
		}
		result[i] = llm.NewTool(t.Name, t.Description, schema)
	}
	return result
}

func ExecuteTool(ctx context.Context, name string, input map[string]any) (string, error) {
	tool := GetTool(name)
	if tool == nil {
		return "", nil
	}
	return tool.Handler(ctx, input)
}

func init() {
	RegisterTool(&ToolDefinition{
		Name:        "file_read",
		Description: "Read the contents of a file. Returns file content with metadata.",
		Handler:     FileRead,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"path": {
					Type:        "string",
					Description: "The path to the file to read",
				},
				"offset": {
					Type:        "integer",
					Description: "Line number to start reading from (0-indexed)",
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum number of lines to read",
				},
			},
			Required: []string{"path"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "file_write",
		Description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does.",
		Handler:     FileWrite,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"path": {
					Type:        "string",
					Description: "The path to the file to write",
				},
				"content": {
					Type:        "string",
					Description: "The content to write to the file",
				},
			},
			Required: []string{"path", "content"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "file_list",
		Description: "List files and directories in a given path.",
		Handler:     FileList,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"path": {
					Type:        "string",
					Description: "The directory path to list",
				},
				"pattern": {
					Type:        "string",
					Description: "Glob pattern to filter files (e.g., '*.go')",
				},
				"recursive": {
					Type:        "boolean",
					Description: "Whether to list files recursively",
				},
			},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "file_search",
		Description: "Search for files or content within files.",
		Handler:     FileSearch,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"path": {
					Type:        "string",
					Description: "The directory path to search in",
				},
				"pattern": {
					Type:        "string",
					Description: "The search pattern (filename pattern or text to search)",
				},
				"type": {
					Type:        "string",
					Description: "Search type: 'filename' or 'content' (default: content)",
					Enum:        []string{"filename", "content"},
				},
			},
			Required: []string{"pattern"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "file_delete",
		Description: "Delete a file or directory.",
		Handler:     FileDelete,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"path": {
					Type:        "string",
					Description: "The path to delete",
				},
				"recursive": {
					Type:        "boolean",
					Description: "Whether to delete directories recursively",
				},
			},
			Required: []string{"path"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "exec",
		Description: "Execute a shell command and return the output.",
		Handler:     Exec,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"command": {
					Type:        "string",
					Description: "The command to execute",
				},
				"workdir": {
					Type:        "string",
					Description: "Working directory for the command",
				},
				"timeout": {
					Type:        "integer",
					Description: "Timeout in seconds (default: 300)",
				},
			},
			Required: []string{"command"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "exec_background",
		Description: "Execute a shell command in the background and return a session ID.",
		Handler:     ExecBackground,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"command": {
					Type:        "string",
					Description: "The command to execute in background",
				},
				"workdir": {
					Type:        "string",
					Description: "Working directory for the command",
				},
			},
			Required: []string{"command"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "process_list",
		Description: "List running background processes.",
		Handler:     ProcessList,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"running": {
					Type:        "boolean",
					Description: "Only show running processes",
				},
			},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "process_kill",
		Description: "Kill a background process by ID.",
		Handler:     ProcessKill,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"id": {
					Type:        "string",
					Description: "The process session ID to kill",
				},
			},
			Required: []string{"id"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "edit",
		Description: "Edit a file by replacing text. Finds and replaces occurrences of oldString with newString.",
		Handler:     Edit,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"path": {
					Type:        "string",
					Description: "The path to the file to edit",
				},
				"oldString": {
					Type:        "string",
					Description: "The text to find and replace",
				},
				"newString": {
					Type:        "string",
					Description: "The text to replace with",
				},
				"replaceAll": {
					Type:        "boolean",
					Description: "Replace all occurrences (default: false)",
				},
			},
			Required: []string{"path", "oldString", "newString"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "edit_regex",
		Description: "Edit a file using regex pattern matching.",
		Handler:     EditRegex,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"path": {
					Type:        "string",
					Description: "The path to the file to edit",
				},
				"pattern": {
					Type:        "string",
					Description: "The regex pattern to match",
				},
				"replace": {
					Type:        "string",
					Description: "The replacement string",
				},
				"global": {
					Type:        "boolean",
					Description: "Replace all matches (default: false)",
				},
			},
			Required: []string{"path", "pattern", "replace"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "apply_patch",
		Description: "Apply a unified diff patch to a file.",
		Handler:     ApplyPatch,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"path": {
					Type:        "string",
					Description: "The path to the file to patch",
				},
				"patch": {
					Type:        "string",
					Description: "The unified diff patch content",
				},
			},
			Required: []string{"path", "patch"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "create",
		Description: "Create a new file with the specified content.",
		Handler:     Create,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"path": {
					Type:        "string",
					Description: "The path for the new file",
				},
				"content": {
					Type:        "string",
					Description: "The initial content for the file",
				},
			},
			Required: []string{"path", "content"},
		},
	})
}
