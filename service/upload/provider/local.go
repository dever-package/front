package provider

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dever-package/front/service/upload/internal/transfer"
)

type localDriver struct{}

func (localDriver) Save(_ context.Context, input SaveInput) error {
	targetPath := ResolveLocalObjectPath(input.ObjectKey)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("创建上传目录失败: %w", err)
	}
	if samePath(input.LocalPath, targetPath) {
		return nil
	}
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	}

	src, err := os.Open(input.LocalPath)
	if err != nil {
		return fmt.Errorf("读取上传临时文件失败: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("创建上传文件失败: %w", err)
	}
	defer dst.Close()

	reader := transfer.WrapReader(src, input.Size, input.Progress)
	if _, err = io.Copy(dst, reader); err != nil {
		return fmt.Errorf("保存上传文件失败: %w", err)
	}
	if input.Progress != nil {
		input.Progress(input.Size, input.Size)
	}
	return nil
}

func (localDriver) InitDirect(_ context.Context, _ Rule, _ Session) (*DirectInitResult, error) {
	return nil, fmt.Errorf("本地上传不支持直传")
}

func (localDriver) ResolveOpen(_ context.Context, file File) (*OpenTarget, error) {
	return &OpenTarget{
		LocalPath: ResolveLocalObjectPath(file.Path),
	}, nil
}

func (localDriver) ResolvePublicURL(file File) string {
	return ResolveLocalPublicURL(file.Storage.Domain, file.Path)
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
