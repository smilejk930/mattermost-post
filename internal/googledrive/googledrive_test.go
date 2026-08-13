package googledrive

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGDocSearchesByNameAndExportsPlainText(t *testing.T) {
	var searchQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files":
			searchQuery = r.URL.Query().Get("q")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"files":[{"id":"document-id","name":"menu"}]}`))
		case "/files/document-id/export":
			if got := r.URL.Query().Get("mimeType"); got != "text/plain" {
				t.Errorf("mimeType = %q", got)
			}
			_, _ = w.Write([]byte("오늘의 메뉴\n김치찌개"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "구내식당_메뉴_20260807.gdoc")
	if err := os.WriteFile(path, []byte("shortcut"), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := &Loader{client: server.Client(), apiBase: server.URL}
	got, err := loader.LoadGDoc(context.Background(), path, "folder-id")
	if err != nil {
		t.Fatalf("LoadGDoc() error = %v", err)
	}
	if string(got) != "오늘의 메뉴\n김치찌개" {
		t.Fatalf("contents = %q", got)
	}
	if !strings.Contains(searchQuery, "name = '구내식당_메뉴_20260807'") || !strings.Contains(searchQuery, "'folder-id' in parents") {
		t.Fatalf("search query = %q", searchQuery)
	}
}

func TestLoadGDocRejectsDuplicateNames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"files":[{"id":"one"},{"id":"two"}]}`))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "menu.gdoc")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	loader := &Loader{client: server.Client(), apiBase: server.URL}
	_, err := loader.LoadGDoc(context.Background(), path, "")
	if err == nil {
		t.Fatal("LoadGDoc() error = nil")
	}
	driveErr, ok := err.(*Error)
	if !ok || driveErr.Code != "google_doc_ambiguous" {
		t.Fatalf("error = %#v", err)
	}
}

func TestEscapeQuery(t *testing.T) {
	got := escapeQuery(`owner's \\ menu`)
	want := `owner\'s \\\\ menu`
	if got != want {
		t.Fatalf("escapeQuery() = %q, want %q", got, want)
	}
	if _, err := url.QueryUnescape(url.QueryEscape(got)); err != nil {
		t.Fatal(err)
	}
}
