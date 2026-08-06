package dailylog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Entry struct {
	Timestamp      string `json:"timestamp"`
	Group          string `json:"group"`
	Team           string `json:"team"`
	Channel        string `json:"channel"`
	CharacterCount int    `json:"character_count"`
	Success        bool   `json:"success"`
	HTTPStatus     int    `json:"http_status,omitempty"`
	PostID         string `json:"post_id,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
}

type Logger struct {
	directory string
	now       func() time.Time
}

func New(directory string) *Logger {
	return &Logger{directory: directory, now: time.Now}
}

func NewWithClock(directory string, now func() time.Time) *Logger {
	return &Logger{directory: directory, now: now}
}

// Write appends one metadata-only JSON line to a date-based log file.
func (l *Logger) Write(entry Entry) error {
	now := l.now()
	entry.Timestamp = now.Format(time.RFC3339Nano)
	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode log entry: %w", err)
	}
	encoded = append(encoded, '\n')

	if err := os.MkdirAll(l.directory, 0o750); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	path := filepath.Join(l.directory, "mattermost-post-"+now.Format("2006-01-02")+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open daily log: %w", err)
	}
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		return fmt.Errorf("write daily log: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close daily log: %w", err)
	}
	return nil
}
