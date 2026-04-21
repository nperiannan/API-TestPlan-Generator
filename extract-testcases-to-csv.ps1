# Script to extract test case information from YAML file to CSV
param(
    [string]$InputFile = "generated-tests\dhcp-server.yaml",
    [string]$OutputFile = "generated-tests\dhcp-server-testplan.csv"
)

# Function to parse YAML-like content and extract test cases
function Extract-TestCases {
    param([string]$FilePath)
    
    $testCases = @()
    $content = Get-Content $FilePath -Raw
    
    # Use regex to find all test case blocks
    $pattern = '(?ms)- testCaseID:\s*(\S+)\s+featureName:\s*([^\r\n]+)\s+.*?type:\s*(\S+)\s+description:\s*([^\r\n]+)\s+.*?isDeploymentTest:\s*(\S+)'
    
    $matches = [regex]::Matches($content, $pattern)
    
    foreach ($match in $matches) {
        $testCase = [PSCustomObject]@{
            testCaseID = $match.Groups[1].Value.Trim()
            featureName = $match.Groups[2].Value.Trim()
            type = $match.Groups[3].Value.Trim()
            description = $match.Groups[4].Value.Trim()
            isDeploymentTest = $match.Groups[5].Value.Trim()
        }
        $testCases += $testCase
    }
    
    return $testCases
}

# Main execution
Write-Host "Extracting test cases from $InputFile..."

$testCases = Extract-TestCases -FilePath $InputFile

if ($testCases.Count -eq 0) {
    Write-Host "No test cases found!" -ForegroundColor Red
    exit 1
}

Write-Host "Found $($testCases.Count) test cases"

# Export to CSV
$testCases | Export-Csv -Path $OutputFile -NoTypeInformation -Encoding UTF8

Write-Host "Successfully exported to $OutputFile" -ForegroundColor Green
Write-Host "Preview of first few rows:"
$testCases | Select-Object -First 5 | Format-Table -AutoSize
