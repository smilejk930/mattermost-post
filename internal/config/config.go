package config

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const tokenEnvironmentVariable = "MATTERMOST_BOT_TOKEN"
const googleCredentialsEnvironmentVariable = "GOOGLE_APPLICATION_CREDENTIALS"

type Mattermost struct {
	URL      string `yaml:"url"`
	BotToken string `yaml:"bot_token"`
}

type Group struct {
	TeamName    string `yaml:"team_name"`
	ChannelName string `yaml:"channel_name"`
}

type Log struct {
	Directory string `yaml:"directory"`
}

type GoogleDrive struct {
	CredentialsFile string `yaml:"credentials_file"`
	FolderID        string `yaml:"folder_id"`
}

type Config struct {
	Mattermost  Mattermost       `yaml:"mattermost"`
	GoogleDrive GoogleDrive      `yaml:"google_drive"`
	Groups      map[string]Group `yaml:"groups"`
	Log         Log              `yaml:"log"`
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

// Load reads and strictly validates a YAML configuration file. Relative log
// paths are resolved from the configuration file's directory.
func Load(path string, getenv func(string) string) (*Config, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, configError("config_path_error", "설정 파일 경로를 확인할 수 없습니다.", err)
	}

	file, err := os.Open(absPath)
	if err != nil {
		return nil, configError("config_read_error", fmt.Sprintf("설정 파일을 읽을 수 없습니다: %s", path), err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)

	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return nil, configError("config_parse_error", "설정 파일의 YAML 형식이 올바르지 않습니다.", err)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, configError("config_parse_error", "설정 파일에는 YAML 문서 하나만 허용됩니다.", nil)
		}
		return nil, configError("config_parse_error", "설정 파일의 YAML 형식이 올바르지 않습니다.", err)
	}

	cfg.Mattermost.URL = strings.TrimSpace(cfg.Mattermost.URL)
	cfg.Mattermost.BotToken = strings.TrimSpace(cfg.Mattermost.BotToken)
	cfg.GoogleDrive.CredentialsFile = strings.TrimSpace(cfg.GoogleDrive.CredentialsFile)
	cfg.GoogleDrive.FolderID = strings.TrimSpace(cfg.GoogleDrive.FolderID)
	if getenv != nil {
		if token := strings.TrimSpace(getenv(tokenEnvironmentVariable)); token != "" {
			cfg.Mattermost.BotToken = token
		}
		if credentials := strings.TrimSpace(getenv(googleCredentialsEnvironmentVariable)); credentials != "" {
			cfg.GoogleDrive.CredentialsFile = credentials
		}
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	cfg.Mattermost.URL = strings.TrimRight(cfg.Mattermost.URL, "/")
	cfg.Log.Directory = strings.TrimSpace(cfg.Log.Directory)
	if cfg.Log.Directory == "" {
		cfg.Log.Directory = "./logs"
	}
	if !filepath.IsAbs(cfg.Log.Directory) {
		cfg.Log.Directory = filepath.Join(filepath.Dir(absPath), cfg.Log.Directory)
	}
	cfg.Log.Directory = filepath.Clean(cfg.Log.Directory)
	if cfg.GoogleDrive.CredentialsFile != "" && !filepath.IsAbs(cfg.GoogleDrive.CredentialsFile) {
		cfg.GoogleDrive.CredentialsFile = filepath.Join(filepath.Dir(absPath), cfg.GoogleDrive.CredentialsFile)
	}
	if cfg.GoogleDrive.CredentialsFile != "" {
		cfg.GoogleDrive.CredentialsFile = filepath.Clean(cfg.GoogleDrive.CredentialsFile)
	}

	return &cfg, nil
}

func validate(cfg *Config) error {
	if cfg.Mattermost.URL == "" {
		return configError("config_validation_error", "mattermost.url 값이 비어 있습니다.", nil)
	}
	parsed, err := url.Parse(cfg.Mattermost.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return configError("config_validation_error", "mattermost.url은 http:// 또는 https://로 시작하는 유효한 URL이어야 합니다.", err)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return configError("config_validation_error", "mattermost.url에는 사용자 정보, 쿼리 또는 프래그먼트를 사용할 수 없습니다.", nil)
	}
	if cfg.Mattermost.BotToken == "" || cfg.Mattermost.BotToken == "replace-with-real-token" {
		return configError("config_validation_error", "유효한 mattermost.bot_token 또는 MATTERMOST_BOT_TOKEN이 필요합니다.", nil)
	}
	if len(cfg.Groups) == 0 {
		return configError("config_validation_error", "groups에는 하나 이상의 그룹이 필요합니다.", nil)
	}
	for name, group := range cfg.Groups {
		trimmedName := strings.TrimSpace(name)
		group.TeamName = strings.TrimSpace(group.TeamName)
		group.ChannelName = strings.TrimSpace(group.ChannelName)
		if trimmedName == "" {
			return configError("config_validation_error", "그룹 이름은 비어 있을 수 없습니다.", nil)
		}
		if trimmedName != name {
			return configError("config_validation_error", fmt.Sprintf("그룹 이름 %q의 앞뒤에는 공백을 사용할 수 없습니다.", name), nil)
		}
		if group.TeamName == "" || group.ChannelName == "" {
			return configError("config_validation_error", fmt.Sprintf("그룹 %q의 team_name과 channel_name은 필수입니다.", name), nil)
		}
		cfg.Groups[name] = group
	}
	return nil
}

func configError(code, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}
