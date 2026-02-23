package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	ctxmgr "github.com/hoorayman/rizzclaw/internal/context"
	"github.com/hoorayman/rizzclaw/internal/llm"
	"github.com/hoorayman/rizzclaw/internal/minimax"
	"github.com/hoorayman/rizzclaw/internal/tools"
)

const defaultBasePrompt = `You are RizzClaw, an AI coding assistant powered by MiniMax.

You help users with software engineering tasks including:
- Reading, writing, and editing files
- Executing shell commands
- Code analysis and debugging
- Answering questions about codebases

Be helpful, accurate, and follow the user's coding style preferences.`

type Agent struct {
	ID           string
	Name         string
	SystemPrompt string
	Model        string
	Client       *minimax.Client
	Session      *Session
	UseTools     bool
	mu           sync.RWMutex
}

type AgentOption func(*Agent) error

func WithName(name string) AgentOption {
	return func(a *Agent) error {
		a.Name = name
		return nil
	}
}

func WithSystemPrompt(prompt string) AgentOption {
	return func(a *Agent) error {
		a.SystemPrompt = prompt
		return nil
	}
}

func WithModel(model string) AgentOption {
	return func(a *Agent) error {
		a.Model = model
		return nil
	}
}

func WithSession(session *Session) AgentOption {
	return func(a *Agent) error {
		a.Session = session
		return nil
	}
}

func WithTools(useTools bool) AgentOption {
	return func(a *Agent) error {
		a.UseTools = useTools
		return nil
	}
}

func NewAgent(id string, opts ...AgentOption) (*Agent, error) {
	client, err := minimax.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create minimax client: %w", err)
	}

	agent := &Agent{
		ID:       id,
		Name:     id,
		Model:    minimax.DefaultModel,
		Client:   client,
		Session:  NewSession(id),
		UseTools: true,
	}

	for _, opt := range opts {
		if err := opt(agent); err != nil {
			return nil, err
		}
	}

	if agent.SystemPrompt == "" {
		mgr := ctxmgr.GetManager()
		agent.SystemPrompt = mgr.BuildSystemPrompt(defaultBasePrompt)
	}

	return agent, nil
}

func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	a.mu.Lock()
	a.Session.Messages = append(a.Session.Messages, Message{
		Role:      string(llm.RoleUser),
		Content:   input,
		Timestamp: timeNow(),
	})
	a.mu.Unlock()

	messages := a.convertMessages()

	var response string
	handler := func(event *llm.StreamEvent) error {
		if event.Delta != nil && event.Delta.Text != "" {
			response += event.Delta.Text
			fmt.Print(event.Delta.Text)
		}
		return nil
	}

	if a.UseTools {
		llmTools := tools.ToLLMTools()
		resp, err := a.Client.ChatWithTools(ctx, messages, a.SystemPrompt, llmTools, 10, handler)
		if err != nil {
			return "", fmt.Errorf("chat with tools failed: %w", err)
		}
		response = extractTextFromResponse(resp)
	} else {
		err := a.Client.ChatStream(ctx, messages, a.SystemPrompt, handler)
		if err != nil {
			return "", fmt.Errorf("chat stream failed: %w", err)
		}
	}

	fmt.Println()

	a.mu.Lock()
	a.Session.Messages = append(a.Session.Messages, Message{
		Role:      string(llm.RoleAssistant),
		Content:   response,
		Timestamp: timeNow(),
	})
	a.Session.UpdatedAt = timeNow()
	a.mu.Unlock()

	go func() {
		SaveSessionToContext(a.Session)
	}()

	return response, nil
}

func (a *Agent) RunWithTools(ctx context.Context, input string, maxIterations int) (string, error) {
	a.mu.Lock()
	a.Session.Messages = append(a.Session.Messages, Message{
		Role:      string(llm.RoleUser),
		Content:   input,
		Timestamp: timeNow(),
	})
	a.mu.Unlock()

	messages := a.convertMessages()

	var response string
	handler := func(event *llm.StreamEvent) error {
		if event.Delta != nil && event.Delta.Text != "" {
			response += event.Delta.Text
			fmt.Print(event.Delta.Text)
		}
		return nil
	}

	llmTools := tools.ToLLMTools()
	resp, err := a.Client.ChatWithTools(ctx, messages, a.SystemPrompt, llmTools, maxIterations, handler)
	if err != nil {
		return "", fmt.Errorf("chat with tools failed: %w", err)
	}

	response = extractTextFromResponse(resp)
	fmt.Println()

	a.mu.Lock()
	a.Session.Messages = append(a.Session.Messages, Message{
		Role:      string(llm.RoleAssistant),
		Content:   response,
		Timestamp: timeNow(),
	})
	a.mu.Unlock()

	return response, nil
}

func (a *Agent) convertMessages() []llm.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()

	messages := make([]llm.Message, len(a.Session.Messages))
	for i, msg := range a.Session.Messages {
		messages[i] = llm.Message{
			Role: llm.MessageRole(msg.Role),
			Content: []llm.ContentBlock{
				{
					Type: "text",
					Text: msg.Content,
				},
			},
		}
	}
	return messages
}

func (a *Agent) GetSession() *Session {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Session
}

func (a *Agent) ClearSession() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Session = NewSession(a.ID)
}

func extractTextFromResponse(resp *llm.ChatResponse) string {
	var texts []string
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			texts = append(texts, block.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func timeNow() time.Time {
	return time.Now()
}
