package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/qiniu/go-sdk/v7/storage"
)

type qiniuDriver struct{}

func (qiniuDriver) Save(ctx context.Context, input SaveInput) error {
	storageConfig := input.Rule.Storage
	if strings.TrimSpace(storageConfig.AccessKey) == "" || strings.TrimSpace(storageConfig.SecretKey) == "" || strings.TrimSpace(storageConfig.Bucket) == "" {
		return fmt.Errorf("七牛云配置不完整")
	}

	mac := auth.New(strings.TrimSpace(storageConfig.AccessKey), strings.TrimSpace(storageConfig.SecretKey))
	policy := storage.PutPolicy{
		Scope:        fmt.Sprintf("%s:%s", strings.TrimSpace(storageConfig.Bucket), input.ObjectKey),
		InsertOnly:   1,
		FsizeLimit:   input.Size,
		MimeLimit:    strings.TrimSpace(input.Rule.Accept),
		ForceSaveKey: false,
	}
	upToken := policy.UploadToken(mac)

	qiniuConfig := storage.NewConfig()
	qiniuConfig.UseHTTPS = true
	if host := strings.TrimSpace(storageConfig.UploadHost); host != "" {
		qiniuConfig.UpHost = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	}

	ret := storage.PutRet{}
	uploader := storage.NewFormUploader(qiniuConfig)
	if err := uploader.PutFile(ctx, &ret, upToken, input.ObjectKey, input.LocalPath, &storage.PutExtra{
		MimeType: input.Mime,
	}); err != nil {
		return fmt.Errorf("上传到七牛云失败: %w", err)
	}
	return nil
}

func (qiniuDriver) InitDirect(_ context.Context, rule Rule, session Session) (*DirectInitResult, error) {
	storageConfig := rule.Storage
	if strings.TrimSpace(storageConfig.AccessKey) == "" || strings.TrimSpace(storageConfig.SecretKey) == "" || strings.TrimSpace(storageConfig.Bucket) == "" {
		return nil, fmt.Errorf("七牛云配置不完整")
	}
	if strings.TrimSpace(session.ObjectKey) == "" {
		return nil, fmt.Errorf("七牛直传缺少文件标识")
	}

	mac := auth.New(strings.TrimSpace(storageConfig.AccessKey), strings.TrimSpace(storageConfig.SecretKey))
	policy := storage.PutPolicy{
		Scope:      fmt.Sprintf("%s:%s", strings.TrimSpace(storageConfig.Bucket), session.ObjectKey),
		InsertOnly: 1,
		FsizeLimit: rule.MaxSizeBytes,
		MimeLimit:  strings.TrimSpace(rule.Accept),
	}
	if storageConfig.TokenTTL > 0 {
		policy.Expires = uint64(storageConfig.TokenTTL)
	}
	upToken := policy.UploadToken(mac)

	uploadURL := strings.TrimSpace(storageConfig.UploadHost)
	if uploadURL == "" {
		uploadURL = "https://upload.qiniup.com"
	} else if !strings.HasPrefix(uploadURL, "http://") && !strings.HasPrefix(uploadURL, "https://") {
		uploadURL = "https://" + uploadURL
	}

	return &DirectInitResult{
		Method:    "post",
		UploadURL: uploadURL,
		Fields: map[string]string{
			"token": upToken,
			"key":   session.ObjectKey,
		},
	}, nil
}

func (qiniuDriver) ResolveOpen(_ context.Context, file File) (*OpenTarget, error) {
	domain := strings.TrimSpace(file.Storage.Domain)
	if domain == "" {
		return nil, fmt.Errorf("七牛云访问域名未配置")
	}
	return &OpenTarget{
		Redirect: JoinPublicURL(domain, file.Path),
	}, nil
}

func (qiniuDriver) ResolvePublicURL(file File) string {
	return JoinPublicURL(file.Storage.Domain, file.Path)
}
