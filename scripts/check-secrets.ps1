param([switch]$WorkingTree)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Push-Location $projectRoot
try {
    # Read candidate content only. Never print a matched value or source line.
    $files = if ($WorkingTree) { @(git -c core.quotepath=false ls-files --cached --others --exclude-standard) } else { @(git -c core.quotepath=false diff --cached --name-only --diff-filter=ACMR) }
    if ($LASTEXITCODE -ne 0) { throw 'Cannot list Git candidates.' }
    $knownSecrets = @()
    foreach ($envPath in @('.env', '.env.local', 'frontend/.env', 'frontend/.env.local')) {
        if (-not (Test-Path -LiteralPath $envPath)) { continue }
        foreach ($line in Get-Content -LiteralPath $envPath -Encoding UTF8) {
            if ($line -notmatch '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$') { continue }
            $key = $Matches[1]
            $value = $Matches[2].Trim().Trim('"').Trim("'")
            if ($key -match 'SECRET|PASSWORD|TOKEN|API_KEY|PRIVATE_KEY|ACCESS_KEY' -and $value.Length -ge 6) { $knownSecrets += $value }
            if ($key -eq 'MYSQL_DSN' -and $value -match '^[^:]+:(.*?)@tcp\(' -and $Matches[1].Length -ge 6) { $knownSecrets += $Matches[1] }
        }
    }
    $patterns = @{
        'private-key' = '-----BEGIN (?:RSA |EC |OPENSSH |DSA |ENCRYPTED )?PRIVATE KEY-----'
        'github-token' = '(?:gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,})'
        'tencent-secret-id' = 'AKID[A-Za-z0-9]{28,}'
        'model-api-key' = '\bsk-(?:proj-)?[A-Za-z0-9_-]{24,}'
        'aws-access-key' = '\b(?:AKIA|ASIA)[A-Z0-9]{16}\b'
    }
    $issues = @()
    foreach ($file in ($files | Sort-Object -Unique)) {
        $name = [System.IO.Path]::GetFileName($file)
        if (($name -match '^\.env(?:\..+)?$' -and $name -ne '.env.example') -or $file -match '(?:^|/)(?:\.cache|node_modules|\.venv|backups|uploads)/|\.(?:sql|db|sqlite3?|pem|key|p12|pfx)(?:$|[.-])') {
            $issues += "$file : forbidden file type"; continue
        }
        $content = if ($WorkingTree) { Get-Content -LiteralPath $file -Raw -Encoding UTF8 } else { (git show ":$file") -join "`n" }
        if ($null -eq $content) { continue }
        foreach ($secret in ($knownSecrets | Sort-Object -Unique)) {
            if ($content.Contains($secret)) { $issues += "$file : matches a local configured secret"; break }
        }
        foreach ($label in $patterns.Keys) { if ($content -match $patterns[$label]) { $issues += "$file : $label pattern" } }
    }
    if ($issues.Count -gt 0) { $issues | ForEach-Object { Write-Output $_ }; throw 'Secret scan failed. Values were redacted. Do not commit or push.' }
    Write-Output ("Secret scan passed: {0} files; checked local configured secrets, token patterns and forbidden file types." -f $files.Count)
} finally { Pop-Location }
