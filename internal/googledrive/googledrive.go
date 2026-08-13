package googledrive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	driveReadOnlyScope = "https://www.googleapis.com/auth/drive.readonly"
	driveAPIBase       = "https://www.googleapis.com/drive/v3"
	maxExportBytes     = 16_383*4 + 4
)

type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }

type Loader struct {
	client  *http.Client
	apiBase string
}

// New creates a Google Drive loader authenticated by a service-account JSON
// key. The service account needs read access to the configured Drive folder.
func New(ctx context.Context, credentialsFile string, baseClient *http.Client) (*Loader, error) {
	if strings.TrimSpace(credentialsFile) == "" {
		return nil, driveError("google_credentials_required", ".gdoc 게시에는 google_drive.credentials_file 또는 GOOGLE_APPLICATION_CREDENTIALS 설정이 필요합니다.", nil)
	}

	credentials, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, driveError("google_credentials_read_error", fmt.Sprintf("Google 서비스 계정 인증 파일을 읽을 수 없습니다: %s", credentialsFile), err)
	}
	jwtConfig, err := google.JWTConfigFromJSON(credentials, driveReadOnlyScope)
	if err != nil {
		return nil, driveError("google_credentials_parse_error", "Google 서비스 계정 인증 파일이 올바르지 않습니다.", err)
	}

	if baseClient == nil {
		baseClient = &http.Client{}
	}
	transport := baseClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	tokenContext := context.WithValue(ctx, oauth2.HTTPClient, baseClient)
	authorized := *baseClient
	authorized.Transport = &oauth2.Transport{
		Source: jwtConfig.TokenSource(tokenContext),
		Base:   transport,
	}
	return &Loader{client: &authorized, apiBase: driveAPIBase}, nil
}

// LoadGDoc finds the native Google Doc represented by a local .gdoc filename
// and exports its current contents as UTF-8 plain text.
func (l *Loader) LoadGDoc(ctx context.Context, gdocPath, folderID string) ([]byte, error) {
	info, err := os.Stat(gdocPath)
	if err != nil {
		return nil, driveError("input_file_error", fmt.Sprintf("Google Docs 바로가기 파일을 확인할 수 없습니다: %s", gdocPath), err)
	}
	if info.IsDir() {
		return nil, driveError("input_file_error", fmt.Sprintf("Google Docs 바로가기 경로가 파일이 아닙니다: %s", gdocPath), nil)
	}
	name := strings.TrimSuffix(filepath.Base(gdocPath), filepath.Ext(gdocPath))
	if strings.TrimSpace(name) == "" {
		return nil, driveError("google_doc_name_error", ".gdoc 파일 이름에서 Google 문서 이름을 확인할 수 없습니다.", nil)
	}

	query := "name = '" + escapeQuery(name) + "' and mimeType = 'application/vnd.google-apps.document' and trashed = false"
	if strings.TrimSpace(folderID) != "" {
		query += " and '" + escapeQuery(strings.TrimSpace(folderID)) + "' in parents"
	}
	params := url.Values{
		"q":                         {query},
		"fields":                    {"files(id,name,modifiedTime)"},
		"orderBy":                   {"modifiedTime desc"},
		"pageSize":                  {"2"},
		"spaces":                    {"drive"},
		"includeItemsFromAllDrives": {"true"},
		"supportsAllDrives":         {"true"},
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, l.apiBase+"/files?"+params.Encode(), nil)
	if err != nil {
		return nil, driveError("google_drive_request_error", "Google Drive 문서 검색 요청을 만들 수 없습니다.", err)
	}
	response, err := l.client.Do(request)
	if err != nil {
		return nil, driveError("google_drive_network_error", "Google Drive에 연결하거나 인증할 수 없습니다.", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, driveError("google_drive_search_error", fmt.Sprintf("Google Drive 문서 검색이 거부되었습니다(HTTP %d).", response.StatusCode), nil)
	}

	var result struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return nil, driveError("google_drive_response_error", "Google Drive 문서 검색 응답이 올바르지 않습니다.", err)
	}
	if len(result.Files) == 0 {
		return nil, driveError("google_doc_not_found", fmt.Sprintf("Google Drive에서 문서 %q을(를) 찾을 수 없습니다. 서비스 계정 공유 권한과 folder_id를 확인하세요.", name), nil)
	}
	if len(result.Files) > 1 {
		return nil, driveError("google_doc_ambiguous", fmt.Sprintf("Google Drive에서 이름이 같은 문서 %q이(가) 여러 개 발견되었습니다. google_drive.folder_id를 설정하세요.", name), nil)
	}

	exportURL := l.apiBase + "/files/" + url.PathEscape(result.Files[0].ID) + "/export?" + url.Values{"mimeType": {"text/plain"}}.Encode()
	exportRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, exportURL, nil)
	if err != nil {
		return nil, driveError("google_drive_request_error", "Google 문서 내보내기 요청을 만들 수 없습니다.", err)
	}
	exportResponse, err := l.client.Do(exportRequest)
	if err != nil {
		return nil, driveError("google_drive_network_error", "Google 문서 내용을 가져올 수 없습니다.", err)
	}
	defer exportResponse.Body.Close()
	if exportResponse.StatusCode != http.StatusOK {
		return nil, driveError("google_doc_export_error", fmt.Sprintf("Google 문서 내보내기가 거부되었습니다(HTTP %d).", exportResponse.StatusCode), nil)
	}

	contents, err := io.ReadAll(io.LimitReader(exportResponse.Body, maxExportBytes+1))
	if err != nil {
		return nil, driveError("google_doc_export_error", "Google 문서 내용을 읽을 수 없습니다.", err)
	}
	if len(contents) > maxExportBytes {
		return nil, driveError("input_too_long", "Google 문서가 Mattermost 게시글 한도를 초과합니다.", nil)
	}
	return contents, nil
}

func escapeQuery(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `'`, `\'`)
}

func driveError(code, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}
