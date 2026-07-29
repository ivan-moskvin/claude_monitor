# Ставит claudestatus из последнего релиза. Дальше утилита обслуживает себя сама:
# обновляется claudestatus update, удаляется claudestatus uninstall.
#
#   irm https://raw.githubusercontent.com/ivan-moskvin/claude_monitor/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$repo = "ivan-moskvin/claude_monitor"
$binDir = if ($env:CLAUDESTATUS_BIN_DIR) { $env:CLAUDESTATUS_BIN_DIR } else { Join-Path $env:LOCALAPPDATA "claudestatus" }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "Неизвестная архитектура: $env:PROCESSOR_ARCHITECTURE — соберите из исходников." }
}

$asset = "claudestatus_windows_$arch.exe"
$base = "https://github.com/$repo/releases/latest/download"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    Write-Host "==> Скачиваю $asset"
    Invoke-WebRequest -Uri "$base/$asset" -OutFile "$tmp\claudestatus.exe"
    Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile "$tmp\checksums.txt"

    $want = (Get-Content "$tmp\checksums.txt" |
        Where-Object { $_ -match "\s\*?$([regex]::Escape($asset))$" } |
        ForEach-Object { ($_ -split '\s+')[0] })
    if (-not $want) { throw "В релизе нет контрольной суммы для $asset." }

    $got = (Get-FileHash "$tmp\claudestatus.exe" -Algorithm SHA256).Hash
    if ($got -ne $want.ToUpper()) { throw "Контрольная сумма не сошлась — загрузка побилась." }

    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
    Move-Item -Force "$tmp\claudestatus.exe" (Join-Path $binDir "claudestatus.exe")
    Write-Host "==> Поставлен в $binDir\claudestatus.exe"

    # Каталог в PATH пользователя — иначе команда claudestatus не найдётся.
    $current = [Environment]::GetEnvironmentVariable("Path", "User")
    $entries = @()
    if ($current) { $entries = $current -split ";" | Where-Object { $_ } }
    if ($entries -notcontains $binDir) {
        [Environment]::SetEnvironmentVariable("Path", (($entries + $binDir) -join ";"), "User")
        Write-Host "==> Каталог добавлен в PATH — заработает в новом терминале"
    }

    & (Join-Path $binDir "claudestatus.exe") install
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
