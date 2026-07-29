# Собирает строку статуса из исходников и прописывает её в Claude Code.
# Бинарь живёт прямо в репозитории: никакой установки в систему нет.

param([switch]$Yes)

$ErrorActionPreference = "Stop"

Set-Location $PSScriptRoot
$root = $PSScriptRoot
$binary = Join-Path $root "bin\claude-statusline.exe"

function Have($name) { return [bool](Get-Command $name -ErrorAction SilentlyContinue) }

# winget правит PATH только для новых процессов — подхватываем изменения на месте.
function Sync-Path {
    $machine = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $user = [Environment]::GetEnvironmentVariable("Path", "User")
    $env:Path = ($machine, $user -join ";")
}

# Go ставим сами: руками остаётся только winget, если его нет.
function Ensure-Go {
    if (Have go) { return }

    Write-Host "Не хватает: go"

    if (-not (Have winget)) {
        Write-Host "Сначала нужен winget — поставьте App Installer из Microsoft Store."
        exit 1
    }

    if (-not $Yes) {
        $answer = Read-Host "Поставить через winget? [Y/n]"
        if ($answer -and $answer -notmatch '^(y|yes|да)$') {
            Write-Host "Отменено."
            exit 1
        }
    }

    Write-Host "==> Устанавливаю go"
    winget install -e --id GoLang.Go --accept-source-agreements --accept-package-agreements

    Sync-Path
    if (-not (Have go)) {
        Write-Host "go не нашёлся и после установки — откройте новый терминал и повторите."
        exit 1
    }
}

Ensure-Go

Write-Host "==> Сборка"
New-Item -ItemType Directory -Force -Path (Join-Path $root "bin") | Out-Null

Push-Location (Join-Path $root "statusline")
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o $binary .
Pop-Location

Write-Host "==> Установка"
& $binary --install

Write-Host ""
Write-Host "Готово. Строка статуса появится в следующей сессии Claude Code."
