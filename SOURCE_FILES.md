# Source File Locations

This document records the locations of source files used for test generation.

## YANG Models
**Location:** `C:\Natarajan\automation\PlatformCommonModels\ConfigState\etc\yang`

**Description:** YANG models that define all feature objects, parameters, constraints, and relationships for the network management system. These models are the single source of truth for:
- Feature objects and their parameters
- Type information and constraints (required fields, enums, ranges, patterns)
- Relationships between profiles, features, and devices

## REST API Specification
**Location:** `C:\Natarajan\automation\PlatformServices\Configuration\src\configuration\infra\rest\openapi.yaml`

**Description:** OpenAPI/Swagger specification defining the REST API endpoints for configuration management. This includes:
- HTTP methods and paths
- Request/response schemas
- Path parameters and query parameters
- Authentication requirements
- Deployment, scoping, and targeting operations

## NOSAPI Specification
**Location:** `C:\Users\nperiannan\Downloads\nos-openapi.yaml`

**Description:** NOSAPI (Network Operating System API) specification for reading configuration directly from NOS devices. Used for:
- Device-level configuration verification
- Final state validation after deployment
- Direct device state queries

## Usage

When running the test generator CLI, reference these locations:

```bash
./testgen \
  --yang-dir "C:\Natarajan\automation\PlatformCommonModels\ConfigState\etc\yang" \
  --rest-spec "C:\Natarajan\automation\PlatformServices\Configuration\src\configuration\infra\rest\openapi.yaml" \
  --nosapi-spec "C:\Users\nperiannan\Downloads\nos-openapi.yaml" \
  --out-dir "./generated-tests"
```

## Notes

- YANG directory contains multiple `.yang` files that need to be parsed
- REST API spec is in OpenAPI 3.0 format (YAML)
- NOSAPI spec is also in OpenAPI format (YAML)
