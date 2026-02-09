package test

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"../obs"
)

func TestNormalUpload(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	testFile, err := createTempFile(10 * 1024 * 1024) // 10MB
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	input := &obs.UploadFileInput{
		Bucket: "test-bucket",
		Key: "test-file.txt",
		UploadFile: testFile,
		EnableCheckpoint: true,
		TaskNum: 3,
	}

	output, err := client.UploadFile(input)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Upload successful: ETag=%s", output.ETag)
}

func TestPauseResumeUpload(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	testFile, err := createTempFile(50 * 1024 * 1024) // 50MB
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	input := &obs.UploadFileInput{
		Bucket: "test-bucket",
		Key: "large-file.txt",
		UploadFile: testFile,
		EnableCheckpoint: true,
		TaskNum: 3,
	}

	controller, err := client.CreateUploadTask(input)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	var uploadErr error
	var uploadOutput *obs.CompleteMultipartUploadOutput

	go func() {
		defer wg.Done()
		uploadOutput, uploadErr = controller.Start()
	}()

	time.Sleep(2 * time.Second)
	err = controller.Pause()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Upload paused. Status: %v", controller.Status())

	time.Sleep(2 * time.Second)
	err = controller.Resume()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Upload resumed. Status: %v", controller.Status())

	wg.Wait()

	if uploadErr != nil {
		t.Fatal(uploadErr)
	}

	t.Logf("Upload completed successfully: ETag=%s", uploadOutput.ETag)
}

func TestCancelUpload(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	testFile, err := createTempFile(30 * 1024 * 1024) // 30MB
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	input := &obs.UploadFileInput{
		Bucket: "test-bucket",
		Key: "cancel-test-file.txt",
		UploadFile: testFile,
		EnableCheckpoint: true,
		TaskNum: 3,
	}

	controller, err := client.CreateUploadTask(input)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	var uploadErr error

	go func() {
		defer wg.Done()
		_, uploadErr = controller.Start()
	}()

	time.Sleep(3 * time.Second)
	err = controller.Cancel()
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait()

	if uploadErr == nil {
		t.Error("Expected upload to be canceled")
	} else {
		t.Logf("Upload canceled as expected: %v", uploadErr)
	}

	t.Logf("Final status: %v", controller.Status())
}

func TestNormalDownload(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(os.TempDir(), "test-download.txt")
	defer os.Remove(outputPath)

	input := &obs.DownloadFileInput{
		Bucket: "test-bucket",
		Key: "large-file.txt",
		DownloadFile: outputPath,
		EnableCheckpoint: true,
		TaskNum: 3,
	}

	output, err := client.DownloadFile(input)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Download successful: Size=%d bytes", output.ContentLength)
}

func TestPauseResumeDownload(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(os.TempDir(), "pause-resume-download.txt")
	defer os.Remove(outputPath)

	input := &obs.DownloadFileInput{
		Bucket: "test-bucket",
		Key: "large-file.txt",
		DownloadFile: outputPath,
		EnableCheckpoint: true,
		TaskNum: 3,
	}

	controller, err := client.CreateDownloadTask(input)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	var downloadErr error
	var downloadOutput *obs.GetObjectMetadataOutput

	go func() {
		defer wg.Done()
		downloadOutput, downloadErr = controller.Start()
	}()

	time.Sleep(2 * time.Second)
	err = controller.Pause()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Download paused. Status: %v", controller.Status())

	time.Sleep(2 * time.Second)
	err = controller.Resume()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Download resumed. Status: %v", controller.Status())

	wg.Wait()

	if downloadErr != nil {
		t.Fatal(downloadErr)
	}

	t.Logf("Download completed successfully: Size=%d bytes", downloadOutput.ContentLength)
}

func TestCancelDownload(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(os.TempDir(), "cancel-download.txt")
	defer os.Remove(outputPath)

	input := &obs.DownloadFileInput{
		Bucket: "test-bucket",
		Key: "large-file.txt",
		DownloadFile: outputPath,
		EnableCheckpoint: true,
		TaskNum: 3,
	}

	controller, err := client.CreateDownloadTask(input)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	var downloadErr error

	go func() {
		defer wg.Done()
		_, downloadErr = controller.Start()
	}()

	time.Sleep(3 * time.Second)
	err = controller.Cancel()
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait()

	if downloadErr == nil {
		t.Error("Expected download to be canceled")
	} else {
		t.Logf("Download canceled as expected: %v", downloadErr)
	}

	t.Logf("Final status: %v", controller.Status())
}

func TestProgressTracking(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	testFile, err := createTempFile(20 * 1024 * 1024) // 20MB
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	input := &obs.UploadFileInput{
		Bucket: "test-bucket",
		Key: "progress-test-file.txt",
		UploadFile: testFile,
		EnableCheckpoint: true,
		TaskNum: 2,
		TransferCallback: func(completed, total int, transferred, totalBytes int64, status obs.TransferStatus) {
			t.Logf("Status: %v, Progress: %d/%d parts, %d/%d bytes", status, completed, total, transferred, totalBytes)
		},
	}

	controller, err := client.CreateUploadTask(input)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		_, err := controller.Start()
		if err != nil {
			t.Logf("Upload error: %v", err)
		}
	}()

	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)

		status := controller.Status()
		completed, total, transferred, totalBytes := controller.Progress()

		t.Logf("Query %d: Status=%v, %d/%d parts, %d/%d bytes",
			i+1, status, completed, total, transferred, totalBytes)

		if status == obs.TransferStatusRunning && i > 0 {
			if completed == 0 && transferred == 0 {
				t.Log("Warning: No progress reported yet")
			}
		}
	}

	err = controller.Cancel()
	if err != nil {
		t.Logf("Cancel error: %v", err)
	}

	wg.Wait()
}

func TestParallelUploads(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	const taskCount = 3
	const fileSize = 10 * 1024 * 1024 // 10MB

	var wg sync.WaitGroup
	errorsChan := make(chan error, taskCount)

	for i := 0; i < taskCount; i++ {
		wg.Add(1)

		go func(taskNum int) {
			defer wg.Done()

			testFile, err := createTempFile(fileSize)
			if err != nil {
				errorsChan <- err
				return
			}
			defer os.Remove(testFile)

			input := &obs.UploadFileInput{
				Bucket: "test-bucket",
				Key: fmt.Sprintf("parallel-upload-%d.txt", taskNum),
				UploadFile: testFile,
				EnableCheckpoint: true,
				TaskNum: 2,
			}

			_, err = client.UploadFile(input)
			if err != nil {
				errorsChan <- err
				return
			}

			t.Logf("Parallel upload %d completed successfully", taskNum)
		}(i)
	}

	wg.Wait()

	close(errorsChan)
	for err := range errorsChan {
		if err != nil {
			t.Errorf("Parallel upload error: %v", err)
		}
	}
}

func TestParallelDownloads(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	const taskCount = 3

	var wg sync.WaitGroup
	errorsChan := make(chan error, taskCount)

	for i := 0; i < taskCount; i++ {
		wg.Add(1)

		go func(taskNum int) {
			defer wg.Done()

			outputPath := filepath.Join(os.TempDir(), fmt.Sprintf("parallel-download-%d.txt", taskNum))
			defer os.Remove(outputPath)

			input := &obs.DownloadFileInput{
				Bucket: "test-bucket",
				Key: "large-file.txt",
				DownloadFile: outputPath,
				EnableCheckpoint: true,
				TaskNum: 2,
			}

			_, err = client.DownloadFile(input)
			if err != nil {
				errorsChan <- err
				return
			}

			t.Logf("Parallel download %d completed successfully", taskNum)
		}(i)
	}

	wg.Wait()

	close(errorsChan)
	for err := range errorsChan {
		if err != nil {
			t.Errorf("Parallel download error: %v", err)
		}
	}
}

func TestMixedParallelOperations(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errorsChan := make(chan error, 5)

	for i := 0; i < 2; i++ {
		wg.Add(1)

		go func(taskNum int) {
			defer wg.Done()

			testFile, err := createTempFile(10 * 1024 * 1024) // 10MB
			if err != nil {
				errorsChan <- err
				return
			}
			defer os.Remove(testFile)

			input := &obs.UploadFileInput{
				Bucket: "test-bucket",
				Key: fmt.Sprintf("mixed-upload-%d.txt", taskNum),
				UploadFile: testFile,
				EnableCheckpoint: true,
				TaskNum: 2,
			}

			_, err = client.UploadFile(input)
			if err != nil {
				errorsChan <- err
				return
			}

			t.Logf("Mixed upload %d completed successfully", taskNum)
		}(i)
	}

	for i := 0; i < 3; i++ {
		wg.Add(1)

		go func(taskNum int) {
			defer wg.Done()

			outputPath := filepath.Join(os.TempDir(), fmt.Sprintf("mixed-download-%d.txt", taskNum))
			defer os.Remove(outputPath)

			input := &obs.DownloadFileInput{
				Bucket: "test-bucket",
				Key: "large-file.txt",
				DownloadFile: outputPath,
				EnableCheckpoint: true,
				TaskNum: 2,
			}

			_, err = client.DownloadFile(input)
			if err != nil {
				errorsChan <- err
				return
			}

			t.Logf("Mixed download %d completed successfully", taskNum)
		}(i)
	}

	wg.Wait()

	close(errorsChan)
	for err := range errorsChan {
		if err != nil {
			t.Errorf("Mixed operation error: %v", err)
		}
	}
}

func TestUploadPerformance(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	fileSizes := []int64{10 * 1024 * 1024, 50 * 1024 * 1024, 100 * 1024 * 1024} // 10MB, 50MB, 100MB
	taskNums := []int{1, 3, 5} // 不同的任务数

	for _, size := range fileSizes {
		for _, taskNum := range taskNums {
			t.Run(fmt.Sprintf("size=%vMB_tasks=%v", size/(1024*1024), taskNum), func(t *testing.T) {
				testFile, err := createTempFile(size)
				if err != nil {
					t.Fatal(err)
				}
				defer os.Remove(testFile)

				input := &obs.UploadFileInput{
					Bucket: "test-bucket",
					Key: fmt.Sprintf("perf-test-%vMB-%vtasks.txt", size/(1024*1024), taskNum),
					UploadFile: testFile,
					EnableCheckpoint: true,
					TaskNum: taskNum,
				}

				startTime := time.Now()
				_, err = client.UploadFile(input)
				duration := time.Since(startTime)

				if err != nil {
					t.Fatal(err)
				}

				speed := float64(size) / duration.Seconds()
				t.Logf("File size: %v MB, Tasks: %v, Time: %v, Speed: %.2f MB/s",
					size/(1024*1024), taskNum, duration, speed/(1024*1024))
			})
		}
	}
}

func TestDownloadPerformance(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	fileSizes := []string{"10MB", "50MB", "100MB"}
	taskNums := []int{1, 3, 5}

	for _, sizeStr := range fileSizes {
		for _, taskNum := range taskNums {
			t.Run(fmt.Sprintf("size=%v_tasks=%v", sizeStr, taskNum), func(t *testing.T) {
				outputPath := filepath.Join(os.TempDir(), fmt.Sprintf("perf-download-%v-%vtasks.txt", sizeStr, taskNum))
				defer os.Remove(outputPath)

				input := &obs.DownloadFileInput{
					Bucket: "test-bucket",
					Key: fmt.Sprintf("perf-test-%v.txt", sizeStr),
					DownloadFile: outputPath,
					EnableCheckpoint: true,
					TaskNum: taskNum,
				}

				startTime := time.Now()
				output, err := client.DownloadFile(input)
				duration := time.Since(startTime)

				if err != nil {
					t.Fatal(err)
				}

				speed := float64(output.ContentLength) / duration.Seconds()
				t.Logf("File size: %v, Tasks: %v, Time: %v, Speed: %.2f MB/s",
					sizeStr, taskNum, duration, speed/(1024*1024))
			})
		}
	}
}

func TestNetworkInterruptionDownload(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(os.TempDir(), "network-interrupt-download.txt")
	defer os.Remove(outputPath)

	input := &obs.DownloadFileInput{
		Bucket: "test-bucket",
		Key: "large-file.txt",
		DownloadFile: outputPath,
		EnableCheckpoint: true,
		TaskNum: 3,
	}

	controller, err := client.CreateDownloadTask(input)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	var downloadErr error

	go func() {
		defer wg.Done()
		_, downloadErr = controller.Start()
	}()

	time.Sleep(4 * time.Second)
	err = controller.Pause()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Network interrupted. Status: %v", controller.Status())

	time.Sleep(3 * time.Second)

	err = controller.Resume()
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Network recovered, download resumed. Status: %v", controller.Status())

	wg.Wait()

	if downloadErr != nil {
		t.Fatal(downloadErr)
	}

	t.Logf("Download completed successfully after network interruption")
}

func TestProgramRestartUpload(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	testFile, err := createTempFile(20 * 1024 * 1024) // 20MB
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	checkpointFile := filepath.Join(os.TempDir(), "program-restart-checkpoint.upload")
	defer os.Remove(checkpointFile)

	input := &obs.UploadFileInput{
		Bucket: "test-bucket",
		Key: "program-restart-test.txt",
		UploadFile: testFile,
		EnableCheckpoint: true,
		CheckpointFile: checkpointFile,
		TaskNum: 2,
	}

	controller, err := client.CreateUploadTask(input)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	var uploadErr error

	go func() {
		defer wg.Done()
		_, uploadErr = controller.Start()
	}()

	time.Sleep(3 * time.Second)
	err = controller.Pause()
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait()

	t.Logf("First execution paused. Status: %v", controller.Status())

	t.Logf("Simulating program restart...")
	newController, err := client.CreateUploadTask(input)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("New controller created. Status: %v", newController.Status())

	wg.Add(1)

	go func() {
		defer wg.Done()
		_, uploadErr = newController.Start()
	}()

	wg.Wait()

	if uploadErr != nil {
		t.Fatal(uploadErr)
	}

	t.Logf("Upload completed successfully after program restart")
}

func TestFileIntegrity(t *testing.T) {
	client, err := obs.New("ak", "sk", "endpoint")
	if err != nil {
		t.Fatal(err)
	}

	testFile, err := createTempFile(15 * 1024 * 1024) // 15MB
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	originalMD5, err := calculateMD5(testFile)
	if err != nil {
		t.Fatal(err)
	}

	uploadInput := &obs.UploadFileInput{
		Bucket: "test-bucket",
		Key: "integrity-test-file.txt",
		UploadFile: testFile,
		EnableCheckpoint: true,
		TaskNum: 3,
	}

	_, err = client.UploadFile(uploadInput)
	if err != nil {
		t.Fatal(err)
	}

	downloadPath := filepath.Join(os.TempDir(), "downloaded-integrity-test-file.txt")
	defer os.Remove(downloadPath)

	downloadInput := &obs.DownloadFileInput{
		Bucket: "test-bucket",
		Key: "integrity-test-file.txt",
		DownloadFile: downloadPath,
		EnableCheckpoint: true,
		TaskNum: 3,
	}

	_, err = client.DownloadFile(downloadInput)
	if err != nil {
		t.Fatal(err)
	}

	downloadedMD5, err := calculateMD5(downloadPath)
	if err != nil {
		t.Fatal(err)
	}

	if originalMD5 != downloadedMD5 {
		t.Error("File integrity check failed. Original MD5 != Downloaded MD5")
	} else {
		t.Logf("File integrity verified. MD5 checksum: %s", originalMD5)
	}
}

func createTempFile(size int64) (string, error) {
	tmpFile, err := ioutil.TempFile("", "obs-sdk-test-")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	data := make([]byte, 1024*1024) // 1MB块
	blocks := size / (1024 * 1024)
	remainder := size % (1024 * 1024)

	for i := int64(0); i < blocks; i++ {
		_, err = rand.Read(data)
		if err != nil {
			os.Remove(tmpFile.Name())
			return "", err
		}
		_, err = tmpFile.Write(data)
		if err != nil {
			os.Remove(tmpFile.Name())
			return "", err
		}
	}

	if remainder > 0 {
		smallData := make([]byte, remainder)
		_, err = rand.Read(smallData)
		if err != nil {
			os.Remove(tmpFile.Name())
			return "", err
		}
		_, err = tmpFile.Write(smallData)
		if err != nil {
			os.Remove(tmpFile.Name())
			return "", err
		}
	}

	return tmpFile.Name(), nil
}

func calculateMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}