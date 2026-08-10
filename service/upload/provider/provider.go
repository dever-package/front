package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	frontmodel "github.com/dever-package/front/model"
)

type Rule struct {
	Storage      frontmodel.UploadStorage
	MimeLimit    string
	MaxSizeBytes int64
}

type Session struct {
	ObjectKey           string
	ObjectKeyCandidates []string
	NameCandidates      []string
	PathMode            string
}

type File struct {
	Path        string
	ProviderKey string
	Storage     frontmodel.UploadStorage
}

type SaveInput struct {
	Rule                Rule
	Session             Session
	LocalPath           string
	ObjectKey           string
	ObjectKeyCandidates []string
	Name                string
	NameCandidates      []string
	SourceKey           string
	SourceName          string
	Mime                string
	Size                int64
	Hash                string
	Ext                 string
	PathMode            string
	Progress            func(loaded int64, total int64)
}

type SaveResult struct {
	ProviderKey string
	StoredName  string
}

type DirectInitResult struct {
	UploadURL   string            `json:"uploadURL"`
	Fields      map[string]string `json:"fields"`
	Method      string            `json:"method"`
	ProviderKey string            `json:"-"`
	StoredName  string            `json:"-"`
}

type saveCandidate struct {
	ObjectKey string
	Name      string
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

type ThumbnailURLDriver interface {
	ResolveThumbnailURL(File) string
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

func resolveSaveCandidates(input SaveInput) []saveCandidate {
	objectKeys := input.ObjectKeyCandidates
	if len(objectKeys) == 0 {
		objectKeys = []string{input.ObjectKey}
	}

	result := make([]saveCandidate, 0, len(objectKeys))
	seen := make(map[string]struct{}, len(objectKeys))
	for index, objectKey := range objectKeys {
		objectKey = strings.TrimSpace(objectKey)
		if objectKey == "" {
			continue
		}
		if _, exists := seen[objectKey]; exists {
			continue
		}
		seen[objectKey] = struct{}{}

		name := strings.TrimSpace(input.Name)
		if index < len(input.NameCandidates) {
			name = strings.TrimSpace(input.NameCandidates[index])
		}
		result = append(result, saveCandidate{ObjectKey: objectKey, Name: name})
	}
	return result
}

func ResolveFileProviderKey(file File) string {
	if providerKey := strings.TrimSpace(file.ProviderKey); providerKey != "" {
		return providerKey
	}
	return strings.TrimSpace(file.Path)
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

func ResolveThumbnailURL(driver Driver, file File) string {
	capability, ok := driver.(ThumbnailURLDriver)
	if !ok {
		return ""
	}
	return strings.TrimSpace(capability.ResolveThumbnailURL(file))
}

func IsVideoFile(kind, mimeType, extension string) bool {
	if strings.EqualFold(strings.TrimSpace(kind), "video") {
		return true
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "video/") {
		return true
	}
	extension = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
	switch extension {
	case "mp4", "mov", "webm", "avi", "mkv", "m4v", "ogv":
		return true
	default:
		return false
	}
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

func JoinPublicURL(domain, objectKey string) string {
	domain = strings.TrimRight(strings.TrimSpace(domain), "/")
	objectKey = strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	if domain == "" || objectKey == "" {
		return ""
	}
	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		domain = "https://" + domain
	}
	return domain + "/" + EscapeObjectKeyPath(objectKey)
}

func EscapeObjectKeyPath(objectKey string) string {
	objectKey = strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	segments := strings.Split(objectKey, "/")
	for index, segment := range segments {
		segments[index] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
