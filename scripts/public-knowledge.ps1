param(
    [ValidateSet('verify', 'export', 'import')][string]$Mode = 'verify',
    [string]$Directory = 'datasets/public-knowledge'
)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$previousDSN = [Environment]::GetEnvironmentVariable('MYSQL_DSN', 'Process')
Push-Location $projectRoot
try {
    if ($Mode -ne 'verify' -and (Test-Path -LiteralPath '.env')) {
        $line = Get-Content -LiteralPath '.env' -Encoding UTF8 | Where-Object { $_ -match '^\s*MYSQL_DSN=' } | Select-Object -First 1
        if ($line) { $env:MYSQL_DSN = ($line -replace '^\s*MYSQL_DSN=', '').Trim().Trim('"').Trim("'") }
    }
    if ($Mode -ne 'verify' -and -not $env:MYSQL_DSN) { throw 'Configure MYSQL_DSN locally before using the database.' }
    $datasetPath = [System.IO.Path]::GetFullPath((Join-Path $projectRoot $Directory))
    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    $goPath = if ($goCommand) { $goCommand.Source } else { Join-Path $env:LOCALAPPDATA 'Programs\Go\bin\go.exe' }
    if (-not (Test-Path -LiteralPath $goPath)) { throw 'Go is not installed or is not on PATH.' }
    & $goPath -C backend run ./cmd/knowledge -mode $Mode -dir $datasetPath
    if ($LASTEXITCODE -ne 0) { throw 'Public knowledge operation failed. No credentials are printed.' }
} finally {
    [Environment]::SetEnvironmentVariable('MYSQL_DSN', $previousDSN, 'Process')
    Pop-Location
}
