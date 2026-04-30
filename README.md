# API Test Case Generator

A comprehensive Go library and CLI tool for auto-generating API test definitions (YAML) for network management systems. The generator derives test cases from YANG models, REST API specifications, and NOSAPI definitions.

## Overview

This tool generates complete test suites including:

- **Functional tests**: CRUD operations, deployment workflows with NOSAPI verification
- **Boundary tests**: Parameter-level boundary testing based on YANG constraints
- **Negative tests**: Invalid values, missing required fields, constraint violations
- **Scale tests**: High-count object creation, large payloads, deeply nested structures
- **Performance tests**: Repeated operations with timing measurements

### Key Features

- **YANG-driven**: Treats YANG models as the single source of truth for features and constraints
- **Deployment-aware**: Supports site-group and device scoping/targeting with full deployment lifecycle
- **NOSAPI verification**: Generates verification steps for NOS device configuration
- **Modular architecture**: Easy to extend for new profile types, test categories, and deployment methods
- **Rich metadata**: Every test includes unique ID, priority, type, description, and metadata
- **Production-quality**: Type-safe, well-tested, idiomatic Go code

## Architecture

```
testcase-generator/
├── cmd/testgen/           # CLI application
│   └── main.go
├── pkg/
│   ├── model/             # Domain models and types
│   │   ├── types.go       # Core test structure types
│   │   ├── feature.go     # Feature and constraint types
│   │   └── config.go      # Generator configuration
│   ├── yang/              # YANG parser
│   │   └── parser.go
│   ├── spec/
│   │   ├── rest/          # REST API spec parser
│   │   │   └── parser.go
│   │   └── nosapi/        # NOSAPI spec parser
│   │       └── parser.go
│   ├── generator/         # Test generation logic
│   │   ├── generator.go
│   │   ├── functional.go
│   │   ├── deployment.go
│   │   ├── boundary_negative.go
│   │   └── scale_performance.go
│   └── yamlout/           # YAML serialization
│       └── writer.go
├── examples/
│   └── sample-output.yaml # Example generated test
├── SOURCE_FILES.md        # Source file locations reference
├── go.mod
└── README.md
```

## Installation

### Prerequisites

- Go 1.21 or later
- Access to YANG models directory
- REST API OpenAPI specification (YAML)
- NOSAPI OpenAPI specification (YAML)

### Build

```bash
# Clone or navigate to the repository
cd "c:\Users\nperiannan\OneDrive - Extreme Networks, Inc\Desktop\Testcase generator"

# Download dependencies
go mod download

# Build the CLI
go build -o testgen.exe ./cmd/testgen
```

## Usage

### Basic Usage

```bash
./testgen.exe \
  --yang-dir "C:\Natarajan\automation\PlatformCommonModels\ConfigState\etc\yang" \
  --rest-spec "C:\Natarajan\automation\PlatformServices\Configuration\src\configuration\infra\rest\openapi.yaml" \
  --nosapi-spec "C:\Users\nperiannan\Downloads\nos-openapi.yaml" \
  --out-dir "./Testplans"
```

### Advanced Options

```bash
./testgen.exe \
  --yang-dir "<path-to-yang>" \
  --rest-spec "<path-to-rest-api-spec>" \
  --nosapi-spec "<path-to-nosapi-spec>" \
  --out-dir "./Testplans" \
  --include-categories functional,boundary,negative,scale,performance \
  --scope-types site-group,device \
  --target-types site-group,device \
  --deployment-methods rolling,immediate \
  --scale-factor 100 \
  --performance-iterations 10 \
  --performance-concurrency 5 \
  --one-file-per-feature true \
  --test-id-prefix TCXM \
  --starting-id-number 1000
```

### CLI Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--yang-dir` | Path to YANG models directory | *Required* |
| `--rest-spec` | Path to REST API OpenAPI spec | *Required* |
| `--nosapi-spec` | Path to NOSAPI OpenAPI spec | *Required* |
| `--out-dir` | Output directory for generated tests | `./Testplans` |
| `--include-categories` | Test categories to generate | `functional,boundary,negative,scale,performance` |
| `--scope-types` | Scope types for deployment tests | `site-group,device` |
| `--target-types` | Target types for deployment tests | `site-group,device` |
| `--deployment-methods` | Deployment methods to test | `rolling,immediate` |
| `--scale-factor` | Number of instances for scale tests | `100` |
| `--performance-iterations` | Iterations for performance tests | `10` |
| `--performance-concurrency` | Concurrency for performance tests | `5` |
| `--one-file-per-feature` | Generate one YAML per feature | `true` |
| `--test-id-prefix` | Prefix for test case IDs | `TCXM` |
| `--starting-id-number` | Starting number for test IDs | `1000` |

## Output Structure

The generator produces:

1. **Individual test files** (if `--one-file-per-feature` is true): One YAML file per feature containing all test categories
2. **test-suite.yaml** (if `--one-file-per-feature` is false): Single file with all tests
3. **test-summary.txt**: Summary report with test counts by feature and category
4. **example-test.yaml**: Detailed example of a deployment test

### Example Test Case Structure

```yaml
testCaseID: "TCXM_1000"
featureName: "VLAN-Configuration"
priority: "P0"
type: "functional"
description: "Create configuration profile, scope to site group, deploy, and verify on NOS devices"
scopeType: "site-group"
targetType: "site-group"
deploymentMethod: "immediate"
isDeploymentTest: true
steps:
  - name: "createProfile"
    method: "POST"
    api: "REST"
    path: "/configuration/v1/configuration-profile"
    body: {...}
    expectedStatus: 201
    validations:
      - type: "statusCode"
        expected: 201
  
  - name: "scopeProfileTosite-group"
    method: "POST"
    api: "REST"
    path: "/configuration/v1/configuration-profile/{profileName}/scope/site-group"
    body: {...}
    expectedStatus: 200
  
  - name: "deployProfileTosite-group"
    method: "POST"
    api: "REST"
    path: "/configuration/v1/configuration-profile/{profileName}/deploy"
    expectedStatus: 202
  
  - name: "checkDeploymentStatus"
    method: "GET"
    api: "REST"
    path: "/configuration/v1/configuration-profile/{profileName}/deploy/status"
    timeout: 300
    validations:
      - type: "jsonPathEquals"
        path: "$.status"
        expected: "SUCCESS"
  
  - name: "verifyNosConfigForsite-group"
    method: "GET"
    api: "NOSAPI"
    path: "/nos/v1/site-groups/{siteGroupId}/devices/config"
    devicesScope: "siteGroup"
    validations:
      - type: "nosConfigMatchesExpected"
```

## Test Categories

### Functional Tests

- **CRUD operations**: Create, Read, Update, Delete with validations
- **Deployment workflows**:
  - Create profile → Scope → Target → Deploy → Verify deployment status → Verify NOS config
  - Both site-group and device scenarios
  - Deployment and non-deployment variants

### Boundary Tests

- Parameter-level boundaries from YANG constraints
- String length (min/max)
- Numeric ranges (min/max)
- Array cardinality (minItems/maxItems)
- Enum boundary values

### Negative Tests

- Missing required fields
- Invalid enum values
- Constraint violations
- Invalid scope/target/deployment combinations
- Type mismatches

### Scale Tests

- Multiple instances (configurable count)
- Large lists/arrays in single objects
- Deeply nested structures
- High payload sizes

### Performance Tests

- Repeated operations with timing validations
- Configurable iterations and concurrency
- Response time thresholds
- Create, Read, and List operations

## Deployment Scenarios

The generator creates comprehensive deployment tests including:

### Site-Group Scoped Deployment

1. Create configuration profile
2. Scope to site group
3. Target to site group
4. Deploy with specified method
5. Verify deployment status via REST API
6. Verify configuration on all NOS devices in site group via NOSAPI

### Device Scoped Deployment

1. Create configuration profile
2. Scope to specific device
3. Target to device
4. Deploy with specified method
5. Verify deployment status via REST API
6. Verify configuration on the specific NOS device via NOSAPI

## Extension Points

The library is designed for extensibility:

### Adding New Profile Types

1. Add new constant to `model.ProfileType`
2. Update detection logic in `rest.Parser.detectProfileType()`

### Adding New Test Categories

1. Add new constant to `model.TestCategory`
2. Implement generator function in `pkg/generator/`
3. Wire into `Generator.generateFeatureTestGroup()`

### Adding New Scope/Target Types

1. Add constants to `model.ScopeType` or `model.TargetType`
2. Update parsers and generators

### Adding New Deployment Methods

1. Add constant to `model.DeploymentMethod`
2. Update deployment test generation logic

## Testing

Run unit tests:

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./pkg/model
go test ./pkg/generator
```

## Development

### Code Structure

- **`pkg/model/`**: Core domain models, type-safe structs
- **`pkg/yang/`**: YANG parsing, constraint extraction
- **`pkg/spec/rest/`**: OpenAPI parsing for REST endpoints
- **`pkg/spec/nosapi/`**: NOSAPI endpoint parsing
- **`pkg/generator/`**: Test generation logic, modular by category
- **`pkg/yamlout/`**: YAML serialization, summary generation
- **`cmd/testgen/`**: CLI application, flag parsing, orchestration

### Adding New Generators

1. Create new file in `pkg/generator/` (e.g., `custom.go`)
2. Implement `generate<Category>Tests()` function
3. Add to category switch in `generateFeatureTestGroup()`

## Example: Generated Test Output

See [`examples/sample-output.yaml`](examples/sample-output.yaml) for a complete example showing:

- Site-group deployment test with NOSAPI verification
- Device deployment test with NOSAPI verification
- Boundary test for VLAN ID
- Negative test for missing required field
- Scale test for multiple instances
- Performance test with timing validation

## Troubleshooting

### YANG Parsing Issues

- Ensure YANG files use standard syntax
- Check for valid module and container declarations
- Verify file permissions on YANG directory

### OpenAPI Parsing Issues

- Validate OpenAPI spec with online validators
- Ensure spec is OpenAPI 3.0 format
- Check for proper schema definitions

### Empty Test Output

- Verify YANG files contain features
- Check that REST API spec has valid endpoints
- Ensure feature names match between YANG and REST spec

## Future Enhancements

- Support for more YANG constructs (choice, grouping, augment)
- Advanced constraint validation (when, must statements)
- Test execution framework integration
- CI/CD pipeline integration
- GraphQL API support
- gRPC/Protobuf support

## License

Copyright © 2025 Extreme Networks, Inc.

## Contact

For questions or issues, please contact the development team.
