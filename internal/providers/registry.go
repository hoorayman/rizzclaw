package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hoorayman/rizzclaw/internal/config"
	"github.com/hoorayman/rizzclaw/internal/llm"
)

type Provider struct {
	ID          string
	Name        string
	BaseURL     string
	APIKey      string
	APIFormat   string
	Headers     map[string]string
	Models      []config.ModelDefinition
	Enabled     bool
	Description string
}

type ProviderRegistry struct {
	providers map[string]*Provider
	filePath  string
	mu        sync.RWMutex
}

var globalRegistry *ProviderRegistry
var registryOnce sync.Once

func GetProviderRegistry() *ProviderRegistry {
	registryOnce.Do(func() {
		home, _ := os.UserHomeDir()
		configDir := filepath.Join(home, ".rizzclaw")
		os.MkdirAll(configDir, 0755)
		globalRegistry = &ProviderRegistry{
			providers: make(map[string]*Provider),
			filePath:  filepath.Join(configDir, "providers.json"),
		}
		globalRegistry.load()
		globalRegistry.registerDefaults()
	})
	return globalRegistry
}

func (r *ProviderRegistry) load() {
	data, err := os.ReadFile(r.filePath)
	if err != nil {
		return
	}

	var providers []*Provider
	if err := json.Unmarshal(data, &providers); err != nil {
		return
	}

	for _, p := range providers {
		r.providers[p.ID] = p
	}
}

func (r *ProviderRegistry) save() error {
	providers := make([]*Provider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}

	data, err := json.MarshalIndent(providers, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.filePath, data, 0644)
}

func (r *ProviderRegistry) registerDefaults() {
	defaults := GetDefaultProviders()
	for _, p := range defaults {
		if _, exists := r.providers[p.ID]; !exists {
			r.providers[p.ID] = p
		}
	}
	r.save()
}

func (r *ProviderRegistry) Register(p *Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p.ID == "" {
		return fmt.Errorf("provider ID is required")
	}

	r.providers[p.ID] = p
	return r.save()
}

func (r *ProviderRegistry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.providers, id)
	return r.save()
}

func (r *ProviderRegistry) Get(id string) *Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[id]
}

func (r *ProviderRegistry) List() []*Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]*Provider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	return providers
}

func (r *ProviderRegistry) ListEnabled() []*Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]*Provider, 0)
	for _, p := range r.providers {
		if p.Enabled {
			providers = append(providers, p)
		}
	}
	return providers
}

func (r *ProviderRegistry) Enable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.providers[id]
	if !ok {
		return fmt.Errorf("provider not found: %s", id)
	}

	p.Enabled = true
	return r.save()
}

func (r *ProviderRegistry) Disable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.providers[id]
	if !ok {
		return fmt.Errorf("provider not found: %s", id)
	}

	p.Enabled = false
	return r.save()
}

func (r *ProviderRegistry) SetAPIKey(id, apiKey string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.providers[id]
	if !ok {
		return fmt.Errorf("provider not found: %s", id)
	}

	p.APIKey = apiKey
	return r.save()
}

func (p *Provider) CreateClient(opts ...llm.ClientOption) *llm.Client {
	defaultOpts := []llm.ClientOption{
		llm.WithBaseURL(p.BaseURL),
		llm.WithAPIKey(p.APIKey),
		llm.WithHeaders(p.Headers),
	}

	allOpts := append(defaultOpts, opts...)
	return llm.NewClient(allOpts...)
}

func (p *Provider) GetModel(id string) *config.ModelDefinition {
	for i := range p.Models {
		if p.Models[i].ID == id {
			return &p.Models[i]
		}
	}
	return nil
}

func (p *Provider) ListModels() []config.ModelDefinition {
	return p.Models
}

func GetDefaultProviders() []*Provider {
	return []*Provider{
		{
			ID:          "minimax",
			Name:        "MiniMax",
			BaseURL:     "https://api.minimaxi.com/anthropic",
			APIFormat:   "anthropic",
			Description: "MiniMax AI - 国内可用，支持M2.5等模型",
			Enabled:     true,
			Models: []config.ModelDefinition{
				{ID: "MiniMax-M2.5", Name: "MiniMax M2.5", Reasoning: true, Input: []string{"text"}},
				{ID: "MiniMax-M2.1", Name: "MiniMax M2.1", Reasoning: false, Input: []string{"text"}},
				{ID: "MiniMax-M2.1-lightning", Name: "MiniMax M2.1 Lightning", Reasoning: true, Input: []string{"text"}},
				{ID: "MiniMax-VL-01", Name: "MiniMax VL 01", Reasoning: true, Input: []string{"text", "image"}},
			},
		},
		{
			ID:          "zhipu",
			Name:        "智谱AI (GLM)",
			BaseURL:     "https://open.bigmodel.cn/api/paas/v4",
			APIFormat:   "openai",
			Description: "智谱AI GLM系列模型 - 国内可用",
			Enabled:     false,
			Models: []config.ModelDefinition{
				{ID: "glm-4-plus", Name: "GLM-4 Plus", Reasoning: false, Input: []string{"text"}},
				{ID: "glm-4-air", Name: "GLM-4 Air", Reasoning: false, Input: []string{"text"}},
				{ID: "glm-4-flash", Name: "GLM-4 Flash", Reasoning: false, Input: []string{"text"}},
				{ID: "glm-4v-plus", Name: "GLM-4V Plus", Reasoning: false, Input: []string{"text", "image"}},
			},
		},
		{
			ID:          "qwen",
			Name:        "通义千问 (Qwen)",
			BaseURL:     "https://dashscope.aliyuncs.com/compatible-mode/v1",
			APIFormat:   "openai",
			Description: "阿里云通义千问 - 国内可用",
			Enabled:     false,
			Models: []config.ModelDefinition{
				{ID: "qwen-max", Name: "Qwen Max", Reasoning: false, Input: []string{"text"}},
				{ID: "qwen-plus", Name: "Qwen Plus", Reasoning: false, Input: []string{"text"}},
				{ID: "qwen-turbo", Name: "Qwen Turbo", Reasoning: false, Input: []string{"text"}},
				{ID: "qwen-vl-max", Name: "Qwen VL Max", Reasoning: false, Input: []string{"text", "image"}},
			},
		},
		{
			ID:          "moonshot",
			Name:        "Moonshot (Kimi)",
			BaseURL:     "https://api.moonshot.cn/v1",
			APIFormat:   "openai",
			Description: "Moonshot Kimi模型 - 国内可用",
			Enabled:     false,
			Models: []config.ModelDefinition{
				{ID: "moonshot-v1-8k", Name: "Moonshot V1 8K", Reasoning: false, Input: []string{"text"}},
				{ID: "moonshot-v1-32k", Name: "Moonshot V1 32K", Reasoning: false, Input: []string{"text"}},
				{ID: "moonshot-v1-128k", Name: "Moonshot V1 128K", Reasoning: false, Input: []string{"text"}},
			},
		},
		{
			ID:          "deepseek",
			Name:        "DeepSeek",
			BaseURL:     "https://api.deepseek.com",
			APIFormat:   "openai",
			Description: "DeepSeek模型 - 国内可用，性价比高",
			Enabled:     false,
			Models: []config.ModelDefinition{
				{ID: "deepseek-chat", Name: "DeepSeek Chat", Reasoning: false, Input: []string{"text"}},
				{ID: "deepseek-reasoner", Name: "DeepSeek Reasoner", Reasoning: true, Input: []string{"text"}},
			},
		},
		{
			ID:          "baidu",
			Name:        "百度千帆",
			BaseURL:     "https://aip.baidubce.com/rpc/2.0/ai_custom/v1/wenxinworkshop",
			APIFormat:   "baidu",
			Description: "百度千帆大模型 - 国内可用",
			Enabled:     false,
			Models: []config.ModelDefinition{
				{ID: "ernie-4.0-8k", Name: "ERNIE 4.0 8K", Reasoning: false, Input: []string{"text"}},
				{ID: "ernie-3.5-8k", Name: "ERNIE 3.5 8K", Reasoning: false, Input: []string{"text"}},
				{ID: "ernie-speed-8k", Name: "ERNIE Speed 8K", Reasoning: false, Input: []string{"text"}},
			},
		},
		{
			ID:          "anthropic",
			Name:        "Anthropic (Claude)",
			BaseURL:     "https://api.anthropic.com",
			APIFormat:   "anthropic",
			Description: "Anthropic Claude - 需要代理",
			Enabled:     false,
			Models: []config.ModelDefinition{
				{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Reasoning: false, Input: []string{"text", "image"}},
				{ID: "claude-opus-4-20250514", Name: "Claude Opus 4", Reasoning: false, Input: []string{"text", "image"}},
				{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet", Reasoning: false, Input: []string{"text", "image"}},
			},
		},
	}
}

type ProviderClient interface {
	Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
	ChatStream(ctx context.Context, req *llm.ChatRequest, handler llm.StreamEventHandler) error
	ChatWithTools(ctx context.Context, req *llm.ChatRequest, maxIterations int, handler llm.StreamEventHandler) (*llm.ChatResponse, error)
}

func CreateProviderClient(providerID string, opts ...llm.ClientOption) (ProviderClient, error) {
	registry := GetProviderRegistry()
	provider := registry.Get(providerID)
	if provider == nil {
		return nil, fmt.Errorf("provider not found: %s", providerID)
	}

	if !provider.Enabled {
		return nil, fmt.Errorf("provider %s is not enabled", providerID)
	}

	if provider.APIKey == "" {
		return nil, fmt.Errorf("API key not configured for provider %s", providerID)
	}

	return provider.CreateClient(opts...), nil
}

func GetDefaultClient(opts ...llm.ClientOption) (ProviderClient, error) {
	registry := GetProviderRegistry()
	
	for _, p := range registry.ListEnabled() {
		if p.APIKey != "" {
			return p.CreateClient(opts...), nil
		}
	}
	
	return nil, fmt.Errorf("no configured provider found")
}

type HTTPClient struct {
	client  *http.Client
	baseURL string
	apiKey  string
	headers map[string]string
}

func NewHTTPClient(baseURL, apiKey string, timeout time.Duration, headers map[string]string) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: timeout,
		},
		baseURL: baseURL,
		apiKey:  apiKey,
		headers: headers,
	}
}

func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	return c.client.Do(req)
}
