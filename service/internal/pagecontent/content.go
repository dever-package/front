package pagecontent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	devercache "github.com/shemic/dever/cache"
	"github.com/shemic/dever/component"
	"github.com/shemic/dever/util"

	frontpagepath "my/package/front/internal/pagepath"
	"my/package/front/service/siteconfig"
)

var (
	componentPageListCache = devercache.New[string, []ComponentPage](
		devercache.WithTTL(5*time.Minute),
		devercache.WithMaxEntries(128),
	)
	componentPageContentCache = devercache.New[string, map[string][]byte](
		devercache.WithTTL(5*time.Minute),
		devercache.WithMaxEntries(128),
	)
)

var (
	errEmptyPagePath   = errors.New("页面路径不能为空")
	errInvalidPagePath = errors.New("页面路径不合法")
	errPagePathFormat  = errors.New("页面路径格式错误")
)

type ContentSignature struct {
	checksum uint32
	size     int
}

type ComponentPage struct {
	Component component.Component
	Path      string
	Content   []byte
	FileName  string
}

type indexedComponentPage struct {
	page     ComponentPage
	priority int
}

func Signature(content []byte) ContentSignature {
	return ContentSignature{
		checksum: crc32.ChecksumIEEE(content),
		size:     len(content),
	}
}

func ReadContent(pathValue string) ([]byte, error) {
	return ReadContentForPage(siteconfig.DefaultPage, pathValue)
}

func ReadContentForContext(ctx context.Context, pathValue string) ([]byte, error) {
	return ReadContentForPage(siteconfig.PageFromContext(ctx), pathValue)
}

func ReadContentForSite(siteKey string, pathValue string) ([]byte, error) {
	cfg, err := siteconfig.Load(context.Background())
	if err == nil {
		if site, ok := cfg.FindBySiteKey(siteKey); ok {
			return ReadContentForPage(site.Page, pathValue)
		}
	}
	return ReadContentForPage(siteKey, pathValue)
}

func ReadContentForPage(pageName string, pathValue string) ([]byte, error) {
	moduleName, fileName, err := splitPagePathForPage(pageName, pathValue)
	if err != nil {
		return nil, err
	}

	contentByPath, err := componentPageContentIndex(pageName)
	if err != nil {
		return nil, err
	}
	routePath := frontpagepath.NormalizePath(filepath.ToSlash(filepath.Join(moduleName, fileName)))
	if content, ok := contentByPath[routePath]; ok {
		return cloneBytes(content), nil
	}

	return nil, fmt.Errorf("页面配置不存在")
}

func WalkComponentPages(pageName string, visit func(ComponentPage) error) error {
	pages, err := ListComponentPages(pageName)
	if err != nil {
		return err
	}
	for _, page := range pages {
		if err := visit(page); err != nil {
			return err
		}
	}
	return nil
}

func ListComponentPages(pageName string) ([]ComponentPage, error) {
	pageName = cleanPageName(pageName)
	pages, err := componentPageListCache.GetOrSet(pageName, func() ([]ComponentPage, error) {
		return loadComponentPages(pageName)
	})
	if err != nil {
		return nil, err
	}
	return cloneComponentPages(pages), nil
}

func ClearContentCache() {
	componentPageListCache.Clear()
	componentPageContentCache.Clear()
}

func loadComponentPages(pageName string) ([]ComponentPage, error) {
	pageNames := siteconfig.LoadPageNames()
	pageByPath := map[string]indexedComponentPage{}
	for _, current := range component.Active() {
		if current.PageFS == nil {
			continue
		}
		prefix := cleanFSPrefix(current.PagePrefix)
		err := fs.WalkDir(current.PageFS, prefix, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry == nil || entry.IsDir() || !frontpagepath.IsPageFileName(entry.Name()) {
				return nil
			}

			relativePath := relativeFSPath(prefix, path)
			relativeParts := frontpagepath.RelativePartsForPage(relativePath, pageName, pageNames)
			if len(relativeParts) == 0 {
				return nil
			}
			routePath := frontpagepath.TrimPageFileExt(filepath.ToSlash(filepath.Join(append([]string{current.Name}, relativeParts...)...)))
			routePath = frontpagepath.NormalizePath(routePath)
			if routePath == "" {
				return nil
			}

			content, err := readPageFSContent(current.PageFS, path)
			if err != nil {
				return err
			}
			page := ComponentPage{
				Component: current,
				Path:      routePath,
				Content:   content,
				FileName:  entry.Name(),
			}
			priority := frontpagepath.PageFilePriority(entry.Name())
			if existing, ok := pageByPath[routePath]; ok && existing.priority <= priority {
				return nil
			}
			pageByPath[routePath] = indexedComponentPage{
				page:     page,
				priority: priority,
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	paths := make([]string, 0, len(pageByPath))
	for path := range pageByPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	pages := make([]ComponentPage, 0, len(paths))
	for _, path := range paths {
		pages = append(pages, pageByPath[path].page)
	}
	return pages, nil
}

func componentPageContentIndex(pageName string) (map[string][]byte, error) {
	pageName = cleanPageName(pageName)
	return componentPageContentCache.GetOrSet(pageName, func() (map[string][]byte, error) {
		pages, err := ListComponentPages(pageName)
		if err != nil {
			return nil, err
		}
		contentByPath := make(map[string][]byte, len(pages))
		for _, page := range pages {
			contentByPath[page.Path] = page.Content
		}
		return contentByPath, nil
	})
}

func splitPagePathForPage(pageName string, pathValue string) (string, string, error) {
	moduleName, fileName, err := splitPagePath(pathValue)
	if err == nil {
		return moduleName, fileName, nil
	}
	if !errors.Is(err, errPagePathFormat) || !canUseSiteLocalPagePath(pageName) {
		return "", "", err
	}

	fileName, cleanErr := cleanPagePath(pathValue)
	if cleanErr != nil {
		return "", "", cleanErr
	}
	return pageName, fileName, nil
}

func canUseSiteLocalPagePath(pageName string) bool {
	pageName = strings.Trim(strings.TrimSpace(pageName), "/")
	return pageName != "" && pageName != siteconfig.DefaultPage
}

func splitPagePath(pathValue string) (string, string, error) {
	cleanPath, err := cleanPagePath(pathValue)
	if err != nil {
		return "", "", err
	}

	parts := strings.Split(cleanPath, "/")
	if len(parts) < 2 {
		return "", "", errPagePathFormat
	}

	return parts[0], filepath.Join(parts[1:]...), nil
}

func cleanPagePath(pathValue string) (string, error) {
	if pathValue == "" {
		return "", errEmptyPagePath
	}

	cleanPath := filepath.ToSlash(filepath.Clean(pathValue))
	cleanPath = strings.Trim(cleanPath, "/")
	if cleanPath == "." || cleanPath == "" {
		return "", errEmptyPagePath
	}
	if strings.HasPrefix(cleanPath, "../") || strings.Contains(cleanPath, "/../") {
		return "", errInvalidPagePath
	}

	return cleanPath, nil
}

func cleanPageName(pageName string) string {
	pageName = strings.Trim(strings.TrimSpace(pageName), "/")
	if pageName == "" {
		return siteconfig.DefaultPage
	}
	return pageName
}

func cleanFSPrefix(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" || value == "." {
		return "."
	}
	return value
}

func relativeFSPath(prefix string, fullPath string) string {
	fullPath = filepath.ToSlash(fullPath)
	if prefix == "." {
		return fullPath
	}
	return strings.TrimPrefix(fullPath, strings.TrimSuffix(prefix, "/")+"/")
}

func readPageFSContent(source fs.FS, fullPath string) ([]byte, error) {
	content, readErr := fs.ReadFile(source, fullPath)
	if readErr != nil {
		return nil, readErr
	}

	normalized, err := util.NormalizeJSONC(content)
	if err != nil || !json.Valid(normalized) {
		return nil, fmt.Errorf("页面配置格式错误")
	}
	return normalized, nil
}

func cloneComponentPages(pages []ComponentPage) []ComponentPage {
	if len(pages) == 0 {
		return []ComponentPage{}
	}
	cloned := make([]ComponentPage, len(pages))
	for index, page := range pages {
		cloned[index] = page
		cloned[index].Content = cloneBytes(page.Content)
	}
	return cloned
}

func cloneBytes(content []byte) []byte {
	if content == nil {
		return nil
	}
	cloned := make([]byte, len(content))
	copy(cloned, content)
	return cloned
}
