package config

type ModelDefinition struct {
	ID            string    `json:"id" mapstructure:"id"`
	Name          string    `json:"name" mapstructure:"name"`
	Reasoning     bool      `json:"reasoning" mapstructure:"reasoning"`
	Input         []string  `json:"input" mapstructure:"input"`
	Cost          ModelCost `json:"cost" mapstructure:"cost"`
	ContextWindow int       `json:"contextWindow" mapstructure:"contextWindow"`
	MaxTokens     int       `json:"maxTokens" mapstructure:"maxTokens"`
}

type ModelCost struct {
	Input      float64 `json:"input" mapstructure:"input"`
	Output     float64 `json:"output" mapstructure:"output"`
	CacheRead  float64 `json:"cacheRead" mapstructure:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite" mapstructure:"cacheWrite"`
}

type ModelProviderConfig struct {
	BaseURL string            `json:"baseUrl" mapstructure:"baseUrl"`
	APIKey  string            `json:"apiKey,omitempty" mapstructure:"apiKey"`
	API     string            `json:"api,omitempty" mapstructure:"api"`
	Headers map[string]string `json:"headers,omitempty" mapstructure:"headers"`
	Models  []ModelDefinition `json:"models" mapstructure:"models"`
}

type ModelsConfig struct {
	Mode      string                         `json:"mode" mapstructure:"mode"`
	Providers map[string]ModelProviderConfig `json:"providers" mapstructure:"providers"`
}

type AgentModelConfig struct {
	Primary string `json:"primary" mapstructure:"primary"`
	Alias   string `json:"alias,omitempty" mapstructure:"alias"`
}

type AgentDefaultsConfig struct {
	Model   map[string]AgentModelConfig `json:"model,omitempty" mapstructure:"model"`
	Timeout int                         `json:"timeout,omitempty" mapstructure:"timeout"`
}

type AgentsConfig struct {
	Defaults AgentDefaultsConfig `json:"defaults" mapstructure:"defaults"`
}

type Config struct {
	Models ModelsConfig `json:"models" mapstructure:"models"`
	Agents AgentsConfig `json:"agents" mapstructure:"agents"`
}
