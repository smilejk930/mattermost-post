package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/smilejk930/mattermost-post/internal/config"
	"github.com/smilejk930/mattermost-post/internal/dailylog"
	"github.com/smilejk930/mattermost-post/internal/mattermost"
	"github.com/smilejk930/mattermost-post/internal/message"
)

const (
	ExitSuccess = 0
	ExitUsage   = 2
	ExitAPI     = 3
)

type Runner struct {
	Version    string
	Getenv     func(string) string
	HTTPClient mattermost.HTTPClient
	Timeout    time.Duration
	Now        func() time.Time
}

type output struct {
	OK         bool   `json:"ok"`
	Group      string `json:"group,omitempty"`
	Team       string `json:"team,omitempty"`
	Channel    string `json:"channel,omitempty"`
	PostID     string `json:"post_id,omitempty"`
	ChannelID  string `json:"channel_id,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Message    string `json:"message,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
}

type trackedString struct {
	value string
	set   bool
}

func (v *trackedString) String() string { return v.value }

func (v *trackedString) Set(value string) error {
	v.value = value
	v.set = true
	return nil
}

func New(version string) *Runner {
	return &Runner{
		Version:    version,
		Getenv:     os.Getenv,
		HTTPClient: &http.Client{},
		Timeout:    30 * time.Second,
		Now:        time.Now,
	}
}

// Run executes the CLI without calling os.Exit, which keeps command behavior
// directly testable.
func (r *Runner) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if r.Getenv == nil {
		r.Getenv = os.Getenv
	}
	if r.HTTPClient == nil {
		r.HTTPClient = &http.Client{}
	}
	if r.Timeout <= 0 {
		r.Timeout = 30 * time.Second
	}
	if r.Now == nil {
		r.Now = time.Now
	}

	flags := flag.NewFlagSet("mattermost-post", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var (
		groupName    string
		configPath   string
		jsonOutput   bool
		useStdin     bool
		showVersion  bool
		messageValue trackedString
		fileValue    trackedString
	)
	flags.StringVar(&groupName, "group", "", "게시 대상 그룹 이름")
	flags.StringVar(&configPath, "config", "./config.yaml", "YAML 설정 파일 경로")
	flags.BoolVar(&jsonOutput, "json", false, "실행 결과를 JSON으로 출력")
	flags.BoolVar(&useStdin, "stdin", false, "표준입력에서 게시글 읽기")
	flags.BoolVar(&showVersion, "version", false, "버전 출력")
	flags.Var(&messageValue, "message", "명령행 게시글")
	flags.Var(&fileValue, "file", "UTF-8 게시글 파일 경로")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			writeUsage(stdout)
			return ExitSuccess
		}
		return writeError(stdout, stderr, jsonOutput, output{
			OK: false, ErrorCode: "flag_error", Message: "명령행 옵션이 올바르지 않습니다: " + err.Error(),
		}, ExitUsage)
	}
	if flags.NArg() != 0 {
		return writeError(stdout, stderr, jsonOutput, output{
			OK: false, ErrorCode: "unexpected_argument", Message: "위치 인자는 사용할 수 없습니다. --message, --file 또는 --stdin을 사용하세요.",
		}, ExitUsage)
	}
	if showVersion {
		fmt.Fprintf(stdout, "mattermost-post %s\n", r.Version)
		return ExitSuccess
	}

	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return writeError(stdout, stderr, jsonOutput, output{
			OK: false, ErrorCode: "group_required", Message: "--group 옵션이 필요합니다.",
		}, ExitUsage)
	}

	cfg, err := config.Load(configPath, r.Getenv)
	if err != nil {
		code := "config_error"
		var configErr *config.Error
		if errors.As(err, &configErr) {
			code = configErr.Code
		}
		return writeError(stdout, stderr, jsonOutput, output{
			OK: false, Group: groupName, ErrorCode: code, Message: err.Error(),
		}, ExitUsage)
	}

	group, ok := cfg.Groups[groupName]
	if !ok {
		result := output{
			OK: false, Group: groupName, ErrorCode: "group_not_found", Message: fmt.Sprintf("설정에서 그룹 %q을 찾을 수 없습니다.", groupName),
		}
		r.writeLog(cfg.Log.Directory, stderr, dailylog.Entry{
			Group: groupName, Success: false, ErrorCode: result.ErrorCode,
		})
		return writeError(stdout, stderr, jsonOutput, result, ExitUsage)
	}

	text, err := message.Load(message.Options{
		Message: messageValue.value, MessageSet: messageValue.set,
		File: fileValue.value, FileSet: fileValue.set,
		Stdin: useStdin,
	}, stdin)
	if err != nil {
		code := "input_error"
		var inputErr *message.Error
		if errors.As(err, &inputErr) {
			code = inputErr.Code
		}
		result := output{
			OK: false, Group: groupName, Team: group.TeamName, Channel: group.ChannelName,
			ErrorCode: code, Message: err.Error(),
		}
		r.writeLog(cfg.Log.Directory, stderr, dailylog.Entry{
			Group: groupName, Team: group.TeamName, Channel: group.ChannelName,
			Success: false, ErrorCode: code,
		})
		return writeError(stdout, stderr, jsonOutput, result, ExitUsage)
	}

	characterCount := utf8.RuneCountInString(text)
	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout)
	defer cancel()
	client := mattermost.NewClient(cfg.Mattermost.URL, cfg.Mattermost.BotToken, r.HTTPClient)
	post, err := client.CreatePost(ctx, group.TeamName, group.ChannelName, text)
	if err != nil {
		result := output{
			OK: false, Group: groupName, Team: group.TeamName, Channel: group.ChannelName,
			ErrorCode: "api_error", Message: err.Error(),
		}
		entry := dailylog.Entry{
			Group: groupName, Team: group.TeamName, Channel: group.ChannelName,
			CharacterCount: characterCount, Success: false, ErrorCode: result.ErrorCode,
		}
		var apiErr *mattermost.Error
		if errors.As(err, &apiErr) {
			result.ErrorCode = apiErr.Code
			result.HTTPStatus = apiErr.HTTPStatus
			result.RequestID = apiErr.RequestID
			entry.ErrorCode = apiErr.Code
			entry.HTTPStatus = apiErr.HTTPStatus
			entry.RequestID = apiErr.RequestID
		}
		r.writeLog(cfg.Log.Directory, stderr, entry)
		return writeError(stdout, stderr, jsonOutput, result, ExitAPI)
	}

	result := output{
		OK: true, Group: groupName, Team: group.TeamName, Channel: group.ChannelName,
		PostID: post.PostID, ChannelID: post.ChannelID,
	}
	r.writeLog(cfg.Log.Directory, stderr, dailylog.Entry{
		Group: groupName, Team: group.TeamName, Channel: group.ChannelName,
		CharacterCount: characterCount, Success: true, PostID: post.PostID,
	})
	writeSuccess(stdout, jsonOutput, result)
	return ExitSuccess
}

func (r *Runner) writeLog(directory string, stderr io.Writer, entry dailylog.Entry) {
	logger := dailylog.NewWithClock(directory, r.Now)
	if err := logger.Write(entry); err != nil {
		fmt.Fprintf(stderr, "경고: 일별 로그를 기록하지 못했습니다: %v\n", err)
	}
}

func writeSuccess(stdout io.Writer, jsonOutput bool, result output) {
	if jsonOutput {
		_ = json.NewEncoder(stdout).Encode(result)
		return
	}
	fmt.Fprintf(stdout, "게시 성공: group=%s team=%s channel=%s post_id=%s\n", result.Group, result.Team, result.Channel, result.PostID)
}

func writeError(stdout, stderr io.Writer, jsonOutput bool, result output, exitCode int) int {
	if jsonOutput {
		_ = json.NewEncoder(stdout).Encode(result)
	} else {
		fmt.Fprintf(stderr, "오류 [%s]: %s\n", result.ErrorCode, result.Message)
		if result.RequestID != "" {
			fmt.Fprintf(stderr, "Mattermost request_id: %s\n", result.RequestID)
		}
	}
	return exitCode
}

func writeUsage(writer io.Writer) {
	fmt.Fprintln(writer, `Mattermost 채널에 게시글 하나를 등록합니다.

사용법:
  mattermost-post --group NAME --message "짧은 게시글" [옵션]
  mattermost-post --group NAME --file article.md [옵션]
  generator | mattermost-post --group NAME --stdin [옵션]

옵션:
  --group NAME    config.yaml의 게시 대상 그룹 (필수)
  --message TEXT  명령행 게시글
  --file PATH     UTF-8 게시글 파일
  --stdin         표준입력에서 게시글 읽기
  --config PATH   YAML 설정 파일 (기본: ./config.yaml)
  --json          결과를 JSON으로 출력
  --version       버전 출력
  --help          도움말 출력`)
}
