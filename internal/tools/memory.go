package tools

import (
	"context"
	"fmt"

	ctxmgr "github.com/hoorayman/rizzclaw/internal/context"
	"github.com/hoorayman/rizzclaw/internal/llm"
)

func init() {
	RegisterTool(&ToolDefinition{
		Name:        "memory_save",
		Description: "Save important information to long-term memory. Use this when the user shares preferences, important context, or information that should be remembered across sessions.",
		Handler:     MemorySave,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"content": {
					Type:        "string",
					Description: "The content to save to memory",
				},
				"evergreen": {
					Type:        "boolean",
					Description: "If true, this memory will not decay over time (for permanent important info)",
				},
			},
			Required: []string{"content"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "memory_search",
		Description: "Search through saved memories. Use this to recall previously stored information.",
		Handler:     MemorySearch,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"query": {
					Type:        "string",
					Description: "The search query to find relevant memories",
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum number of results to return (default: 5)",
				},
			},
			Required: []string{"query"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "memory_list",
		Description: "List all saved memories.",
		Handler:     MemoryList,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"limit": {
					Type:        "integer",
					Description: "Maximum number of memories to list (default: 20)",
				},
			},
		},
	})
}

func MemorySave(ctx context.Context, input map[string]any) (string, error) {
	content, ok := input["content"].(string)
	if !ok || content == "" {
		return "", fmt.Errorf("content is required")
	}

	evergreen := false
	if e, ok := input["evergreen"].(bool); ok {
		evergreen = e
	}

	mgr := ctxmgr.GetSessionManager()
	if err := mgr.SaveImportantMemory(content, evergreen); err != nil {
		return "", fmt.Errorf("failed to save memory: %w", err)
	}

	marker := ""
	if evergreen {
		marker = " [EVERGREEN]"
	}

	return fmt.Sprintf("Memory saved%s: %s", marker, content), nil
}

func MemorySearch(ctx context.Context, input map[string]any) (string, error) {
	query, ok := input["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("query is required")
	}

	limit := 5
	if l, ok := input["limit"].(float64); ok {
		limit = int(l)
	}

	store := ctxmgr.GetMemoryStore()
	results, err := store.Search(ctx, &ctxmgr.SearchOptions{
		Query:      query,
		MaxResults: limit,
	})
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		return "No memories found matching the query.", nil
	}

	var output string
	output = fmt.Sprintf("Found %d memories:\n\n", len(results))
	for i, r := range results {
		output += fmt.Sprintf("%d. [Score: %.2f] %s\n", i+1, r.Score, r.Entry.Content)
		if len(output) > 2000 {
			output += "\n... (truncated)"
			break
		}
	}

	return output, nil
}

func MemoryList(ctx context.Context, input map[string]any) (string, error) {
	limit := 20
	if l, ok := input["limit"].(float64); ok {
		limit = int(l)
	}

	store := ctxmgr.GetMemoryStore()
	memories, err := store.ListMemories(limit, 0)
	if err != nil {
		return "", fmt.Errorf("failed to list memories: %w", err)
	}

	if len(memories) == 0 {
		return "No memories saved yet.", nil
	}

	var output string
	output = fmt.Sprintf("Total %d memories:\n\n", len(memories))
	for i, m := range memories {
		evergreen := ""
		if m.IsEvergreen {
			evergreen = " [EVERGREEN]"
		}
		output += fmt.Sprintf("%d.%s %s\n", i+1, evergreen, m.Content)
		if len(output) > 2000 {
			output += "\n... (truncated)"
			break
		}
	}

	return output, nil
}
