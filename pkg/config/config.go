package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Agents    AgentsConfig    `json:"agents"`
	Channels  ChannelsConfig  `json:"channels"`
	Providers ProvidersConfig `json:"providers"`
	Tools     ToolsConfig     `json:"tools,omitempty"`
	Heartbeat HeartbeatConfig `json:"heartbeat"`
	Devices   DevicesConfig   `json:"devices"`
	Gateway   GatewayConfig   `json:"gateway"`
	Logging   LoggingConfig   `json:"logging,omitempty"`
}

type LoggingConfig struct {
	Format    string `json:"format,omitempty"`
	Level     string `json:"level,omitempty"`
	AddSource bool   `json:"add_source,omitempty"`
}

type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
}

type AgentDefaults struct {
	Type                string  `json:"type"`
	Workspace           string  `json:"workspace"`
	RestrictToWorkspace bool    `json:"restrict_to_workspace"`
	Provider            string  `json:"provider"`
	Model               string  `json:"model"`
	MaxTokens           int     `json:"max_tokens"`
	Temperature         float64 `json:"temperature"`
	MaxToolIterations   int     `json:"max_tool_iterations"`
}

type ProvidersConfig struct {
	OpenCode OpenCodeProviderConfig `json:"opencode"`
	OpenAI   OpenAIProviderConfig   `json:"openai"`
}

type OpenCodeProviderConfig struct {
	BaseURL               string `json:"base_url"`
	Username              string `json:"username"`
	PasswordEnv           string `json:"password_env"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
}

type OpenAIProviderConfig struct {
	BaseURL               string `json:"base_url"`
	Organization          string `json:"organization"`
	Project               string `json:"project"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
}

type ChannelsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
}

type TelegramConfig struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	Proxy     string   `json:"proxy"`
	AllowFrom []string `json:"allow_from"`
}

type ToolsConfig struct {
	Web    WebToolsConfig `json:"web"`
	Cron   CronConfig     `json:"cron"`
	Exec   ExecConfig     `json:"exec"`
	Skills SkillsConfig   `json:"skills"`
}

type WebToolsConfig struct {
	Brave      SearchProviderConfig `json:"brave"`
	DuckDuckGo SearchProviderConfig `json:"duckduckgo"`
	Perplexity SearchProviderConfig `json:"perplexity"`
}

type SearchProviderConfig struct {
	Enabled    bool   `json:"enabled"`
	APIKey     string `json:"api_key"`
	MaxResults int    `json:"max_results"`
}

type CronConfig struct {
	ExecTimeoutMinutes int `json:"exec_timeout_minutes"`
}

type ExecConfig struct {
	EnableDenyPatterns bool     `json:"enable_deny_patterns"`
	CustomDenyPatterns []string `json:"custom_deny_patterns"`
}

type SkillsConfig struct {
	Registries map[string]RegistryConfig `json:"registries"`
}

type RegistryConfig struct {
	Enabled      bool   `json:"enabled"`
	BaseURL      string `json:"base_url"`
	SearchPath   string `json:"search_path"`
	SkillsPath   string `json:"skills_path"`
	DownloadPath string `json:"download_path"`
}

type HeartbeatConfig struct {
	Enabled  bool `json:"enabled"`
	Interval int  `json:"interval"`
}

type DevicesConfig struct {
	Enabled    bool `json:"enabled"`
	MonitorUSB bool `json:"monitor_usb"`
}

type GatewayConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func LoadConfig() (*Config, error) {
	configPath, err := findConfigPath()
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	return &cfg, nil
}

func findConfigPath() (string, error) {
	if value := strings.TrimSpace(os.Getenv("MINICLAW_CONFIG")); value != "" {
		if info, err := os.Stat(value); err == nil && !info.IsDir() {
			return value, nil
		}
		return "", fmt.Errorf("MINICLAW_CONFIG does not point to a file: %s", value)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current working directory: %w", err)
	}

	candidates := []string{
		filepath.Join(cwd, "config.json"),
		filepath.Join(cwd, "config", "config.json"),
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("config.json not found (checked %s and %s)", candidates[0], candidates[1])
}
