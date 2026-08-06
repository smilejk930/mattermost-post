package mattermost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreatePost(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer bot-secret" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			if r.Method != http.MethodGet || r.URL.Path != "/api/v4/teams/name/company" {
				t.Errorf("team request = %s %s", r.Method, r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"id":"team-id"}`))
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api/v4/teams/team-id/channels/name/daily-news" {
				t.Errorf("channel request = %s %s", r.Method, r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"id":"channel-id"}`))
		case 3:
			if r.Method != http.MethodPost || r.URL.Path != "/api/v4/posts" {
				t.Errorf("post request = %s %s", r.Method, r.URL.Path)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
			}
			var body struct {
				ChannelID string `json:"channel_id"`
				Message   string `json:"message"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.ChannelID != "channel-id" || body.Message != "첫 줄\n둘째 줄" {
				t.Errorf("post body = %#v", body)
			}
			_, _ = w.Write([]byte(`{"id":"post-id","channel_id":"channel-id"}`))
		default:
			t.Errorf("unexpected call %d", calls)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "bot-secret", server.Client())
	result, err := client.CreatePost(context.Background(), "company", "daily-news", "첫 줄\n둘째 줄")
	if err != nil {
		t.Fatalf("CreatePost() error = %v", err)
	}
	if result.PostID != "post-id" || result.ChannelID != "channel-id" || calls != 3 {
		t.Fatalf("result = %#v, calls = %d", result, calls)
	}
}

func TestCreatePostClassifiesHTTPFailures(t *testing.T) {
	tests := []struct {
		status int
		code   string
	}{
		{http.StatusBadRequest, "bad_request"},
		{http.StatusUnauthorized, "unauthorized"},
		{http.StatusForbidden, "forbidden"},
		{http.StatusNotFound, "team_not_found"},
		{http.StatusTooManyRequests, "rate_limited"},
		{http.StatusInternalServerError, "server_error"},
	}
	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Request-Id", "request-id")
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(`{"message":"server detail"}`))
			}))
			defer server.Close()

			client := NewClient(server.URL, "token", server.Client())
			_, err := client.CreatePost(context.Background(), "team", "channel", "message")
			apiErr, ok := err.(*Error)
			if !ok {
				t.Fatalf("error type = %T", err)
			}
			if apiErr.Code != test.code || apiErr.HTTPStatus != test.status || apiErr.RequestID != "request-id" {
				t.Fatalf("error = %#v", apiErr)
			}
		})
	}
}

func TestHTTPErrorDoesNotExposeServerMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"sensitive post body"}`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "token", server.Client()).CreatePost(context.Background(), "team", "channel", "message")
	apiErr := err.(*Error)
	if strings.Contains(apiErr.Message, "sensitive post body") {
		t.Fatalf("server message leaked: %q", apiErr.Message)
	}
}

func TestCreatePostChannelNotFound(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"id":"team-id"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "token", server.Client()).CreatePost(context.Background(), "team", "channel", "message")
	apiErr := err.(*Error)
	if apiErr.Code != "channel_not_found" {
		t.Fatalf("code = %q", apiErr.Code)
	}
}

func TestCreatePostRejectsBadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	_, err := NewClient(server.URL, "token", server.Client()).CreatePost(context.Background(), "team", "channel", "message")
	apiErr := err.(*Error)
	if apiErr.Code != "bad_response" {
		t.Fatalf("code = %q", apiErr.Code)
	}
}

func TestCreatePostTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := NewClient(server.URL, "token", server.Client()).CreatePost(ctx, "team", "channel", strings.Repeat("x", 10))
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Code != "network_timeout" {
		t.Fatalf("error = %#v", err)
	}
}
