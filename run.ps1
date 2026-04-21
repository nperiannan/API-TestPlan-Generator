# Quick start script for testgen with predefined paths

Write-Host "=======================================" -ForegroundColor Cyan
Write-Host "Running testgen with predefined paths" -ForegroundColor Cyan
Write-Host "=======================================" -ForegroundColor Cyan
Write-Host ""

# Paths from SOURCE_FILES.md
$yangDir = "C:\Natarajan\automation\PlatformCommonModels\ConfigState\etc\yang"
$restSpec = "./qaopenapi.yaml"
$nosapiSpec = "C:\Users\nperiannan\Downloads\nos-openapi.yaml"
$outDir = "./generated-tests"

# Check if executable exists
if (-not (Test-Path "testgen.exe")) {
    Write-Host "testgen.exe not found. Building..." -ForegroundColor Yellow
    .\build.ps1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Build failed" -ForegroundColor Red
        exit 1
    }
}

# Check if source files exist
Write-Host "Checking source files..." -ForegroundColor Green

if (-not (Test-Path $yangDir)) {
    Write-Host "Warning: YANG directory not found: $yangDir" -ForegroundColor Yellow
    Write-Host "Please update the path in run.ps1 or SOURCE_FILES.md" -ForegroundColor Yellow
}

if (-not (Test-Path $restSpec)) {
    Write-Host "Warning: REST API spec not found: $restSpec" -ForegroundColor Yellow
    Write-Host "Please update the path in run.ps1 or SOURCE_FILES.md" -ForegroundColor Yellow
}

if (-not (Test-Path $nosapiSpec)) {
    Write-Host "Warning: NOSAPI spec not found: $nosapiSpec" -ForegroundColor Yellow
    Write-Host "Please update the path in run.ps1 or SOURCE_FILES.md" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Running testgen..." -ForegroundColor Green
Write-Host ""

# Run the generator
.\testgen.exe `
    --yang-dir "$yangDir" `
    --rest-spec "$restSpec" `
    --nosapi-spec "$nosapiSpec" `
    --out-dir "$outDir" `
    --features "" `
    --feature-categories "global-profile,wired-blueprint,service-profile" `
    --include-categories "functional,boundary,negative,scale,performance" `
    --scope-types "site-group,device" `
    --target-types "site-group,device" `
    --deployment-methods "rolling,immediate" `
    --scale-factor 100 `
    --performance-iterations 10 `
    --one-file-per-feature true

if ($LASTEXITCODE -eq 0) {
    Write-Host ""
    Write-Host "=======================================" -ForegroundColor Cyan
    Write-Host "Test generation completed!" -ForegroundColor Green
    Write-Host "Check output in: $outDir" -ForegroundColor Green
    Write-Host "=======================================" -ForegroundColor Cyan
} else {
    Write-Host ""
    Write-Host "Test generation failed" -ForegroundColor Red
    exit 1
}
