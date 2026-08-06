package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const maxResponseBytes = 1 << 20

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	baseURL    string
	token      string
	httpClient HTTPClient
}

type PostResult struct {
	PostID    string
	ChannelID string
}

type Error struct {
	Code       string
	Message    string
	Operation  string
	HTTPStatus int
	RequestID  string
	Cause      error
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func NewClient(baseURL, token string, httpClient HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: httpClient,
	}
}

// CreatePost resolves a team and channel by their Mattermost names and creates
// one post. It intentionally performs no automatic retries.
func (c *Client) CreatePost(ctx context.Context, teamName, channelName, message string) (*PostResult, error) {
	var team struct {
		ID string `json:"id"`
	}
	teamPath := "/api/v4/teams/name/" + url.PathEscape(teamName)
	if err := c.doJSON(ctx, http.MethodGet, teamPath, nil, &team, "lookup_team"); err != nil {
		return nil, err
	}
	if team.ID == "" {
		return nil, apiError("bad_response", "Mattermost 팀 조회 응답에 ID가 없습니다.", "lookup_team", 0, "", nil)
	}

	var channel struct {
		ID string `json:"id"`
	}
	channelPath := "/api/v4/teams/" + url.PathEscape(team.ID) + "/channels/name/" + url.PathEscape(channelName)
	if err := c.doJSON(ctx, http.MethodGet, channelPath, nil, &channel, "lookup_channel"); err != nil {
		return nil, err
	}
	if channel.ID == "" {
		return nil, apiError("bad_response", "Mattermost 채널 조회 응답에 ID가 없습니다.", "lookup_channel", 0, "", nil)
	}

	payload := struct {
		ChannelID string `json:"channel_id"`
		Message   string `json:"message"`
	}{ChannelID: channel.ID, Message: message}
	var post struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v4/posts", payload, &post, "create_post"); err != nil {
		return nil, err
	}
	if post.ID == "" {
		return nil, apiError("bad_response", "Mattermost 게시 응답에 post ID가 없습니다.", "create_post", 0, "", nil)
	}
	if post.ChannelID == "" {
		post.ChannelID = channel.ID
	}
	return &PostResult{PostID: post.ID, ChannelID: post.ChannelID}, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload, target any, operation string) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return apiError("request_error", "Mattermost 요청을 생성할 수 없습니다.", operation, 0, "", err)
		}
		body = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return apiError("request_error", "Mattermost 요청을 생성할 수 없습니다.", operation, 0, "", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			return apiError("network_timeout", "Mattermost 요청 제한시간(30초)을 초과했습니다.", operation, 0, "", err)
		}
		return apiError("network_error", "Mattermost 서버에 연결할 수 없습니다.", operation, 0, "", err)
	}
	defer response.Body.Close()

	requestID := response.Header.Get("X-Request-Id")
	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return apiError("bad_response", "Mattermost 응답을 읽을 수 없습니다.", operation, response.StatusCode, requestID, err)
	}
	if len(data) > maxResponseBytes {
		return apiError("bad_response", "Mattermost 응답이 허용 크기를 초과했습니다.", operation, response.StatusCode, requestID, nil)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return parseHTTPError(response.StatusCode, requestID, operation, data)
	}
	if len(data) == 0 {
		return apiError("bad_response", "Mattermost 응답이 비어 있습니다.", operation, response.StatusCode, requestID, nil)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return apiError("bad_response", "Mattermost 응답 JSON이 올바르지 않습니다.", operation, response.StatusCode, requestID, err)
	}
	return nil
}

func parseHTTPError(status int, requestID, operation string, data []byte) error {
	var response struct {
		ID        string `json:"id"`
		RequestID string `json:"request_id"`
	}
	_ = json.Unmarshal(data, &response)
	if requestID == "" {
		requestID = response.RequestID
	}

	code := "api_error"
	message := "Mattermost API 요청이 거부되었습니다."
	switch status {
	case http.StatusBadRequest:
		code = "bad_request"
		message = "Mattermost가 게시 요청을 거부했습니다. 서버의 게시글 길이 및 형식 제한을 확인하세요."
	case http.StatusUnauthorized:
		code = "unauthorized"
		message = "봇 토큰이 유효하지 않거나 만료되었습니다."
	case http.StatusForbidden:
		code = "forbidden"
		message = "봇에 필요한 팀·채널 조회 또는 게시 권한이 없습니다."
	case http.StatusNotFound:
		switch operation {
		case "lookup_team":
			code = "team_not_found"
			message = "설정한 Mattermost 팀을 찾을 수 없습니다. team_name을 확인하세요."
		case "lookup_channel":
			code = "channel_not_found"
			message = "설정한 Mattermost 채널을 찾을 수 없습니다. channel_name과 봇 멤버십을 확인하세요."
		default:
			code = "not_found"
			message = "Mattermost 게시 대상을 찾을 수 없습니다."
		}
	case http.StatusTooManyRequests:
		code = "rate_limited"
		message = "Mattermost API 요청 한도를 초과했습니다. 잠시 후 다시 실행하세요."
	default:
		if status >= http.StatusInternalServerError {
			code = "server_error"
			message = "Mattermost 서버 오류가 발생했습니다."
		} else {
			message = fmt.Sprintf("Mattermost API가 HTTP %d 오류를 반환했습니다.", status)
		}
	}
	return apiError(code, message, operation, status, requestID, nil)
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func apiError(code, message, operation string, status int, requestID string, cause error) error {
	return &Error{
		Code:       code,
		Message:    message,
		Operation:  operation,
		HTTPStatus: status,
		RequestID:  requestID,
		Cause:      cause,
	}
}
