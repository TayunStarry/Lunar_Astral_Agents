# Git Large File Cleanup Script
# Find and remove files > 10MB from git history

$ErrorActionPreference = "Stop"

$THRESHOLD = 10 * 1024 * 1024

Write-Host "`n=== Git Large File Cleanup ===" -ForegroundColor Cyan

if (-not (Test-Path ".git")) {
    Write-Host "Error: Not a git repository" -ForegroundColor Red
    exit 1
}

Write-Host "`n[1/4] Scanning git history for large files..." -ForegroundColor Yellow

$largeFiles = @()
$objects = git rev-list --objects --all 2>$null

foreach ($obj in $objects) {
    $parts = $obj -split ' ', 2
    if ($parts.Count -lt 2) { continue }
    
    $hash = $parts[0]
    $filename = $parts[1]
    
    if ([string]::IsNullOrWhiteSpace($filename)) { continue }
    
    try {
        $size = [long](git cat-file -s $hash 2>$null)
        if ($size -gt $THRESHOLD) {
            $largeFiles += [PSCustomObject]@{
                Hash = $hash
                Path = $filename
                SizeMB = [math]::Round($size / 1MB, 2)
            }
        }
    }
    catch { continue }
}

if ($largeFiles.Count -eq 0) {
    Write-Host "No files > 10MB found." -ForegroundColor Green
    exit 0
}

$largeFiles = $largeFiles | Sort-Object -Property SizeMB -Descending
$uniquePaths = $largeFiles | Select-Object -ExpandProperty Path -Unique

Write-Host "`nFound $($largeFiles.Count) large files:" -ForegroundColor Red
Write-Host "----------------------------------------" -ForegroundColor Gray

$totalMB = 0
$idx = 1
foreach ($f in $largeFiles) {
    Write-Host ("  {0:D2}. [{1,8:F2} MB] {2}" -f $idx, $f.SizeMB, $f.Path) -ForegroundColor White
    $totalMB += $f.SizeMB
    $idx++
}

Write-Host "----------------------------------------" -ForegroundColor Gray
Write-Host ("Total: {0:F2} MB ({1} files, {2} unique paths)" -f $totalMB, $largeFiles.Count, $uniquePaths.Count) -ForegroundColor Yellow

Write-Host "`n[2/4] Confirm cleanup" -ForegroundColor Yellow
Write-Host "This will remove large files from git history but KEEP local files." -ForegroundColor Yellow
Write-Host "After cleanup, you need to force push to remote." -ForegroundColor Red

$response = Read-Host "`nType 'yes' to continue"
if ($response -ne "yes") {
    Write-Host "Cancelled." -ForegroundColor Yellow
    exit 0
}

Write-Host "`n[3/4] Cleaning git history..." -ForegroundColor Yellow

foreach ($filePath in $uniquePaths) {
    Write-Host "  Cleaning: $filePath" -ForegroundColor White
    $filterCmd = "git rm --cached --ignore-unmatch '$filePath'"
    git filter-branch --force --index-filter $filterCmd --prune-empty -- --all 2>$null
}

Write-Host "`n  Cleaning stash refs..." -ForegroundColor Gray
git for-each-ref --format "git update-ref -d %(refname)" refs/original | Invoke-Expression 2>$null
git for-each-ref --format "git update-ref -d %(refname)" refs/stash | Invoke-Expression 2>$null

Write-Host "`n[4/4] Running garbage collection..." -ForegroundColor Yellow
git reflog expire --expire=now --all 2>$null
git gc --prune=now --aggressive 2>$null

Write-Host "`n=== Cleanup Complete ===" -ForegroundColor Cyan
Write-Host "Files cleaned: $($uniquePaths.Count)" -ForegroundColor White
Write-Host "Estimated space saved: $totalMB MB" -ForegroundColor White
Write-Host "`nNext steps:" -ForegroundColor Yellow
Write-Host "  1. Check result: git rev-list --objects HEAD | git cat-file --batch-check" -ForegroundColor White
Write-Host "  2. Force push: git push --force origin main" -ForegroundColor White
