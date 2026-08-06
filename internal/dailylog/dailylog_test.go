package dailylog

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoggerWritesMetadataOnlyDailyJSONLine(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 5, 18, 30, 0, 123, time.FixedZone("KST", 9*60*60))
	logger := NewWithClock(dir, func() time.Time { return now })
	entry := Entry{
		Group: "news", Team: "company", Channel: "daily-news",
		CharacterCount: 12, Success: true, PostID: "post-id",
	}
	if err := logger.Write(entry); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	path := filepath.Join(dir, "mattermost-post-2026-08-05.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("secret-token")) || bytes.Contains(data, []byte("message body")) {
		t.Fatalf("log contains secret data: %s", data)
	}
	var got Entry
	if err := json.Unmarshal(bytes.TrimSpace(data), &got); err != nil {
		t.Fatalf("invalid JSON log: %v", err)
	}
	if got.Timestamp != now.Format(time.RFC3339Nano) || got.PostID != "post-id" || !got.Success {
		t.Fatalf("entry = %#v", got)
	}
}
