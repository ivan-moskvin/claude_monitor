# Собирает виджет из исходников и перезапускает его.
# Приложение работает прямо из репозитория: никакой установки в систему нет.

$ErrorActionPreference = "Stop"

Set-Location $PSScriptRoot
$root = $PSScriptRoot
$target = Join-Path $root "app\src-tauri\target\release"

$missing = @()
function Have($name) { return [bool](Get-Command $name -ErrorAction SilentlyContinue) }
if (-not (Have go)) { $missing += "go     - winget install GoLang.Go" }
if (-not (Have node)) { $missing += "node   - winget install OpenJS.NodeJS" }
if (-not (Have cargo)) { $missing += "rust   - winget install Rustlang.Rustup" }

if ($missing.Count -gt 0) {
    Write-Host "Не хватает инструментов:"
    $missing | ForEach-Object { Write-Host "  $_" }
    exit 1
}

Write-Host "==> Writer"
# Кладём рядом с бинарём: вне бандла Tauri ищет ресурсы в каталоге исполняемого файла.
New-Item -ItemType Directory -Force -Path (Join-Path $target "resources") | Out-Null

Push-Location (Join-Path $root "statusline")
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o (Join-Path $target "resources\claude-statusline.exe") .
Pop-Location

Write-Host "==> Виджет"
if (-not (Test-Path (Join-Path $root "app\node_modules"))) {
    Push-Location (Join-Path $root "app")
    npm ci
    Pop-Location
}
Push-Location (Join-Path $root "app")
npx tauri build --no-bundle
Pop-Location

$binary = Join-Path $target "claude-usage-bar.exe"
if (-not (Test-Path $binary)) { throw "Бинарь не собрался: $binary" }

Write-Host "==> Запуск"
Get-Process claude-usage-bar -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Milliseconds 300
Start-Process $binary

Write-Host ""
Write-Host "Виджет в области уведомлений. Автозапуск и строку статуса включите в попапе."
Write-Host "Остановить: Stop-Process -Name claude-usage-bar"
