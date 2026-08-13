param(
    [Parameter(Position = 0)]
    [string]$RunDate = ""
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($RunDate)) {
    $RunDate = Get-Date -Format "yyyyMMdd"
}

if ($RunDate -notmatch "^\d{8}$") {
    [Console]::Error.WriteLine("[오류] 날짜는 yyyyMMdd 형식이어야 합니다: $RunDate")
    exit 2
}

$baseDirectory = Split-Path -Parent $MyInvocation.MyCommand.Path
$appPath = Join-Path $baseDirectory "dist\mattermost-post-windows-amd64.exe"
$configPath = Join-Path $baseDirectory "config.yaml"
$menuDirectory = "G:\내 드라이브\01.공유\구내식당"
$menuFile = Join-Path $menuDirectory "구내식당_메뉴_$RunDate.gdoc"

try {
    if (-not (Test-Path -LiteralPath $appPath -PathType Leaf)) {
        throw "실행 파일을 찾을 수 없습니다: $appPath"
    }

    if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
        throw "설정 파일을 찾을 수 없습니다: $configPath"
    }

    if (-not (Test-Path -LiteralPath $menuFile -PathType Leaf)) {
        throw "메뉴 파일을 찾을 수 없습니다: $menuFile"
    }
}
catch {
    [Console]::Error.WriteLine("[오류] $($_.Exception.Message)")
    exit 2
}

Write-Output "[정보] 게시 파일: $menuFile"
& $appPath --config $configPath --group lunch --file $menuFile
$exitCode = $LASTEXITCODE

if ($exitCode -eq 0) {
    Write-Output "[정보] Mattermost 게시를 완료했습니다."
}
else {
    [Console]::Error.WriteLine("[오류] Mattermost 게시에 실패했습니다. 종료 코드: $exitCode")
}

exit $exitCode
