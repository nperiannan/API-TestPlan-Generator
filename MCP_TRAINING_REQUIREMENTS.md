# MCP Server Training Requirements
# API Test Case Generator - Knowledge Base for Model Training

**Document Purpose**: This file captures comprehensive requirements, patterns, and domain knowledge for training AI models or configuring local MCP servers to assist with API test case generation for network management systems.

**Version**: 1.0  
**Date**: December 24, 2025  
**Target Use**: AI Model Training, MCP Server Configuration, LLM Fine-tuning

---

## Table of Contents

1. [Domain Context](#domain-context)
2. [Data Sources and Parsing](#data-sources-and-parsing)
3. [Test Generation Patterns](#test-generation-patterns)
4. [Deployment and Scoping Model](#deployment-and-scoping-model)
5. [Test Case Structure](#test-case-structure)
6. [Code Patterns and Examples](#code-patterns-and-examples)
7. [Common Tasks and Solutions](#common-tasks-and-solutions)
8. [Error Patterns and Resolution](#error-patterns-and-resolution)
9. [Best Practices](#best-practices)

---

## 1. Domain Context

### 1.1 Project Overview

**What**: Automated API test case generator for network management systems  
**Input**: YANG models, REST API specs (OpenAPI), NOSAPI specs  
**Output**: YAML test definitions covering functional, boundary, negative, scale, and performance tests

### 1.2 Key Concepts

#### Profile Types
```yaml
Global Profile:
  - System-wide configurations
  - No device scoping needed
  - Applied globally across the network

Configuration Profile:
  - Device-specific configurations
  - Requires scoping (site-group or device)
  - Requires targeting for deployment
  - Can be deployed to actual devices

Service Profile:
  - Service-level configurations
  - Blueprint-based patterns
  - Deployment-capable
```

#### API Surfaces
```yaml
REST API:
  Purpose: "Management API used by GUI"
  Usage: "Create/update/delete configuration profiles"
  Base Path: "/api/v1/config"
  
NOSAPI:
  Purpose: "Network Operating System API"
  Usage: "Verify actual device configuration"
  Verification: "Read-only, confirms deployment worked"
  Base Path: "/api/nos/v1"
```

#### Configuration Lifecycle
```
1. Create Configuration Profile (REST API)
   ↓
2. Scope to Site-Group or Device (REST API)
   ↓
3. Target to Deployment Destinations (REST API)
   ↓
4. Deploy Configuration (REST API)
   ↓
5. Verify Deployment Status (REST API)
   ↓
6. Verify NOS Device Configuration (NOSAPI)
```

### 1.3 File Locations and Paths

```go
// Source file locations (used in requirements)
const (
    YANGModelsDir = "C:\\Natarajan\\automation\\PlatformCommonModels\\ConfigState\\etc\\yang"
    RESTAPISpec   = "C:\\Natarajan\\automation\\PlatformServices\\Configuration\\src\\configuration\\infra\\rest\\openapi.yaml"
    NOSAPISpec    = "C:\\Users\\nperiannan\\Downloads\\nos-openapi.yaml"
)

// Project structure
ProjectRoot/
├── cmd/testgen/           # CLI entry point
├── pkg/
│   ├── model/            # Domain models
│   ├── yang/             # YANG parser
│   ├── spec/
│   │   ├── rest/        # REST API parser
│   │   └── nosapi/      # NOSAPI parser
│   ├── generator/        # Test generators
│   └── yamlout/          # YAML output
└── generated-tests/      # Output directory
```

---

## 2. Data Sources and Parsing

### 2.1 YANG Model Parsing

#### What to Extract from YANG

```yang
// Example YANG module structure
module example-config {
    namespace "http://example.com/config";
    prefix "cfg";
    
    container vlan {                    // Extract: Feature name
        leaf vlanId {                   // Extract: Parameter name
            type uint16;                // Extract: Type
            mandatory true;             // Extract: Required constraint
        }
        
        leaf vlanName {
            type string {
                length "1..32";         // Extract: Length constraint
                pattern "[a-zA-Z0-9-]+"; // Extract: Pattern constraint
            }
        }
        
        leaf-list allowedPorts {        // Extract: Array type
            type string;
            min-elements 1;             // Extract: Min items constraint
            max-elements 48;            // Extract: Max items constraint
        }
    }
}
```

#### YANG Extraction Patterns

```go
// Pattern: Extract from YANG and map to constraints
type YANGConstraints struct {
    Mandatory    bool             // from "mandatory true"
    Pattern      string           // from "pattern"
    MinElements  int              // from "min-elements"
    MaxElements  int              // from "max-elements"
    Range        string           // from "range"
    Length       string           // from "length"
    EnumValues   []string         // from "type enumeration"
    DefaultValue interface{}      // from "default"
}

// Type mappings
YANGTypeToGoType = map[string]string{
    "string":           "string",
    "int8":             "int8",
    "int16":            "int16",
    "int32":            "int32",
    "int64":            "int64",
    "uint8":            "uint8",
    "uint16":           "uint16",
    "uint32":           "uint32",
    "uint64":           "uint64",
    "boolean":          "bool",
    "enumeration":      "string",  // with enum constraints
    "decimal64":        "float64",
    "inet:ipv4-address": "string", // with pattern constraint
}
```

### 2.2 REST API Spec Parsing

#### OpenAPI Extraction Patterns

```yaml
# Example OpenAPI structure to parse
paths:
  /api/v1/config/vlans:
    post:
      operationId: createVlan              # Extract: Operation name
      requestBody:
        content:
          application/json:
            schema:
              properties:
                vlanId:                     # Extract: Field name
                  type: integer             # Extract: Type
                  minimum: 1                # Extract: Constraint
                  maximum: 4094             # Extract: Constraint
                vlanName:
                  type: string
                  minLength: 1              # Extract: Constraint
                  maxLength: 32             # Extract: Constraint
                  pattern: "^[a-zA-Z0-9-]+$" # Extract: Constraint
              required:                     # Extract: Required fields
                - vlanId
                - vlanName
      responses:
        201:
          description: Created             # Extract: Success status
```

#### REST API Operation Types

```go
// Pattern: Detect operation type from path and method
type OperationType string

const (
    OperationCreate     OperationType = "create"     // POST to collection
    OperationRead       OperationType = "read"       // GET with ID
    OperationUpdate     OperationType = "update"     // PUT/PATCH with ID
    OperationDelete     OperationType = "delete"     // DELETE with ID
    OperationList       OperationType = "list"       // GET collection
    OperationScope      OperationType = "scope"      // POST to /scope
    OperationTarget     OperationType = "target"     // POST to /target
    OperationDeploy     OperationType = "deploy"     // POST to /deploy
    OperationStatus     OperationType = "status"     // GET /status
)

// Detection heuristics
func DetectOperationType(method, path string) OperationType {
    if strings.Contains(path, "/scope") && method == "POST" {
        return OperationScope
    }
    if strings.Contains(path, "/target") && method == "POST" {
        return OperationTarget
    }
    if strings.Contains(path, "/deploy") && method == "POST" {
        return OperationDeploy
    }
    // ... more patterns
}
```

### 2.3 NOSAPI Spec Parsing

#### NOSAPI Pattern Recognition

```yaml
# NOSAPI endpoint patterns
/api/nos/v1/site-groups/{siteGroupId}/vlans:
  get:
    # Pattern: Site-group scoped verification
    # Extract: Scope type = site-group
    # Use for: Verifying all devices in site group

/api/nos/v1/devices/{deviceId}/vlans:
  get:
    # Pattern: Device scoped verification
    # Extract: Scope type = device
    # Use for: Verifying specific device
```

```go
// Pattern: Map NOSAPI endpoints to features
type NOSAPIEndpoint struct {
    Path         string           // "/api/nos/v1/devices/{deviceId}/vlans"
    Method       string           // "GET"
    FeatureName  string           // "vlan"
    ScopeType    string           // "device" or "site-group"
    PathParams   []string         // ["deviceId"]
}
```

---

## 3. Test Generation Patterns

### 3.1 Test Categories and Their Purposes

```yaml
Functional Tests:
  Purpose: "Verify normal CRUD operations and deployment workflows"
  Includes:
    - Create with valid data
    - Read to verify creation
    - Update with valid changes
    - Delete and verify removal
    - Full deployment lifecycle (for config profiles)
  
Boundary Tests:
  Purpose: "Test edge cases and limits"
  Includes:
    - Minimum values (e.g., minLength=1)
    - Maximum values (e.g., maxLength=32)
    - Empty arrays (minItems=0)
    - Full arrays (maxItems=48)
    - First/last enum values
  
Negative Tests:
  Purpose: "Verify proper error handling"
  Includes:
    - Missing required fields
    - Invalid data types
    - Out-of-range values
    - Pattern violations
    - Constraint violations
  
Scale Tests:
  Purpose: "Test system under high load"
  Includes:
    - Many objects (scaleFactor=100)
    - Large payloads
    - Deep nesting
    - Long lists
  
Performance Tests:
  Purpose: "Measure response times"
  Includes:
    - Repeated operations (iterations=10)
    - Concurrent requests (concurrency=5)
    - Timing validations
```

### 3.2 Functional Test Pattern

```yaml
# Pattern: Standard CRUD functional test
testCaseID: "TCXM_1001"
featureName: "vlan"
priority: "P0"
type: "functional"
description: "Create, read, update, and delete VLAN configuration"
steps:
  - name: "create_vlan"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/vlans"
    body:
      vlanId: 100
      vlanName: "test-vlan"
    expectedStatus: 201
    validations:
      - type: "statusCode"
        value: 201
  
  - name: "get_vlan"
    method: "GET"
    api: "REST"
    path: "/api/v1/config/vlans/{id}"
    pathParams:
      id: "{{create_vlan.response.id}}"
    expectedStatus: 200
    validations:
      - type: "jsonPathEquals"
        path: "$.vlanId"
        value: 100
  
  - name: "delete_vlan"
    method: "DELETE"
    api: "REST"
    path: "/api/v1/config/vlans/{id}"
    pathParams:
      id: "{{create_vlan.response.id}}"
    expectedStatus: 204
```

### 3.3 Deployment Test Pattern

```yaml
# Pattern: Full deployment lifecycle test
testCaseID: "TCXM_2001"
featureName: "vlan"
priority: "P0"
type: "functional"
description: "Deploy VLAN configuration to site-group and verify on NOS devices"
isDeploymentTest: true
scopeType: "site-group"
targetType: "site-group"
deploymentMethod: "rolling"
steps:
  - name: "create_config_profile"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/profiles"
    body:
      name: "vlan-profile"
      features:
        vlan:
          vlanId: 100
          vlanName: "prod-vlan"
    expectedStatus: 201
  
  - name: "scope_to_site_group"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/profiles/{id}/scope"
    pathParams:
      id: "{{create_config_profile.response.id}}"
    body:
      scopeType: "site-group"
      siteGroupId: "site-123"
    expectedStatus: 200
  
  - name: "target_site_group"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/profiles/{id}/target"
    pathParams:
      id: "{{create_config_profile.response.id}}"
    body:
      targetType: "site-group"
      targetId: "site-123"
    expectedStatus: 200
  
  - name: "deploy_configuration"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/profiles/{id}/deploy"
    pathParams:
      id: "{{create_config_profile.response.id}}"
    body:
      deploymentMethod: "rolling"
    expectedStatus: 202
    timeout: 300
  
  - name: "verify_deployment_status"
    method: "GET"
    api: "REST"
    path: "/api/v1/config/profiles/{id}/deployment/status"
    pathParams:
      id: "{{create_config_profile.response.id}}"
    expectedStatus: 200
    validations:
      - type: "jsonPathEquals"
        path: "$.status"
        value: "completed"
  
  - name: "verify_nos_configuration"
    method: "GET"
    api: "NOSAPI"
    path: "/api/nos/v1/site-groups/{siteGroupId}/vlans"
    pathParams:
      siteGroupId: "site-123"
    devicesScope: "siteGroup"
    expectedStatus: 200
    validations:
      - type: "nosConfigMatchesExpected"
        expected:
          vlanId: 100
          vlanName: "prod-vlan"
```

### 3.4 Boundary Test Pattern

```yaml
# Pattern: Test minimum/maximum constraints
testCaseID: "TCXM_3001"
featureName: "vlan"
priority: "P1"
type: "boundary"
description: "Test VLAN ID at minimum boundary (1)"
steps:
  - name: "create_vlan_min_id"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/vlans"
    body:
      vlanId: 1              # Minimum value from constraint
      vlanName: "min-vlan"
    expectedStatus: 201

---
testCaseID: "TCXM_3002"
featureName: "vlan"
priority: "P1"
type: "boundary"
description: "Test VLAN ID at maximum boundary (4094)"
steps:
  - name: "create_vlan_max_id"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/vlans"
    body:
      vlanId: 4094           # Maximum value from constraint
      vlanName: "max-vlan"
    expectedStatus: 201
```

### 3.5 Negative Test Pattern

```yaml
# Pattern: Test constraint violation
testCaseID: "TCXM_4001"
featureName: "vlan"
priority: "P1"
type: "negative"
description: "Attempt to create VLAN with ID below minimum (expect failure)"
steps:
  - name: "create_vlan_invalid_low"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/vlans"
    body:
      vlanId: 0              # Below minimum (1)
      vlanName: "invalid-vlan"
    expectedStatus: 400      # Bad request
    validations:
      - type: "statusCode"
        value: 400
      - type: "jsonPathExists"
        path: "$.error"

---
testCaseID: "TCXM_4002"
featureName: "vlan"
priority: "P1"
type: "negative"
description: "Attempt to create VLAN without required field (expect failure)"
steps:
  - name: "create_vlan_missing_required"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/vlans"
    body:
      vlanId: 100
      # Missing vlanName (required field)
    expectedStatus: 400
    validations:
      - type: "statusCode"
        value: 400
```

---

## 4. Deployment and Scoping Model

### 4.1 Scoping and Targeting Matrix

```yaml
Deployment Scenarios:

Scenario 1: Site-Group to Site-Group
  Scope: site-group
  Target: site-group (same as scope)
  Deploy: Rolling deployment across all devices in site-group
  Verify: NOSAPI check on all devices in site-group
  Use Case: "Deploy configuration to entire data center"

Scenario 2: Device to Device
  Scope: device
  Target: device (same as scope)
  Deploy: Immediate deployment to single device
  Verify: NOSAPI check on specific device
  Use Case: "Deploy configuration to single switch"

Scenario 3: Non-Deployment
  Scope: none
  Target: none
  Deploy: none
  Verify: REST API only (no NOSAPI)
  Use Case: "Stage configuration without deploying"
```

### 4.2 Deployment Method Characteristics

```yaml
Rolling Deployment:
  Description: "Gradual deployment across devices"
  Use Case: "Minimize risk, deploy to subset at a time"
  Timeout: 300 seconds
  Status Checks: Poll every 10 seconds

Immediate Deployment:
  Description: "Deploy to all devices at once"
  Use Case: "Emergency changes, small device count"
  Timeout: 120 seconds
  Status Checks: Poll every 5 seconds

Staged Deployment:
  Description: "Deploy in defined stages"
  Use Case: "Complex multi-phase rollouts"
  Timeout: 600 seconds
  Status Checks: Poll every 15 seconds
```

---

## 5. Test Case Structure

### 5.1 Test Metadata Template

```yaml
# Every test must include these fields
testCaseID: "TCXM_XXXX"           # Format: PREFIX_NUMBER (4 digits)
featureName: "feature-name"        # Human-readable feature name
priority: "P0|P1|P2|P3"           # P0=Critical, P1=Important, P2=Standard, P3=Nice-to-have
type: "functional|boundary|negative|scale|performance"
description: "Clear description of what is being tested"

# Optional fields (for deployment tests)
isDeploymentTest: true|false
scopeType: "site-group|device"
targetType: "site-group|device"
deploymentMethod: "rolling|immediate|staged"

# Test steps array
steps:
  - name: "step_identifier"
    description: "What this step does"
    method: "GET|POST|PUT|PATCH|DELETE"
    api: "REST|NOSAPI"
    path: "/api/path/{param}"
    pathParams:
      param: "value"
    queryParams:
      query: "value"
    headers:
      Content-Type: "application/json"
    body: {}
    expectedStatus: 200
    validations: []
    timeout: 30
    devicesScope: "siteGroup|device"  # For NOSAPI only
```

### 5.2 Validation Types and Examples

```yaml
Validation Type: statusCode
  Example:
    - type: "statusCode"
      value: 201

Validation Type: jsonPathEquals
  Example:
    - type: "jsonPathEquals"
      path: "$.vlanId"
      value: 100

Validation Type: jsonPathExists
  Example:
    - type: "jsonPathExists"
      path: "$.id"

Validation Type: nosConfigMatchesExpected
  Example:
    - type: "nosConfigMatchesExpected"
      expected:
        vlanId: 100
        vlanName: "test-vlan"

Validation Type: responseTimeUnder
  Example:
    - type: "responseTimeUnder"
      milliseconds: 1000
```

### 5.3 Variable Reference Pattern

```yaml
# Pattern: Reference previous step response
steps:
  - name: "create_resource"
    method: "POST"
    path: "/api/v1/resources"
    body:
      name: "test"
    # Response: { "id": "abc-123", "name": "test" }
  
  - name: "get_resource"
    method: "GET"
    path: "/api/v1/resources/{id}"
    pathParams:
      id: "{{create_resource.response.id}}"  # Reference: {{step_name.response.field}}
    
  - name: "update_resource"
    method: "PUT"
    path: "/api/v1/resources/{id}"
    pathParams:
      id: "{{create_resource.response.id}}"  # Reuse same reference
    body:
      name: "updated-test"
```

---

## 6. Code Patterns and Examples

### 6.1 Domain Model Structures

```go
// Core test structure
type TestCase struct {
    TestCaseID       string            `yaml:"testCaseID"`
    FeatureName      string            `yaml:"featureName"`
    Priority         string            `yaml:"priority"`
    Type             string            `yaml:"type"`
    Description      string            `yaml:"description"`
    IsDeploymentTest bool              `yaml:"isDeploymentTest,omitempty"`
    ScopeType        string            `yaml:"scopeType,omitempty"`
    TargetType       string            `yaml:"targetType,omitempty"`
    DeploymentMethod string            `yaml:"deploymentMethod,omitempty"`
    Steps            []TestStep        `yaml:"steps"`
    Metadata         map[string]string `yaml:"metadata,omitempty"`
}

type TestStep struct {
    Name            string                 `yaml:"name"`
    Description     string                 `yaml:"description"`
    Method          string                 `yaml:"method"`
    API             string                 `yaml:"api"`
    Path            string                 `yaml:"path"`
    PathParams      map[string]string      `yaml:"pathParams,omitempty"`
    QueryParams     map[string]string      `yaml:"queryParams,omitempty"`
    Headers         map[string]string      `yaml:"headers,omitempty"`
    Body            map[string]interface{} `yaml:"body,omitempty"`
    ExpectedStatus  int                    `yaml:"expectedStatus"`
    Validations     []Validation           `yaml:"validations,omitempty"`
    Timeout         int                    `yaml:"timeout,omitempty"`
    DevicesScope    string                 `yaml:"devicesScope,omitempty"`
}

type Validation struct {
    Type     string                 `yaml:"type"`
    Path     string                 `yaml:"path,omitempty"`
    Value    interface{}            `yaml:"value,omitempty"`
    Expected map[string]interface{} `yaml:"expected,omitempty"`
}
```

### 6.2 Feature Model Structures

```go
type Feature struct {
    Name            string                  `yaml:"name"`
    Path            string                  `yaml:"path"`
    ProfileType     string                  `yaml:"profileType"`
    Description     string                  `yaml:"description"`
    Parameters      []Parameter             `yaml:"parameters"`
    Operations      map[string]Operation    `yaml:"operations"`
    NOSAPIEndpoints []NOSAPIEndpoint        `yaml:"nosapiEndpoints"`
    Constraints     FeatureConstraints      `yaml:"constraints"`
}

type Parameter struct {
    Name        string         `yaml:"name"`
    Type        string         `yaml:"type"`
    Description string         `yaml:"description"`
    Required    bool           `yaml:"required"`
    Constraints Constraints    `yaml:"constraints"`
}

type Constraints struct {
    Pattern      string        `yaml:"pattern,omitempty"`
    MinValue     *int          `yaml:"minValue,omitempty"`
    MaxValue     *int          `yaml:"maxValue,omitempty"`
    MinLength    *int          `yaml:"minLength,omitempty"`
    MaxLength    *int          `yaml:"maxLength,omitempty"`
    MinItems     *int          `yaml:"minItems,omitempty"`
    MaxItems     *int          `yaml:"maxItems,omitempty"`
    EnumValues   []string      `yaml:"enumValues,omitempty"`
    DefaultValue interface{}   `yaml:"defaultValue,omitempty"`
}
```

### 6.3 Test ID Generation Pattern

```go
// Pattern: Ensure unique test IDs across entire suite
type TestIDGenerator struct {
    prefix      string
    nextID      int
    usedIDs     map[string]bool
}

func NewTestIDGenerator(prefix string, startingID int) *TestIDGenerator {
    return &TestIDGenerator{
        prefix:  prefix,
        nextID:  startingID,
        usedIDs: make(map[string]bool),
    }
}

func (g *TestIDGenerator) NextID() string {
    for {
        id := fmt.Sprintf("%s_%04d", g.prefix, g.nextID)
        g.nextID++
        
        if !g.usedIDs[id] {
            g.usedIDs[id] = true
            return id
        }
    }
}

// Usage in generators
idGen := NewTestIDGenerator("TCXM", 1000)
testCase.TestCaseID = idGen.NextID()  // "TCXM_1000"
testCase.TestCaseID = idGen.NextID()  // "TCXM_1001"
```

### 6.4 Constraint Merging Pattern

```go
// Pattern: Merge constraints from YANG and REST API
func MergeConstraints(yangConstraints, apiConstraints Constraints) Constraints {
    merged := Constraints{}
    
    // Pattern is more specific in API, use it if present
    if apiConstraints.Pattern != "" {
        merged.Pattern = apiConstraints.Pattern
    } else {
        merged.Pattern = yangConstraints.Pattern
    }
    
    // For numeric constraints, use most restrictive
    merged.MinValue = maxInt(yangConstraints.MinValue, apiConstraints.MinValue)
    merged.MaxValue = minInt(yangConstraints.MaxValue, apiConstraints.MaxValue)
    
    // For string length, use most restrictive
    merged.MinLength = maxInt(yangConstraints.MinLength, apiConstraints.MinLength)
    merged.MaxLength = minInt(yangConstraints.MaxLength, apiConstraints.MaxLength)
    
    // For arrays, use most restrictive
    merged.MinItems = maxInt(yangConstraints.MinItems, apiConstraints.MinItems)
    merged.MaxItems = minInt(yangConstraints.MaxItems, apiConstraints.MaxItems)
    
    // Enum values: intersection of both (if both defined)
    if len(yangConstraints.EnumValues) > 0 && len(apiConstraints.EnumValues) > 0 {
        merged.EnumValues = intersection(yangConstraints.EnumValues, apiConstraints.EnumValues)
    } else if len(apiConstraints.EnumValues) > 0 {
        merged.EnumValues = apiConstraints.EnumValues
    } else {
        merged.EnumValues = yangConstraints.EnumValues
    }
    
    return merged
}
```

---

## 7. Common Tasks and Solutions

### 7.1 Task: Generate Boundary Tests for a Parameter

```go
// Solution pattern
func GenerateBoundaryTests(param Parameter, feature Feature) []TestCase {
    tests := []TestCase{}
    
    // Test minimum value
    if param.Constraints.MinValue != nil {
        tests = append(tests, TestCase{
            TestCaseID:  idGen.NextID(),
            FeatureName: feature.Name,
            Priority:    "P1",
            Type:        "boundary",
            Description: fmt.Sprintf("Test %s at minimum value (%d)", param.Name, *param.Constraints.MinValue),
            Steps: []TestStep{
                {
                    Name:   "test_minimum_value",
                    Method: "POST",
                    Path:   feature.Path,
                    Body: map[string]interface{}{
                        param.Name: *param.Constraints.MinValue,
                    },
                    ExpectedStatus: 201,
                },
            },
        })
    }
    
    // Test maximum value
    if param.Constraints.MaxValue != nil {
        tests = append(tests, TestCase{
            TestCaseID:  idGen.NextID(),
            FeatureName: feature.Name,
            Priority:    "P1",
            Type:        "boundary",
            Description: fmt.Sprintf("Test %s at maximum value (%d)", param.Name, *param.Constraints.MaxValue),
            Steps: []TestStep{
                {
                    Name:   "test_maximum_value",
                    Method: "POST",
                    Path:   feature.Path,
                    Body: map[string]interface{}{
                        param.Name: *param.Constraints.MaxValue,
                    },
                    ExpectedStatus: 201,
                },
            },
        })
    }
    
    // Similar patterns for string length, array size, etc.
    
    return tests
}
```

### 7.2 Task: Generate Negative Tests for Missing Required Fields

```go
// Solution pattern
func GenerateNegativeTestsMissingRequired(feature Feature) []TestCase {
    tests := []TestCase{}
    
    requiredParams := []Parameter{}
    for _, param := range feature.Parameters {
        if param.Required {
            requiredParams = append(requiredParams, param)
        }
    }
    
    // Generate test for each missing required field
    for _, missingParam := range requiredParams {
        body := buildValidBody(feature)
        delete(body, missingParam.Name)  // Remove the required field
        
        tests = append(tests, TestCase{
            TestCaseID:  idGen.NextID(),
            FeatureName: feature.Name,
            Priority:    "P1",
            Type:        "negative",
            Description: fmt.Sprintf("Attempt to create %s without required field '%s' (expect 400)", feature.Name, missingParam.Name),
            Steps: []TestStep{
                {
                    Name:           "test_missing_required_field",
                    Method:         "POST",
                    Path:           feature.Path,
                    Body:           body,
                    ExpectedStatus: 400,
                    Validations: []Validation{
                        {Type: "statusCode", Value: 400},
                        {Type: "jsonPathExists", Path: "$.error"},
                    },
                },
            },
        })
    }
    
    return tests
}
```

### 7.3 Task: Build Valid Request Body from Constraints

```go
// Solution pattern
func BuildValidRequestBody(feature Feature) map[string]interface{} {
    body := make(map[string]interface{})
    
    for _, param := range feature.Parameters {
        if param.Required || shouldIncludeOptional() {
            body[param.Name] = generateValidValue(param)
        }
    }
    
    return body
}

func generateValidValue(param Parameter) interface{} {
    switch param.Type {
    case "string":
        if len(param.Constraints.EnumValues) > 0 {
            return param.Constraints.EnumValues[0]  // First enum value
        }
        if param.Constraints.Pattern != "" {
            return generateStringMatchingPattern(param.Constraints.Pattern)
        }
        length := 10
        if param.Constraints.MinLength != nil {
            length = *param.Constraints.MinLength
        }
        return generateRandomString(length)
        
    case "int", "int32", "int64":
        if param.Constraints.MinValue != nil {
            return *param.Constraints.MinValue + 1  // Slightly above minimum
        }
        return 1
        
    case "bool":
        return true
        
    case "array":
        // Generate array with minimum items (or 1 if no constraint)
        minItems := 1
        if param.Constraints.MinItems != nil {
            minItems = *param.Constraints.MinItems
        }
        arr := []interface{}{}
        for i := 0; i < minItems; i++ {
            arr = append(arr, generateValidValue(param.ArrayItemType))
        }
        return arr
        
    default:
        return nil
    }
}
```

### 7.4 Task: Detect if Feature Supports Deployment

```go
// Solution pattern
func SupportsDeployment(feature Feature) bool {
    // Check if feature has deployment-related operations
    hasScope := false
    hasTarget := false
    hasDeploy := false
    
    for opType := range feature.Operations {
        if opType == "scope" {
            hasScope = true
        }
        if opType == "target" {
            hasTarget = true
        }
        if opType == "deploy" {
            hasDeploy = true
        }
    }
    
    // Must have all three for deployment support
    return hasScope && hasTarget && hasDeploy
}

// Check if feature has NOSAPI verification endpoints
func HasNOSAPIVerification(feature Feature) bool {
    return len(feature.NOSAPIEndpoints) > 0
}

// Determine if should generate deployment tests
func ShouldGenerateDeploymentTests(feature Feature) bool {
    return SupportsDeployment(feature) && HasNOSAPIVerification(feature)
}
```

---

## 8. Error Patterns and Resolution

### 8.1 Common Parsing Errors

```yaml
Error: "Failed to parse YANG file"
Cause: Invalid YANG syntax or unsupported constructs
Resolution:
  - Validate YANG files with pyang
  - Check for unsupported constructs (choice, grouping, augment)
  - Enhance parser for new constructs if needed

Error: "Failed to parse OpenAPI spec"
Cause: Invalid OpenAPI 3.0 format
Resolution:
  - Validate OpenAPI with swagger-cli
  - Ensure all $ref references resolve
  - Check for required fields in schemas

Error: "Duplicate test ID generated"
Cause: Test ID generator collision or manual ID reuse
Resolution:
  - Use TestIDGenerator with tracking
  - Never manually assign test IDs
  - Ensure sequential numbering
```

### 8.2 Common Generation Errors

```yaml
Error: "No valid value for required field"
Cause: Constraints too restrictive or contradictory
Resolution:
  - Log constraint details for debugging
  - Use constraint merging carefully
  - Provide fallback default values

Error: "Missing NOSAPI endpoint for feature"
Cause: NOSAPI spec doesn't include verification endpoint
Resolution:
  - Skip deployment tests for this feature
  - Log warning about missing verification
  - Generate non-deployment tests only

Error: "Operation type detection failed"
Cause: Ambiguous path/method combination
Resolution:
  - Add more detection heuristics
  - Use explicit operation type hints in spec
  - Default to generic operation type
```

---

## 9. Best Practices

### 9.1 Test Generation Best Practices

```yaml
1. Always Generate Valid Data for Functional Tests:
   - Use constraints to generate valid values
   - Never use placeholder or invalid data
   - Ensure all required fields are present

2. Prioritize Tests Appropriately:
   - P0: CRUD operations, deployment workflows
   - P1: Boundary conditions, key negative cases
   - P2: Advanced scenarios, edge cases
   - P3: Nice-to-have coverage

3. Keep Test Descriptions Clear:
   - State what is being tested
   - Indicate expected outcome
   - Mention deployment/scope context if applicable

4. Use Proper HTTP Status Codes:
   - 200: Successful GET/PUT/PATCH
   - 201: Successful POST (creation)
   - 204: Successful DELETE
   - 400: Bad request (validation errors)
   - 404: Not found
   - 409: Conflict
   - 500: Server error

5. Include Comprehensive Validations:
   - Always validate status code
   - Validate key response fields
   - Validate deployment status for deployment tests
   - Validate NOSAPI data for deployed configs
```

### 9.2 Code Organization Best Practices

```go
// 1. Separate concerns into packages
pkg/yang/       // YANG parsing only
pkg/spec/rest/  // REST API parsing only
pkg/generator/  // Test generation only
pkg/model/      // Domain models only

// 2. Use small, focused functions
func GenerateFunctionalTests(feature Feature) []TestCase {
    tests := []TestCase{}
    tests = append(tests, generateCRUDTests(feature)...)
    if ShouldGenerateDeploymentTests(feature) {
        tests = append(tests, generateDeploymentTests(feature)...)
    }
    return tests
}

// 3. Make functions testable
func generateCRUDTests(feature Feature) []TestCase {
    // No external dependencies
    // Pure function based on inputs
    // Easy to unit test
}

// 4. Use constants for magic strings
const (
    ProfileTypeGlobal        = "global"
    ProfileTypeConfiguration = "configuration"
    ProfileTypeService       = "service"
    
    TestTypeFunction    = "functional"
    TestTypeBoundary    = "boundary"
    TestTypeNegative    = "negative"
    TestTypeScale       = "scale"
    TestTypePerformance = "performance"
)
```

### 9.3 YAML Output Best Practices

```yaml
1. Consistent Formatting:
   - Use 2-space indentation
   - Quote strings with special characters
   - Use flow style for simple maps
   - Use block style for complex structures

2. Field Ordering:
   - Metadata first (testCaseID, featureName, etc.)
   - Configuration second (scopeType, deploymentMethod, etc.)
   - Steps last

3. Omit Empty Fields:
   - Use yaml:",omitempty" tags
   - Don't include null or empty values
   - Keep output clean and readable

4. Include Comments for Generated Files:
   version: "1.0"
   generatedAt: "2025-12-24T10:00:00Z"
   # Generated by API Test Case Generator
   # Source: YANG models + REST API + NOSAPI specs
```

---

## 10. CLI Usage Patterns

### 10.1 Basic Usage

```powershell
# Minimal usage (required flags only)
.\testgen.exe `
  --yang-dir "C:\path\to\yang" `
  --rest-spec "C:\path\to\openapi.yaml" `
  --nosapi-spec "C:\path\to\nos-openapi.yaml" `
  --out-dir "./generated-tests"

# Full usage (all options)
.\testgen.exe `
  --yang-dir "C:\path\to\yang" `
  --rest-spec "C:\path\to\openapi.yaml" `
  --nosapi-spec "C:\path\to\nos-openapi.yaml" `
  --out-dir "./generated-tests" `
  --include-categories "functional,boundary,negative" `
  --scope-types "site-group,device" `
  --target-types "site-group,device" `
  --deployment-methods "rolling,immediate" `
  --scale-factor 100 `
  --performance-iterations 10 `
  --performance-concurrency 5 `
  --one-file-per-feature true `
  --test-id-prefix "TCXM" `
  --starting-id-number 1000
```

### 10.2 Configuration Examples

```yaml
# Generate only functional tests
--include-categories "functional"

# Generate functional and boundary only
--include-categories "functional,boundary"

# Generate all test types
--include-categories "functional,boundary,negative,scale,performance"

# Test only site-group scope
--scope-types "site-group"

# Test both scope types
--scope-types "site-group,device"

# Use custom test ID prefix
--test-id-prefix "TC_VLAN"
# Results in: TC_VLAN_1000, TC_VLAN_1001, etc.

# Increase scale factor for stress testing
--scale-factor 1000
# Generates tests with 1000 objects instead of default 100
```

---

## 11. Example Workflows

### 11.1 Workflow: Adding a New Test Category

```go
// Step 1: Define constant
const TestTypeStability = "stability"

// Step 2: Add to CLI flags
includeCategories := flag.String("include-categories", 
    "functional,boundary,negative,scale,performance,stability", 
    "Test categories to generate")

// Step 3: Create generator function
func GenerateStabilityTests(feature Feature, config GeneratorConfig) []TestCase {
    tests := []TestCase{}
    
    // Generate long-running tests
    tests = append(tests, TestCase{
        TestCaseID:  config.IDGen.NextID(),
        FeatureName: feature.Name,
        Priority:    "P2",
        Type:        TestTypeStability,
        Description: fmt.Sprintf("Long-running stability test for %s", feature.Name),
        Steps: []TestStep{
            // Steps for stability testing
        },
    })
    
    return tests
}

// Step 4: Integrate into main generator
func (g *Generator) GenerateTests(feature Feature) []TestCase {
    tests := []TestCase{}
    
    if g.config.IncludeCategory("functional") {
        tests = append(tests, GenerateFunctionalTests(feature, g.config)...)
    }
    // ... other categories
    if g.config.IncludeCategory("stability") {
        tests = append(tests, GenerateStabilityTests(feature, g.config)...)
    }
    
    return tests
}
```

### 11.2 Workflow: Adding Support for New Profile Type

```go
// Step 1: Define constant
const ProfileTypeWireless = "wireless"

// Step 2: Update profile type detection
func DetectProfileType(path string) string {
    if strings.Contains(path, "/wireless/") {
        return ProfileTypeWireless
    }
    // ... other types
    return ProfileTypeGlobal
}

// Step 3: Add profile-specific logic
func GenerateWirelessSpecificTests(feature Feature) []TestCase {
    // Wireless profile may have specific deployment patterns
    // or unique verification requirements
    return []TestCase{}
}

// Step 4: Update feature processing
func ProcessFeature(feature Feature) {
    switch feature.ProfileType {
    case ProfileTypeWireless:
        // Wireless-specific processing
    case ProfileTypeConfiguration:
        // Configuration-specific processing
    default:
        // Default processing
    }
}
```

---

## 12. Training Data Examples

### 12.1 Example: Full Deployment Test

```yaml
# This is a comprehensive example showing all aspects of deployment testing
testCaseID: "TCXM_5001"
featureName: "port"
priority: "P0"
type: "functional"
description: "Deploy port configuration to device and verify on NOS"
isDeploymentTest: true
scopeType: "device"
targetType: "device"
deploymentMethod: "immediate"

steps:
  # Step 1: Create configuration profile
  - name: "create_port_config_profile"
    description: "Create a configuration profile with port settings"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/profiles"
    headers:
      Content-Type: "application/json"
    body:
      name: "port-config-profile-001"
      profileType: "configuration"
      features:
        port:
          portId: "1/1/1"
          adminStatus: "enabled"
          speed: "10G"
          duplex: "full"
    expectedStatus: 201
    validations:
      - type: "statusCode"
        value: 201
      - type: "jsonPathExists"
        path: "$.id"
  
  # Step 2: Scope to device
  - name: "scope_to_device"
    description: "Scope the profile to a specific device"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/profiles/{profileId}/scope"
    pathParams:
      profileId: "{{create_port_config_profile.response.id}}"
    body:
      scopeType: "device"
      deviceId: "device-001"
    expectedStatus: 200
    validations:
      - type: "statusCode"
        value: 200
      - type: "jsonPathEquals"
        path: "$.scopeType"
        value: "device"
  
  # Step 3: Target device
  - name: "target_device"
    description: "Set deployment target to the same device"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/profiles/{profileId}/target"
    pathParams:
      profileId: "{{create_port_config_profile.response.id}}"
    body:
      targetType: "device"
      targetId: "device-001"
    expectedStatus: 200
    validations:
      - type: "statusCode"
        value: 200
  
  # Step 4: Deploy
  - name: "deploy_configuration"
    description: "Deploy the configuration to the device"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/profiles/{profileId}/deploy"
    pathParams:
      profileId: "{{create_port_config_profile.response.id}}"
    body:
      deploymentMethod: "immediate"
    expectedStatus: 202
    timeout: 120
    validations:
      - type: "statusCode"
        value: 202
      - type: "jsonPathExists"
        path: "$.deploymentId"
  
  # Step 5: Poll deployment status
  - name: "verify_deployment_status"
    description: "Verify deployment completed successfully"
    method: "GET"
    api: "REST"
    path: "/api/v1/config/profiles/{profileId}/deployment/status"
    pathParams:
      profileId: "{{create_port_config_profile.response.id}}"
    expectedStatus: 200
    validations:
      - type: "statusCode"
        value: 200
      - type: "jsonPathEquals"
        path: "$.status"
        value: "completed"
      - type: "jsonPathEquals"
        path: "$.success"
        value: true
  
  # Step 6: Verify on NOS device
  - name: "verify_nos_port_configuration"
    description: "Verify port configuration on the actual device"
    method: "GET"
    api: "NOSAPI"
    path: "/api/nos/v1/devices/{deviceId}/ports/{portId}"
    pathParams:
      deviceId: "device-001"
      portId: "1/1/1"
    devicesScope: "device"
    expectedStatus: 200
    validations:
      - type: "statusCode"
        value: 200
      - type: "nosConfigMatchesExpected"
        expected:
          portId: "1/1/1"
          adminStatus: "enabled"
          speed: "10G"
          duplex: "full"
  
  # Step 7: Cleanup - delete configuration
  - name: "delete_configuration"
    description: "Delete the configuration profile"
    method: "DELETE"
    api: "REST"
    path: "/api/v1/config/profiles/{profileId}"
    pathParams:
      profileId: "{{create_port_config_profile.response.id}}"
    expectedStatus: 204
    validations:
      - type: "statusCode"
        value: 204
  
  # Step 8: Deploy cleanup
  - name: "deploy_cleanup"
    description: "Deploy the deletion to remove config from device"
    method: "POST"
    api: "REST"
    path: "/api/v1/config/profiles/{profileId}/deploy"
    pathParams:
      profileId: "{{create_port_config_profile.response.id}}"
    body:
      deploymentMethod: "immediate"
    expectedStatus: 202
    timeout: 120
  
  # Step 9: Verify cleanup status
  - name: "verify_cleanup_deployment_status"
    description: "Verify cleanup deployment completed"
    method: "GET"
    api: "REST"
    path: "/api/v1/config/profiles/{profileId}/deployment/status"
    pathParams:
      profileId: "{{create_port_config_profile.response.id}}"
    expectedStatus: 200
    validations:
      - type: "jsonPathEquals"
        path: "$.status"
        value: "completed"
  
  # Step 10: Verify NOS device cleaned up
  - name: "verify_nos_port_removed"
    description: "Verify port configuration removed from device"
    method: "GET"
    api: "NOSAPI"
    path: "/api/nos/v1/devices/{deviceId}/ports/{portId}"
    pathParams:
      deviceId: "device-001"
      portId: "1/1/1"
    devicesScope: "device"
    expectedStatus: 404
    validations:
      - type: "statusCode"
        value: 404
```

---

## 13. Q&A for Common Scenarios

### Q: How do I generate tests for a feature with optional fields?

**A**: Include optional fields in some functional tests, exclude in others. For boundary/negative tests, focus on required fields primarily.

```go
// Example: Generate variant with all fields
bodyAllFields := buildRequestBodyWithAllFields(feature)
// Example: Generate variant with only required fields
bodyRequiredOnly := buildRequestBodyWithRequiredFieldsOnly(feature)
```

### Q: What if YANG and OpenAPI constraints conflict?

**A**: Use the most restrictive constraints. If YANG says max=100 and OpenAPI says max=50, use 50.

### Q: Should every feature have deployment tests?

**A**: No. Only features with:
1. Configuration profile type
2. Scope/target/deploy operations
3. NOSAPI verification endpoints

### Q: How many tests should be generated per feature?

**A**: Typical distribution:
- Functional: 3-5 tests (CRUD, deployment variants)
- Boundary: 5-10 tests (per parameter with constraints)
- Negative: 5-15 tests (missing fields, invalid values)
- Scale: 1-2 tests
- Performance: 1-2 tests

---

## 14. Glossary

```yaml
Configuration Profile:
  Definition: "A profile that defines device-specific configuration"
  Characteristics: "Requires scoping and targeting for deployment"
  
CRUD:
  Definition: "Create, Read, Update, Delete operations"
  Usage: "Standard API operations for resource management"
  
Deployment:
  Definition: "Process of pushing configuration to actual network devices"
  Steps: "Scope → Target → Deploy → Verify Status → Verify NOS"
  
Feature:
  Definition: "A configurable aspect of the network (e.g., VLAN, port, DHCP)"
  Source: "Defined in YANG models"
  
NOSAPI:
  Definition: "Network Operating System API"
  Purpose: "Read actual configuration from network devices"
  Usage: "Verification step in deployment tests"
  
Scope:
  Definition: "Define where configuration applies (site-group or device)"
  Example: "Scope this VLAN config to site-group-001"
  
Target:
  Definition: "Define where to deploy configuration"
  Example: "Target deployment to site-group-001"
  
YANG:
  Definition: "Data modeling language for network configurations"
  Usage: "Single source of truth for features and constraints"
```

---

**End of MCP Training Requirements Document**

This document should be used as a comprehensive reference for:
- Training AI models on the test generation domain
- Configuring MCP servers with domain knowledge
- Fine-tuning LLMs for code generation assistance
- Onboarding new developers to the project
- Understanding patterns and best practices

For questions or clarifications, refer to:
- [REQUIREMENTS_SPECIFICATION.md](REQUIREMENTS_SPECIFICATION.md) for detailed requirements
- [README.md](README.md) for usage instructions
- [SOURCE_FILES.md](SOURCE_FILES.md) for source code locations
