# Quick Reference Guide

## Quick Start

### 1. Build the Project

```powershell
cd "c:\Users\nperiannan\OneDrive - Extreme Networks, Inc\Desktop\Testcase generator"
.\build.ps1
```

### 2. Run with Default Settings

```powershell
.\run.ps1
```

### 3. View Generated Tests

```powershell
cd generated-tests
ls
cat test-summary.txt
cat example-test.yaml
```

## Common Commands

### Build Only
```powershell
go build -o testgen.exe ./cmd/testgen
```

### Run Tests
```powershell
go test ./...
```

### Run with Custom Paths
```powershell
.\testgen.exe `
  --yang-dir "C:\custom\path\yang" `
  --rest-spec "C:\custom\path\openapi.yaml" `
  --nosapi-spec "C:\custom\path\nos-openapi.yaml" `
  --out-dir "./my-tests"
```

### Generate Only Functional Tests
```powershell
.\testgen.exe `
  --yang-dir "<path>" `
  --rest-spec "<path>" `
  --nosapi-spec "<path>" `
  --include-categories "functional" `
  --out-dir "./functional-only"
```

### Generate High-Scale Tests
```powershell
.\testgen.exe `
  --yang-dir "<path>" `
  --rest-spec "<path>" `
  --nosapi-spec "<path>" `
  --scale-factor 1000 `
  --performance-iterations 100 `
  --out-dir "./high-scale-tests"
```

## Source File Locations

| Resource | Path |
|----------|------|
| YANG Models | `C:\Natarajan\automation\PlatformCommonModels\ConfigState\etc\yang` |
| REST API Spec | `C:\Natarajan\automation\PlatformServices\Configuration\src\configuration\infra\rest\openapi.yaml` |
| NOSAPI Spec | `C:\Users\nperiannan\Downloads\nos-openapi.yaml` |

*See [`SOURCE_FILES.md`](SOURCE_FILES.md) for details*

## Project Structure

```
testcase-generator/
├── cmd/testgen/           # CLI application
├── pkg/
│   ├── model/             # Domain models
│   ├── yang/              # YANG parser
│   ├── spec/rest/         # REST API parser
│   ├── spec/nosapi/       # NOSAPI parser
│   ├── generator/         # Test generators
│   └── yamlout/           # YAML writer
├── examples/              # Example output
├── generated-tests/       # Output directory (created on run)
├── build.ps1              # Build script
├── run.ps1                # Quick run script
└── README.md              # Full documentation
```

## Test Categories

| Category | Description | Example |
|----------|-------------|---------|
| **Functional** | CRUD operations, deployment workflows | Create → Deploy → Verify NOS |
| **Boundary** | Parameter limits from YANG constraints | Max VLAN ID (4094) |
| **Negative** | Invalid inputs, missing fields | Create without required field |
| **Scale** | High-count operations | Create 100 instances |
| **Performance** | Timed operations with thresholds | 10 creates under 5s each |

## Deployment Test Scenarios

### Site-Group Deployment
```yaml
Create Profile → Scope to Site-Group → Target to Site-Group 
→ Deploy → Check Status → Verify NOS Config (all devices in group)
```

### Device Deployment
```yaml
Create Profile → Scope to Device → Target to Device 
→ Deploy → Check Status → Verify NOS Config (specific device)
```

## Key Flags Reference

| Flag | Default | Purpose |
|------|---------|---------|
| `--scale-factor` | 100 | Number of instances in scale tests |
| `--performance-iterations` | 10 | Iterations for performance tests |
| `--test-id-prefix` | TCXM | Prefix for test IDs |
| `--starting-id-number` | 1000 | First test ID number |
| `--one-file-per-feature` | true | Separate file per feature |

## Troubleshooting

### Build Fails
```powershell
# Clean and rebuild
Remove-Item testgen.exe
go clean
go mod download
go build -o testgen.exe ./cmd/testgen
```

### Tests Fail
```powershell
# Run verbose tests
go test -v ./...

# Run specific package
go test -v ./pkg/generator
```

### No Output Generated
- Check that YANG directory exists and contains .yang files
- Verify REST API spec is valid OpenAPI 3.0 format
- Ensure NOSAPI spec is accessible

### Empty Test Files
- Verify feature names in YANG match those in REST API paths
- Check that REST API spec has valid operations
- Enable verbose output to see parsing details

## File Outputs

After running, check these files:

| File | Contains |
|------|----------|
| `test-summary.txt` | Count of tests by feature and category |
| `example-test.yaml` | Sample deployment test with all steps |
| `<feature-name>.yaml` | Tests for specific feature |

## Quick Testing

### Test Model Package
```powershell
go test ./pkg/model -v
```

### Test Generator
```powershell
go test ./pkg/generator -v
```

### Test Everything
```powershell
go test ./... -cover
```

## Modifying Generation

### Change Test ID Format
Edit `cmd/testgen/main.go`:
```go
--test-id-prefix "MYTEST"
--starting-id-number 5000
```
Result: `MYTEST_5000`, `MYTEST_5001`, etc.

### Add New Test Category
1. Add to `pkg/model/types.go`: `TestCategoryMyCategory`
2. Create `pkg/generator/mycategory.go`
3. Add to `generateFeatureTestGroup()` in `generator.go`

### Customize Scale Factor
```powershell
.\testgen.exe --scale-factor 500 ... # Generate 500 instances
```

## Help

### Get All Available Flags
```powershell
.\testgen.exe --help
```

### Check Version Info
```powershell
go version
```

### View Example Output
```powershell
cat examples\sample-output.yaml
```

## Next Actions

1. **Validate** with real source files
2. **Review** generated tests for accuracy
3. **Customize** categories and scale factors as needed
4. **Integrate** with your test execution framework
5. **Extend** with custom generators for specific needs

## Support

For detailed documentation, see:
- [`README.md`](README.md) - Full documentation
- [`IMPLEMENTATION_SUMMARY.md`](IMPLEMENTATION_SUMMARY.md) - Technical details
- [`SOURCE_FILES.md`](SOURCE_FILES.md) - File locations
- [`examples/sample-output.yaml`](examples/sample-output.yaml) - Example output
