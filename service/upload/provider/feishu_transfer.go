package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	feishuUploadTimeout       = 2 * time.Minute
	feishuDownloadMaxDuration = 30 * time.Minute
	feishuDownloadIdleTimeout = 45 * time.Second
)

type feishuFormField struct {
	Name  string
	Value string
}

type feishuUploadResponse struct {
	feishuBaseResponse
	Data struct {
		FileToken string `json:"file_token"`
	} `json:"data"`
}

type feishuPrepareUploadResponse struct {
	feishuBaseResponse
	Data struct {
		UploadID  string `json:"upload_id"`
		BlockSize int64  `json:"block_size"`
		BlockNum  int    `json:"block_num"`
	} `json:"data"`
}

func (client *feishuClient) uploadAll(ctx context.Context, accessToken, folderToken, remoteName, localPath string, size int64) (string, error) {
	body, contentType, err := buildFeishuMultipart(localPath, remoteName, 0, size, []feishuFormField{
		{Name: "file_name", Value: remoteName},
		{Name: "parent_type", Value: "explorer"},
		{Name: "parent_node", Value: folderToken},
		{Name: "size", Value: strconv.FormatInt(size, 10)},
	})
	if err != nil {
		return "", err
	}
	var response feishuUploadResponse
	if err = client.doDriveMultipart(ctx, "/drive/v1/files/upload_all", accessToken, body, contentType, &response, "上传文件"); err != nil {
		return "", err
	}
	fileToken := strings.TrimSpace(response.Data.FileToken)
	if fileToken == "" {
		return "", fmt.Errorf("飞书云盘上传文件失败：未返回文件标识")
	}
	return fileToken, nil
}

func (client *feishuClient) uploadMultipart(ctx context.Context, accessToken, folderToken, remoteName, localPath string, size int64, progress func(int64, int64)) (string, error) {
	var prepare feishuPrepareUploadResponse
	if err := client.doDriveJSON(ctx, http.MethodPost, "/drive/v1/files/upload_prepare", accessToken, map[string]any{
		"file_name":   remoteName,
		"parent_type": "explorer",
		"parent_node": folderToken,
		"size":        size,
	}, &prepare, "初始化分片上传"); err != nil {
		return "", err
	}
	uploadID := strings.TrimSpace(prepare.Data.UploadID)
	blockSize := prepare.Data.BlockSize
	blockNum := prepare.Data.BlockNum
	if uploadID == "" || blockSize <= 0 || blockSize > feishuUploadAllMaxBytes || blockNum <= 0 {
		return "", fmt.Errorf("飞书云盘初始化分片上传失败：未返回有效分片策略")
	}
	expectedBlocks := int((size + blockSize - 1) / blockSize)
	if expectedBlocks != blockNum {
		return "", fmt.Errorf("飞书云盘初始化分片上传失败：分片数量不一致")
	}

	for sequence := 0; sequence < blockNum; sequence++ {
		offset := int64(sequence) * blockSize
		partSize := blockSize
		if remaining := size - offset; remaining < partSize {
			partSize = remaining
		}
		body, contentType, err := buildFeishuMultipart(localPath, remoteName, offset, partSize, []feishuFormField{
			{Name: "upload_id", Value: uploadID},
			{Name: "seq", Value: strconv.Itoa(sequence)},
			{Name: "size", Value: strconv.FormatInt(partSize, 10)},
		})
		if err != nil {
			return "", err
		}
		var partResponse feishuBaseResponse
		if err = client.doDriveMultipart(ctx, "/drive/v1/files/upload_part", accessToken, body, contentType, &partResponse, "上传文件分片"); err != nil {
			return "", err
		}
		if progress != nil {
			progress(offset+partSize, size)
		}
	}

	var finish feishuUploadResponse
	if err := client.doDriveJSON(ctx, http.MethodPost, "/drive/v1/files/upload_finish", accessToken, map[string]any{
		"upload_id": uploadID,
		"block_num": blockNum,
	}, &finish, "完成分片上传"); err != nil {
		return "", err
	}
	fileToken := strings.TrimSpace(finish.Data.FileToken)
	if fileToken == "" {
		return "", fmt.Errorf("飞书云盘完成分片上传失败：未返回文件标识")
	}
	return fileToken, nil
}

func (client *feishuClient) download(ctx context.Context, accessToken, fileToken, rangeHeader string) (*OpenResult, error) {
	var response *http.Response
	var responseCancel context.CancelFunc
	err := withFeishuRetry(ctx, func() error {
		if err := client.waitDriveSlot(ctx); err != nil {
			return err
		}
		requestCtx, cancel := context.WithTimeout(nonNilContext(ctx), feishuDownloadMaxDuration)
		request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, feishuAPIBase+"/drive/v1/files/"+url.PathEscape(fileToken)+"/download", nil)
		if err != nil {
			cancel()
			return fmt.Errorf("构造飞书云盘下载请求失败")
		}
		request.Header.Set("Authorization", "Bearer "+accessToken)
		if normalizedRange := normalizeFeishuRange(rangeHeader); normalizedRange != "" {
			request.Header.Set("Range", normalizedRange)
		}
		current, err := feishuHTTPClient.Do(request)
		if err != nil {
			cancel()
			return fmt.Errorf("飞书云盘下载请求失败：%w", err)
		}
		if current.StatusCode != http.StatusOK && current.StatusCode != http.StatusPartialContent {
			responseErr := decodeFeishuHTTPError(current, "下载文件")
			_ = current.Body.Close()
			cancel()
			return responseErr
		}
		response = current
		responseCancel = cancel
		return nil
	})
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, fmt.Errorf("飞书云盘下载文件失败：未返回文件内容")
	}
	header := make(http.Header)
	for _, name := range []string{"Content-Type", "Content-Disposition", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Cache-Control"} {
		if value := strings.TrimSpace(response.Header.Get(name)); value != "" {
			header.Set(name, value)
		}
	}
	return &OpenResult{
		Stream:        newFeishuDownloadBody(response.Body, responseCancel),
		StatusCode:    response.StatusCode,
		ContentLength: response.ContentLength,
		Header:        header,
	}, nil
}

type feishuDownloadBody struct {
	body      io.ReadCloser
	cancel    context.CancelFunc
	timer     *time.Timer
	timerMu   sync.Mutex
	stopped   bool
	closeOnce sync.Once
	closeErr  error
}

func newFeishuDownloadBody(body io.ReadCloser, cancel context.CancelFunc) io.ReadCloser {
	stream := &feishuDownloadBody{
		body:   body,
		cancel: cancel,
	}
	stream.timer = time.AfterFunc(feishuDownloadIdleTimeout, cancel)
	return stream
}

func (stream *feishuDownloadBody) Read(buffer []byte) (int, error) {
	read, err := stream.body.Read(buffer)
	if read > 0 {
		stream.resetTimer()
	}
	if err != nil {
		stream.stopTimer()
	}
	return read, err
}

func (stream *feishuDownloadBody) Close() error {
	stream.closeOnce.Do(func() {
		stream.stopTimer()
		stream.closeErr = stream.body.Close()
	})
	return stream.closeErr
}

func (stream *feishuDownloadBody) resetTimer() {
	stream.timerMu.Lock()
	defer stream.timerMu.Unlock()
	if !stream.stopped {
		stream.timer.Reset(feishuDownloadIdleTimeout)
	}
}

func (stream *feishuDownloadBody) stopTimer() {
	stream.timerMu.Lock()
	if !stream.stopped {
		stream.stopped = true
		stream.timer.Stop()
		stream.cancel()
	}
	stream.timerMu.Unlock()
}

func wrapFeishuOpenError(err error) error {
	if err == nil {
		return nil
	}
	statusCode := http.StatusBadGateway
	header := make(http.Header)
	var apiErr *feishuAPIError
	if errors.As(err, &apiErr) {
		header = apiErr.Header.Clone()
		switch apiErr.HTTPStatus {
		case http.StatusNotFound, http.StatusRequestedRangeNotSatisfiable:
			statusCode = apiErr.HTTPStatus
		case http.StatusTooManyRequests:
			statusCode = http.StatusServiceUnavailable
		}
	}
	return &OpenError{
		Err:        err,
		StatusCode: statusCode,
		Header:     header,
	}
}

func (client *feishuClient) doDriveMultipart(ctx context.Context, requestPath, accessToken string, body []byte, contentType string, response feishuResponsePayload, operation string) error {
	if err := client.waitDriveSlot(ctx); err != nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(nonNilContext(ctx), feishuUploadTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, feishuAPIBase+requestPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("构造飞书云盘%s请求失败", operation)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", contentType)
	if err = executeFeishuJSONRequest(request, response, operation); err != nil {
		return err
	}
	return feishuResponseError(operation, response)
}

func buildFeishuMultipart(localPath, remoteName string, offset, size int64, fields []feishuFormField) ([]byte, string, error) {
	file, err := os.Open(strings.TrimSpace(localPath))
	if err != nil {
		return nil, "", fmt.Errorf("读取飞书云盘上传文件失败：%w", err)
	}
	defer file.Close()
	if offset > 0 {
		if _, err = file.Seek(offset, io.SeekStart); err != nil {
			return nil, "", fmt.Errorf("定位飞书云盘上传分片失败：%w", err)
		}
	}

	buffer := bytes.NewBuffer(make([]byte, 0, int(size)+2048))
	writer := multipart.NewWriter(buffer)
	for _, field := range fields {
		if err = writer.WriteField(field.Name, field.Value); err != nil {
			return nil, "", fmt.Errorf("构造飞书云盘上传参数失败")
		}
	}
	part, err := writer.CreateFormFile("file", remoteName)
	if err != nil {
		return nil, "", fmt.Errorf("构造飞书云盘上传文件失败")
	}
	written, err := io.CopyN(part, file, size)
	if err != nil || written != size {
		return nil, "", fmt.Errorf("读取飞书云盘上传文件失败")
	}
	if err = writer.Close(); err != nil {
		return nil, "", fmt.Errorf("完成飞书云盘上传请求失败")
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func normalizeFeishuRange(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 128 || !strings.HasPrefix(strings.ToLower(value), "bytes=") || strings.ContainsAny(value, "\r\n") {
		return ""
	}
	return value
}
