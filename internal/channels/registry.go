package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hoorayman/rizzclaw/internal/providers"
)

type ChannelConfig struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Enabled     bool              `json:"enabled"`
	Description string            `json:"description"`
	ProviderID  string            `json:"providerId"`
	BaseURL     string            `json:"baseUrl"`
	APIKey      string            `json:"apiKey,omitempty"`
	Timeout     int               `json:"timeout"`
	Headers     map[string]string `json:"headers,omitempty"`
	Models      []string          `json:"models,omitempty"`
	Priority    int               `json:"priority"`
}

type Channel struct {
	config   *ChannelConfig
	provider *providers.Provider
	client   *http.Client
	mu       sync.RWMutex
}

type ChannelRegistry struct {
	channels map[string]*Channel
	filePath string
	mu       sync.RWMutex
}

var globalRegistry *ChannelRegistry
var registryOnce sync.Once

func GetChannelRegistry() *ChannelRegistry {
	registryOnce.Do(func() {
		home, _ := os.UserHomeDir()
		configDir := filepath.Join(home, ".rizzclaw")
		os.MkdirAll(configDir, 0755)
		globalRegistry = &ChannelRegistry{
			channels: make(map[string]*Channel),
			filePath: filepath.Join(configDir, "channels.json"),
		}
		globalRegistry.load()
		globalRegistry.registerDefaults()
	})
	return globalRegistry
}

func (r *ChannelRegistry) load() {
	data, err := os.ReadFile(r.filePath)
	if err != nil {
		return
	}

	var configs []*ChannelConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return
	}

	for _, cfg := range configs {
		r.channels[cfg.ID] = &Channel{config: cfg}
	}
}

func (r *ChannelRegistry) save() error {
	configs := make([]*ChannelConfig, 0, len(r.channels))
	for _, ch := range r.channels {
		configs = append(configs, ch.config)
	}

	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.filePath, data, 0644)
}

func (r *ChannelRegistry) registerDefaults() {
	providerRegistry := providers.GetProviderRegistry()
	
	for _, p := range providerRegistry.List() {
		channelID := "channel-" + p.ID
		if _, exists := r.channels[channelID]; !exists {
			r.channels[channelID] = &Channel{
				config: &ChannelConfig{
					ID:          channelID,
					Name:        p.Name,
					Type:        p.APIFormat,
					Enabled:     p.Enabled,
					Description: p.Description,
					ProviderID:  p.ID,
					BaseURL:     p.BaseURL,
					Timeout:     120,
					Priority:    100,
				},
			}
		}
	}
	r.save()
}

func (r *ChannelRegistry) Register(cfg *ChannelConfig) (*Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cfg.ID == "" {
		return nil, fmt.Errorf("channel ID is required")
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 120
	}

	ch := &Channel{
		config: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
	}

	r.channels[cfg.ID] = ch
	return ch, r.save()
}

func (r *ChannelRegistry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.channels, id)
	return r.save()
}

func (r *ChannelRegistry) Get(id string) *Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.channels[id]
}

func (r *ChannelRegistry) List() []*Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	channels := make([]*Channel, 0, len(r.channels))
	for _, ch := range r.channels {
		channels = append(channels, ch)
	}
	return channels
}

func (r *ChannelRegistry) ListEnabled() []*Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	channels := make([]*Channel, 0)
	for _, ch := range r.channels {
		if ch.config.Enabled {
			channels = append(channels, ch)
		}
	}
	return channels
}

func (r *ChannelRegistry) Enable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch, ok := r.channels[id]
	if !ok {
		return fmt.Errorf("channel not found: %s", id)
	}

	ch.config.Enabled = true
	return r.save()
}

func (r *ChannelRegistry) Disable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch, ok := r.channels[id]
	if !ok {
		return fmt.Errorf("channel not found: %s", id)
	}

	ch.config.Enabled = false
	return r.save()
}

func (r *ChannelRegistry) GetByProvider(providerID string) []*Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	channels := make([]*Channel, 0)
	for _, ch := range r.channels {
		if ch.config.ProviderID == providerID {
			channels = append(channels, ch)
		}
	}
	return channels
}

func (r *ChannelRegistry) GetByModel(modelID string) []*Channel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	channels := make([]*Channel, 0)
	for _, ch := range r.channels {
		for _, m := range ch.config.Models {
			if m == modelID {
				channels = append(channels, ch)
				break
			}
		}
	}
	return channels
}

func NewChannel(config *ChannelConfig) (*Channel, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required for channel %s", config.ID)
	}

	if config.Timeout == 0 {
		config.Timeout = 120
	}

	ch := &Channel{
		config: config,
		client: &http.Client{
			Timeout: time.Duration(config.Timeout) * time.Second,
		},
	}

	return ch, nil
}

func (c *Channel) ID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.ID
}

func (c *Channel) Name() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.Name
}

func (c *Channel) Description() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.Description
}

func (c *Channel) Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.Enabled
}

func (c *Channel) ProviderID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.ProviderID
}

func (c *Channel) Client() *http.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

func (c *Channel) Config() *ChannelConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

func (c *Channel) SetAPIKey(apiKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.APIKey = apiKey
}

func (c *Channel) SetBaseURL(baseURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.BaseURL = baseURL
}

func (c *Channel) SetTimeout(timeout int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.Timeout = timeout
	if c.client != nil {
		c.client.Timeout = time.Duration(timeout) * time.Second
	}
}

func (c *Channel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.client != nil {
		c.client.CloseIdleConnections()
	}
	return nil
}

func (c *Channel) HealthCheck(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("client not initialized")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.BaseURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("health check failed with status: %d", resp.StatusCode)
	}

	return nil
}

func GetDefaultChannel() *Channel {
	registry := GetChannelRegistry()
	
	for _, ch := range registry.ListEnabled() {
		if ch.config.APIKey != "" {
			return ch
		}
	}
	
	return nil
}

func GetChannelForModel(modelID string) *Channel {
	registry := GetChannelRegistry()
	
	channels := registry.GetByModel(modelID)
	for _, ch := range channels {
		if ch.config.Enabled && ch.config.APIKey != "" {
			return ch
		}
	}
	
	return GetDefaultChannel()
}
