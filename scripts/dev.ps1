param(
    [ValidateSet('sqlite', 'mysql')][string]$Database,
    [switch]$Stop,
    [switch]$Restart
)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$backendRoot = Join-Path $projectRoot 'backend'
$frontendRoot = Join-Path $projectRoot 'frontend'
$cacheRoot = Join-Path $backendRoot '.cache'
$stateFile = Join-Path $cacheRoot 'dev-processes.json'
New-Item -ItemType Directory -Path $cacheRoot -Force | Out-Null
$owned = @()
if (Test-Path -LiteralPath $stateFile) {
    $parsedState = Get-Content -LiteralPath $stateFile -Raw -Encoding UTF8 | ConvertFrom-Json
    $owned = @($parsedState | Where-Object { $_.id -and $_.started })
}
if ($Stop -or $Restart) {
    foreach ($entry in $owned) {
        $process = Get-Process -Id $entry.id -ErrorAction SilentlyContinue
        if ($process -and $process.StartTime.ToUniversalTime().Ticks.ToString() -eq $entry.started) {
            Stop-Process -Id $entry.id -ErrorAction Stop
            Write-Host "已停止本脚本启动的 $($entry.name)。"
        }
    }
    $owned = @()
    '[]' | Set-Content -LiteralPath $stateFile -Encoding utf8
    if ($Stop) { return }
}

# Load configuration without printing credentials. The database mode is explicit.
$envFile = Join-Path $projectRoot '.env'
if (Test-Path -LiteralPath $envFile) {
    foreach ($line in Get-Content -LiteralPath $envFile -Encoding UTF8) {
        if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$') {
            $value = $Matches[2].Trim().Trim('"').Trim("'")
            [Environment]::SetEnvironmentVariable($Matches[1], $value, 'Process')
        }
    }
}
if (-not $PSBoundParameters.ContainsKey('Database')) {
    $Database = if ($env:DATABASE_DRIVER) { $env:DATABASE_DRIVER } else { 'mysql' }
}
if ($Database -notin @('mysql', 'sqlite')) { throw 'DATABASE_DRIVER must be mysql or sqlite.' }
$goCommand = Get-Command go -ErrorAction SilentlyContinue
$goExe = if ($goCommand) { $goCommand.Source } else { Join-Path $env:LOCALAPPDATA 'Programs\Go\bin\go.exe' }
if (-not (Test-Path -LiteralPath $goExe)) { throw '未找到 Go。请安装 Go 并重新打开终端。' }
$nodeExe = (Get-Command node -ErrorAction Stop).Source
$viteEntry = Join-Path $frontendRoot 'node_modules\vite\bin\vite.js'
if (-not (Test-Path -LiteralPath $viteEntry)) { throw '请先执行 corepack pnpm --dir frontend install --frozen-lockfile。' }
$env:GOCACHE = Join-Path $cacheRoot 'go-build'
$env:DATABASE_DRIVER = $Database
$env:APP_PORT = '8080'
if ($Database -eq 'sqlite') { $env:SQLITE_PATH = Join-Path $cacheRoot 'mathwiki-dev.db' }

function Test-HTTP([string]$Address) {
    try { $result = Invoke-WebRequest -Uri $Address -TimeoutSec 2 -UseBasicParsing; return $result.StatusCode -eq 200 }
    catch { return $false }
}
function Register-Child($Process, [string]$Name) {
    $script:owned += [pscustomobject]@{ id=$Process.Id; name=$Name; started=$Process.StartTime.ToUniversalTime().Ticks.ToString() }
    ConvertTo-Json -InputObject @($script:owned) | Set-Content -LiteralPath $stateFile -Encoding utf8
}

if (-not (Test-HTTP 'http://127.0.0.1:8080/health')) {
    $serverExe = Join-Path $cacheRoot 'mathwiki-server.exe'
    Push-Location $backendRoot
    try {
        & $goExe build -o $serverExe ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw '后端编译失败。' }
    } finally { Pop-Location }
    $apiProcess = Start-Process -FilePath $serverExe -WorkingDirectory $backendRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $cacheRoot 'api.stdout.log') -RedirectStandardError (Join-Path $cacheRoot 'api.stderr.log')
    Register-Child $apiProcess 'Go API'
    $ready = $false
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        if (Test-HTTP 'http://127.0.0.1:8080/health') { $ready = $true; break }
        $apiProcess.Refresh()
        if ($apiProcess.HasExited) { break }
        Start-Sleep -Milliseconds 200
    }
    if (-not $ready) { throw "后端启动失败，请检查 $cacheRoot\api.stderr.log。MySQL 模式需要可用的 MYSQL_DSN。" }
} else {
    $runningHealth = Invoke-RestMethod -Uri 'http://127.0.0.1:8080/health' -TimeoutSec 2
    if ($runningHealth.databaseDriver -ne $Database) {
        throw 'Running API database differs from configuration or uses an old build. Stop the old project API, then restart this script.'
    }
    Write-Host '已检测到运行中的 API；如需更新本脚本启动的服务，请使用 -Restart。'
}

if (-not (Test-HTTP 'http://127.0.0.1:5173')) {
    $webProcess = Start-Process -FilePath $nodeExe -ArgumentList @(('"' + $viteEntry + '"'), '--host', '127.0.0.1', '--port', '5173', '--strictPort') -WorkingDirectory $frontendRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $cacheRoot 'web.stdout.log') -RedirectStandardError (Join-Path $cacheRoot 'web.stderr.log')
    Register-Child $webProcess 'Vue 前端'
    $ready = $false
    for ($attempt = 0; $attempt -lt 20; $attempt++) {
        if (Test-HTTP 'http://127.0.0.1:5173') { $ready = $true; break }
        Start-Sleep -Milliseconds 200
    }
    if (-not $ready) { throw "前端启动失败，请检查 $cacheRoot\web.stderr.log。" }
}
Write-Host "数据库模式：$Database（sqlite 为本地开发库，mysql 为正式配置）"
Write-Host '前端：http://localhost:5173/'
Write-Host '后端：http://localhost:8080/health'
Write-Host '服务在后台运行。重启：.\scripts\dev.ps1 -Restart；停止：.\scripts\dev.ps1 -Stop'
