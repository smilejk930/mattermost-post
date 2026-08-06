package message

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLoadSources(t *testing.T) {
	t.Run("message", func(t *testing.T) {
		got, err := Load(Options{Message: "첫 줄\n둘째 줄", MessageSet: true}, nil)
		if err != nil || got != "첫 줄\n둘째 줄" {
			t.Fatalf("Load() = %q, %v", got, err)
		}
	})

	t.Run("file with BOM", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "post.md")
		want := "제목\n\n본문 😀\n"
		if err := os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, []byte(want)...), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := Load(Options{File: path, FileSet: true}, nil)
		if err != nil || got != want {
			t.Fatalf("Load() = %q, %v", got, err)
		}
	})

	t.Run("stdin", func(t *testing.T) {
		got, err := Load(Options{Stdin: true}, strings.NewReader("파이프 입력"))
		if err != nil || got != "파이프 입력" {
			t.Fatalf("Load() = %q, %v", got, err)
		}
	})
}

func TestLoadRequiresExactlyOneSource(t *testing.T) {
	tests := []Options{
		{},
		{Message: "a", MessageSet: true, Stdin: true},
		{Message: "a", MessageSet: true, File: "x", FileSet: true},
	}
	for _, options := range tests {
		_, err := Load(options, strings.NewReader("stdin"))
		assertInputErrorCode(t, err, "input_source_error")
	}
}

func TestLoadRejectsEmptyAndInvalidUTF8(t *testing.T) {
	_, err := Load(Options{Message: " \n\t", MessageSet: true}, nil)
	assertInputErrorCode(t, err, "input_empty_error")

	_, err = Load(Options{Stdin: true}, bytes.NewReader([]byte{0xff, 0xfe}))
	assertInputErrorCode(t, err, "input_encoding_error")
}

func TestLoadMessageRuneLimit(t *testing.T) {
	atLimit := strings.Repeat("😀", MaxRunes)
	got, err := Load(Options{Message: atLimit, MessageSet: true}, nil)
	if err != nil {
		t.Fatalf("at limit error = %v", err)
	}
	if utf8.RuneCountInString(got) != MaxRunes {
		t.Fatalf("rune count = %d", utf8.RuneCountInString(got))
	}

	_, err = Load(Options{Message: atLimit + "x", MessageSet: true}, nil)
	assertInputErrorCode(t, err, "input_too_long")
}

func assertInputErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	inputErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T", err)
	}
	if inputErr.Code != code {
		t.Fatalf("error code = %q, want %q", inputErr.Code, code)
	}
}
