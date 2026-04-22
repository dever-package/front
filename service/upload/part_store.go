package upload

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func uploadSessionPartDir(sessionID uint64) string {
	return filepath.Join(resolveUploadFileRoot(), ".session", strconv.FormatUint(sessionID, 10))
}

func uploadSessionPartPath(sessionID uint64, partNumber int) string {
	return filepath.Join(uploadSessionPartDir(sessionID), fmt.Sprintf("part-%06d.tmp", partNumber))
}

func uploadSessionMergedPath(sessionID uint64) string {
	return filepath.Join(uploadSessionPartDir(sessionID), "merged.bin")
}

func resolveUploadDataRoot() string {
	return "data"
}

func resolveUploadFileRoot() string {
	return filepath.Join(resolveUploadDataRoot(), "upload")
}

func saveUploadSessionMarker(sessionID uint64) error {
	return os.MkdirAll(uploadSessionPartDir(sessionID), 0o755)
}

func saveUploadPart(sessionID uint64, partNumber int, src io.Reader) error {
	partPath := uploadSessionPartPath(sessionID, partNumber)
	if err := os.MkdirAll(filepath.Dir(partPath), 0o755); err != nil {
		return fmt.Errorf("创建分片目录失败: %w", err)
	}

	dst, err := os.Create(partPath)
	if err != nil {
		return fmt.Errorf("写入分片失败: %w", err)
	}
	if _, err = io.Copy(dst, src); err != nil {
		dst.Close()
		return fmt.Errorf("保存分片失败: %w", err)
	}
	if err = dst.Close(); err != nil {
		return fmt.Errorf("保存分片失败: %w", err)
	}
	return nil
}

func appendUploadPart(encoded string, partNumber int) []int {
	parts := decodeUploadParts(encoded)
	for _, current := range parts {
		if current == partNumber {
			return parts
		}
	}
	parts = append(parts, partNumber)
	sort.Ints(parts)
	return parts
}

func decodeUploadParts(encoded string) []int {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return []int{}
	}
	var parts []int
	if err := json.Unmarshal([]byte(encoded), &parts); err != nil {
		return []int{}
	}
	sort.Ints(parts)
	return parts
}

func encodeUploadParts(parts []int) string {
	if len(parts) == 0 {
		return "[]"
	}
	bytes, err := json.Marshal(parts)
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

func mergeUploadSessionParts(session resolvedUploadSession) (string, string, int64, error) {
	mergedPath := uploadSessionMergedPath(session.ID)
	if err := os.MkdirAll(filepath.Dir(mergedPath), 0o755); err != nil {
		return "", "", 0, fmt.Errorf("创建上传目录失败: %w", err)
	}

	dst, err := os.Create(mergedPath)
	if err != nil {
		return "", "", 0, fmt.Errorf("创建合并文件失败: %w", err)
	}
	defer dst.Close()

	hasher := sha256.New()
	size := int64(0)
	writer := io.MultiWriter(dst, hasher)
	for index := 1; index <= session.ChunkTotal; index++ {
		partPath := uploadSessionPartPath(session.ID, index)
		src, openErr := os.Open(partPath)
		if openErr != nil {
			return "", "", 0, fmt.Errorf("上传分片缺失")
		}
		n, copyErr := io.Copy(writer, src)
		_ = src.Close()
		if copyErr != nil {
			return "", "", 0, fmt.Errorf("合并分片失败: %w", copyErr)
		}
		size += n
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	if len(hash) > 32 {
		hash = hash[:32]
	}
	return mergedPath, hash, size, nil
}

func cleanupUploadSession(sessionID uint64) error {
	return os.RemoveAll(uploadSessionPartDir(sessionID))
}
