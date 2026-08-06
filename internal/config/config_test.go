package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validConfig = `mattermost:
  url: "https://chat.example.com/"
  bot_token: "file-token"
groups:
  news:
    team_name: " company "
    channel_name: " daily-news "
log:
  directory: "./runtime-logs"
`

func TestLoadValidConfigAndEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, validConfig)

	cfg, err := Load(path, func(name string) string {
		if name == "MATTERMOST_BOT_TOKEN" {
			return "environment-token"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Mattermost.URL != "https://chat.example.com" {
		t.Fatalf("URL = %q", cfg.Mattermost.URL)
	}
	if cfg.Mattermost.BotToken != "environment-token" {
		t.Fatalf("BotToken = %q", cfg.Mattermost.BotToken)
	}
	if cfg.Groups["news"].TeamName != "company" || cfg.Groups["news"].ChannelName != "daily-news" {
		t.Fatalf("group not trimmed: %#v", cfg.Groups["news"])
	}
	wantLog := filepath.Join(dir, "runtime-logs")
	if cfg.Log.Directory != wantLog {
		t.Fatalf("Log.Directory = %q, want %q", cfg.Log.Directory, wantLog)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, strings.Replace(validConfig, "  url:", "  unknown: true\n  url:", 1))

	_, err := Load(path, func(string) string { return "" })
	assertConfigErrorCode(t, err, "config_parse_error")
}

func TestLoadRejectsPlaceholderToken(t *testing.T) {
	dir := t.TempDir()
	content := strings.Replace(validConfig, "file-token", "replace-with-real-token", 1)
	path := writeConfig(t, dir, content)

	_, err := Load(path, func(string) string { return "" })
	assertConfigErrorCode(t, err, "config_validation_error")
}

func TestLoadRejectsInvalidURL(t *testing.T) {
	dir := t.TempDir()
	content := strings.Replace(validConfig, "https://chat.example.com/", "ftp://chat.example.com", 1)
	path := writeConfig(t, dir, content)

	_, err := Load(path, func(string) string { return "" })
	assertConfigErrorCode(t, err, "config_validation_error")
}

func TestLoadDefaultsLogDirectory(t *testing.T) {
	dir := t.TempDir()
	content := strings.Replace(validConfig, "  directory: \"./runtime-logs\"", "  directory: \"\"", 1)
	path := writeConfig(t, dir, content)

	cfg, err := Load(path, func(string) string { return "" })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Log.Directory != filepath.Join(dir, "logs") {
		t.Fatalf("Log.Directory = %q", cfg.Log.Directory)
	}
}

func TestLoadRejectsGroupNameSurroundedByWhitespace(t *testing.T) {
	dir := t.TempDir()
	content := strings.Replace(validConfig, "  news:", "  ' news ':", 1)
	path := writeConfig(t, dir, content)

	_, err := Load(path, func(string) string { return "" })
	assertConfigErrorCode(t, err, "config_validation_error")
}

func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertConfigErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	configErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T", err)
	}
	if configErr.Code != code {
		t.Fatalf("error code = %q, want %q", configErr.Code, code)
	}
}
