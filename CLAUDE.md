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

## Resume Transfer Enhancement

### Overview
This document describes the implementation of resume transfer enhancement for Huawei Cloud OBS Go SDK, including pause, cancel, and resume operations.

### Key Changes

#### 1. Type Definitions (`obs/type.go`)
- Added `TransferStatus` enum type to track transfer states:
  - `TransferStatusPending` - Task is being prepared
  - `TransferStatusRunning` - Task is in progress
  - `TransferStatusPaused` - Task is paused
  - `TransferStatusCanceled` - Task is canceled
  - `TransferStatusCompleted` - Task is completed
  - `TransferStatusFailed` - Task failed

- Added `TransferController` interface for managing transfer tasks:
  - `Status() TransferStatus` - Get current task status
  - `Pause() error` - Pause task
  - `Cancel() error` - Cancel task
  - `Resume() error` - Resume task (only valid in paused state)
  - `Progress() (int, int, int64, int64)` - Get transfer progress (completed parts/total parts, transferred bytes/total bytes)

- Implemented `transferContext` struct to manage task state and progress:
  - Uses atomic operations for thread-safe state management
  - Tracks completed parts and transferred bytes
  - Manages pause/cancel flags

#### 2. Input Parameters (`obs/model_object.go`)
- Added `TransferCallback` field to `UploadFileInput` and `DownloadFileInput` structs:
  - Callback function to receive transfer progress updates
  - Parameters: completed parts, total parts, transferred bytes, total bytes, transfer status

#### 3. API Implementation (`obs/client_resume.go`)
- Added `CreateUploadTask` and `CreateDownloadTask` functions to create transfer controllers
- Implemented `uploadController` and `downloadController` structs to handle task execution and control
- Added `Start()` method to each controller to initiate transfer
- Modified existing `UploadFile` and `DownloadFile` functions to use new transfer context

#### 4. Transfer Logic (`obs/transfer.go`)
- Updated `resumeUpload` and `resumeDownload` functions to accept transfer context
- Modified `uploadPartConcurrent` and `downloadFileConcurrent` to handle pause and cancel logic
- Updated `uploadPartTask` and `downloadPartTask` to check pause flag during execution
- Added progress tracking using ProgressListener

#### 5. Testing (`tests/resume_transfer_test.go`)
- Created comprehensive test file for resume transfer enhancement
- Included test cases for:
  - Normal upload/download
  - Pause/resume operations
  - Cancel operations
  - Progress tracking
  - Parallel transfers
  - Performance testing
  - Data reliability testing

### Usage Examples

#### Upload with Pause/Resume
```go
input := &obs.UploadFileInput{
    Bucket: "your-bucket",
    Key: "large-file.txt",
    UploadFile: "local-file.txt",
    EnableCheckpoint: true,
    TaskNum: 3,
}

controller, err := client.CreateUploadTask(input)
if err != nil {
    fmt.Println("Error creating upload task:", err)
    return
}

// Start upload in goroutine
var wg sync.WaitGroup
wg.Add(1)
go func() {
    defer wg.Done()
    _, err = controller.Start()
    if err != nil {
        fmt.Println("Upload error:", err)
    } else {
        fmt.Println("Upload completed successfully")
    }
}()

// Pause after 2 seconds
time.Sleep(2 * time.Second)
controller.Pause()
fmt.Println("Upload paused")

// Resume after 1 second
time.Sleep(1 * time.Second)
controller.Resume()
fmt.Println("Upload resumed")

// Wait for completion
wg.Wait()
```

#### Download with Progress Tracking
```go
input := &obs.DownloadFileInput{
    Bucket: "your-bucket",
    Key: "large-file.txt",
    DownloadFile: "local-file.txt",
    EnableCheckpoint: true,
    TaskNum: 3,
    TransferCallback: func(completed, total int, transferred, totalBytes int64, status obs.TransferStatus) {
        fmt.Printf("Status: %v, Progress: %d/%d parts, %d/%d bytes\n",
            status, completed, total, transferred, totalBytes)
    },
}

controller, err := client.CreateDownloadTask(input)
if err != nil {
    fmt.Println("Error creating download task:", err)
    return
}

_, err = controller.Start()
if err != nil {
    fmt.Println("Download error:", err)
} else {
    fmt.Println("Download completed successfully")
}
```

### Build and Test

To build the SDK with the new features:
```bash
go build ./...
```

To run the resume transfer tests:
```bash
cd tests
go test -run TestResumeTransfer -v
```
