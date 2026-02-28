# 华为云OBS Go SDK 断点续传功能增强设计文档

## 1. 概述

当前华为云OBS Go SDK的`client_resume.go`文件中实现了基本的断点续传上传和下载功能，但缺少暂停、取消和继续等操作能力。本文档将设计这些增强功能，使SDK提供更完整的文件传输控制能力。

## 2. 问题分析

### 2.1 当前实现

当前`UploadFile`和`DownloadFile`函数通过以下方式实现断点续传：
- 使用检查点文件(`CheckpointFile`)记录分块上传/下载进度
- 并发执行分块任务，使用原子操作`atomic.CompareAndSwapInt32`控制任务中止
- 支持在程序崩溃或网络中断后从检查点恢复

### 2.2 功能缺陷

1. **缺乏暂停机制**：无法主动暂停正在进行的传输
2. **取消操作不完整**：当前`errAbort`错误只能终止任务，但无法提供明确的取消API
3. **状态管理缺失**：没有任务状态跟踪和查询能力
4. **继续操作不清晰**：从暂停状态恢复的操作流程不明确

## 3. 功能设计

### 3.1 核心概念

#### 3.1.1 任务状态管理

设计任务状态枚举：
```go
type TransferStatus int32

const (
    TransferStatusPending  TransferStatus = 0 // 任务正在准备中
    TransferStatusRunning  TransferStatus = 1 // 任务正在进行中
    TransferStatusPaused   TransferStatus = 2 // 任务已暂停
    TransferStatusCanceled TransferStatus = 3 // 任务已取消
    TransferStatusCompleted TransferStatus = 4 // 任务已完成
    TransferStatusFailed   TransferStatus = 5 // 任务失败
)
```

#### 3.1.2 任务控制器

设计任务控制器接口：
```go
type TransferController interface {
    // 获取当前任务状态
    Status() TransferStatus

    // 暂停任务
    Pause() error

    // 取消任务
    Cancel() error

    // 继续任务（仅在暂停状态下有效）
    Resume() error

    // 获取传输进度（已完成分块数/总块数，已传输字节数/总字节数）
    Progress() (int, int, int64, int64)
}
```

### 3.2 传输上下文设计

创建统一的传输上下文结构体，用于管理任务状态和控制操作：

```go
type transferContext struct {
    status     atomic.Value         // 任务状态 TransferStatus
    abort      int32               // 取消标志（原子操作）
    pause      int32               // 暂停标志（原子操作）
    totalParts int                 // 总块数
    completedParts int             // 已完成块数
    totalBytes int64               // 总字节数
    transferredBytes int64         // 已传输字节数
    partLock   sync.Mutex          // 分块进度更新锁
}

func newTransferContext(totalParts int, totalBytes int64) *transferContext {
    ctx := &transferContext{
        totalParts: totalParts,
        totalBytes: totalBytes,
    }
    ctx.status.Store(TransferStatusPending)
    return ctx
}

func (ctx *transferContext) Status() TransferStatus {
    return ctx.status.Load().(TransferStatus)
}

func (ctx *transferContext) setStatus(status TransferStatus) {
    ctx.status.Store(status)
}

func (ctx *transferContext) Pause() error {
    if ctx.Status() != TransferStatusRunning {
        return fmt.Errorf("can't pause transfer in status %v", ctx.Status())
    }
    atomic.CompareAndSwapInt32(&ctx.pause, 0, 1)
    ctx.setStatus(TransferStatusPaused)
    return nil
}

func (ctx *transferContext) Cancel() error {
    if ctx.Status() == TransferStatusCompleted || ctx.Status() == TransferStatusCanceled {
        return fmt.Errorf("can't cancel transfer in status %v", ctx.Status())
    }
    atomic.CompareAndSwapInt32(&ctx.abort, 0, 1)
    atomic.CompareAndSwapInt32(&ctx.pause, 0, 0) // 确保暂停标志被清除
    ctx.setStatus(TransferStatusCanceled)
    return nil
}

func (ctx *transferContext) Resume() error {
    if ctx.Status() != TransferStatusPaused {
        return fmt.Errorf("can't resume transfer in status %v", ctx.Status())
    }
    atomic.CompareAndSwapInt32(&ctx.pause, 1, 0)
    ctx.setStatus(TransferStatusRunning)
    return nil
}

func (ctx *transferContext) isPaused() bool {
    return atomic.LoadInt32(&ctx.pause) == 1
}

func (ctx *transferContext) isCanceled() bool {
    return atomic.LoadInt32(&ctx.abort) == 1
}

func (ctx *transferContext) incrementCompletedParts(partSize int64) {
    ctx.partLock.Lock()
    defer ctx.partLock.Unlock()
    ctx.completedParts++
    ctx.transferredBytes += partSize
}

func (ctx *transferContext) Progress() (int, int, int64, int64) {
    ctx.partLock.Lock()
    defer ctx.partLock.Unlock()
    return ctx.completedParts, ctx.totalParts, ctx.transferredBytes, ctx.totalBytes
}
```

### 3.3 API接口设计

#### 3.3.1 上传功能增强

修改`UploadFileInput`结构体：
```go
type UploadFileInput struct {
    ObjectOperationInput
    ContentType      string
    UploadFile       string
    PartSize         int64
    TaskNum          int
    EnableCheckpoint bool
    CheckpointFile   string
    EncodingType     string

    // 新增字段
    TransferCallback func(completedParts, totalParts int, transferredBytes, totalBytes int64, status TransferStatus)
}
```

新增API：
```go
// 创建上传任务（返回控制器）
func (obsClient ObsClient) CreateUploadTask(input *UploadFileInput, extensions ...extensionOptions) (TransferController, error)

// 上传任务控制器
type uploadController struct {
    obsClient        ObsClient
    input            *UploadFileInput
    ctx              *transferContext
    ufc              *UploadCheckpoint
    checkpointFile   string
    enableCheckpoint bool
    extensions       []extensionOptions
    resultChan       chan struct {
        output *CompleteMultipartUploadOutput
        err    error
    }
    running          bool
}

// 启动上传任务
func (c *uploadController) Start() (*CompleteMultipartUploadOutput, error)
```

#### 3.3.2 下载功能增强

修改`DownloadFileInput`结构体：
```go
type DownloadFileInput struct {
    GetObjectMetadataInput
    IfMatch           string
    IfNoneMatch       string
    IfModifiedSince   time.Time
    IfUnmodifiedSince time.Time
    DownloadFile      string
    PartSize          int64
    TaskNum           int
    EnableCheckpoint  bool
    CheckpointFile    string

    // 新增字段
    TransferCallback func(completedParts, totalParts int, transferredBytes, totalBytes int64, status TransferStatus)
}
```

新增API：
```go
// 创建下载任务（返回控制器）
func (obsClient ObsClient) CreateDownloadTask(input *DownloadFileInput, extensions ...extensionOptions) (TransferController, error)

// 下载任务控制器
type downloadController struct {
    obsClient        ObsClient
    input            *DownloadFileInput
    ctx              *transferContext
    dfc              *DownloadCheckpoint
    checkpointFile   string
    enableCheckpoint bool
    extensions       []extensionOptions
    objectInfo       *GetObjectMetadataOutput
    resultChan       chan struct {
        output *GetObjectMetadataOutput
        err    error
    }
    running          bool
}

// 启动下载任务
func (c *downloadController) Start() (*GetObjectMetadataOutput, error)
```

### 3.4 内部实现修改

#### 3.4.1 任务执行逻辑修改

在`transfer.go`中：

1. 修改`resumeUpload`和`resumeDownload`函数，接受`*transferContext`参数
2. 更新分块任务(`uploadPartTask`和`downloadPartTask`)，使其在执行过程中检查暂停标志
3. 优化任务调度逻辑，在暂停状态下停止调度新任务
4. 修改`uploadPartConcurrent`和`downloadFileConcurrent`函数，处理暂停和恢复场景

示例修改：
```go
func (task *uploadPartTask) Run() interface{} {
    if ctx.isCanceled() {
        return errAbort
    }
    if ctx.isPaused() {
        // 等待恢复或取消信号
        for ctx.isPaused() && !ctx.isCanceled() {
            time.Sleep(100 * time.Millisecond)
        }
        if ctx.isCanceled() {
            return errAbort
        }
    }

    // 原任务执行逻辑...
}
```


## 4. 实现步骤

### 4.1 阶段一：基础设施建设（预计1天）

1. 在`obs/type.go`中添加任务状态枚举类型
2. 创建任务控制器接口和上下文结构体
3. 实现传输上下文的核心功能

### 4.2 阶段二：API接口和输入参数修改（预计1天）

1. 修改`UploadFileInput`和`DownloadFileInput`结构体，添加回调字段
2. 创建新的API接口函数
3. 实现任务创建和启动方法

### 4.3 阶段三：内部实现修改（预计2天）

1. 修改`resumeUpload`和`resumeDownload`函数
2. 更新分块任务实现
3. 优化任务并发控制和状态管理
4. 增强检查点文件格式

### 4.4 阶段四：测试（预计1天）

1. 创建功能测试用例
2. 测试暂停/取消/继续场景
3. 验证检查点文件的完整性和恢复功能
4. 性能测试和优化

## 5. 使用示例

### 5.1 上传示例

```go
func main() {
    ak := "your-access-key"
    sk := "your-secret-key"
    endpoint := "your-endpoint"

    obsClient, err := obs.New(ak, sk, endpoint)
    if err != nil {
        fmt.Printf("Create client err:%v\n", err)
        return
    }

    input := &obs.UploadFileInput{
        Bucket: "your-bucket",
        Key: "large-file.bin",
        UploadFile: "local-large-file.bin",
        TaskNum: 5,
        EnableCheckpoint: true,
        TransferCallback: func(completedParts, totalParts int, transferredBytes, totalBytes int64, status obs.TransferStatus) {
            fmt.Printf("Status: %v, Progress: %d/%d parts, %d/%d bytes\n",
                status, completedParts, totalParts, transferredBytes, totalBytes)
        },
    }

    // 创建上传任务
    controller, err := obsClient.CreateUploadTask(input)
    if err != nil {
        fmt.Printf("Create upload task err:%v\n", err)
        return
    }

    // 启动任务（异步执行）
    go func() {
        output, err := controller.Start()
        if err != nil {
            fmt.Printf("Upload err:%v\n", err)
            return
        }
        fmt.Printf("Upload success, etag:%s\n", output.ETag)
    }()

    // 模拟用户交互：5秒后暂停，3秒后继续，再5秒后取消
    time.Sleep(5 * time.Second)
    fmt.Println("Pausing upload...")
    controller.Pause()

    time.Sleep(3 * time.Second)
    fmt.Println("Resuming upload...")
    controller.Resume()

    time.Sleep(5 * time.Second)
    fmt.Println("Canceling upload...")
    controller.Cancel()

    // 等待任务完成
    time.Sleep(2 * time.Second)
    fmt.Printf("Final status: %v\n", controller.Status())
}
```

### 5.2 下载示例

```go
func main() {
    ak := "your-access-key"
    sk := "your-secret-key"
    endpoint := "your-endpoint"

    obsClient, err := obs.New(ak, sk, endpoint)
    if err != nil {
        fmt.Printf("Create client err:%v\n", err)
        return
    }

    input := &obs.DownloadFileInput{
        Bucket: "your-bucket",
        Key: "large-file.bin",
        DownloadFile: "local-downloaded-file.bin",
        TaskNum: 3,
        EnableCheckpoint: true,
        TransferCallback: func(completedParts, totalParts int, transferredBytes, totalBytes int64, status obs.TransferStatus) {
            fmt.Printf("Status: %v, Progress: %d/%d parts, %d/%d bytes\n",
                status, completedParts, totalParts, transferredBytes, totalBytes)
        },
    }

    // 创建下载任务
    controller, err := obsClient.CreateDownloadTask(input)
    if err != nil {
        fmt.Printf("Create download task err:%v\n", err)
        return
    }

    // 启动任务
    go func() {
        output, err := controller.Start()
        if err != nil {
            fmt.Printf("Download err:%v\n", err)
            return
        }
        fmt.Printf("Download success, size:%d bytes\n", output.ContentLength)
    }()

    // 等待任务完成
    time.Sleep(10 * time.Second)

    status := controller.Status()
    if status == obs.TransferStatusRunning {
        fmt.Println("Downloading in progress...")

        // 暂停任务
        fmt.Println("Pausing download...")
        controller.Pause()

        // 获取进度
        completed, total, transferred, totalBytes := controller.Progress()
        fmt.Printf("Progress: %d/%d parts, %d/%d bytes\n",
            completed, total, transferred, totalBytes)

        // 继续任务
        fmt.Println("Resuming download...")
        controller.Resume()
    }

    time.Sleep(10 * time.Second)
    fmt.Printf("Final status: %v\n", controller.Status())
}
```

## 6. 架构考虑

### 6.1 线程安全

- 使用原子操作(`sync/atomic`)保证状态标志的线程安全访问
- 使用互斥锁(`sync.Mutex`)保护分块进度计数的更新
- 确保任务控制器的API能够安全地被多个goroutine调用

### 6.2 性能优化

- 暂停操作使用轮询机制避免忙等待
- 分块任务在暂停时会释放CPU资源
- 任务恢复时会继续从检查点恢复，无需重新开始

### 6.3 向后兼容性

- 保持现有API接口不变
- 新API采用可选参数和新方法的方式
- 检查点文件格式向前兼容

## 7. 测试策略

已创建完整的测试文件 `tests/resume_transfer_test.go`，包含以下测试内容：

### 7.1 单元测试

- 测试任务状态转换逻辑
- 测试控制器API的功能（Status、Pause、Cancel、Resume、Progress）
- 测试检查点文件的读写

### 7.2 集成测试

- **功能性测试**：
  - 测试正常上传流程
  - 测试正常下载流程
  - 测试暂停和继续上传操作
  - 测试暂停和继续下载操作
  - 测试取消上传操作
  - 测试取消下载操作

- **边界条件测试**：
  - 测试上传空文件
  - 测试下载小文件（小于分块大小）
  - 测试分块大小为最大值的文件传输

### 7.3 性能测试

- 测试不同分块大小对传输速度的影响
- 测试不同并发任务数对传输速度的影响
- 测试暂停和恢复操作的性能开销
- 测试大文件传输的效率

### 7.4 数据可靠性测试

- 测试传输过程中程序崩溃后的恢复功能
- 测试网络中断后的恢复功能
- 验证传输前后文件的完整性（MD5校验）

## 8. 总结

本设计方案为华为云OBS Go SDK的断点续传功能提供了完整的暂停、取消和继续操作支持。通过引入任务控制器和状态管理机制，用户可以更好地控制文件传输过程，并提供了详细的进度反馈。

这个增强功能将使SDK在处理大文件传输和不稳定网络环境时更加健壮，为开发可靠的云存储应用提供了更好的支持。