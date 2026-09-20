param(
    [Parameter(Mandatory=$true)][string]$Username,
    [ValidateSet('admin', 'learner')][string]$Role = 'admin'
)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $projectRoot '.env'
if (Test-Path -LiteralPath $envFile) {
    foreach ($line in Get-Content -LiteralPath $envFile -Encoding UTF8) {
        if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$') {
            [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2].Trim().Trim('"').Trim("'"), 'Process')
        }
    }
}
$goCommand = Get-Command go -ErrorAction SilentlyContinue
$goExe = if ($goCommand) { $goCommand.Source } else { Join-Path $env:LOCALAPPDATA 'Programs\Go\bin\go.exe' }
if (-not (Test-Path -LiteralPath $goExe)) { throw 'Go is not installed.' }
$backendRoot = Join-Path $projectRoot 'backend'
$env:GOCACHE = Join-Path $backendRoot '.cache\go-build'
if ($env:DATABASE_DRIVER -eq 'sqlite' -and -not $env:SQLITE_PATH) { $env:SQLITE_PATH = Join-Path $backendRoot '.cache\mathwiki-dev.db' }
Push-Location -LiteralPath $backendRoot
try {
    & $goExe run ./cmd/admin -username $Username -role $Role
    if ($LASTEXITCODE -ne 0) { throw 'Role update failed.' }
} finally { Pop-Location }
