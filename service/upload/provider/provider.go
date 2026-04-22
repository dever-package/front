package provider

import (
	"context"
	"fmt"
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
}

type DirectInitResult struct {
	UploadURL string
	Fields    map[string]string
	Method    string
}

type OpenTarget struct {
	LocalPath string
	Redirect  string
}

type Driver interface {
	Save(context.Context, SaveInput) error
	InitDirect(context.Context, Rule, Session) (*DirectInitResult, error)
	ResolveOpen(context.Context, File) (*OpenTarget, error)
	ResolvePublicURL(File) string
}

func Resolve(name string) (Driver, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "local":
		return localDriver{}, nil
	case "qiniu":
		return qiniuDriver{}, nil
	default:
		return nil, fmt.Errorf("上传 provider 不支持: %s", name)
	}
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
