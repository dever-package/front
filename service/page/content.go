package page

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"path/filepath"
	"strings"

	frontroot "github.com/dever-package/front"
	"github.com/shemic/dever/util"
)

const pageDirName = "page"

var pageFileExtCandidates = []string{
	".jsonc",
	".json",
}

var embeddedContentCache util.ConcurrentMap[string, []byte]

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
	moduleName, fileName, err := splitPagePath(pathValue)
	if err != nil {
		return nil, err
	}

	for _, ext := range pageFileExtCandidates {
		diskPath := filepath.Join("module", moduleName, pageDirName, fileName+ext)
		if content, _, readErr := util.ReadJSONCFile(diskPath); readErr == nil {
			if !json.Valid(content) {
				return nil, fmt.Errorf("页面配置格式错误")
			}
			return content, nil
		}
	}

	if moduleName == "front" {
		for _, fullPath := range buildEmbeddedJSONPaths(fileName) {
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

func IsPageDir(name string) bool {
	return name == pageDirName
}

func IsPageFileName(name string) bool {
	for _, ext := range pageFileExtCandidates {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func TrimPageFileExt(path string) string {
	for _, ext := range pageFileExtCandidates {
		if strings.HasSuffix(path, ext) {
			return strings.TrimSuffix(path, ext)
		}
	}
	return path
}

func PageFilePriority(name string) int {
	for index, ext := range pageFileExtCandidates {
		if strings.HasSuffix(name, ext) {
			return index
		}
	}
	return len(pageFileExtCandidates)
}

func splitPagePath(pathValue string) (string, string, error) {
	if pathValue == "" {
		return "", "", fmt.Errorf("页面路径不能为空")
	}

	cleanPath := filepath.ToSlash(filepath.Clean(pathValue))
	cleanPath = strings.Trim(cleanPath, "/")
	if cleanPath == "." || cleanPath == "" {
		return "", "", fmt.Errorf("页面路径不能为空")
	}
	if strings.HasPrefix(cleanPath, "../") || strings.Contains(cleanPath, "/../") {
		return "", "", fmt.Errorf("页面路径不合法")
	}

	parts := strings.Split(cleanPath, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("页面路径格式错误")
	}

	return parts[0], filepath.Join(parts[1:]...), nil
}

func buildJSONPaths(moduleName, fileName string) []string {
	result := make([]string, 0, len(pageFileExtCandidates))
	for _, ext := range pageFileExtCandidates {
		fullPath := filepath.ToSlash(filepath.Join(moduleName, pageDirName, fileName+ext))
		result = append(result, filepath.Clean(fullPath))
	}
	return result
}

func buildEmbeddedJSONPaths(fileName string) []string {
	result := make([]string, 0, len(pageFileExtCandidates))
	for _, ext := range pageFileExtCandidates {
		fullPath := filepath.ToSlash(filepath.Join(pageDirName, fileName+ext))
		result = append(result, filepath.Clean(fullPath))
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
