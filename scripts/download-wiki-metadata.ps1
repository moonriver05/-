param(
  [string]$DataRoot = $(if ($env:MATHWIKI_DATA_ROOT) { $env:MATHWIKI_DATA_ROOT } else { 'E:\MathWikiData' })
)

$ErrorActionPreference = 'Stop'
$edition = 'zhwiki'
$dumpDir = Join-Path $DataRoot 'dumps'
New-Item -ItemType Directory -Force -Path $dumpDir | Out-Null
$base = "https://dumps.wikimedia.org/$edition/latest"
$files = @(
  "$edition-latest-page.sql.gz",
  "$edition-latest-linktarget.sql.gz",
  "$edition-latest-categorylinks.sql.gz",
  "$edition-latest-redirect.sql.gz",
  "$edition-latest-md5sums.txt"
)

foreach ($file in $files) {
  $target = Join-Path $dumpDir $file
  if (Test-Path $target) {
    Write-Host "已存在，跳过: $target"
    continue
  }
  Write-Host "下载中文维基元数据: $file"
  & curl.exe --fail --location --continue-at - --output $target "$base/$file"
  if ($LASTEXITCODE -ne 0) { throw "下载失败: $file" }
}

$checksums = Get-Content (Join-Path $dumpDir "$edition-latest-md5sums.txt")
foreach ($file in $files | Where-Object { $_ -ne "$edition-latest-md5sums.txt" }) {
  $dumpSuffix = $file.Replace("$edition-latest-", '')
  $expectedLine = $checksums | Where-Object { $_ -match ("-" + [regex]::Escape($dumpSuffix) + '$') } | Select-Object -First 1
  if (-not $expectedLine) { throw "校验文件中找不到 $file" }
  $expected = ($expectedLine -split '\s+')[0].ToLowerInvariant()
  $actual = (Get-FileHash -Algorithm MD5 (Join-Path $dumpDir $file)).Hash.ToLowerInvariant()
  if ($actual -ne $expected) { throw "$file MD5 不一致" }
  Write-Host "校验通过: $file"
}

Write-Host "中文维基元数据已准备在 $dumpDir"
