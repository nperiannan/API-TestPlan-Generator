# Requirements Specification - API Test Case Generator

## Document Information

- **Project**: API Test Case Generator for Network Management System
- **Version**: 1.0
- **Date**: December 24, 2025
- **Status**: Implementation Complete

## Executive Summary

Design and implement a modular Go library that auto-generates API test definitions (YAML format) for a network management system. The single source of truth is a set of YANG models, combined with REST API and NOSAPI definitions.

## 1. System Overview

### 1.1 Purpose

Generate comprehensive, YAML-based API test definitions that cover:
- Management API operations (REST)
- Network Operating System device verification (NOSAPI)
- Full deployment lifecycle testing
- Multiple test categories (functional, boundary, negative, scale, performance)

### 1.2 High-Level Context

- **Profile Types**: The system manages three main profile types:
  - Global Profile
  - Configuration Profile
  - Service Profile

- **Single Source of Truth**: YANG models define all features, parameters, and constraints

- **API Surfaces**:
  - **REST API**: Used by GUI for device configuration
  - **NOSAPI**: Used to read actual configuration from NOS devices

- **Configuration Lifecycle**: 
  - Configuration Profile is the scoped object
  - Handles device/NOS targeting
  - Manages conflict resolution
  - Deploys configuration
  - Pushes final config to devices

## 2. Data Source Requirements

### 2.1 YANG Models

**Location**: `C:\Natarajan\automation\PlatformCommonModels\ConfigState\etc\yang`

**Requirements**:
- Parse all `.yang` files in the directory
- Extract feature objects and their relationships
- Extract parameters with full type information
- Extract constraints (required, enums, ranges, patterns, list cardinality)
- Derive relationships between profiles, features, and devices
- Handle nested structures (containers, lists, leaf-lists)

**Must Extract**:
- Module names and namespaces
- Container definitions (feature objects)
- Leaf definitions (parameters)
- Leaf-list definitions (array parameters)
- Type information (string, int, bool, enum, etc.)
- Mandatory flags
- Default values
- Pattern constraints
- Min/max elements for lists
- Min/max values for numeric types
- Enumeration values

### 2.2 REST API Specification

**Location**: `C:\Natarajan\automation\PlatformServices\Configuration\src\configuration\infra\rest\openapi.yaml`

**Requirements**:
- Parse OpenAPI/Swagger specification (OpenAPI 3.0 format)
- Extract all HTTP operations (GET, POST, PUT, PATCH, DELETE)
- Extract path parameters and query parameters
- Extract request body schemas with nested properties
- Extract response schemas
- Identify operation types (create, read, update, delete, list, scope, target, deploy, status)
- Detect profile types from paths
- Identify deployment-related endpoints

**Must Extract**:
- HTTP method for each operation
- Full path template with parameters
- Request body schema (all properties, types, constraints)
- Response schema (all properties, types)
- Required vs optional fields
- Enum values from schemas
- Min/max, minLength/maxLength from schemas
- Nested object structures
- Array types with item schemas

### 2.3 NOSAPI Specification

**Location**: `C:\Users\nperiannan\Downloads\nos-openapi.yaml`

**Requirements**:
- Parse NOSAPI OpenAPI specification
- Extract endpoints for device configuration verification
- Identify device scope (site-group, device, global)
- Map NOSAPI endpoints to features for verification
- Extract response schemas for validation

**Must Extract**:
- GET endpoints for reading device configuration
- Path parameters (deviceId, siteGroupId, etc.)
- Response schemas showing device configuration structure
- Scope indicators (site-group vs device endpoints)

## 3. Deployment and Scoping Model

### 3.1 Deployment Scenarios

Tests must support **both deployment and non-deployment scenarios**.

### 3.2 Deployment Methods

- **Rolling**: Gradual deployment across devices
- **Immediate**: Deploy to all devices at once
- **Staged**: Deploy in defined stages

### 3.3 Scoping Strategies

- **Site Group Scope**: Configuration applies to all devices in a site group
- **Device Scope**: Configuration applies to a specific device

### 3.4 Targeting Strategies

- **Site Group Target**: Deploy to all devices in target site group
- **Device Target**: Deploy to specific target device

### 3.5 Required Test Scenarios

Generate at least one representative test for each scenario group:

**Scenario 1: Site Group Deployment**
```
Scope to site group → Target to same site group → Deploy to site group 
→ Verify deployment status → Verify NOS config on all devices in site group
```

**Scenario 2: Device Deployment**
```
Scope to device → Target to device → Deploy to device 
→ Verify deployment status → Verify NOS config on specific device
```

**Scenario 3: Non-Deployment**
```
Create/Update configuration profile → Verify in management database only
(No deployment or NOSAPI verification)
```

## 4. Test Categories

### 4.1 Functional Tests

**Requirements**:
- Valid create/update/delete flows
- Realistic request bodies derived from YANG constraints
- End-to-end configuration lifecycle for configuration profiles:
  - Create profile
  - Get profile (verify creation)
  - Scope profile (site-group or device)
  - Target profile
  - Deploy
  - Verify deployment status via management API
  - Verify effective configuration on NOS devices via NOSAPI
  - Delete configuration
  - Deploy cleanup
  - Verify deployment status
  - Verify NOS devices are cleaned up

**Variants**:
- Deployment tests (with NOSAPI verification)
- Non-deployment tests (management API only)

### 4.2 Boundary Tests

**Requirements**:
- Test parameter-level boundaries using YANG and API constraints
- String length boundaries (minLength, maxLength)
- Numeric boundaries (min, max)
- List/array boundaries (minItems, maxItems)
- Edge cases for nested objects
- Optional vs required field combinations
- Enum boundary values (first, last, middle values)

### 4.3 Negative Tests

**Requirements**:
- Invalid values for each parameter type
- Missing required fields (from YANG mandatory constraints)
- Constraint violations:
  - Pattern mismatches
  - Out-of-range values
  - Invalid enum values
  - List cardinality violations
- Invalid scope/target/deployment combinations
- Unauthorized operations
- Invalid state transitions

### 4.4 Scale Tests

**Requirements**:
- High count of feature objects (configurable scale factor)
- Many entries in lists/arrays
- Large payloads
- Deeply nested structures
- Multiple concurrent operations
- Configuration profiles with many features

**Configurable**:
- Scale factor (default: 100)
- Controls instance counts and list sizes

### 4.5 Performance Tests

**Requirements**:
- Repeated operations at scale
- Configurable iteration count (default: 10)
- Configurable concurrency level (default: 5)
- Response time validations
- Throughput measurements
- Support for external performance tooling integration

**Test Operations**:
- Create operations
- Read operations
- List operations
- Update operations
- Deployment operations

## 5. Test Definition Structure

### 5.1 Metadata Requirements

Every test case must include:

- **testCaseID**: Globally unique identifier
  - Format: `{PREFIX}_{NUMBER:04d}` (e.g., TCXM_1000)
  - Configurable prefix (default: TCXM)
  - Sequential numbering
  - No duplicates across entire suite

- **featureName**: Human-readable feature name from YANG/API

- **priority**: Test priority level
  - P0: Critical functionality
  - P1: Important functionality
  - P2: Standard functionality
  - P3: Nice-to-have functionality

- **type**: Test category
  - functional
  - boundary
  - negative
  - scale
  - performance

- **description**: Clear, concise description including:
  - What is being tested
  - Deployment vs non-deployment
  - Scope/target type if applicable
  - Expected outcome

- **scopeType** (for deployment tests): site-group | device

- **targetType** (for deployment tests): site-group | device

- **deploymentMethod** (for deployment tests): rolling | immediate | staged

- **isDeploymentTest**: Boolean flag

### 5.2 Test Step Structure

Each step must include:

- **name**: Step identifier
- **description**: What the step does
- **method**: HTTP method (GET, POST, PUT, PATCH, DELETE)
- **api**: API surface (REST or NOSAPI)
- **path**: API endpoint path with parameter placeholders
- **pathParams**: Map of path parameter values
- **queryParams**: Map of query parameter values
- **headers**: Map of HTTP headers
- **body**: Request body (object/map)
- **expectedStatus**: Expected HTTP status code
- **validations**: Array of validation checks
- **timeout**: Timeout in seconds (for long operations like deployment)
- **devicesScope**: For NOSAPI steps (siteGroup, device)

### 5.3 Validation Types

- **statusCode**: Verify HTTP status code
- **jsonPathEquals**: Verify JSON field value
- **jsonPathExists**: Verify JSON field exists
- **nosConfigMatchesExpected**: Verify NOS device config matches expected
- **responseTimeUnder**: Verify response time under threshold

### 5.4 YAML Output Structure

```yaml
version: "1.0"
generatedAt: "<timestamp>"
sourceYangDir: "<path>"
sourceRESTAPI: "<path>"
sourceNOSAPI: "<path>"
features:
  - featureName: "<name>"
    featurePath: "<api-path>"
    profileType: "configuration|global|service"
    description: "<description>"
    tests:
      functional:
        - testCaseID: "TCXM_XXXX"
          # ... test details
      boundary:
        - testCaseID: "TCXM_XXXX"
          # ... test details
      negative:
        - testCaseID: "TCXM_XXXX"
          # ... test details
      scale:
        - testCaseID: "TCXM_XXXX"
          # ... test details
      performance:
        - testCaseID: "TCXM_XXXX"
          # ... test details
```

## 6. Architecture Requirements

### 6.1 Package Structure

```
pkg/yang/           - YANG model parsing
pkg/spec/rest/      - REST API spec parsing
pkg/spec/nosapi/    - NOSAPI spec parsing
pkg/model/          - Domain models
pkg/generator/      - Test generation logic
pkg/yamlout/        - YAML serialization
cmd/testgen/        - CLI application
```

### 6.2 Modularity

Must be easy to extend for:
- New profile types
- New test categories
- New scope/target/deployment flavors
- New output formats
- New validation types

### 6.3 Type Safety

- Strong typing with Go structs
- Type-safe enums (constants)
- No magic strings in core logic
- Clear interfaces between components

### 6.4 Code Quality

- Production-quality, idiomatic Go
- Clear package boundaries
- Small, testable functions
- Unit tests for core functionality
- GoDoc comments on public APIs

## 7. CLI Requirements

### 7.1 Required Flags

- `--yang-dir <path>`: Path to YANG models directory (REQUIRED)
- `--rest-spec <path>`: Path to REST API spec file (REQUIRED)
- `--nosapi-spec <path>`: Path to NOSAPI spec file (REQUIRED)
- `--out-dir <path>`: Output directory (default: ./generated-tests)

### 7.2 Optional Flags

- `--include-categories <list>`: Test categories to generate
  - Default: functional,boundary,negative,scale,performance
  
- `--scope-types <list>`: Scope types to test
  - Default: site-group,device
  
- `--target-types <list>`: Target types to test
  - Default: site-group,device
  
- `--deployment-methods <list>`: Deployment methods to test
  - Default: rolling,immediate
  
- `--scale-factor <int>`: Scale factor for scale tests
  - Default: 100
  
- `--performance-iterations <int>`: Iterations for performance tests
  - Default: 10
  
- `--performance-concurrency <int>`: Concurrency for performance tests
  - Default: 5
  
- `--one-file-per-feature <bool>`: Output format
  - Default: true
  
- `--test-id-prefix <string>`: Test ID prefix
  - Default: TCXM
  
- `--starting-id-number <int>`: Starting test ID number
  - Default: 1000

### 7.3 CLI Behavior

When executed, the CLI must:
1. Parse YANG models from specified directory
2. Parse REST API spec from specified file
3. Parse NOSAPI spec from specified file
4. Build internal model of features, paths, constraints
5. Generate test definitions for each feature path
6. Assign unique test IDs (no duplicates)
7. Write YAML files to output directory
8. Generate summary report
9. Generate example test file
10. Display progress and results

## 8. Output Requirements

### 8.1 Generated Files

- **Individual feature files** (if one-file-per-feature):
  - `<feature-name>.yaml` for each feature
  - Contains all test categories for that feature

- **Single test suite** (if not one-file-per-feature):
  - `test-suite.yaml` with all tests

- **Summary file**:
  - `test-summary.txt`
  - Test counts by feature and category
  - Deployment test count
  - Overall statistics

- **Example file**:
  - `example-test.yaml`
  - Detailed example showing deployment test structure

### 8.2 File Organization

- Organized by feature
- Tests grouped by category
- Clear metadata in each test
- Valid YAML syntax
- Human-readable formatting

## 9. Constraint and Validation Requirements

### 9.1 From YANG Models

Extract and apply:
- `mandatory true` → Required field constraints
- `pattern "<regex>"` → Pattern validation constraints
- `type enumeration` → Enum value constraints
- `min-elements <n>` → Array minimum length
- `max-elements <n>` → Array maximum length
- `range "<min>..<max>"` → Numeric range constraints
- `length "<min>..<max>"` → String length constraints

### 9.2 From REST API Spec

Extract and apply:
- `required: [field1, field2]` → Required fields
- `enum: [val1, val2]` → Enum values
- `minimum: <n>` → Numeric minimum
- `maximum: <n>` → Numeric maximum
- `minLength: <n>` → String minimum length
- `maxLength: <n>` → String maximum length
- `minItems: <n>` → Array minimum items
- `maxItems: <n>` → Array maximum items
- `pattern: "<regex>"` → Pattern constraints

### 9.3 Constraint Application

- Use in boundary tests (test min/max values)
- Use in negative tests (violate constraints)
- Use in functional tests (ensure valid values)
- Validate no "functional" tests have invalid data

## 10. NOSAPI Verification Requirements

### 10.1 Integration Points

- Deployment tests must include NOSAPI verification steps
- NOSAPI steps come after deployment status verification
- NOSAPI steps verify actual device state

### 10.2 Site-Group Verification

- Use NOSAPI endpoint for site-group queries
- Verify configuration on all devices in the site group
- Path includes site group ID parameter

### 10.3 Device Verification

- Use NOSAPI endpoint for device queries
- Verify configuration on specific device
- Path includes device ID parameter

### 10.4 Verification Steps

Each NOSAPI verification step must:
- Use `api: NOSAPI`
- Specify GET method
- Include appropriate path and parameters
- Set `devicesScope` field
- Include validations for expected config state

## 11. Success Criteria

### 11.1 Functional Requirements ✅

- [x] Parse YANG models and extract all features, parameters, constraints
- [x] Parse REST API spec and extract all operations, schemas, constraints
- [x] Parse NOSAPI spec and extract verification endpoints
- [x] Generate functional tests with CRUD operations
- [x] Generate deployment tests for site-group scope
- [x] Generate deployment tests for device scope
- [x] Include NOSAPI verification in deployment tests
- [x] Generate boundary tests from constraints
- [x] Generate negative tests from constraints
- [x] Generate scale tests with configurable factor
- [x] Generate performance tests with timing validations
- [x] Assign unique test IDs to all tests
- [x] Output valid YAML with rich metadata
- [x] Generate summary and example files

### 11.2 Non-Functional Requirements ✅

- [x] Modular, extensible architecture
- [x] Production-quality Go code
- [x] Type-safe domain models
- [x] Unit tests for core functionality
- [x] Comprehensive documentation
- [x] CLI with all required flags
- [x] Example outputs demonstrating all features

### 11.3 Deliverables ✅

- [x] Complete Go library with all packages
- [x] CLI application with flag support
- [x] Unit tests
- [x] Example YAML output
- [x] README with usage instructions
- [x] Architecture documentation
- [x] Build and run scripts
- [x] Source file location reference

## 12. Future Enhancements

### 12.1 Phase 2 Potential Features

- Support for additional YANG constructs (choice, grouping, augment, uses)
- Advanced constraint validation (when, must statements)
- Test execution framework integration
- Test result reporting
- Coverage analysis
- GraphQL API support
- gRPC/Protobuf support
- Web UI for configuration
- Template system for custom test patterns
- Plugin architecture for custom generators

### 12.2 Integration Points

- CI/CD pipeline integration
- Test management system integration
- Performance monitoring tools
- Configuration management systems
- Device management platforms

## 13. Assumptions and Constraints

### 13.1 Assumptions

- YANG files follow standard YANG 1.0/1.1 syntax
- REST API specs are valid OpenAPI 3.0 format
- NOSAPI specs are valid OpenAPI 3.0 format
- File system access to all source files
- Go 1.21+ runtime environment

### 13.2 Known Limitations

- YANG parser handles basic constructs (may need enhancement for advanced features)
- Feature-to-endpoint mapping uses heuristics (may need manual refinement)
- Generated tests require execution framework (generator only creates definitions)
- NOSAPI verification steps are placeholders (actual validation logic needed in execution framework)

## 14. Notes

### 14.1 Source File Locations

- **YANG Models**: `C:\Natarajan\automation\PlatformCommonModels\ConfigState\etc\yang`
- **REST API Spec**: `C:\Natarajan\automation\PlatformServices\Configuration\src\configuration\infra\rest\openapi.yaml`
- **NOSAPI Spec**: `C:\Users\nperiannan\Downloads\nos-openapi.yaml`

### 14.2 Implementation Status

All requirements have been implemented and delivered. The system is ready for:
1. Execution against real source files
2. Validation of parsing accuracy
3. Refinement based on actual schema structures
4. Integration into development workflow

---

**End of Requirements Specification**
