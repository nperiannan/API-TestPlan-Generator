# Project Conventions

Conventions and practices for the API Test Plan Generator project.

## Repository Layout

```
cmd/testgen/       — Test plan generator CLI entry point
cmd/yaml2excel/    — Go YAML-to-Excel converter CLI
cmd/yaml2csv/      — Go YAML-to-CSV converter CLI
pkg/               — All Go library packages
config/            — Configuration files (config.yaml, checkout-sources.ps1)
sources/           — Input source files (partially gitignored)
Testplans/         — Generated YAML test plans committed as reference output
TestplansXlsx/     — Generated Excel output (gitignored)
TestplansCsv/      — Generated CSV output (gitignored)
reports/           — Generated HTML reports
bin/               — Local Windows/Linux binaries (gitignored)
```

### What gets committed

| Directory / File | Committed | Notes |
|------------------|-----------|-------|
| `sources/nosapi/` | Yes | NOS OpenAPI spec |
| `sources/qaapi/` | Yes | QA OpenAPI spec |
| `sources/PlatformCommonModels/` | No | Cloned from enterprise GitHub |
| `sources/PlatformServices/` | No | Cloned from enterprise GitHub |
| `Testplans/` | Yes | Generated YAML baseline/reference plans |
| `reports/summary_report.html` | Yes | Generated summary report reference |
| `TestplansXlsx/` | No | Generated Excel output |
| `TestplansCsv/` | No | Generated CSV output |
| `bin/windows/` | No | Build artifacts (Windows `.exe` files) |
| `bin/linux/` | No | Build artifacts (Linux binaries) |

## Go Conventions

### Module

- Module path: `github.com/extremenetworks/testcase-generator`
- Go 1.24.x (`go.mod` sets `go 1.24.0` and toolchain `go1.24.4`)

### Package Organization

- `pkg/model/` — Domain types only, no I/O. Types are shared across all packages.
- `pkg/yang/` — YANG file parser. Reads `.yang` files, produces `model.Feature` structs.
- `pkg/spec/rest/` — REST OpenAPI parser. Reads OpenAPI YAML, enriches features with endpoints.
- `pkg/spec/nosapi/` — NOSAPI parser. Reads NOSAPI spec, adds verification endpoints.
- `pkg/generator/` — Test case generation. One file per category/concern. Main orchestrator in `generator.go`.
- `pkg/yamlout/` — Output writers. YAML files, HTML reports, text summaries.
- `cmd/testgen/` — CLI only. Flag parsing, wiring packages together. No business logic.
- `cmd/yaml2excel/` — CLI only. Batch-converts all YAML plans to Excel output.
- `cmd/yaml2csv/` — CLI only. Batch-converts all YAML plans to CSV output.

### Naming

- Generator files are named by what they generate: `functional.go`, `boundary_negative.go`, `scale_performance.go`, `deployment.go`
- Helper/support files describe their concern: `ip_classification.go`, `pattern_values.go`, `permutations.go`
- Test files use `_test.go` suffix in the same package

### Error Handling

- Return `fmt.Errorf("context: %w", err)` with wrapped errors
- CLI exits with `os.Exit(1)` on fatal errors
- Parsers use `log.Printf` for warnings (non-fatal parse issues)

## Source Management

### config.yaml

All input source locations are defined in `config/config.yaml`. Each source entry specifies:

- `repo` — Git clone URL (enterprise GitHub, omitted for committed sources)
- `branch` — Git branch to checkout
- `sparse` — Sparse checkout path (always a **directory**, not a file)
- `localDir` — Directory name under `sources/`
- `localFile` — Specific file within the sparse path (optional)

### Sparse Checkout

- Always use **directory-level** sparse paths, not file-level
- Git sparse-checkout with `--filter=blob:none` for efficient clones
- The `-force` flag backs up existing sources before re-cloning; restores on failure

### checkout-sources.ps1

- Located at `config/checkout-sources.ps1`, run from project root
- Uses `$PSScriptRoot` to resolve `config.yaml` relative to itself
- Safe re-checkout: old files are renamed to `sources.bak/`, not deleted
- Failed checkouts restore from backup automatically

## Test Generation

### Test IDs

- Format: `{prefix}_{number}` (e.g., `TCXM_1000`)
- Prefix configurable via `--test-id-prefix`
- Starting number via `--starting-id-number`
- IDs are sequential across all features

### Test Categories

| Category | File | Priority |
|----------|------|----------|
| Functional | `functional.go` | P0–P1 |
| Deployment | `deployment.go` | P0 |
| Boundary | `boundary_negative.go` | P1–P2 |
| Negative | `boundary_negative.go` | P1–P2 |
| Scale | `scale_performance.go` | P2–P3 |
| Performance | `scale_performance.go` | P2–P3 |

### Blueprint Categories

Features are classified into blueprint categories:
- **Wired** — Switch/port/VLAN/routing features
- **Wireless** — WLAN/SSID/radio features
- **Global** — Features that span both (e.g., RADIUS, DNS, NTP)

### Output

- One YAML file per feature by default (`--one-file-per-feature`)
- Summary report goes to `reports/summary_report.html`
- YAML output goes to `Testplans/`
- Excel output goes to `TestplansXlsx/`
- CSV output goes to `TestplansCsv/`
- `Testplans/` and `reports/summary_report.html` are committed as reference output when generation logic changes

### Generation correctness rules

> Generated test plans should be executable with minimal manual intervention. Prefer fewer, accurate tests over more tests with guessed paths, guessed payload identity, or guessed validations.

- REST paths must come from the OpenAPI spec or an explicitly reviewed mapping. Do not hardcode deployment/scope/target/status paths unless those exact paths exist in the spec.
- Feature-to-endpoint matching must prefer exact `featurePath`/`objectType` mappings. Fuzzy matches must be treated as lower confidence and should be visible in logs or reports.
- YANG list keys are identity fields. Include key fields in create, update, read-verify, and delete payload logic even when the key leaves are not marked `mandatory true`.
- YANG defaults may be quoted or unquoted. Parsers must preserve boolean, numeric, enum, and string defaults accurately.
- Read validations must assert values that were actually sent by create/update steps, or values that are proven defaults from YANG/OpenAPI. Do not validate optional fields using unrelated sample values.
- Sample values must be selected from YANG type, constraints, enum/defaults, and descriptions in that order. Name-based heuristics are a fallback only.
- IP/subnet/gateway fields require field-specific values: names should be names, subnet addresses should use subnet/CIDR-compatible values, masks should be masks or prefix lengths as described, and gateway addresses should be host addresses.
- Delete steps must carry enough identity to delete the exact object created by the test: object ID, object IDs, or all YANG key fields required by the API shape.
- NOSAPI verification endpoints are optional unless confidently mapped. Do not attach unrelated NOSAPI endpoints just because names partially match.
- When generation logic changes, regenerate `Testplans/`, `reports/summary_report.html`, and Excel output, then spot-check representative global, service, wired, wireless, and nested sub-object plans.

## Scripts

### testgen.ps1 / testgen.sh

- `testgen.ps1` — Windows PowerShell pipeline: generate → summary → Excel batch export
- `testgen.sh`  — Linux bash equivalent; identical feature set
- Accepts a features argument: `wired` (default), `wireless`, `all`, or specific feature names
- Auto-builds the required binary if not present

### Script Parity Rule

> **Any change to `testgen.ps1` must be mirrored in `testgen.sh`, and vice versa.**
> Both scripts must always have identical behaviour.

### PowerShell Style

- Scripts use `param()` blocks for parameters
- Helper functions for colored output: `Write-Step`, `Write-Ok`, `Write-Warn`, `Write-Err`
- Exit codes: 0 = success, 1 = error

## Build Rules

### Dual-platform binaries

> **Every code change to `cmd/testgen`, `cmd/yaml2excel`, or `cmd/yaml2csv` must produce fresh binaries for both platforms before committing.**

```powershell
# Windows binaries
go build -o bin/windows/testgen.exe    ./cmd/testgen/
go build -o bin/windows/yaml2excel.exe ./cmd/yaml2excel/
go build -o bin/windows/yaml2csv.exe   ./cmd/yaml2csv/

# Linux binaries (cross-compile)
$env:GOOS="linux"; $env:GOARCH="amd64"
go build -o bin/linux/testgen    ./cmd/testgen/
go build -o bin/linux/yaml2excel ./cmd/yaml2excel/
go build -o bin/linux/yaml2csv   ./cmd/yaml2csv/
Remove-Item Env:GOOS, Env:GOARCH
```

Binaries are gitignored and not committed; the rule ensures local copies stay in sync with the source.

## Git Workflow

- Single branch: `main`
- Remote: `origin` → `github.com/nperiannan/API-TestPlan-Generator`
- Enterprise repos: `github.extremenetworks.com/Engineering/*`
- Commits use imperative mood: "Add feature", "Fix bug", "Update config"
- Tag releases as `v{major}.{minor}.{patch}`

### Commit cadence

> **Commit and push to `origin/main` as soon as a logical change is complete** — do not batch unrelated changes into a single commit.

### Clean working branch

> **The working branch must always be clean before starting new work and after completing a task.**
> Run `git status` and ensure no modified or untracked source files remain outside of intentionally gitignored paths (`bin/`, `sources/PlatformCommonModels/`, `sources/PlatformServices/`, `.venv/`, `Testplans/`).
> A clean branch means `git status` reports either "nothing to commit" or only those ignored paths.
