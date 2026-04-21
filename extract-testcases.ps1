$yamlPath = "c:\Users\nperiannan\OneDrive - Extreme Networks, Inc\Desktop\Testcase generator\generated-tests\dns-server.yaml"
$outputPath = "c:\Users\nperiannan\OneDrive - Extreme Networks, Inc\Desktop\Testcase generator\generated-tests\dns-server-testplan.csv"

Write-Host "Reading YAML file..."
$content = Get-Content $yamlPath -Raw
$lines = $content -split "`n"

$testCases = @()
$currentTestCase = $null
$inTestCase = $false

Write-Host "Parsing test cases..."
foreach ($line in $lines) {
    if ($line -match '^\s+- testCaseID:\s*(.+)$') {
        if ($currentTestCase) {
            $testCases += $currentTestCase
        }
        $currentTestCase = @{
            TestCaseID = $matches[1].Trim()
            FeatureName = ''
            Description = ''
            IsDeploymentTest = ''
        }
        $inTestCase = $true
    }
    elseif ($inTestCase -and $line -match '^\s+featureName:\s*(.+)$') {
        $currentTestCase.FeatureName = $matches[1].Trim()
    }
    elseif ($inTestCase -and $line -match '^\s+description:\s*(.+)$') {
        # Only capture the main test case description, not step descriptions
        if ($currentTestCase.Description -eq '') {
            $desc = $matches[1].Trim()
            # Remove quotes if present
            $desc = $desc -replace '^[''"]|[''"]$', ''
            $currentTestCase.Description = $desc
        }
    }
    elseif ($inTestCase -and $line -match '^\s+isDeploymentTest:\s*(.+)$') {
        $currentTestCase.IsDeploymentTest = $matches[1].Trim()
    }
    elseif ($line -match '^\s+steps:') {
        # We've entered the steps section, stop looking for test case metadata
        $inTestCase = $false
    }
}

# Add the last test case
if ($currentTestCase) {
    $testCases += $currentTestCase
}

Write-Host "Creating CSV content..."
# Create CSV content
$csvContent = "TestCaseID,FeatureName,Description,IsDeploymentTest`n"
foreach ($tc in $testCases) {
    $desc = $tc.Description -replace '"', '""'  # Escape quotes for CSV
    $csvContent += "`"$($tc.TestCaseID)`",`"$($tc.FeatureName)`",`"$desc`",`"$($tc.IsDeploymentTest)`"`n"
}

# Write to file
Write-Host "Writing to CSV file..."
Set-Content -Path $outputPath -Value $csvContent -Encoding UTF8

Write-Host "`nCSV file created successfully at: $outputPath"
Write-Host "Total test cases extracted: $($testCases.Count)"
Write-Host "`nOpening the file..."
Invoke-Item $outputPath
