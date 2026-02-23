package tools

import (
	"context"
	"fmt"

	"github.com/hoorayman/rizzclaw/internal/llm"
)

func init() {
	RegisterTool(&ToolDefinition{
		Name:        "thinking",
		Description: "Use this tool to think through complex problems step by step. Helps with reasoning, planning, and breaking down tasks. The thoughts are not shown to the user.",
		Handler:     Thinking,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"thought": {
					Type:        "string",
					Description: "The thought or reasoning to record",
				},
				"next_action": {
					Type:        "string",
					Description: "What to do next after this thought (optional)",
				},
			},
			Required: []string{"thought"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "ask",
		Description: "Ask the user a question to clarify or get more information. Use this when you need user input to proceed.",
		Handler:     Ask,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"question": {
					Type:        "string",
					Description: "The question to ask the user",
				},
				"options": {
					Type:        "array",
					Description: "Optional list of choices for the user to select from",
					Items: &llm.ToolParameterProperty{
						Type: "string",
					},
				},
			},
			Required: []string{"question"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "todo_write",
		Description: "Manage a todo list for tracking progress on complex multi-step tasks. Use this to plan and track work.",
		Handler:     TodoWrite,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"todos": {
					Type:        "array",
					Description: "List of todo items",
					Items: &llm.ToolParameterProperty{
						Type: "object",
						Properties: map[string]llm.ToolParameterProperty{
							"id": {
								Type:        "string",
								Description: "Unique identifier for the todo",
							},
							"content": {
								Type:        "string",
								Description: "Description of the task",
							},
							"status": {
								Type:        "string",
								Description: "Status: pending, in_progress, completed",
							},
							"priority": {
								Type:        "string",
								Description: "Priority: high, medium, low",
							},
						},
					},
				},
			},
			Required: []string{"todos"},
		},
	})

	RegisterTool(&ToolDefinition{
		Name:        "exit_plan_mode",
		Description: "Exit plan mode and present the plan to the user for approval. Use after creating a plan.",
		Handler:     ExitPlanMode,
		InputSchema: llm.InputSchema{
			Type: "object",
			Properties: map[string]llm.ToolParameterProperty{
				"title": {
					Type:        "string",
					Description: "Title of the plan",
				},
				"plan": {
					Type:        "string",
					Description: "The plan content in markdown",
				},
			},
			Required: []string{"title", "plan"},
		},
	})
}

func Thinking(ctx context.Context, input map[string]any) (string, error) {
	thought, ok := input["thought"].(string)
	if !ok || thought == "" {
		return "", fmt.Errorf("thought is required")
	}

	nextAction, _ := input["next_action"].(string)

	result := fmt.Sprintf("Thought recorded: %s", thought)
	if nextAction != "" {
		result += fmt.Sprintf("\nNext action: %s", nextAction)
	}

	return result, nil
}

func Ask(ctx context.Context, input map[string]any) (string, error) {
	question, ok := input["question"].(string)
	if !ok || question == "" {
		return "", fmt.Errorf("question is required")
	}

	options, _ := input["options"].([]interface{})

	result := fmt.Sprintf("QUESTION_FOR_USER: %s", question)
	
	if len(options) > 0 {
		result += "\n\nOptions:"
		for i, opt := range options {
			if s, ok := opt.(string); ok {
				result += fmt.Sprintf("\n%d. %s", i+1, s)
			}
		}
	}

	return result, nil
}

func TodoWrite(ctx context.Context, input map[string]any) (string, error) {
	todos, ok := input["todos"].([]interface{})
	if !ok || len(todos) == 0 {
		return "", fmt.Errorf("todos is required and must be a non-empty array")
	}

	var result string
	result = "Todo list updated:\n\n"

	for i, todo := range todos {
		if m, ok := todo.(map[string]interface{}); ok {
			id, _ := m["id"].(string)
			content, _ := m["content"].(string)
			status, _ := m["status"].(string)
			priority, _ := m["priority"].(string)

			if status == "" {
				status = "pending"
			}

			statusIcon := "○"
			switch status {
			case "in_progress":
				statusIcon = "◐"
			case "completed":
				statusIcon = "●"
			}

			priorityStr := ""
			if priority != "" {
				priorityStr = fmt.Sprintf(" [%s]", priority)
			}

			result += fmt.Sprintf("%d. %s %s%s\n", i+1, statusIcon, content, priorityStr)
			_ = id
		}
	}

	return result, nil
}

func ExitPlanMode(ctx context.Context, input map[string]any) (string, error) {
	title, ok := input["title"].(string)
	if !ok || title == "" {
		return "", fmt.Errorf("title is required")
	}

	plan, ok := input["plan"].(string)
	if !ok || plan == "" {
		return "", fmt.Errorf("plan is required")
	}

	return fmt.Sprintf("PLAN_READY: %s\n\n%s\n\nWaiting for user approval...", title, plan), nil
}
