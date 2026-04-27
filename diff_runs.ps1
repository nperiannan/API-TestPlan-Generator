$r1 = (.\run.ps1 2>&1 | Select-String "^Generated \d+ tests for feature") | ForEach-Object {
    $line = $_.ToString()
    $parts = $line -split "feature "
    if ($parts.Count -gt 1) { ($parts[1] -split ":")[0].Trim() }
} | Sort-Object

$r2 = (.\run.ps1 2>&1 | Select-String "^Generated \d+ tests for feature") | ForEach-Object {
    $line = $_.ToString()
    $parts = $line -split "feature "
    if ($parts.Count -gt 1) { ($parts[1] -split ":")[0].Trim() }
} | Sort-Object

Write-Host "Run 1: $($r1.Count) features"
Write-Host "Run 2: $($r2.Count) features"
Write-Host ""
Write-Host "In run1 but not run2:"
Compare-Object $r1 $r2 | Where-Object { $_.SideIndicator -eq "<=" } | ForEach-Object { "  " + $_.InputObject }
Write-Host "In run2 but not run1:"
Compare-Object $r1 $r2 | Where-Object { $_.SideIndicator -eq "=>" } | ForEach-Object { "  " + $_.InputObject }
