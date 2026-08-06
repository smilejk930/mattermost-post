package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerJSONSuccessAndSecretSafeLog(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer environment-secret" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch calls {
		case 1:
			_, _ = w.Write([]byte(`{"id":"team-id"}`))
		case 2:
			_, _ = w.Write([]byte(`{"id":"channel-id"}`))
		case 3:
			_, _ = w.Write([]byte(`{"id":"post-id","channel_id":"channel-id"}`))
		default:
			t.Errorf("unexpected request %d", calls)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	configPath := writeRunnerConfig(t, dir, server.URL)
	var stdout, stderr bytes.Buffer
	runner := testRunner(server.Client())
	runner.Getenv = func(name string) string {
		if name == "MATTERMOST_BOT_TOKEN" {
			return "environment-secret"
		}
		return ""
	}
	exit := runner.Run([]string{
		"--config", configPath,
		"--group", "news",
		"--message", "민감한 게시글 본문",
		"--json",
	}, nil, &stdout, &stderr)
	if exit != ExitSuccess {
		t.Fatalf("exit = %d, stderr = %s", exit, stderr.String())
	}
	var result output
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not JSON: %v (%s)", err, stdout.String())
	}
	if !result.OK || result.PostID != "post-id" || result.ChannelID != "channel-id" {
		t.Fatalf("result = %#v", result)
	}

	logPath := filepath.Join(dir, "logs", "mattermost-post-2026-08-05.log")
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"environment-secret", "file-secret", "민감한 게시글 본문"} {
		if bytes.Contains(logData, []byte(secret)) || strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
			t.Fatalf("secret %q leaked; stdout=%s stderr=%s log=%s", secret, stdout.String(), stderr.String(), logData)
		}
	}
}

func TestRunnerAPIFailureUsesExitThree(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-123")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	dir := t.TempDir()
	configPath := writeRunnerConfig(t, dir, server.URL)
	var stdout, stderr bytes.Buffer
	runner := testRunner(server.Client())
	exit := runner.Run([]string{"--config", configPath, "--group", "news", "--message", "hello"}, nil, &stdout, &stderr)
	if exit != ExitAPI {
		t.Fatalf("exit = %d", exit)
	}
	if !strings.Contains(stderr.String(), "unauthorized") || !strings.Contains(stderr.String(), "req-123") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestRunnerUsageErrorsUseExitTwo(t *testing.T) {
	t.Run("missing group", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exit := testRunner(http.DefaultClient).Run([]string{"--message", "hello"}, nil, &stdout, &stderr)
		if exit != ExitUsage || !strings.Contains(stderr.String(), "group_required") {
			t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
		}
	})

	t.Run("multiple inputs", func(t *testing.T) {
		dir := t.TempDir()
		configPath := writeRunnerConfig(t, dir, "http://127.0.0.1:1")
		var stdout, stderr bytes.Buffer
		exit := testRunner(http.DefaultClient).Run([]string{
			"--config", configPath, "--group", "news", "--message", "hello", "--stdin",
		}, strings.NewReader("stdin"), &stdout, &stderr)
		if exit != ExitUsage || !strings.Contains(stderr.String(), "input_source_error") {
			t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
		}
	})
}

func TestRunnerHelpAndVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := testRunner(http.DefaultClient)
	if exit := runner.Run([]string{"--help"}, nil, &stdout, &stderr); exit != ExitSuccess {
		t.Fatalf("help exit = %d", exit)
	}
	if !strings.Contains(stdout.String(), "사용법") {
		t.Fatalf("help = %s", stdout.String())
	}

	stdout.Reset()
	if exit := runner.Run([]string{"--version"}, nil, &stdout, &stderr); exit != ExitSuccess {
		t.Fatalf("version exit = %d", exit)
	}
	if strings.TrimSpace(stdout.String()) != "mattermost-post test-version" {
		t.Fatalf("version = %q", stdout.String())
	}
}

func testRunner(client *http.Client) *Runner {
	return &Runner{
		Version:    "test-version",
		Getenv:     func(string) string { return "" },
		HTTPClient: client,
		Timeout:    time.Second,
		Now: func() time.Time {
			return time.Date(2026, 8, 5, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60))
		},
	}
}

func writeRunnerConfig(t *testing.T, dir, serverURL string) string {
	t.Helper()
	content := fmt.Sprintf(`mattermost:
  url: %q
  bot_token: "file-secret"
groups:
  news:
    team_name: "company"
    channel_name: "daily-news"
log:
  directory: "./logs"
`, serverURL)
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
