# Build script for testgen

Write-Host "=======================================" -ForegroundColor Cyan
Write-Host "Building testgen CLI..." -ForegroundColor Cyan
Write-Host "=======================================" -ForegroundColor Cyan
Write-Host ""

# Clean previous build
if (Test-Path "testgen.exe") {
    Write-Host "Removing previous build..." -ForegroundColor Yellow
    Remove-Item "testgen.exe"
}

# Download dependencies
Write-Host "Downloading dependencies..." -ForegroundColor Green
go mod tidy

if ($LASTEXITCODE -ne 0) {
    Write-Host "Failed to download dependencies" -ForegroundColor Red
    exit 1
}

go mod download

if ($LASTEXITCODE -ne 0) {
    Write-Host "Failed to download dependencies" -ForegroundColor Red
    exit 1
}

# Run tests
Write-Host ""
Write-Host "Running unit tests..." -ForegroundColor Green
go test ./...

if ($LASTEXITCODE -ne 0) {
    Write-Host "Tests failed" -ForegroundColor Red
    exit 1
}

# Build
Write-Host ""
Write-Host "Building executable..." -ForegroundColor Green
go build -o testgen.exe ./cmd/testgen

if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "=======================================" -ForegroundColor Cyan
Write-Host "Build completed successfully!" -ForegroundColor Green
Write-Host "Executable: testgen.exe" -ForegroundColor Green
Write-Host "=======================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Usage example:" -ForegroundColor Yellow
Write-Host "  .\testgen.exe --yang-dir `"<path>`" --rest-spec `"<path>`" --nosapi-spec `"<path>`" --out-dir `"./output`"" -ForegroundColor Gray
