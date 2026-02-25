package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	envTelegramBotToken  = "TELEGRAM_BOT_TOKEN"
	envTelegramAllowFrom = "TELEGRAM_ALLOW_FROM"
)

// Config is the root runtime configuration loaded from config.json.
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

// LoggingConfig controls structured log output format and verbosity.
type LoggingConfig struct {
	Format    string `json:"format,omitempty"`
	Level     string `json:"level,omitempty"`
	AddSource bool   `json:"add_source,omitempty"`
}

// AgentsConfig contains agent runtime defaults.
type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
}

// AgentDefaults describes default model/runtime settings for new agent instances.
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

// ProvidersConfig stores per-provider connection settings.
type ProvidersConfig struct {
	OpenCode OpenCodeProviderConfig `json:"opencode"`
	OpenAI   OpenAIProviderConfig   `json:"openai"`
}

// OpenCodeProviderConfig configures the OpenCode provider client.
type OpenCodeProviderConfig struct {
	BaseURL               string `json:"base_url"`
	Username              string `json:"username"`
	PasswordEnv           string `json:"password_env"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
}

// OpenAIProviderConfig configures the OpenAI provider client.
type OpenAIProviderConfig struct {
	BaseURL               string `json:"base_url"`
	Organization          string `json:"organization"`
	Project               string `json:"project"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
}

// ChannelsConfig stores transport adapter settings.
type ChannelsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
}

// TelegramConfig configures Telegram channel integration.
type TelegramConfig struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	Proxy     string   `json:"proxy"`
	AllowFrom []string `json:"allow_from"`
}

// ToolsConfig groups optional tool-system configuration.
type ToolsConfig struct {
	Web    WebToolsConfig `json:"web"`
	Cron   CronConfig     `json:"cron"`
	Exec   ExecConfig     `json:"exec"`
	Skills SkillsConfig   `json:"skills"`
}

// WebToolsConfig configures web/search providers for tool usage.
type WebToolsConfig struct {
	Brave      SearchProviderConfig `json:"brave"`
	DuckDuckGo SearchProviderConfig `json:"duckduckgo"`
	Perplexity SearchProviderConfig `json:"perplexity"`
}

// SearchProviderConfig configures one external search provider.
type SearchProviderConfig struct {
	Enabled    bool   `json:"enabled"`
	APIKey     string `json:"api_key"`
	MaxResults int    `json:"max_results"`
}

// CronConfig configures cron/tool execution limits.
type CronConfig struct {
	ExecTimeoutMinutes int `json:"exec_timeout_minutes"`
}

// ExecConfig configures local command execution safety behavior.
type ExecConfig struct {
	EnableDenyPatterns bool     `json:"enable_deny_patterns"`
	CustomDenyPatterns []string `json:"custom_deny_patterns"`
}

// SkillsConfig configures external skill registries.
type SkillsConfig struct {
	Registries map[string]RegistryConfig `json:"registries"`
}

// RegistryConfig describes one skill-registry endpoint contract.
type RegistryConfig struct {
	Enabled      bool   `json:"enabled"`
	BaseURL      string `json:"base_url"`
	SearchPath   string `json:"search_path"`
	SkillsPath   string `json:"skills_path"`
	DownloadPath string `json:"download_path"`
}

// HeartbeatConfig controls periodic prompt queue draining.
type HeartbeatConfig struct {
	Enabled  bool `json:"enabled"`
	Interval int  `json:"interval"`
}

// DevicesConfig controls optional device-monitoring features.
type DevicesConfig struct {
	Enabled    bool `json:"enabled"`
	MonitorUSB bool `json:"monitor_usb"`
}

// GatewayConfig configures HTTP gateway bind settings.
type GatewayConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// LoadConfig resolves config.json, unmarshals it, and applies environment overrides.
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

	applyEnvOverrides(&cfg)

	return &cfg, nil
}

// applyEnvOverrides injects selected env-driven settings on top of file config.
func applyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}

	if token := strings.TrimSpace(os.Getenv(envTelegramBotToken)); token != "" {
		cfg.Channels.Telegram.Token = token
	}

	if rawAllowFrom := strings.TrimSpace(os.Getenv(envTelegramAllowFrom)); rawAllowFrom != "" {
		cfg.Channels.Telegram.AllowFrom = parseCSV(rawAllowFrom)
	}
}

// parseCSV splits comma-separated values and returns a trimmed compact slice.
func parseCSV(input string) []string {
	parts := strings.Split(input, ",")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		clean = append(clean, trimmed)
	}

	return slices.Clip(clean)
}

// findConfigPath resolves the active config file location.
//
// Precedence is MINICLAW_CONFIG first, then cwd-local fallback paths.
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
