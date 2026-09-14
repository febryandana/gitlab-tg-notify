package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoad_UseTopicsRequiresThreadID(t *testing.T) {
	path := writeConfig(t, `
telegram:
  bot_token: "token"
  chat_id: -100
  use_topics: true
gitlab:
  webhook_secret: "secret"
projects:
  - gitlab_project_id: 42
    gitlab_project_path: "backend/api"
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected Load to fail when use_topics is true and a project has no thread ID")
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	path := writeConfig(t, `
telegram:
  bot_token: "token"
  chat_id: -100
  use_topics: true
gitlab:
  webhook_secret: "secret"
projects:
  - gitlab_project_id: 42
    gitlab_project_path: "backend/api"
    telegram_message_thread_id: 15
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.ListenAddr != defaultListenAddr {
		t.Errorf("ListenAddr = %q, want default %q", cfg.Server.ListenAddr, defaultListenAddr)
	}

	if _, ok := cfg.FindProject(42); !ok {
		t.Error("FindProject(42) not found, want found")
	}
	if _, ok := cfg.FindProject(999); ok {
		t.Error("FindProject(999) found, want not found (not on allow-list)")
	}
}

func TestLoad_UseTopicsFalseIgnoresMissingThreadID(t *testing.T) {
	path := writeConfig(t, `
telegram:
  bot_token: "token"
  chat_id: -100
  use_topics: false
gitlab:
  webhook_secret: "secret"
projects:
  - gitlab_project_id: 42
    gitlab_project_path: "backend/api"
`)

	if _, err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
}
