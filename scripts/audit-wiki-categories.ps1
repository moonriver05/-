param(
  [string[]]$Roots = @('Category:数学', 'Category:算法'),
  [int]$MaxDepth = 2
)

$ErrorActionPreference = 'Stop'
$headers = @{ 'User-Agent' = 'MathWikiLabResearch/0.1 (student feasibility study)' }
$api = 'https://zh.wikipedia.org/w/api.php'
$seenCategories = [System.Collections.Generic.HashSet[string]]::new()
$seenPages = [System.Collections.Generic.HashSet[int]]::new()
$frontier = [System.Collections.Generic.List[object]]::new()
$depthStats = @{}

foreach ($root in $Roots) {
  $frontier.Add([pscustomobject]@{ title = $root; depth = 0 })
  [void]$seenCategories.Add($root)
}

function Get-CategoryMembers([string]$title) {
  $members = [System.Collections.Generic.List[object]]::new()
  $continueToken = $null
  do {
    $query = @{
      action = 'query'; format = 'json'; formatversion = 2
      list = 'categorymembers'; cmtitle = $title
      cmtype = 'page|subcat'; cmlimit = '500'
    }
    if ($continueToken) { $query.cmcontinue = $continueToken }
    $qs = ($query.GetEnumerator() | ForEach-Object {
      [uri]::EscapeDataString($_.Key) + '=' + [uri]::EscapeDataString([string]$_.Value)
    }) -join '&'
    $attempt = 0
    do {
      try {
        $response = Invoke-RestMethod -Uri "$api`?$qs" -Headers $headers
        $ok = $true
      } catch {
        $attempt++
        if ($attempt -ge 5) { throw }
        Start-Sleep -Milliseconds (500 * $attempt)
        $ok = $false
      }
    } until ($ok)
    Start-Sleep -Milliseconds 180
    foreach ($member in $response.query.categorymembers) { $members.Add($member) }
    $continueToken = $response.continue.cmcontinue
  } while ($continueToken)
  return $members
}

$cursor = 0
while ($cursor -lt $frontier.Count) {
  $current = $frontier[$cursor++]
  if (-not $depthStats.ContainsKey($current.depth)) {
    $depthStats[$current.depth] = [ordered]@{ categoriesVisited = 0; newArticles = 0 }
  }
  $depthStats[$current.depth].categoriesVisited++
  $members = Get-CategoryMembers $current.title
  foreach ($member in $members) {
    if ($member.ns -eq 0) {
      if ($seenPages.Add([int]$member.pageid)) { $depthStats[$current.depth].newArticles++ }
    } elseif ($member.ns -eq 14 -and $current.depth -lt $MaxDepth) {
      if ($seenCategories.Add([string]$member.title)) {
        $frontier.Add([pscustomobject]@{ title = [string]$member.title; depth = $current.depth + 1 })
      }
    }
  }
}

[pscustomobject]@{
  roots = $Roots
  maxDepth = $MaxDepth
  uniqueCategories = $seenCategories.Count
  uniqueArticles = $seenPages.Count
  perDepth = $depthStats
  measuredAt = (Get-Date).ToString('o')
} | ConvertTo-Json -Depth 8
