param(
  [string]$DataRoot = $(if ($env:MATHWIKI_DATA_ROOT) { $env:MATHWIKI_DATA_ROOT } else { 'E:\MathWikiData' })
)

$ErrorActionPreference = 'Stop'
$edition = 'zhwiki'
$dumpDir = Join-Path $DataRoot 'dumps'
New-Item -ItemType Directory -Force -Path $dumpDir | Out-Null
$files = @(
  "$edition-latest-pages-articles-multistream.xml.bz2",
  "$edition-latest-pages-articles-multistream-index.txt.bz2"
)

$checksumFile = Join-Path $dumpDir "$edition-latest-md5sums.txt"
if (-not (Test-Path $checksumFile)) {
  & curl.exe --fail --location --output $checksumFile "https://dumps.wikimedia.org/$edition/latest/$edition-latest-md5sums.txt"
}

function Get-ExpectedHash([string]$file) {
  $dumpSuffix = $file.Replace("$edition-latest-", '')
  $line = Get-Content $checksumFile | Where-Object { $_ -match ("-" + [regex]::Escape($dumpSuffix) + '$') } | Select-Object -First 1
  if (-not $line) { throw "校验清单中找不到: $file" }
  return ($line -split '\s+')[0].ToLowerInvariant()
}

foreach ($file in $files) {
  $target = Join-Path $dumpDir $file
  $url = "https://dumps.wikimedia.org/$edition/latest/$file"
  $expected = Get-ExpectedHash $file
  if (Test-Path $target) {
    $existingHash = (Get-FileHash -Algorithm MD5 $target).Hash.ToLowerInvariant()
    if ($existingHash -eq $expected) { Write-Host "文件已完整，跳过: $target"; continue }
    Write-Host "从现有文件断点续传: $target"
  } else { Write-Host "下载中文维基正文组件: $url" }
  $attempt = 0
  do {
    $attempt++
    & curl.exe --fail --location --continue-at - --output $target $url
    if ($LASTEXITCODE -ne 0) {
      if ($attempt -ge 20) { throw "中文维基正文组件连续下载失败: $file" }
      Write-Host "连接中断，第 $attempt 次断点重连…"
      Start-Sleep -Seconds ([math]::Min(10, $attempt * 2))
    }
  } while ($LASTEXITCODE -ne 0)
}
foreach ($file in $files) {
  $target = Join-Path $dumpDir $file
  $expected = Get-ExpectedHash $file
  $actual = (Get-FileHash -Algorithm MD5 $target).Hash.ToLowerInvariant()
  if ($actual -ne $expected) { throw "$file MD5 不一致" }
  Write-Host "中文维基正文组件校验通过: $target"
}
