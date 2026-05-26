package page

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"path/filepath"
	"strings"

	"github.com/shemic/dever/util"
	frontroot "my/package/front"
	frontpagepath "my/package/front/internal/pagepath"
)

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

	for _, root := range []string{"module", "package"} {
		content, ok, err := readDiskPageContent(root, moduleName, fileName)
		if err != nil {
			return nil, err
		}
		if ok {
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

func readDiskPageContent(root, moduleName, fileName string) ([]byte, bool, error) {
	for _, ext := range frontpagepath.FileExtCandidates() {
		for _, diskPath := range diskPageContentPaths(root, moduleName, fileName+ext) {
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

func diskPageContentPaths(root, moduleName, fileName string) []string {
	paths := make([]string, 0, 2)
	if moduleName != "front" {
		paths = append(paths, filepath.Join(root, moduleName, "front", frontpagepath.DirName, fileName))
	}
	paths = append(paths, filepath.Join(root, moduleName, frontpagepath.DirName, fileName))
	return paths
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
	candidates := frontpagepath.FileExtCandidates()
	result := make([]string, 0, len(candidates))
	for _, ext := range candidates {
		fullPath := filepath.ToSlash(filepath.Join(moduleName, frontpagepath.DirName, fileName+ext))
		result = append(result, filepath.Clean(fullPath))
	}
	return result
}

func buildEmbeddedJSONPaths(fileName string) []string {
	candidates := frontpagepath.FileExtCandidates()
	result := make([]string, 0, len(candidates))
	for _, ext := range candidates {
		fullPath := filepath.ToSlash(filepath.Join(frontpagepath.DirName, fileName+ext))
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
