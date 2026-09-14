// Package config loads and validates config.yaml (PRD §8).
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the root shape of config.yaml.
type Config struct {
	Server   ServerConfig    `yaml:"server"`
	Telegram TelegramConfig  `yaml:"telegram"`
	GitLab   GitLabConfig    `yaml:"gitlab"`
	Database DatabaseConfig  `yaml:"database"`
	Logging  LoggingConfig   `yaml:"logging"`
	Projects []ProjectConfig `yaml:"projects"`
}

type ServerConfig struct {
	ListenAddr  string `yaml:"listen_addr"`
	WebhookPath string `yaml:"webhook_path"`
}

type TelegramConfig struct {
	BotToken  string `yaml:"bot_token"`
	ChatID    int64  `yaml:"chat_id"`
	UseTopics bool   `yaml:"use_topics"`
}

type GitLabConfig struct {
	BaseURL       string `yaml:"base_url"`
	WebhookSecret string `yaml:"webhook_secret"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// ProjectConfig is one entry in the projects allow-list.
// ThreadID is a pointer so a missing YAML key is distinguishable from an
// explicit 0 — required to enforce PRD §8 rule 1 at startup.
type ProjectConfig struct {
	GitLabProjectID   int    `yaml:"gitlab_project_id"`
	GitLabProjectPath string `yaml:"gitlab_project_path"`
	ThreadID          *int64 `yaml:"telegram_message_thread_id"`
}

// defaults applied when the corresponding key is absent from config.yaml.
const (
	defaultListenAddr  = "0.0.0.0:8080"
	defaultWebhookPath = "/webhook/gitlab"
	defaultDBPath      = "/app/data/bot.db"
	defaultLogLevel    = "info"
	defaultLogFormat   = "text"
)

// Load reads, parses, and validates the config file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", path, err)
	}

	cfg.applyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.ListenAddr == "" {
		c.Server.ListenAddr = defaultListenAddr
	}
	if c.Server.WebhookPath == "" {
		c.Server.WebhookPath = defaultWebhookPath
	}
	if c.Database.Path == "" {
		c.Database.Path = defaultDBPath
	}
	if c.Logging.Level == "" {
		c.Logging.Level = defaultLogLevel
	}
	if c.Logging.Format == "" {
		c.Logging.Format = defaultLogFormat
	}
}

// Validate applies the config rules from PRD §8. In particular, rule 1:
// when use_topics is true, every project must carry a thread ID, or the bot
// must refuse to start.
func (c *Config) Validate() error {
	if c.Telegram.BotToken == "" {
		return fmt.Errorf("config: telegram.bot_token is required")
	}
	if c.Telegram.ChatID == 0 {
		return fmt.Errorf("config: telegram.chat_id is required")
	}
	if c.GitLab.WebhookSecret == "" {
		return fmt.Errorf("config: gitlab.webhook_secret is required")
	}

	seen := make(map[int]string, len(c.Projects))
	for _, p := range c.Projects {
		if dup, ok := seen[p.GitLabProjectID]; ok {
			return fmt.Errorf("config: gitlab_project_id %d is listed more than once (%q and %q)", p.GitLabProjectID, dup, p.GitLabProjectPath)
		}
		seen[p.GitLabProjectID] = p.GitLabProjectPath

		if c.Telegram.UseTopics && p.ThreadID == nil {
			return fmt.Errorf(
				"config: telegram.use_topics is true but project %q (gitlab_project_id=%d) has no telegram_message_thread_id set",
				p.GitLabProjectPath, p.GitLabProjectID,
			)
		}
	}

	return nil
}

// FindProject implements the allow-list check from PRD §13.1: a project ID
// not present here means the event is dropped.
func (c *Config) FindProject(gitlabProjectID int) (ProjectConfig, bool) {
	for _, p := range c.Projects {
		if p.GitLabProjectID == gitlabProjectID {
			return p, true
		}
	}
	return ProjectConfig{}, false
}
