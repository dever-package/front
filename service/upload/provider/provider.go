package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"

	frontmodel "github.com/dever-package/front/model"
)

type Rule struct {
	Storage      frontmodel.UploadStorage
	Accept       string
	MaxSizeBytes int64
}

type Session struct {
	ObjectKey string
}

type File struct {
	Path    string
	Storage frontmodel.UploadStorage
}

type SaveInput struct {
	Rule      Rule
	Session   Session
	LocalPath string
	ObjectKey string
	Name      string
	Mime      string
	Size      int64
	Hash      string
	Ext       string
	Progress  func(loaded int64, total int64)
}

type SaveResult struct {
	ProviderKey string
}

type DirectInitResult struct {
	UploadURL string            `json:"uploadURL"`
	Fields    map[string]string `json:"fields"`
	Method    string            `json:"method"`
}

type OpenTarget struct {
	LocalPath string
	Redirect  string
}

type OpenInput struct {
	File        File
	ProviderKey string
	Name        string
	Mime        string
	Range       string
}

type OpenResult struct {
	LocalPath     string
	Redirect      string
	Stream        io.ReadCloser
	StatusCode    int
	ContentLength int64
	Header        http.Header
}

type OpenError struct {
	Err        error
	StatusCode int
	Header     http.Header
}

func (err *OpenError) Error() string {
	if err == nil || err.Err == nil {
		return "读取存储文件失败"
	}
	return err.Err.Error()
}

func (err *OpenError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

type Driver interface {
	Save(context.Context, SaveInput) error
	InitDirect(context.Context, Rule, Session) (*DirectInitResult, error)
	ResolveOpen(context.Context, File) (*OpenTarget, error)
	ResolvePublicURL(File) string
}

type SaveResultDriver interface {
	SaveWithResult(context.Context, SaveInput) (SaveResult, error)
}

type OpenResultDriver interface {
	ResolveOpenWithResult(context.Context, OpenInput) (*OpenResult, error)
}

type StorageChecker interface {
	CheckStorage(context.Context, frontmodel.UploadStorage) error
}

type SignedPublicOpenDriver interface {
	UsesSignedPublicOpen() bool
}

var drivers = map[string]Driver{
	"local":  localDriver{},
	"qiniu":  qiniuDriver{},
	"feishu": feishuDriver{},
}

func Resolve(name string) (Driver, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if driver, exists := drivers[normalized]; exists {
		return driver, nil
	}
	return nil, fmt.Errorf("上传 provider 不支持: %s", name)
}

func Save(ctx context.Context, driver Driver, input SaveInput) (SaveResult, error) {
	if resultDriver, ok := driver.(SaveResultDriver); ok {
		return resultDriver.SaveWithResult(ctx, input)
	}
	if err := driver.Save(ctx, input); err != nil {
		return SaveResult{}, err
	}
	return SaveResult{ProviderKey: input.ObjectKey}, nil
}

func Open(ctx context.Context, driver Driver, input OpenInput) (*OpenResult, error) {
	if resultDriver, ok := driver.(OpenResultDriver); ok {
		result, err := resultDriver.ResolveOpenWithResult(ctx, input)
		if err != nil {
			if result != nil && result.Stream != nil {
				_ = result.Stream.Close()
			}
			return nil, err
		}
		return normalizeStreamOpenResult(result, input), nil
	}
	target, err := driver.ResolveOpen(ctx, input.File)
	if err != nil || target == nil {
		return nil, err
	}
	return &OpenResult{
		LocalPath: target.LocalPath,
		Redirect:  target.Redirect,
	}, nil
}

func normalizeStreamOpenResult(result *OpenResult, input OpenInput) *OpenResult {
	if result == nil || result.Stream == nil {
		return result
	}
	if result.Header == nil {
		result.Header = make(http.Header)
	}
	if shouldUseStoredContentType(result.Header.Get("Content-Type")) {
		if contentType := strings.TrimSpace(input.Mime); contentType != "" {
			result.Header.Set("Content-Type", contentType)
		}
	}
	if fileName := openFileName(input.Name); fileName != "" {
		result.Header.Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{
			"filename": fileName,
		}))
	}
	return result
}

func shouldUseStoredContentType(value string) bool {
	contentType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return true
	}
	switch strings.ToLower(contentType) {
	case "", "application/octet-stream", "binary/octet-stream":
		return true
	default:
		return false
	}
}

func openFileName(value string) string {
	fileName := path.Base(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"))
	if fileName == "." || fileName == "/" {
		return ""
	}
	return fileName
}

func CheckStorage(ctx context.Context, storage frontmodel.UploadStorage) error {
	driver, err := Resolve(storage.Type)
	if err != nil {
		return err
	}
	checker, ok := driver.(StorageChecker)
	if !ok {
		return fmt.Errorf("%s存储暂不支持连接检测", strings.TrimSpace(storage.Name))
	}
	return checker.CheckStorage(ctx, storage)
}

func UsesSignedPublicOpen(driver Driver) bool {
	capability, ok := driver.(SignedPublicOpenDriver)
	return ok && capability.UsesSignedPublicOpen()
}

func ResolveOpenError(err error) (int, http.Header, bool) {
	var openErr *OpenError
	if !errors.As(err, &openErr) || openErr == nil {
		return 0, nil, false
	}
	statusCode := openErr.StatusCode
	if statusCode < http.StatusBadRequest || statusCode > 599 {
		statusCode = http.StatusBadGateway
	}
	return statusCode, openErr.Header.Clone(), true
}

func JoinPublicURL(domain, path string) string {
	domain = strings.TrimRight(strings.TrimSpace(domain), "/")
	path = strings.TrimLeft(strings.TrimSpace(path), "/")
	if domain == "" || path == "" {
		return ""
	}
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		domain = "https://" + domain
	}
	return domain + "/" + path
}
