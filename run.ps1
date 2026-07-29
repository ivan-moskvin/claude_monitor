# Собирает claudestatus из исходников и прописывает строку статуса в Claude Code.
# Бинарь живёт прямо в репозитории, наружу торчит только запись bin\ в PATH.

param([switch]$Yes)

$ErrorActionPreference = "Stop"

Set-Location $PSScriptRoot
$root = $PSScriptRoot
$binDir = Join-Path $root "bin"
$binary = Join-Path $binDir "claudestatus.exe"

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

# Симлинков без прав администратора в Windows нет, поэтому команда становится
# доступной через PATH пользователя — сам каталог bin\ остаётся в клоне.
function Add-ToUserPath($dir) {
    $current = [Environment]::GetEnvironmentVariable("Path", "User")
    $entries = @()
    if ($current) { $entries = $current -split ";" | Where-Object { $_ } }
    if ($entries -contains $dir) { return $false }

    [Environment]::SetEnvironmentVariable("Path", (($entries + $dir) -join ";"), "User")
    return $true
}

Ensure-Go

# Версию вшиваем в бинарь: по ней claudestatus check понимает, отстал ли клон.
$version = (git -C $root describe --tags --always --dirty 2>$null)
if (-not $version) { $version = "dev" }

Write-Host "==> Сборка ($version)"
New-Item -ItemType Directory -Force -Path $binDir | Out-Null

Push-Location (Join-Path $root "claudestatus")
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w -X main.version=$version" -o $binary .
Pop-Location

# Бинарь до переименования утилиты — иначе в bin\ остаётся мёртвый файл.
Remove-Item -Force -ErrorAction SilentlyContinue (Join-Path $binDir "claude-statusline.exe")

Write-Host "==> Путь"
$added = Add-ToUserPath $binDir

Write-Host "==> Установка"
& $binary install

Write-Host ""
Write-Host "Готово. Строка статуса появится в следующей сессии Claude Code."
if ($added) {
    Write-Host "Каталог bin добавлен в PATH — команда claudestatus заработает в новом терминале."
} else {
    Write-Host "Команда: claudestatus help"
}
