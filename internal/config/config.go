package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/viper"
)

var (
	configCache *Config
	configOnce  sync.Once
	configMu    sync.RWMutex
)

func GetDefaultConfig() *Config {
	return &Config{
		Models: ModelsConfig{
			Mode: "merge",
			Providers: map[string]ModelProviderConfig{
				"minimax": {
					BaseURL: "https://api.minimaxi.com/anthropic",
					API:     "anthropic-messages",
					Models:  GetDefaultMinimaxModels(),
				},
			},
		},
		Agents: AgentsConfig{
			Defaults: AgentDefaultsConfig{
				Model: map[string]AgentModelConfig{
					"minimax/MiniMax-M2.5": {
						Primary: "minimax/MiniMax-M2.5",
						Alias:   "Minimax",
					},
				},
			},
		},
	}
}

func GetDefaultMinimaxModels() []ModelDefinition {
	cost := ModelCost{
		Input:      0.3,
		Output:     1.2,
		CacheRead:  0.03,
		CacheWrite: 0.12,
	}
	return []ModelDefinition{
		{
			ID:            "MiniMax-M2.1",
			Name:          "MiniMax M2.1",
			Reasoning:     false,
			Input:         []string{"text"},
			Cost:          cost,
			ContextWindow: 200000,
			MaxTokens:     8192,
		},
		{
			ID:            "MiniMax-M2.1-lightning",
			Name:          "MiniMax M2.1 Lightning",
			Reasoning:     false,
			Input:         []string{"text"},
			Cost:          cost,
			ContextWindow: 200000,
			MaxTokens:     8192,
		},
		{
			ID:            "MiniMax-M2.5",
			Name:          "MiniMax M2.5",
			Reasoning:     true,
			Input:         []string{"text"},
			Cost:          cost,
			ContextWindow: 200000,
			MaxTokens:     8192,
		},
		{
			ID:            "MiniMax-M2.5-Lightning",
			Name:          "MiniMax M2.5 Lightning",
			Reasoning:     true,
			Input:         []string{"text"},
			Cost:          cost,
			ContextWindow: 200000,
			MaxTokens:     8192,
		},
		{
			ID:            "MiniMax-VL-01",
			Name:          "MiniMax VL 01",
			Reasoning:     false,
			Input:         []string{"text", "image"},
			Cost:          cost,
			ContextWindow: 200000,
			MaxTokens:     8192,
		},
	}
}

func GetConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".rizzclaw", "config.json")
}

func LoadConfig() (*Config, error) {
	configMu.RLock()
	if configCache != nil {
		defer configMu.RUnlock()
		return configCache, nil
	}
	configMu.RUnlock()

	configMu.Lock()
	defer configMu.Unlock()

	var cfg Config
	configPath := GetConfigPath()

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configCache = GetDefaultConfig()
		return configCache, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if cfg.Models.Mode == "" {
		cfg.Models.Mode = "merge"
	}

	configCache = &cfg
	return configCache, nil
}

func SaveConfig(cfg *Config) error {
	configMu.Lock()
	defer configMu.Unlock()

	configPath := GetConfigPath()
	configDir := filepath.Dir(configPath)

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	configCache = cfg
	return nil
}

func GetAPIKey(provider string) string {
	envKey := fmt.Sprintf("%s_API_KEY", provider)
	if key := os.Getenv(envKey); key != "" {
		return key
	}

	cfg, err := LoadConfig()
	if err != nil {
		return ""
	}

	if p, ok := cfg.Models.Providers[provider]; ok {
		return p.APIKey
	}

	return ""
}

func GetProviderConfig(provider string) (*ModelProviderConfig, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	if p, ok := cfg.Models.Providers[provider]; ok {
		return &p, nil
	}

	return nil, fmt.Errorf("provider %s not found", provider)
}

func InitViper() {
	viper.SetConfigName("config")
	viper.SetConfigType("json")
	viper.AddConfigPath("$HOME/.rizzclaw")
	viper.AddConfigPath(".")

	viper.SetEnvPrefix("RIZZCLAW")
	viper.AutomaticEnv()
}
