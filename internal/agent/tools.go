package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/hoorayman/rizzclaw/internal/llm"
)

type Tool struct {
	Name        string
	Description string
	Handler     ToolHandler
	InputSchema llm.InputSchema
}

type ToolHandler func(ctx context.Context, input map[string]any) (string, error)

type ToolRegistry struct {
	tools map[string]*Tool
	mu    sync.RWMutex
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]*Tool),
	}
}

func (r *ToolRegistry) Register(tool *Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name] = tool
}

func (r *ToolRegistry) Get(name string) *Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

func (r *ToolRegistry) List() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]*Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

func (r *ToolRegistry) ToLLMTools() []llm.Tool {
	tools := r.List()
	result := make([]llm.Tool, len(tools))
	for i, t := range tools {
		result[i] = llm.NewTool(t.Name, t.Description, t.InputSchema)
	}
	return result
}

func (r *ToolRegistry) Execute(ctx context.Context, name string, input map[string]any) (string, error) {
	tool := r.Get(name)
	if tool == nil {
		return "", fmt.Errorf("tool %s not found", name)
	}
	return tool.Handler(ctx, input)
}
