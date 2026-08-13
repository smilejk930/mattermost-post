package message

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const MaxRunes = 16_383

const maxBytes = MaxRunes*utf8.UTFMax + 3 // includes an optional UTF-8 BOM

type Options struct {
	Message    string
	MessageSet bool
	File       string
	FileSet    bool
	Stdin      bool
}

type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// Load reads exactly one message source and preserves its contents, except for
// removing a leading UTF-8 BOM.
func Load(options Options, stdin io.Reader) (string, error) {
	sources := 0
	if options.MessageSet {
		sources++
	}
	if options.FileSet {
		sources++
	}
	if options.Stdin {
		sources++
	}
	if sources != 1 {
		return "", inputError("input_source_error", "--message, --file, --stdin 중 정확히 하나를 지정해야 합니다.", nil)
	}

	var raw []byte
	var err error
	switch {
	case options.MessageSet:
		raw = []byte(options.Message)
	case options.FileSet:
		if strings.TrimSpace(options.File) == "" {
			return "", inputError("input_file_error", "--file 경로가 비어 있습니다.", nil)
		}
		file, openErr := os.Open(options.File)
		if openErr != nil {
			return "", inputError("input_file_error", fmt.Sprintf("게시글 파일을 읽을 수 없습니다: %s", options.File), openErr)
		}
		raw, err = readLimited(file)
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return "", inputError("input_file_error", fmt.Sprintf("게시글 파일을 읽을 수 없습니다: %s", options.File), err)
		}
	case options.Stdin:
		if stdin == nil {
			return "", inputError("input_stdin_error", "표준입력을 읽을 수 없습니다.", nil)
		}
		raw, err = readLimited(stdin)
		if err != nil {
			return "", inputError("input_stdin_error", "표준입력을 읽을 수 없습니다.", err)
		}
	}

	return Validate(raw)
}

// Validate checks externally loaded message bytes using the same rules as
// --file and --stdin input.
func Validate(raw []byte) (string, error) {
	if len(raw) > maxBytes {
		return "", tooLongError()
	}
	if !utf8.Valid(raw) {
		return "", inputError("input_encoding_error", "게시글은 유효한 UTF-8이어야 합니다.", nil)
	}

	text := strings.TrimPrefix(string(raw), "\uFEFF")
	if strings.TrimSpace(text) == "" {
		return "", inputError("input_empty_error", "게시글이 비어 있거나 공백만 포함하고 있습니다.", nil)
	}
	if count := utf8.RuneCountInString(text); count > MaxRunes {
		return "", tooLongError()
	}
	return text, nil
}

func readLimited(reader io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(reader, int64(maxBytes)+1))
}

func tooLongError() error {
	return inputError("input_too_long", fmt.Sprintf("게시글은 Mattermost 한도인 %d자를 초과할 수 없습니다.", MaxRunes), nil)
}

func inputError(code, text string, cause error) error {
	return &Error{Code: code, Message: text, Cause: cause}
}
