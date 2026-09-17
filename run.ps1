# Clash-SpeedTest launcher (Windows; also works on macOS if pwsh is installed)
# Runs from source with `go run`. No exe is built.
#
# Usage:
#   .\run.ps1
#   .\run.ps1 -c "https://example.com/subscribe?token=xxx&flag=meta"
#   .\run.ps1 -fast
#
# Double-click run.bat. Without -c, the program opens the airport menu:
# select / add / update / delete airports, then pick countries to test.

$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

function Find-Go {
    $cmd = Get-Command go -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }

    $homeGo = Join-Path $HOME "go\bin\go.exe"
    $homeGoUnix = Join-Path $HOME "go/bin/go"
    $candidates = @(
        (Join-Path $env:ProgramFiles "Go\bin\go.exe"),
        "C:\Go\bin\go.exe",
        (Join-Path $env:LOCALAPPDATA "Programs\Go\bin\go.exe"),
        $homeGo,
        "/usr/local/go/bin/go",
        "/opt/homebrew/bin/go",
        "/usr/local/bin/go",
        $homeGoUnix,
        (Join-Path $HOME ".local/go/bin/go")
    )
    foreach ($path in $candidates) {
        if ($path -and (Test-Path -LiteralPath $path)) {
            return $path
        }
    }
    return $null
}

$go = Find-Go
if (-not $go) {
    Write-Host "未找到 Go。本脚本用 go run 从源码启动，需要先安装 Go 1.24+。"
    if ($env:OS -eq "Windows_NT") {
        Write-Host "  Windows: winget install --id GoLang.Go -e"
    }
    else {
        Write-Host "  macOS:   brew install go"
        Write-Host "  其它:    https://go.dev/doc/install"
    }
    Write-Host "装完后重新打开终端再运行本脚本。"
    exit 1
}

if (-not $env:GOPROXY) {
    $env:GOPROXY = "https://goproxy.cn,direct"
}

Write-Host "从源码启动 clash-speedtest（go run，不生成 exe）..."
if ($null -ne $args -and $args.Count -gt 0) {
    & $go run . @args
}
else {
    & $go run .
}
exit $LASTEXITCODE
