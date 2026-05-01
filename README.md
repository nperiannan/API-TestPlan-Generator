# API Test Plan Generator

A Go CLI tool that auto-generates comprehensive API test plans (YAML) for Extreme Networks cloud platform features. It derives test cases from **YANG models**, **REST OpenAPI specs**, and **NOSAPI definitions**.

## Overview

The generator produces complete test suites covering:

- **Functional tests** — CRUD operations, deployment workflows with NOSAPI verification
- **Boundary tests** — Parameter-level boundary testing based on YANG constraints
- **Negative tests** — Invalid values, missing required fields, constraint violations
- **Scale tests** — High-count object creation, large payloads, deeply nested structures
- **Performance tests** — Repeated operations with timing measurements
- **Additional coverage** — IP classification, value transitions, pattern-based values, permutations

### Key Features

- **YANG-driven** — YANG models are the single source of truth for features and constraints
- **Deployment-aware** — Site-group and device scoping/targeting with full deployment lifecycle
- **NOSAPI verification** — Generates verification steps against NOS device configuration
- **Blueprint categories** — Wired, wireless, and global profile support
- **Rich metadata** — Every test includes unique ID, priority, type, description, and metadata
- **HTML reports** — Summary report and per-feature coverage reports

## Project Structure

```
API-TestPlan-Generator/
├── cmd/testgen/                    # CLI application
│   └── main.go
├── pkg/
│   ├── model/                      # Domain models and types
│   │   ├── types.go                # Core test structure types
│   │   ├── feature.go              # Feature and constraint types
│   │   ├── config.go               # Generator configuration
│   │   └── teststep_yaml.go        # YAML serialization helpers
│   ├── yang/                       # YANG model parser
│   │   ├── parser.go
│   │   └── types.go
│   ├── spec/
│   │   ├── rest/                   # REST OpenAPI spec parser
│   │   │   ├── parser.go
│   │   │   └── response_parser.go
│   │   └── nosapi/                 # NOSAPI spec parser
│   │       └── parser.go
│   ├── generator/                  # Test generation logic
│   │   ├── generator.go            # Main orchestrator
│   │   ├── functional.go           # Functional test generation
│   │   ├── deployment.go           # Deployment workflow tests
│   │   ├── boundary_negative.go    # Boundary & negative tests
│   │   ├── scale_performance.go    # Scale & performance tests
│   │   ├── additional_coverage.go  # Extra coverage generators
│   │   ├── ip_classification.go    # IP-based test generation
│   │   ├── pattern_values.go       # Pattern/regex value generation
│   │   ├── permutations.go         # Parameter permutation tests
│   │   ├── response_validations.go # Response validation helpers
│   │   ├── value_transition.go     # State transition tests
│   │   └── coverage_report.go      # Coverage analysis
│   └── yamlout/                    # Output writers
│       ├── writer.go               # YAML file writer
│       └── html_report.go          # HTML report generator
├── config/                         # Configuration files
│   ├── config.yaml                 # Source definitions and paths
│   └── checkout-sources.ps1        # Automated source checkout
├── sources/                        # Input source files (partially gitignored)
│   ├── nosapi/                     # NOS OpenAPI spec (committed)
│   ├── qaapi/                      # QA OpenAPI spec (committed)
│   ├── PlatformCommonModels/       # YANG models (gitignored, cloned)
│   └── PlatformServices/           # REST OpenAPI spec (gitignored, cloned)
├── reports/                        # Generated HTML reports
├── TestplansXlsx/                  # Excel exports (default output for yaml2excel)
├── TestplansCsv/                   # CSV exports (default output for yaml2csv)
├── bin/
│   ├── windows/                    # Windows executables (.exe)
│   └── linux/                      # Linux executables (amd64)
├── cmd/
│   ├── testgen/                    # CLI: test plan generator
│   ├── yaml2excel/                 # CLI: YAML → Excel converter
│   └── yaml2csv/                   # CLI: YAML → CSV converter
├── generated-tests/                # Sample generated outputs (committed)
├── testgen.ps1                     # Windows: generate test plans + Excel export
├── testgen.sh                      # Linux:   generate test plans + Excel export
├── go.mod
└── README.md
```

## Quick Start

### Prerequisites

- Go 1.21 or later
- Git (for source checkout from enterprise GitHub)
- PowerShell (for helper scripts)

### 1. Build

**Windows:**
```powershell
New-Item -ItemType Directory -Force -Path bin/windows
go build -o bin/windows/testgen.exe    ./cmd/testgen
go build -o bin/windows/yaml2excel.exe ./cmd/yaml2excel
go build -o bin/windows/yaml2csv.exe   ./cmd/yaml2csv
```

**Linux (cross-compile from Windows):**
```powershell
$env:GOOS="linux"; $env:GOARCH="amd64"
New-Item -ItemType Directory -Force -Path bin/linux
go build -o bin/linux/testgen    ./cmd/testgen
go build -o bin/linux/yaml2excel ./cmd/yaml2excel
go build -o bin/linux/yaml2csv   ./cmd/yaml2csv
Remove-Item Env:GOOS, Env:GOARCH
```

Pre-built binaries are in `bin/windows/` and `bin/linux/`.

### 2. Checkout Sources

Enterprise sources (YANG models, REST spec) are checked out via sparse git clone:

```powershell
.\config\checkout-sources.ps1          # First-time checkout or update
.\config\checkout-sources.ps1 -force   # Clean re-checkout (old files backed up)
```

Source locations are defined in [config/config.yaml](config/config.yaml). The `nosapi` and `qaapi` specs are committed to git; `PlatformCommonModels` and `PlatformServices` are cloned from `github.extremenetworks.com` and gitignored.

### 3. Generate Test Plans

The scripts run the full pipeline automatically:
1. Generate YAML test plans via `testgen`
2. Print a per-category summary and total test count in the console
3. Batch-export all YAMLs to Excel (`TestplansXlsx/`) via `yaml2excel`
4. Print the final output locations

**Windows (PowerShell):**
```powershell
.\testgen.ps1                              # Default: wired features
.\testgen.ps1 -features wireless           # Wireless features only
.\testgen.ps1 -features all                # All features
.\testgen.ps1 -features radius-server      # Single feature
.\testgen.ps1 -features "radius-server,ntp-server"  # Multiple features
```

**Linux (bash):**
```bash
./testgen.sh                               # Default: wired features
./testgen.sh wireless                      # Wireless features only
./testgen.sh all                           # All features
./testgen.sh radius-server                 # Single feature
./testgen.sh "radius-server,ntp-server"    # Multiple features
```

Or run the CLI directly:

```powershell
.\bin\windows\testgen.exe `
  --yang-dir "./sources/PlatformCommonModels/ConfigState/etc/yang" `
  --rest-spec "./sources/qaapi/qaopenapi.yaml" `
  --nosapi-spec "./sources/nosapi/nos-openapi.yaml" `
  --out-dir "./Testplans"
```

### CLI Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--yang-dir` | Path to YANG models directory | *Required* |
| `--rest-spec` | Path to REST API OpenAPI spec | *Required* |
| `--nosapi-spec` | Path to NOSAPI OpenAPI spec | *Required* |
| `--out-dir` | Output directory for generated tests | `./Testplans` |
| `--include-categories` | Test categories to generate | `functional,boundary,negative,scale,performance` |
| `--feature-categories` | Blueprint categories: wired, wireless, all | `wired` |
| `--features` | Comma-separated feature filter | *(all)* |
| `--scope-types` | Scope types for deployment tests | `site-group,device` |
| `--target-types` | Target types for deployment tests | `site-group,device` |
| `--deployment-methods` | Deployment methods to test | `rolling,immediate` |
| `--scale-factor` | Number of instances for scale tests | `100` |
| `--performance-iterations` | Iterations for performance tests | `10` |
| `--performance-concurrency` | Concurrency for performance tests | `5` |
| `--one-file-per-feature` | Generate one YAML per feature | `true` |
| `--test-id-prefix` | Prefix for test case IDs | `TCXM` |
| `--starting-id-number` | Starting number for test IDs | `1000` |

## Output

The generator produces into `--out-dir`:

| File | Description |
|------|-------------|
| `<feature>.yaml` | One YAML test plan per feature (with `--one-file-per-feature`) |
| `test-summary.txt` | Summary with test counts by feature and category |
| `coverage-report.json` | Machine-readable coverage data |
| `coverage-report.txt` | Human-readable coverage report |
| `test-coverage-report.html` | Per-feature HTML coverage report |
| `example-test.yaml` | Example deployment test for reference |

Additionally, `reports/summary_report.html` is generated with a cross-feature summary dashboard.

## Source Configuration

All input source paths are defined in [config/config.yaml](config/config.yaml):

- **YANG models** — Sparse-cloned from `PlatformCommonModels` (enterprise GitHub)
- **REST OpenAPI spec** — Sparse-cloned from `PlatformServices` (enterprise GitHub)
- **NOS OpenAPI spec** — Committed in `sources/nosapi/`
- **QA OpenAPI spec** — Committed in `sources/qaapi/`

The checkout script uses sparse git clones with `--filter=blob:none` for efficient checkout of only needed paths. On `-force`, old sources are backed up and restored if the checkout fails.

## Testing

```powershell
go test ./...                  # Run all tests
go test -cover ./...           # With coverage
go test ./pkg/generator        # Specific package
```

## Tools

### YAML to Excel (`yaml2excel`)

Converts a generated YAML test plan into a styled Excel workbook (one sheet per test category). Output defaults to `TestplansXlsx/`.

```powershell
# No args — batch convert every YAML in Testplans/ to TestplansXlsx/
.\bin\windows\yaml2excel.exe

# By feature name — searches Testplans/ recursively
.\bin\windows\yaml2excel.exe port

# By full path
.\bin\windows\yaml2excel.exe Testplans/wired-blueprint/port.yaml

# Specify output path explicitly
.\bin\windows\yaml2excel.exe port output/port-tests.xlsx
```

**Linux:**
```bash
./bin/linux/yaml2excel              # batch all
./bin/linux/yaml2excel port         # single feature
```

### YAML to CSV (`yaml2csv`)

Converts a generated YAML test plan into a flat CSV summary. Output defaults to `TestplansCsv/`.

```powershell
# No args — batch convert every YAML in Testplans/ to TestplansCsv/
.\bin\windows\yaml2csv.exe

# By feature name
.\bin\windows\yaml2csv.exe port

# By full path
.\bin\windows\yaml2csv.exe Testplans/global-profile/radius-server.yaml

# Specify output path explicitly
.\bin\windows\yaml2csv.exe port output/port.csv
```

**Linux:**
```bash
./bin/linux/yaml2csv              # batch all
./bin/linux/yaml2csv port         # single feature
```

CSV columns: `TestCaseID`, `Type`, `Priority`, `Automation`, `Description`, `IsDeploymentTest`.

### Excel column config

Column definitions, ordering, widths, and header styling are driven by [config/yaml2excel.yaml](config/yaml2excel.yaml). Edit that file to add/remove/reorder columns, change header colors, or set fixed values — no code changes needed.

## Extension Points

- **New test categories**: Add generator in `pkg/generator/`, wire into `generator.go`
- **New profile types**: Add to `model.ProfileType`, update `rest.Parser`
- **New scope/target types**: Add to `model.ScopeType`/`model.TargetType`
- **New deployment methods**: Add to `model.DeploymentMethod`
- CI/CD pipeline integration
- GraphQL API support
- gRPC/Protobuf support

## License

Copyright © 2025 Extreme Networks, Inc.

## Contact

For questions or issues, please contact the development team.
