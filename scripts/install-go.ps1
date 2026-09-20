param(
    [string]$Archive = (Join-Path $env:LOCALAPPDATA 'Temp\go1.27.1.windows-amd64.zip'),
    [string]$ExtractedGo = (Join-Path $env:LOCALAPPDATA 'Temp\mathwiki-go-1.27.1\go'),
    [string]$InstallRoot = (Join-Path $env:LOCALAPPDATA 'Programs\Go')
)
$ErrorActionPreference = 'Stop'
$expectedHash = 'a3911b5e0e1b1053f25ed0675f4c1c6aad1e2bfcf253df2b9be4caabd2edd95d'
if ((Get-FileHash -LiteralPath $Archive -Algorithm SHA256).Hash -ne $expectedHash) {
    throw 'Go 官方安装包 SHA256 校验失败，安装已停止。'
}
if (-not (Test-Path -LiteralPath (Join-Path $ExtractedGo 'bin\go.exe'))) {
    throw '未找到已解压的 Go 工具链。'
}
if (Test-Path -LiteralPath $InstallRoot) {
    throw "目标目录已存在，为保护现有安装未覆盖：$InstallRoot"
}
New-Item -ItemType Directory -Path (Split-Path -Parent $InstallRoot) -Force | Out-Null
Copy-Item -LiteralPath $ExtractedGo -Destination $InstallRoot -Recurse
$goBin = Join-Path $InstallRoot 'bin'
& (Join-Path $goBin 'go.exe') version
if ($LASTEXITCODE -ne 0) { throw 'Go 安装验证失败。' }
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$entries = @($userPath -split ';' | Where-Object { $_ })
if ($entries -notcontains $goBin) {
    [Environment]::SetEnvironmentVariable('Path', (($entries + $goBin) -join ';'), 'User')
}
$env:Path = $goBin + ';' + $env:Path
Write-Host "Go 已安装到 $InstallRoot，并加入用户 PATH。"
Write-Host '请重新打开终端；若终端由旧进程启动，请重新启动对应编辑器。'
go version
