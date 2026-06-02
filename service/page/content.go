package page

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"path/filepath"
	"strings"

	"github.com/shemic/dever/util"
	frontroot "my/package/front"
	frontpagepath "my/package/front/internal/pagepath"
	"my/package/front/service/siteconfig"
)

var embeddedContentCache util.ConcurrentMap[string, []byte]

var (
	errEmptyPagePath   = errors.New("页面路径不能为空")
	errInvalidPagePath = errors.New("页面路径不合法")
	errPagePathFormat  = errors.New("页面路径格式错误")
)

type ContentSignature struct {
	checksum uint32
	size     int
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

	for _, root := range []string{"module", "package"} {
		content, ok, err := readDiskPageContent(root, moduleName, pageName, fileName)
		if err != nil {
			return nil, err
		}
		if ok {
			return content, nil
		}
	}

	if moduleName == "front" {
		for _, fullPath := range buildEmbeddedJSONPaths(pageName, fileName) {
			content, ok, readErr := readCachedEmbeddedContent(fullPath)
			if readErr != nil {
				return nil, readErr
			}
			if ok {
				return content, nil
			}
		}
	}

	return nil, fmt.Errorf("页面配置不存在")
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

func readDiskPageContent(root, moduleName, pageName, fileName string) ([]byte, bool, error) {
	for _, ext := range frontpagepath.FileExtCandidates() {
		for _, diskPath := range diskPageContentPaths(root, moduleName, pageName, fileName+ext) {
			content, _, readErr := util.ReadJSONCFile(diskPath)
			if readErr != nil {
				continue
			}
			if !json.Valid(content) {
				return nil, false, fmt.Errorf("页面配置格式错误")
			}
			return content, true, nil
		}
	}
	return nil, false, nil
}

func diskPageContentPaths(root, moduleName, pageName, fileName string) []string {
	pageName = strings.Trim(strings.TrimSpace(pageName), "/")
	paths := make([]string, 0, 4)
	if moduleName != "front" {
		if pageName != "" {
			paths = append(paths, filepath.Join(root, moduleName, "front", frontpagepath.DirName, pageName, fileName))
		}
		if shouldReadLegacyPageDir(pageName) {
			paths = append(paths, filepath.Join(root, moduleName, "front", frontpagepath.DirName, fileName))
		}
	}
	if pageName != "" {
		paths = append(paths, filepath.Join(root, moduleName, frontpagepath.DirName, pageName, fileName))
	}
	if shouldReadLegacyPageDir(pageName) {
		paths = append(paths, filepath.Join(root, moduleName, frontpagepath.DirName, fileName))
	}
	return paths
}

func shouldReadLegacyPageDir(pageName string) bool {
	return pageName == "" || pageName == siteconfig.DefaultPage
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

func buildEmbeddedJSONPaths(pageName, fileName string) []string {
	candidates := frontpagepath.FileExtCandidates()
	pageName = strings.Trim(strings.TrimSpace(pageName), "/")
	result := make([]string, 0, len(candidates)*2)
	for _, ext := range candidates {
		if pageName != "" {
			sitePath := filepath.ToSlash(filepath.Join(frontpagepath.DirName, pageName, fileName+ext))
			result = append(result, filepath.Clean(sitePath))
		}
		if shouldReadLegacyPageDir(pageName) {
			fullPath := filepath.ToSlash(filepath.Join(frontpagepath.DirName, fileName+ext))
			result = append(result, filepath.Clean(fullPath))
		}
	}
	return result
}

func readCachedEmbeddedContent(fullPath string) ([]byte, bool, error) {
	if cached, ok := embeddedContentCache.Load(fullPath); ok {
		return cached, true, nil
	}

	content, readErr := frontroot.PageFS.ReadFile(fullPath)
	if readErr != nil {
		return nil, false, nil
	}

	normalized, err := util.NormalizeJSONC(content)
	if err != nil || !json.Valid(normalized) {
		return nil, false, fmt.Errorf("页面配置格式错误")
	}

	embeddedContentCache.Store(fullPath, normalized)
	return normalized, true, nil
}
