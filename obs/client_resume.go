// Copyright 2019 Huawei Technologies Co.,Ltd.
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use
// this file except in compliance with the License.  You may obtain a copy of the
// License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations under the License.

package obs

import (
	"errors"
	"fmt"
	"os"
)

// UploadFile resume uploads.
//
// This API is an encapsulated and enhanced version of multipart upload, and aims to eliminate large file
// upload failures caused by poor network conditions and program breakdowns.
func (obsClient ObsClient) UploadFile(input *UploadFileInput, extensions ...extensionOptions) (output *CompleteMultipartUploadOutput, err error) {
	if input.EnableCheckpoint && input.CheckpointFile == "" {
		input.CheckpointFile = input.UploadFile + ".uploadfile_record"
	}

	if input.TaskNum <= 0 {
		input.TaskNum = 1
	}
	if input.PartSize < MIN_PART_SIZE {
		input.PartSize = MIN_PART_SIZE
	} else if input.PartSize > MAX_PART_SIZE {
		input.PartSize = MAX_PART_SIZE
	}

	ctx := newTransferContext(0, 0)
	output, err = obsClient.resumeUpload(input, ctx, extensions)
	return
}

// DownloadFile resume downloads.
//
// This API is an encapsulated and enhanced version of partial download, and aims to eliminate large file
// download failures caused by poor network conditions and program breakdowns.
func (obsClient ObsClient) DownloadFile(input *DownloadFileInput, extensions ...extensionOptions) (output *GetObjectMetadataOutput, err error) {
	if input.DownloadFile == "" {
		input.DownloadFile = input.Key
	}

	if input.EnableCheckpoint && input.CheckpointFile == "" {
		input.CheckpointFile = input.DownloadFile + ".downloadfile_record"
	}

	if input.TaskNum <= 0 {
		input.TaskNum = 1
	}
	if input.PartSize <= 0 {
		input.PartSize = DEFAULT_PART_SIZE
	}

	ctx := newTransferContext(0, 0)
	output, err = obsClient.resumeDownload(input, ctx, extensions)
	return
}

// CreateUploadTask creates an upload task and returns a TransferController for controlling the transfer.
func (obsClient ObsClient) CreateUploadTask(input *UploadFileInput, extensions ...extensionOptions) (TransferController, error) {
	if input.EnableCheckpoint && input.CheckpointFile == "" {
		input.CheckpointFile = input.UploadFile + ".uploadfile_record"
	}

	if input.TaskNum <= 0 {
		input.TaskNum = 1
	}
	if input.PartSize < MIN_PART_SIZE {
		input.PartSize = MIN_PART_SIZE
	} else if input.PartSize > MAX_PART_SIZE {
		input.PartSize = MAX_PART_SIZE
	}

	// Check if file exists and is valid
	uploadFileStat, err := os.Stat(input.UploadFile)
	if err != nil {
		doLog(LEVEL_ERROR, fmt.Sprintf("Failed to stat uploadFile with error: [%v].", err))
		return nil, err
	}
	if uploadFileStat.IsDir() {
		doLog(LEVEL_ERROR, "UploadFile can not be a folder.")
		return nil, errors.New("uploadFile can not be a folder")
	}

	ufc := &UploadCheckpoint{}

	var needCheckpoint = true
	var checkpointFilePath = input.CheckpointFile
	var enableCheckpoint = input.EnableCheckpoint
	if enableCheckpoint {
		needCheckpoint, err = getCheckpointFile(ufc, uploadFileStat, input, &obsClient, extensions)
		if err != nil {
			return nil, err
		}
	}
	if needCheckpoint {
		err = prepareUpload(ufc, uploadFileStat, input, &obsClient, extensions)
		if err != nil {
			return nil, err
		}

		if enableCheckpoint {
			err = updateCheckpointFile(ufc, checkpointFilePath)
			if err != nil {
				doLog(LEVEL_ERROR, "Failed to update checkpoint file with error [%v].", err)
				_err := abortTask(ufc.Bucket, ufc.Key, ufc.UploadId, &obsClient, extensions)
				if _err != nil {
					doLog(LEVEL_WARN, "Failed to abort task [%s].", ufc.UploadId)
				}
				return nil, err
			}
		}
	}

	// Create transfer context
	totalParts := len(ufc.UploadParts)
	var totalBytes int64
	for _, part := range ufc.UploadParts {
		totalBytes += part.PartSize
	}
	ctx := newTransferContext(totalParts, totalBytes)

	// Create and return controller
	controller := &uploadController{
		obsClient:        obsClient,
		input:            input,
		ctx:              ctx,
		ufc:              ufc,
		checkpointFile:   checkpointFilePath,
		enableCheckpoint: enableCheckpoint,
		extensions:       extensions,
	}

	return controller, nil
}

// CreateDownloadTask creates a download task and returns a TransferController for controlling the transfer.
func (obsClient ObsClient) CreateDownloadTask(input *DownloadFileInput, extensions ...extensionOptions) (TransferController, error) {
	if input.DownloadFile == "" {
		input.DownloadFile = input.Key
	}

	if input.EnableCheckpoint && input.CheckpointFile == "" {
		input.CheckpointFile = input.DownloadFile + ".downloadfile_record"
	}

	if input.TaskNum <= 0 {
		input.TaskNum = 1
	}
	if input.PartSize <= 0 {
		input.PartSize = DEFAULT_PART_SIZE
	}

	getObjectmetaOutput, err := getObjectInfo(input, &obsClient, extensions)
	if err != nil {
		return nil, err
	}

	objectSize := getObjectmetaOutput.ContentLength
	partSize := input.PartSize
	dfc := &DownloadCheckpoint{}

	var needCheckpoint = true
	var checkpointFilePath = input.CheckpointFile
	var enableCheckpoint = input.EnableCheckpoint
	if enableCheckpoint {
		needCheckpoint, err = getDownloadCheckpointFile(dfc, input, getObjectmetaOutput)
		if err != nil {
			return nil, err
		}
	}

	if needCheckpoint {
		dfc.Bucket = input.Bucket
		dfc.Key = input.Key
		dfc.VersionId = input.VersionId
		dfc.DownloadFile = input.DownloadFile
		dfc.ObjectInfo = ObjectInfo{}
		dfc.ObjectInfo.LastModified = getObjectmetaOutput.LastModified.Unix()
		dfc.ObjectInfo.Size = getObjectmetaOutput.ContentLength
		dfc.ObjectInfo.ETag = getObjectmetaOutput.ETag
		dfc.TempFileInfo = TempFileInfo{}
		dfc.TempFileInfo.TempFileUrl = input.DownloadFile + ".tmp"
		dfc.TempFileInfo.Size = getObjectmetaOutput.ContentLength

		sliceObject(objectSize, partSize, dfc)
		_err := prepareTempFile(dfc.TempFileInfo.TempFileUrl, dfc.TempFileInfo.Size)
		if _err != nil {
			return nil, _err
		}

		if enableCheckpoint {
			_err := updateCheckpointFile(dfc, checkpointFilePath)
			if _err != nil {
				doLog(LEVEL_ERROR, "Failed to update checkpoint file with error [%v].", _err)
				_errMsg := os.Remove(dfc.TempFileInfo.TempFileUrl)
				if _errMsg != nil {
					doLog(LEVEL_WARN, "Failed to remove temp download file with error [%v].", _errMsg)
				}
				return nil, _err
			}
		}
	}

	// Create transfer context
	totalParts := len(dfc.DownloadParts)
	var totalBytes int64
	for _, part := range dfc.DownloadParts {
		totalBytes += (part.RangeEnd - part.Offset + 1)
	}
	ctx := newTransferContext(totalParts, totalBytes)

	// Create and return controller
	controller := &downloadController{
		obsClient:        obsClient,
		input:            input,
		ctx:              ctx,
		dfc:              dfc,
		checkpointFile:   checkpointFilePath,
		enableCheckpoint: enableCheckpoint,
		extensions:       extensions,
		objectInfo:       getObjectmetaOutput,
	}

	return controller, nil
}

// uploadController implements TransferController for upload tasks
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

func (c *uploadController) Status() TransferStatus {
	return c.ctx.Status()
}

func (c *uploadController) Pause() error {
	return c.ctx.Pause()
}

func (c *uploadController) Cancel() error {
	return c.ctx.Cancel()
}

func (c *uploadController) Resume() error {
	return c.ctx.Resume()
}

func (c *uploadController) Progress() (int, int, int64, int64) {
	return c.ctx.Progress()
}

func (c *uploadController) Start() (*CompleteMultipartUploadOutput, error) {
	if c.running {
		return nil, fmt.Errorf("task already running")
	}

	c.running = true
	c.resultChan = make(chan struct {
		output *CompleteMultipartUploadOutput
		err    error
	}, 1)

	go func() {
		c.ctx.setStatus(TransferStatusRunning)
		uploadPartError := c.obsClient.uploadPartConcurrent(c.ufc, c.checkpointFile, c.input, c.ctx, c.extensions)
		err := handleUploadFileResult(uploadPartError, c.ufc, c.enableCheckpoint, &c.obsClient, c.extensions)
		if err != nil {
			if c.ctx.Status() == TransferStatusCanceled {
				c.resultChan <- struct {
					output *CompleteMultipartUploadOutput
					err    error
				}{nil, errors.New("upload canceled")}
			} else if c.ctx.Status() == TransferStatusPaused {
				c.resultChan <- struct {
					output *CompleteMultipartUploadOutput
					err    error
				}{nil, errors.New("upload paused")}
			} else {
				c.ctx.setStatus(TransferStatusFailed)
				c.resultChan <- struct {
					output *CompleteMultipartUploadOutput
					err    error
				}{nil, err}
			}
		} else {
			completeOutput, err := completeParts(c.ufc, c.enableCheckpoint, c.checkpointFile, &c.obsClient, c.input.EncodingType, c.extensions)
			if err != nil {
				c.ctx.setStatus(TransferStatusFailed)
			} else {
				c.ctx.setStatus(TransferStatusCompleted)
			}
			c.resultChan <- struct {
				output *CompleteMultipartUploadOutput
				err    error
			}{completeOutput, err}
		}
	}()

	// Wait for result
	result := <-c.resultChan
	close(c.resultChan)
	c.running = false

	output := result.output
	err := result.err
	return output, err
}

// downloadController implements TransferController for download tasks
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

func (c *downloadController) Status() TransferStatus {
	return c.ctx.Status()
}

func (c *downloadController) Pause() error {
	return c.ctx.Pause()
}

func (c *downloadController) Cancel() error {
	return c.ctx.Cancel()
}

func (c *downloadController) Resume() error {
	return c.ctx.Resume()
}

func (c *downloadController) Progress() (int, int, int64, int64) {
	return c.ctx.Progress()
}

func (c *downloadController) Start() (*GetObjectMetadataOutput, error) {
	if c.running {
		return nil, fmt.Errorf("task already running")
	}

	c.running = true
	c.resultChan = make(chan struct {
		output *GetObjectMetadataOutput
		err    error
	}, 1)

	go func() {
		c.ctx.setStatus(TransferStatusRunning)
		downloadFileError := c.obsClient.downloadFileConcurrent(c.input, c.dfc, c.ctx, c.extensions)
		err := handleDownloadFileResult(c.dfc.TempFileInfo.TempFileUrl, c.enableCheckpoint, downloadFileError)
		if err != nil {
			if c.ctx.Status() == TransferStatusCanceled {
				c.resultChan <- struct {
					output *GetObjectMetadataOutput
					err    error
				}{nil, errors.New("download canceled")}
			} else if c.ctx.Status() == TransferStatusPaused {
				c.resultChan <- struct {
					output *GetObjectMetadataOutput
					err    error
				}{nil, errors.New("download paused")}
			} else {
				c.ctx.setStatus(TransferStatusFailed)
				c.resultChan <- struct {
					output *GetObjectMetadataOutput
					err    error
				}{nil, err}
			}
		} else {
			err = os.Rename(c.dfc.TempFileInfo.TempFileUrl, c.input.DownloadFile)
			if err != nil {
				doLog(LEVEL_ERROR, "Failed to rename temp download file [%s] to download file [%s] with error [%v].", c.dfc.TempFileInfo.TempFileUrl, c.input.DownloadFile, err)
				c.ctx.setStatus(TransferStatusFailed)
			} else {
				if c.enableCheckpoint {
					err = os.Remove(c.checkpointFile)
					if err != nil {
						doLog(LEVEL_WARN, "Download file successfully, but remove checkpoint file failed with error [%v].", err)
					}
				}
				c.ctx.setStatus(TransferStatusCompleted)
			}
			c.resultChan <- struct {
				output *GetObjectMetadataOutput
				err    error
			}{c.objectInfo, err}
		}
	}()

	// Wait for result
	result := <-c.resultChan
	close(c.resultChan)
	c.running = false

	output := result.output
	err := result.err
	return output, err
}
