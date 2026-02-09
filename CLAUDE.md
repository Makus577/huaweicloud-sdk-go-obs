# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is the Huawei Cloud OBS (Object Storage Service) Go SDK. It provides a Go client for interacting with Huawei Cloud's Object Storage Service.

## Build and Test

To build the SDK:
```bash
go build ./...
```

To run tests (if available):
```bash
go test ./...
```

To run the sample code:
```bash
cd main
go run obs_go_sample.go
```

## Package Structure

- `obs/` - Core SDK package containing all client functionality
- `main/` - Sample/demo code showing how to use the SDK
- `examples/` - Additional example code

## Core Architecture

The SDK is organized around the `ObsClient` type in `obs/client_base.go`. The main entry point is:

```go
obs.New(ak, sk, endpoint, configurers...)
```

### Key Components

**Configuration (`obs/conf.go`)**
- Uses functional options pattern with `With*` configurers (e.g., `WithSecurityToken`, `WithProxyUrl`, `WithPathStyle`)
- Handles URL parsing, timeout settings, SSL verification, and HTTP transport configuration
- Supports multiple signature types: V2, V4, and OBS-specific

**Security Providers (`obs/provider.go`)**
- `BasicSecurityProvider` - Standard AK/SK authentication
- `EnvSecurityProvider` - Reads credentials from environment variables (`OBS_ACCESS_KEY_ID`, `OBS_SECRET_ACCESS_KEY`, `OBS_SECURITY_TOKEN`)
- `EcsSecurityProvider` - Fetches temporary credentials from ECS metadata endpoint

**HTTP Layer (`obs/http.go`)**
- Handles request/response processing
- Implements retry logic and redirect handling
- Manages authentication via signature generation

**API Organization**
- `client_bucket.go` - Bucket operations (create, delete, list, etc.)
- `client_object.go` - Object operations (put, get, delete, etc.)
- `client_part.go` - Multipart upload operations
- `client_resume.go` - Resumable upload/download
- `temporary_*.go` - Temporary credentials and signed URL operations
- `trait_*.go` - Protocol-specific implementations (trait pattern)

**Models**
- `model_*.go` - Input/output types for API operations
- `convert.go` - Conversion between internal types and wire format

**Support**
- `auth.go`, `authV2.go`, `authV4.go` - Signature calculation
- `pool.go` - Connection pool management
- `transfer.go` - Data transfer utilities
- `util.go` - Common utilities
- `log.go` - Logging framework

## Signature Types

The SDK supports multiple signature types for compatibility:
- `SignatureObs` - OBS-specific signature
- `SignatureV2` - AWS signature V2
- `SignatureV4` - AWS signature V4 (default)

When using path-style access with OBS signature, it automatically falls back to V2.

## Access Modes

- **Virtual Hosting Style** (default): `https://bucket.endpoint`
- **Path Style**: `https://endpoint/bucket` (automatically enabled when endpoint is an IP address)

Path style can be explicitly set via `WithPathStyle(true)`.
