# mattermost-post

Mattermost REST API v4를 사용해 설정된 팀·채널에 봇 게시글 하나를 등록하는 전용 CLI입니다. 스크래퍼, 생성형 AI 도구, 배치 작업과 분리해 재사용할 수 있으며 Windows와 Linux에서 단일 바이너리로 실행됩니다.

## 주요 기능

- 그룹 이름으로 하나의 Mattermost 팀·채널 쌍 선택
- 짧은 명령행 메시지, UTF-8 파일, 표준입력 파이프 지원
- 사람이 읽는 기본 결과와 자동화용 `--json` 결과
- 본문과 토큰을 제외한 일별 JSON Lines 로그
- Mattermost 단일 게시글 한도 16,383 Unicode 문자 사전 검증
- 게시 중복을 유발할 수 있는 POST 자동 재시도 없음

## 사전 준비

1. Mattermost에서 봇 계정을 만들고 봇 액세스 토큰을 발급합니다.
2. 봇을 게시 대상 팀과 채널에 추가합니다. 비공개 채널도 반드시 봇이 멤버여야 합니다.
3. 설정의 `team_name`과 `channel_name`에는 화면 표시명이 아니라 URL에서 사용하는 고유 name/slug를 입력합니다.
4. Mattermost 서버의 REST API v4가 활성화되어 있어야 합니다.

## 설정

예제 파일을 실제 설정 파일로 복사합니다.

PowerShell:

```powershell
Copy-Item config.example.yaml config.yaml
```

Bash:

```bash
cp config.example.yaml config.yaml
```

`config.yaml`을 환경에 맞게 수정합니다.

```yaml
mattermost:
  url: "https://mattermost.example.com"
  bot_token: "your-real-bot-token"

groups:
  daily-news:
    team_name: "company"
    channel_name: "daily-news"
  alerts:
    team_name: "operations"
    channel_name: "alerts"

log:
  directory: "./logs"
```

- Mattermost URL에는 `/api/v4`를 붙이지 않습니다. 서버가 하위 경로에 설치된 경우 해당 기본 경로까지 입력할 수 있습니다.
- `log.directory`가 상대 경로이면 `config.yaml`이 위치한 디렉터리를 기준으로 해석합니다. 생략하거나 비워 두면 `./logs`입니다.
- `MATTERMOST_BOT_TOKEN` 환경변수가 비어 있지 않으면 설정 파일의 `bot_token`보다 우선합니다.
- 실제 `config.yaml`은 Git에서 제외됩니다. `config.example.yaml`에는 실제 토큰을 입력하지 마세요.

환경변수 사용 예:

```powershell
$env:MATTERMOST_BOT_TOKEN = "your-real-bot-token"
```

```bash
export MATTERMOST_BOT_TOKEN="your-real-bot-token"
```

## 사용법

간단한 게시글:

```powershell
./mattermost-post.exe --group daily-news --message "간단한 게시글"
```

```bash
./mattermost-post --group daily-news --message "간단한 게시글"
```

줄바꿈이 있거나 긴 UTF-8 파일:

```powershell
./mattermost-post.exe --group daily-news --file article.md
```

```bash
./mattermost-post --group daily-news --file article.md
```

다른 프로그램의 UTF-8 출력 파이프:

```powershell
Get-Content -Raw -Encoding utf8 article.md | ./mattermost-post.exe --group daily-news --stdin
```

```bash
cat article.md | ./mattermost-post --group daily-news --stdin
```

Windows PowerShell 5.1은 네이티브 프로그램 파이프 인코딩이 UTF-8이 아닐 수 있습니다. 이 경우 `--file`을 사용하거나 PowerShell 7 이상에서 실행하세요.

설정 파일 위치가 기본값 `./config.yaml`과 다르면 명시합니다.

```text
mattermost-post --config /opt/mattermost-post/config.yaml --group alerts --file alert.md
```

세 메시지 입력 옵션 `--message`, `--file`, `--stdin` 중 정확히 하나만 사용할 수 있습니다. 입력 내용의 줄바꿈과 앞뒤 공백은 보존되며 UTF-8 BOM만 제거됩니다.

## 자동화용 JSON

`--json`은 성공과 실패 모두 stdout에 JSON 객체 하나를 출력합니다. 실패 시에도 종료 코드는 0이 아닙니다.

```text
mattermost-post --group daily-news --message "hello" --json
```

성공 예:

```json
{"ok":true,"group":"daily-news","team":"company","channel":"daily-news","post_id":"post-id","channel_id":"channel-id"}
```

실패 예:

```json
{"ok":false,"group":"daily-news","team":"company","channel":"daily-news","error_code":"unauthorized","message":"봇 토큰이 유효하지 않거나 만료되었습니다.","http_status":401}
```

종료 코드:

| 코드 | 의미 |
|---:|---|
| `0` | 게시 성공, 도움말 또는 버전 출력 성공 |
| `2` | 옵션, 입력 또는 설정 오류 |
| `3` | 네트워크 또는 Mattermost API 오류 |

## 로그

로그는 `mattermost-post-YYYY-MM-DD.log` 이름으로 하루 한 파일씩 append됩니다. 각 줄은 독립된 JSON 객체이며 다음 메타데이터만 포함합니다.

- 실행 시각, 그룹, 팀, 채널, Unicode 문자 수
- 성공 여부, HTTP 상태, Mattermost request ID, post ID, 오류 코드

봇 토큰과 게시글 본문은 로그에 기록하지 않습니다. 로그 기록 실패는 stderr에 경고하지만 게시 요청은 계속 진행합니다. 로그 자동 삭제나 보존 기간 관리는 v1 범위에 포함되지 않습니다.

## 빌드 및 테스트

Go 1.26 이상이 필요합니다.

```text
go test ./...
go vet ./...
go build -buildvcs=false -trimpath -o mattermost-post ./cmd/mattermost-post
```

Windows PowerShell에서 Linux amd64 바이너리를 교차 빌드하는 예:

```powershell
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -buildvcs=false -trimpath -o dist/mattermost-post-linux-amd64 ./cmd/mattermost-post
```

GitHub Actions는 각 push와 pull request에서 테스트와 `go vet`을 실행하고 다음 파일 및 SHA-256 체크섬을 아티팩트로 생성합니다.

- `mattermost-post-windows-amd64.exe`
- `mattermost-post-linux-amd64`

## 오프라인 배포

1. 온라인 환경의 GitHub Actions에서 대상 OS 바이너리와 `.sha256` 파일을 내려받습니다.
2. `sha256sum` 또는 `Get-FileHash -Algorithm SHA256`으로 파일을 검증합니다.
3. 바이너리와 실제 `config.yaml`을 오프라인 환경으로 복사합니다.
4. Linux에서는 `chmod +x mattermost-post-linux-amd64`를 실행합니다.
5. 오프라인 환경에서 접근 가능한 Mattermost URL을 설정하고 실행합니다.

Go 런타임이나 외부 패키지는 배포처에 설치할 필요가 없습니다. 단, 대상 호스트에서 Mattermost 서버로의 네트워크 연결과 신뢰할 수 있는 TLS 인증서가 필요합니다. 자체 서명 인증서를 무시하는 옵션은 제공하지 않습니다.

## 범위

v1은 파일 첨부, 메시지 자동 분할, 예약 게시, 여러 채널 동시 게시, 댓글 작성, 자동 재시도를 지원하지 않습니다.

## 라이선스

[MIT License](LICENSE)
