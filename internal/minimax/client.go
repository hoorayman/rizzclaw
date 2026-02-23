package minimax

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hoorayman/rizzclaw/internal/config"
	"github.com/hoorayman/rizzclaw/internal/llm"
	"github.com/hoorayman/rizzclaw/internal/tools"
)

const (
	ProviderName         = "minimax"
	DefaultModel         = "MiniMax-M2.5"
	DefaultContextWindow = 200000
	DefaultMaxTokens     = 8192
)

type Client struct {
	*llm.Client
	model         string
	contextWindow int
	maxTokens     int
}

type ClientOption func(*Client)

func WithModel(model string) ClientOption {
	return func(c *Client) {
		c.model = model
	}
}

func WithContextWindow(window int) ClientOption {
	return func(c *Client) {
		c.contextWindow = window
	}
}

func WithMaxTokens(tokens int) ClientOption {
	return func(c *Client) {
		c.maxTokens = tokens
	}
}

func WithToolExecutor(executor func(ctx context.Context, name string, input map[string]any) (string, error)) ClientOption {
	return func(c *Client) {
		c.Client.ToolExecutor = executor
	}
}

func WithDebug(debug bool) ClientOption {
	return func(c *Client) {
		c.Client.Debug = debug
	}
}

func NewClient(opts ...ClientOption) (*Client, error) {
	apiKey := os.Getenv("MINIMAX_API_KEY")
	if apiKey == "" {
		cfg, err := config.LoadConfig()
		if err == nil {
			if p, ok := cfg.Models.Providers[ProviderName]; ok {
				apiKey = p.APIKey
			}
		}
	}

	if apiKey == "" {
		return nil, fmt.Errorf("MINIMAX_API_KEY not set")
	}

	baseURL := os.Getenv("MINIMAX_BASE_URL")
	if baseURL == "" {
		cfg, _ := config.LoadConfig()
		if p, ok := cfg.Models.Providers[ProviderName]; ok && p.BaseURL != "" {
			baseURL = p.BaseURL
		}
	}
	if baseURL == "" {
		baseURL = "https://api.minimaxi.com/v1"
	}

	c := &Client{
		Client: llm.NewClient(
			llm.WithBaseURL(baseURL),
			llm.WithAPIKey(apiKey),
			llm.WithTimeout(180*time.Second),
			llm.WithToolExecutor(tools.ExecuteTool),
		),
		model:         DefaultModel,
		contextWindow: DefaultContextWindow,
		maxTokens:     DefaultMaxTokens,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

func (c *Client) Chat(ctx context.Context, messages []llm.Message, systemPrompt string) (*llm.ChatResponse, error) {
	req := &llm.ChatRequest{
		Model:     c.model,
		Messages:  messages,
		MaxTokens: c.maxTokens,
		System:    systemPrompt,
	}

	return c.Client.Chat(ctx, req)
}

func (c *Client) ChatStream(ctx context.Context, messages []llm.Message, systemPrompt string, handler llm.StreamEventHandler) error {
	req := &llm.ChatRequest{
		Model:     c.model,
		Messages:  messages,
		MaxTokens: c.maxTokens,
		System:    systemPrompt,
	}

	return c.Client.ChatStream(ctx, req, handler)
}

func (c *Client) ChatWithTools(ctx context.Context, messages []llm.Message, systemPrompt string, toolsList []llm.Tool, maxIterations int, handler llm.StreamEventHandler) (*llm.ChatResponse, error) {
	req := &llm.ChatRequest{
		Model:     c.model,
		Messages:  messages,
		MaxTokens: c.maxTokens,
		System:    systemPrompt,
		Tools:     toolsList,
	}

	return c.Client.ChatWithTools(ctx, req, maxIterations, handler)
}

func (c *Client) GetModel() string {
	return c.model
}

func (c *Client) GetContextWindow() int {
	return c.contextWindow
}

func (c *Client) GetMaxTokens() int {
	return c.maxTokens
}

func GetModelDefinition(modelID string) *config.ModelDefinition {
	models := config.GetDefaultMinimaxModels()
	for _, m := range models {
		if m.ID == modelID {
			return &m
		}
	}
	return nil
}

func ListModels() []config.ModelDefinition {
	return config.GetDefaultMinimaxModels()
}
