package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	frontmodel "github.com/dever-package/front/model"
)

const feishuUploadAllMaxBytes = int64(20 * 1024 * 1024)

type feishuConfig struct {
	AppID           string
	AppSecret       string
	RootFolderToken string
}

type feishuDriver struct{}

func (driver feishuDriver) Save(ctx context.Context, input SaveInput) error {
	_, err := driver.SaveWithResult(ctx, input)
	return err
}

func (feishuDriver) SaveWithResult(ctx context.Context, input SaveInput) (SaveResult, error) {
	client, err := newFeishuClient(input.Rule.Storage)
	if err != nil {
		return SaveResult{}, err
	}
	if input.Size <= 0 {
		return SaveResult{}, fmt.Errorf("飞书云盘不支持上传空文件")
	}
	if strings.TrimSpace(input.LocalPath) == "" {
		return SaveResult{}, fmt.Errorf("飞书云盘上传缺少本地文件")
	}

	client.appState.uploadMu.Lock()
	defer client.appState.uploadMu.Unlock()

	accessToken, err := client.tenantAccessToken(ctx)
	if err != nil {
		return SaveResult{}, err
	}
	remoteName, shard := feishuRemoteObject(input.ObjectKey, input.Ext)
	folderToken, err := client.ensureShardFolder(ctx, accessToken, shard)
	if err != nil {
		return SaveResult{}, err
	}
	fileToken, err := client.findFileToken(ctx, accessToken, folderToken, remoteName)
	if err != nil {
		return SaveResult{}, err
	}
	if fileToken != "" {
		if input.Progress != nil {
			input.Progress(input.Size, input.Size)
		}
		return SaveResult{ProviderKey: fileToken}, nil
	}

	if input.Progress != nil {
		input.Progress(0, input.Size)
	}
	if input.Size <= feishuUploadAllMaxBytes {
		fileToken, err = client.uploadAll(ctx, accessToken, folderToken, remoteName, input.LocalPath, input.Size)
	} else {
		fileToken, err = client.uploadMultipart(ctx, accessToken, folderToken, remoteName, input.LocalPath, input.Size, input.Progress)
	}
	if err != nil {
		recoveredToken, recoveryErr := client.findFileToken(ctx, accessToken, folderToken, remoteName)
		if recoveryErr != nil || recoveredToken == "" {
			return SaveResult{}, err
		}
		fileToken = recoveredToken
	}
	if input.Progress != nil {
		input.Progress(input.Size, input.Size)
	}
	return SaveResult{ProviderKey: fileToken}, nil
}

func (feishuDriver) InitDirect(_ context.Context, _ Rule, _ Session) (*DirectInitResult, error) {
	return nil, fmt.Errorf("飞书云盘仅支持后端中转上传")
}

func (feishuDriver) ResolveOpen(_ context.Context, _ File) (*OpenTarget, error) {
	return nil, fmt.Errorf("飞书云盘文件需要通过统一流式接口读取")
}

func (feishuDriver) ResolveOpenWithResult(ctx context.Context, input OpenInput) (*OpenResult, error) {
	file := input.File
	client, err := newFeishuClient(file.Storage)
	if err != nil {
		return nil, wrapFeishuOpenError(err)
	}
	fileToken := strings.TrimSpace(input.ProviderKey)
	if fileToken == "" {
		fileToken = strings.TrimSpace(file.Path)
	}
	if fileToken == "" {
		return nil, &OpenError{Err: fmt.Errorf("飞书云盘文件标识不存在"), StatusCode: 404}
	}
	accessToken, err := client.tenantAccessToken(ctx)
	if err != nil {
		return nil, wrapFeishuOpenError(err)
	}
	result, err := client.download(ctx, accessToken, fileToken, input.Range)
	if err != nil {
		return nil, wrapFeishuOpenError(err)
	}
	return result, nil
}

func (feishuDriver) ResolvePublicURL(_ File) string {
	return ""
}

func (feishuDriver) UsesSignedPublicOpen() bool {
	return true
}

func (feishuDriver) CheckStorage(ctx context.Context, storage frontmodel.UploadStorage) error {
	client, err := newFeishuClient(storage)
	if err != nil {
		return err
	}
	accessToken, err := client.tenantAccessToken(ctx)
	if err != nil {
		return err
	}
	_, err = client.listFolderPage(ctx, accessToken, client.config.RootFolderToken, "")
	return err
}

func resolveFeishuConfig(storage frontmodel.UploadStorage) (feishuConfig, error) {
	config := feishuConfig{
		AppID:           strings.TrimSpace(storage.AccessKey),
		AppSecret:       strings.TrimSpace(storage.SecretKey),
		RootFolderToken: strings.TrimSpace(storage.Bucket),
	}
	if config.AppID == "" {
		return feishuConfig{}, fmt.Errorf("飞书云盘 App ID 未配置")
	}
	if config.AppSecret == "" {
		return feishuConfig{}, fmt.Errorf("飞书云盘 App Secret 未配置")
	}
	if config.RootFolderToken == "" {
		return feishuConfig{}, fmt.Errorf("飞书云盘目标文件夹 Token 未配置")
	}
	return config, nil
}

func feishuRemoteObject(objectKey, ext string) (string, string) {
	sum := sha256.Sum256([]byte(strings.TrimSpace(objectKey)))
	digest := hex.EncodeToString(sum[:])
	return digest + normalizeFeishuExtension(ext), digest[:2]
}

func normalizeFeishuExtension(value string) string {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext("file" + strings.TrimSpace(value))))
	if ext == "" || len(ext) > 24 {
		return ""
	}
	for _, char := range strings.TrimPrefix(ext, ".") {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) {
			return ""
		}
	}
	return ext
}
