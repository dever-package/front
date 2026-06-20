package upload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dever-package/front/service/upload/internal/transfer"
	uploadprovider "github.com/dever-package/front/service/upload/provider"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

type ImportFileInput struct {
	RuleID     uint64
	Kind       string
	Name       string
	Mime       string
	LocalPath  string
	Content    []byte
	BizKey     string
	BizName    string
	CategoryID uint64
	Progress   func(text string, progress int)
}

func ImportFile(ctx context.Context, input ImportFileInput) (resolvedUploadFile, error) {
	rule, err := uploadrepo.FindUploadRule(ctx, input.RuleID)
	if err != nil {
		return resolvedUploadFile{}, err
	}
	return importFileWithRule(ctx, input, rule)
}

func importFileWithRule(ctx context.Context, input ImportFileInput, rule resolvedUploadRule) (resolvedUploadFile, error) {
	localPath, cleanup, err := resolveImportFileLocalPath(input)
	if err != nil {
		return resolvedUploadFile{}, err
	}
	defer cleanup()

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = filepath.Base(localPath)
	}

	size, hash, detectedMime, err := inspectImportFile(localPath, name, input.Mime)
	if err != nil {
		return resolvedUploadFile{}, err
	}
	notifyImportProgress(input.Progress, "正在校验资源", 48)
	kind := resolveUploadKind(input.Kind, name, detectedMime)
	if err = validateUploadInit(rule, uploadInitInput{
		RuleID:     input.RuleID,
		Name:       name,
		Size:       size,
		Mime:       detectedMime,
		Hash:       hash,
		Kind:       kind,
		BizKey:     input.BizKey,
		BizName:    input.BizName,
		CategoryID: input.CategoryID,
	}); err != nil {
		return resolvedUploadFile{}, err
	}

	bizRecord, err := uploadrepo.EnsureUploadBiz(ctx, input.BizKey, input.BizName)
	if err != nil {
		return resolvedUploadFile{}, err
	}
	categoryID, err := uploadrepo.EnsureUploadCateID(ctx, input.CategoryID)
	if err != nil {
		return resolvedUploadFile{}, err
	}

	ext := resolveUploadExt(name, detectedMime)
	objectKey := buildUploadObjectKey(rule.ID, hash, ext, bizRecord.Key)
	if existing := uploadrepo.FindUploadFileByPath(ctx, objectKey); existing != nil {
		notifyImportProgress(input.Progress, "资源已存在，正在复用记录", 95)
		if err := updateUploadFileRelationMetaIfNeeded(ctx, *existing, bizRecord.ID, categoryID); err == nil {
			if refreshed, refreshErr := uploadrepo.FindUploadFile(ctx, existing.ID); refreshErr == nil {
				notifyImportProgress(input.Progress, "资源保存完成", 100)
				return refreshed, nil
			}
		}
		notifyImportProgress(input.Progress, "资源保存完成", 100)
		return *existing, nil
	}

	provider, err := uploadprovider.Resolve(resolveUploadStorageProvider(rule.Storage))
	if err != nil {
		return resolvedUploadFile{}, err
	}
	notifyImportProgress(input.Progress, "正在保存到存储", 50)
	if err = provider.Save(ctx, uploadprovider.SaveInput{
		Rule: uploadprovider.Rule{
			Storage:      rule.Storage,
			Accept:       rule.Accept,
			MaxSizeBytes: uploadRuleMaxSizeBytes(rule),
		},
		Session: uploadprovider.Session{
			ObjectKey: objectKey,
		},
		LocalPath: localPath,
		ObjectKey: objectKey,
		Name:      name,
		Mime:      detectedMime,
		Size:      size,
		Hash:      hash,
		Ext:       ext,
		Progress: func(loaded int64, total int64) {
			if progress := transfer.Percent(loaded, total, 50, 95); progress >= 0 {
				notifyImportProgress(input.Progress, "正在保存到存储", progress)
			}
		},
	}); err != nil {
		return resolvedUploadFile{}, err
	}
	notifyImportProgress(input.Progress, "正在写入资源记录", 98)

	return persistUploadFile(ctx, rule, resolvedUploadSession{
		RuleID:     rule.ID,
		StorageID:  rule.StorageID,
		Kind:       kind,
		BizID:      bizRecord.ID,
		BizKey:     bizRecord.Key,
		BizName:    bizRecord.Name,
		CategoryID: categoryID,
		Name:       name,
		Ext:        ext,
		Mime:       detectedMime,
		Size:       size,
		Hash:       hash,
		ObjectKey:  objectKey,
		Status:     uploadSessionComplete,
	}, hash, objectKey)
}

func notifyImportProgress(progress func(text string, progress int), text string, percent int) {
	if progress == nil {
		return
	}
	if percent > 100 {
		percent = 100
	}
	progress(strings.TrimSpace(text), percent)
}

func resolveImportFileLocalPath(input ImportFileInput) (string, func(), error) {
	if strings.TrimSpace(input.LocalPath) != "" {
		return strings.TrimSpace(input.LocalPath), func() {}, nil
	}
	if len(input.Content) == 0 {
		return "", nil, fmt.Errorf("导入文件内容不能为空")
	}

	pattern := "dever-import-*"
	if ext := filepath.Ext(strings.TrimSpace(input.Name)); ext != "" {
		pattern += ext
	}
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, fmt.Errorf("创建导入临时文件失败: %w", err)
	}
	if _, err = file.Write(input.Content); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, fmt.Errorf("写入导入临时文件失败: %w", err)
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, fmt.Errorf("关闭导入临时文件失败: %w", err)
	}
	return file.Name(), func() {
		_ = os.Remove(file.Name())
	}, nil
}

func inspectImportFile(localPath, fileName, rawMime string) (int64, string, string, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return 0, "", "", fmt.Errorf("读取导入文件失败: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return 0, "", "", fmt.Errorf("读取导入文件信息失败: %w", err)
	}

	header := make([]byte, 512)
	n, _ := file.Read(header)
	mimeType := detectUploadMimeFromHeader(header[:n], fileName, rawMime)
	if _, err = file.Seek(0, 0); err != nil {
		return 0, "", "", fmt.Errorf("重置导入文件读取位置失败: %w", err)
	}

	hasher := sha256.New()
	if _, err = io.Copy(hasher, file); err != nil {
		return 0, "", "", fmt.Errorf("计算导入文件摘要失败: %w", err)
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	if len(hash) > 32 {
		hash = hash[:32]
	}
	return info.Size(), hash, mimeType, nil
}
