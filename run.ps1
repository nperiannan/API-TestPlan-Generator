# Quick start script for testgen with predefined paths
# Reads source paths from config/config.yaml
# Usage:
#   .\run.ps1                          # Generate wired features (default)
#   .\run.ps1 -features wired          # Generate wired features only
#   .\run.ps1 -features wireless       # Generate wireless features only
#   .\run.ps1 -features all            # Generate all features (wired + wireless)
#   .\run.ps1 -features radius-server  # Generate a specific feature only
#   .\run.ps1 -features "radius-server,ntp-server"  # Multiple specific features

param(
    [string]$features = "wired"
)

Write-Host "=======================================" -ForegroundColor Cyan
Write-Host "Running testgen with predefined paths" -ForegroundColor Cyan
Write-Host "=======================================" -ForegroundColor Cyan
Write-Host ""

# ── Parse config/config.yaml for source paths ───────────────────────

$configPath = Join-Path $PSScriptRoot "config" "config.yaml"
if (-not (Test-Path $configPath)) {
    Write-Host "config/config.yaml not found. Cannot resolve source paths." -ForegroundColor Red
    exit 1
}

$configLines = Get-Content $configPath -Encoding UTF8
$sourcesDir  = "./sources"
$sources     = @{}
$currentSrc  = $null

foreach ($line in $configLines) {
    if ($line -match '^\s*#' -or $line -match '^\s*$') { continue }
    if ($line -match '^\s*sourcesDir:\s*(.+)') {
        $sourcesDir = $Matches[1].Trim().Trim('"', "'")
        continue
    }
    if ($line -match '^\s{2}(\w[\w-]*):\s*$') {
        $currentSrc = $Matches[1]
        $sources[$currentSrc] = @{}
        continue
    }
    if ($currentSrc -and $line -match '^\s{4}(\w[\w-]*):\s*(.+)') {
        $sources[$currentSrc][$Matches[1]] = $Matches[2].Trim().Trim('"', "'")
    }
}

# ── Resolve paths from config ───────────────────────────────────────

# YANG dir: sourcesDir / localDir / sparse
$yangSrc = $sources["yang"]
$yangDir = Join-Path $sourcesDir $yangSrc["localDir"] $yangSrc["sparse"]

# REST spec: sourcesDir / localDir / sparse / localFile
$restSrc = $sources["restSpec"]
$restSpec = Join-Path $sourcesDir $restSrc["localDir"] $restSrc["sparse"] $restSrc["localFile"]

# NOSAPI spec: sourcesDir / localDir / localFile
$nosapiSrc = $sources["nosapiSpec"]
$nosapiSpec = Join-Path $sourcesDir $nosapiSrc["localDir"] $nosapiSrc["localFile"]

$outDir = "./Testplans"

Write-Host "Source paths (from config/config.yaml):" -ForegroundColor Gray
Write-Host "  YANG dir:    $yangDir" -ForegroundColor Gray
Write-Host "  REST spec:   $restSpec" -ForegroundColor Gray
Write-Host "  NOSAPI spec: $nosapiSpec" -ForegroundColor Gray
Write-Host ""

# Check if executable exists
if (-not (Test-Path "testgen.exe")) {
    Write-Host "testgen.exe not found. Building..." -ForegroundColor Yellow
    go build -o testgen.exe ./cmd/testgen/
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Build failed" -ForegroundColor Red
        exit 1
    }
}

# Check if source files exist
Write-Host "Checking source files..." -ForegroundColor Green

if (-not (Test-Path $yangDir)) {
    Write-Host "Warning: YANG directory not found: $yangDir" -ForegroundColor Yellow
}

if (-not (Test-Path $restSpec)) {
    Write-Host "Warning: REST API spec not found: $restSpec" -ForegroundColor Yellow
}

if (-not (Test-Path $nosapiSpec)) {
    Write-Host "Warning: NOSAPI spec not found: $nosapiSpec" -ForegroundColor Yellow
}

# Determine feature-categories and features filter based on -features arg
$featureCategories = ""
$featureFilter = ""

switch ($features.ToLower()) {
    "wired" {
        Write-Host "Mode: Wired features (global-profile, wired-blueprint, service-profile)" -ForegroundColor Green
        $featureCategories = "global-profile,wired-blueprint,service-profile"
    }
    "wireless" {
        Write-Host "Mode: Wireless features only (wireless-blueprint)" -ForegroundColor Green
        $featureCategories = "wireless-blueprint"
    }
    "all" {
        Write-Host "Mode: All features (wired + wireless)" -ForegroundColor Green
        $featureCategories = "global-profile,wired-blueprint,wireless-blueprint,service-profile"
    }
    default {
        # Treat as specific feature name(s)
        Write-Host "Mode: Specific features: $features" -ForegroundColor Green
        $featureCategories = "global-profile,wired-blueprint,wireless-blueprint,service-profile"
        $featureFilter = $features
    }
}

Write-Host ""
Write-Host "Running testgen..." -ForegroundColor Green
Write-Host ""

# Build command args
$args = @(
    "--yang-dir", $yangDir,
    "--rest-spec", $restSpec,
    "--nosapi-spec", $nosapiSpec,
    "--out-dir", $outDir,
    "--feature-categories", $featureCategories,
    "--include-categories", "functional,boundary,negative,scale,performance",
    "--scope-types", "site-group,device",
    "--target-types", "site-group,device",
    "--deployment-methods", "rolling,immediate",
    "--scale-factor", "100",
    "--performance-iterations", "10",
    "--one-file-per-feature", "true"
)

if ($featureFilter -ne "") {
    $args += "--features"
    $args += $featureFilter
} else {
    $args += "--features"
    $args += ""
}

# Run the generator
& .\testgen.exe @args

if ($LASTEXITCODE -ne 0) {
    Write-Host ""
    Write-Host "Test generation failed" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "=======================================" -ForegroundColor Cyan
Write-Host "All done! Check output in: $outDir" -ForegroundColor Green
Write-Host "=======================================" -ForegroundColor Cyan
