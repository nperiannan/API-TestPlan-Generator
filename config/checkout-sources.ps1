# ─────────────────────────────────────────────────────────────────────
# checkout-sources.ps1
# Reads config.yaml and checks out the required source files into
# the ./sources/ directory using Git sparse checkout.
# ─────────────────────────────────────────────────────────────────────
# Usage:
#   .\config\checkout-sources.ps1              # Checkout / update all sources
#   .\config\checkout-sources.ps1 -force       # Remove sources/ and re-checkout
# ─────────────────────────────────────────────────────────────────────

param(
    [switch]$force
)

# ── Helpers ──────────────────────────────────────────────────────────

function Write-Step($msg) { Write-Host ">> $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "   $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "   $msg" -ForegroundColor Yellow }
function Write-Err($msg)  { Write-Host "   $msg" -ForegroundColor Red }

# ── Parse config.yaml ────────────────────────────────────────────────

Write-Step "Reading config/config.yaml"

$configPath = Join-Path $PSScriptRoot "config.yaml"
if (-not (Test-Path $configPath)) {
    Write-Err "config/config.yaml not found."
    exit 1
}

# Lightweight YAML parser — sufficient for our flat/nested structure.
# Extracts sourcesDir and each source entry's repo, branch, sparse, localDir, localFile.

$configLines = Get-Content $configPath -Encoding UTF8
$sourcesDir  = "./sources"
$sources     = @{}
$currentSrc  = $null

foreach ($line in $configLines) {
    # Skip comments and blank lines
    if ($line -match '^\s*#' -or $line -match '^\s*$') { continue }

    # Top-level sourcesDir
    if ($line -match '^\s*sourcesDir:\s*(.+)') {
        $sourcesDir = $Matches[1].Trim().Trim('"', "'")
        continue
    }

    # Source entry header (indented name followed by colon)
    if ($line -match '^\s{2}(\w[\w-]*):\s*$') {
        $currentSrc = $Matches[1]
        $sources[$currentSrc] = @{}
        continue
    }

    # Source entry properties
    if ($currentSrc -and $line -match '^\s{4}(\w[\w-]*):\s*(.+)') {
        $key = $Matches[1]
        $val = $Matches[2].Trim().Trim('"', "'")
        $sources[$currentSrc][$key] = $val
    }
}

Write-Ok "Sources directory: $sourcesDir"
Write-Ok "Found $($sources.Count) source entries"

# ── Prepare sources directory ────────────────────────────────────────

if ($force -and (Test-Path $sourcesDir)) {
    Write-Step "Removing existing sources/ (--force)"
    Remove-Item -Recurse -Force $sourcesDir
}

if (-not (Test-Path $sourcesDir)) {
    New-Item -ItemType Directory -Path $sourcesDir | Out-Null
}

# ── Checkout each source ─────────────────────────────────────────────

foreach ($name in $sources.Keys) {
    $src = $sources[$name]
    $repo     = $src["repo"]
    $branch   = $src["branch"]
    $sparse   = $src["sparse"]
    $localDir = $src["localDir"]

    Write-Host ""
    Write-Step "Source: $name"

    # If no repo is defined, this is a manual-placement source.
    if (-not $repo) {
        $localFile = $src["localFile"]
        $targetDir = Join-Path $sourcesDir $localDir
        if (-not (Test-Path $targetDir)) {
            New-Item -ItemType Directory -Path $targetDir | Out-Null
        }
        if ($localFile) {
            $targetPath = Join-Path $targetDir $localFile
            if (Test-Path $targetPath) {
                Write-Ok "$localFile already present at $targetPath"
            } else {
                Write-Warn "$localFile not found at $targetPath"
                Write-Warn "Please copy it manually:"
                Write-Warn "  Copy-Item <path-to-$localFile> $targetPath"
            }
        }
        continue
    }

    $cloneDir = Join-Path $sourcesDir $localDir

    if (Test-Path $cloneDir) {
        # Already cloned — pull latest
        Write-Ok "Directory exists, pulling latest..."
        Push-Location $cloneDir
        git fetch origin $branch --quiet 2>&1 | Out-Null
        git checkout $branch --quiet 2>&1 | Out-Null
        git pull origin $branch --quiet 2>&1 | Out-Null
        Pop-Location
        Write-Ok "Updated $localDir"
    } else {
        # Fresh sparse checkout
        Write-Ok "Cloning $repo (branch: $branch)..."
        git clone --filter=blob:none --sparse --branch $branch $repo $cloneDir 2>&1 | ForEach-Object { Write-Host "   $_" }
        if ($LASTEXITCODE -ne 0) {
            Write-Err "Clone failed for $name"
            continue
        }

        # Set sparse-checkout to only the path we need
        Push-Location $cloneDir
        git sparse-checkout set $sparse 2>&1 | Out-Null
        Pop-Location
        Write-Ok "Sparse checkout: $sparse"
    }

    # Verify the expected path exists
    if ($sparse) {
        $expectedPath = Join-Path $cloneDir $sparse
        if (Test-Path $expectedPath) {
            Write-Ok "Verified: $expectedPath"
        } else {
            Write-Warn "Expected path not found: $expectedPath"
        }
    }
}

# ── Summary ──────────────────────────────────────────────────────────

Write-Host ""
Write-Host "=======================================" -ForegroundColor Cyan
Write-Host "Source checkout complete." -ForegroundColor Green
Write-Host ""
Write-Host "Directory structure:" -ForegroundColor Cyan
if (Test-Path $sourcesDir) {
    Get-ChildItem -Path $sourcesDir -Recurse -Depth 3 -Directory | ForEach-Object {
        $rel = $_.FullName.Replace((Resolve-Path $sourcesDir).Path, "sources")
        Write-Host "  $rel/" -ForegroundColor Gray
    }
    Get-ChildItem -Path $sourcesDir -Recurse -Depth 4 -File | ForEach-Object {
        $rel = $_.FullName.Replace((Resolve-Path $sourcesDir).Path, "sources")
        Write-Host "  $rel" -ForegroundColor White
    }
}
Write-Host "=======================================" -ForegroundColor Cyan
