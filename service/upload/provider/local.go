package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	frontmodel "github.com/dever-package/front/model"
	"github.com/dever-package/front/service/upload/internal/transfer"
)

type localDriver struct{}

func (driver localDriver) Save(ctx context.Context, input SaveInput) error {
	_, err := driver.SaveWithResult(ctx, input)
	return err
}

func (localDriver) SaveWithResult(_ context.Context, input SaveInput) (SaveResult, error) {
	candidates := resolveSaveCandidates(input)
	if len(candidates) == 0 {
		return SaveResult{}, fmt.Errorf("本地存储缺少文件路径")
	}

	for _, candidate := range candidates {
		occupied, err := saveLocalCandidate(input, candidate.ObjectKey)
		if err != nil {
			return SaveResult{}, err
		}
		if occupied && input.PathMode != frontmodel.UploadStoragePathModeHash {
			continue
		}
		if input.Progress != nil {
			input.Progress(input.Size, input.Size)
		}
		return SaveResult{ProviderKey: candidate.ObjectKey, StoredName: candidate.Name}, nil
	}
	return SaveResult{}, fmt.Errorf("本地存储目标目录中没有可用文件名")
}

func saveLocalCandidate(input SaveInput, objectKey string) (bool, error) {
	targetPath := ResolveLocalObjectPath(objectKey)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return false, fmt.Errorf("创建上传目录失败: %w", err)
	}
	if samePath(input.LocalPath, targetPath) {
		return false, nil
	}

	src, err := os.Open(input.LocalPath)
	if err != nil {
		return false, fmt.Errorf("读取上传临时文件失败: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return true, nil
		}
		return false, fmt.Errorf("创建上传文件失败: %w", err)
	}

	reader := transfer.WrapReader(src, input.Size, input.Progress)
	if _, err = io.Copy(dst, reader); err != nil {
		_ = dst.Close()
		_ = os.Remove(targetPath)
		return false, fmt.Errorf("保存上传文件失败: %w", err)
	}
	if err = dst.Close(); err != nil {
		_ = os.Remove(targetPath)
		return false, fmt.Errorf("完成上传文件写入失败: %w", err)
	}
	return false, nil
}

func (localDriver) InitDirect(_ context.Context, _ Rule, _ Session) (*DirectInitResult, error) {
	return nil, fmt.Errorf("本地上传不支持直传")
}

func (localDriver) ResolveOpen(_ context.Context, file File) (*OpenTarget, error) {
	return &OpenTarget{
		LocalPath: ResolveLocalObjectPath(ResolveFileProviderKey(file)),
	}, nil
}

func (localDriver) ResolvePublicURL(file File) string {
	return ResolveLocalPublicURL(file.Storage.Domain, ResolveFileProviderKey(file))
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return left == right
	}
	return leftAbs == rightAbs
}

func resolveUploadDataRoot() string {
	return "data"
}
