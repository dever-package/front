package upload

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	operationlog "my/package/front/service/operationlog"
	uploadprovider "my/package/front/service/upload/provider"
	uploadrepo "my/package/front/service/upload/repository"
)

func InitUpload(c *server.Context) error {
	var input uploadInitInput
	if err := c.BindJSON(&input); err != nil {
		return c.Error("请求体格式错误")
	}

	rule, err := uploadrepo.FindUploadRule(c.Context(), input.RuleID)
	if err != nil {
		return c.Error(err)
	}
	if err := validateUploadInit(rule, input); err != nil {
		return c.Error(err)
	}

	hash := normalizeUploadHash(input.Hash)
	ext := resolveUploadExt(input.Name, input.Mime)
	kind := resolveUploadKind(input.Kind, input.Name, input.Mime)
	bizRecord, err := uploadrepo.EnsureUploadBiz(c.Context(), input.BizKey, input.BizName)
	if err != nil {
		return c.Error(err)
	}
	categoryID, err := uploadrepo.EnsureUploadCateID(c.Context(), input.CategoryID)
	if err != nil {
		return c.Error(err)
	}

	objectKey := ""
	if hash != "" {
		objectKey = buildUploadObjectKey(rule.ID, hash, ext, bizRecord.Key)
	}

	chunkSize := uploadRuleChunkSizeBytes(rule)
	chunkTotal := int((input.Size + chunkSize - 1) / chunkSize)
	if chunkTotal <= 0 {
		chunkTotal = 1
	}

	sessionModel, err := uploadrepo.ResolveSessionModel()
	if err != nil {
		return c.Error(err)
	}
	sessionToken, err := newUploadSessionToken()
	if err != nil {
		return c.Error(err)
	}

	sessionID := util.ToUint64(sessionModel.Insert(c.Context(), map[string]any{
		"rule_id":            rule.ID,
		"storage_id":         rule.StorageID,
		"kind":               kind,
		"biz_id":             bizRecord.ID,
		"category_id":        categoryID,
		"name":               strings.TrimSpace(input.Name),
		"ext":                ext,
		"mime":               strings.TrimSpace(input.Mime),
		"size":               input.Size,
		"hash":               hash,
		"token":              sessionToken,
		"object_key":         objectKey,
		"chunk_size":         chunkSize,
		"chunk_total":        chunkTotal,
		"uploaded_parts":     "[]",
		"provider_upload_id": "",
		"status":             uploadSessionPending,
		"created_at":         time.Now(),
		"expired_at":         time.Now().Add(6 * time.Hour),
	}))
	if sessionID == 0 {
		return c.Error("创建上传会话失败")
	}

	if err := ensureUploadSessionDir(sessionID); err != nil {
		return c.Error(err)
	}

	session := resolvedUploadSession{
		ID:         sessionID,
		RuleID:     rule.ID,
		StorageID:  rule.StorageID,
		Kind:       kind,
		BizID:      bizRecord.ID,
		BizKey:     bizRecord.Key,
		BizName:    bizRecord.Name,
		CategoryID: categoryID,
		Name:       strings.TrimSpace(input.Name),
		Ext:        ext,
		Mime:       strings.TrimSpace(input.Mime),
		Size:       input.Size,
		Hash:       hash,
		Token:      sessionToken,
		ObjectKey:  objectKey,
		ChunkSize:  chunkSize,
		ChunkTotal: chunkTotal,
		Status:     uploadSessionPending,
	}

	result := map[string]any{
		"session_id":  sessionID,
		"token":       sessionToken,
		"transport":   rule.Transport,
		"chunk_size":  chunkSize,
		"chunk_total": chunkTotal,
	}

	if strings.EqualFold(rule.Transport, "direct") {
		provider, err := uploadprovider.Resolve(resolveUploadStorageProvider(rule.Storage))
		if err != nil {
			return c.Error(err)
		}
		direct, err := provider.InitDirect(c.Context(), uploadprovider.Rule{
			Storage:      rule.Storage,
			Accept:       rule.Accept,
			MaxSizeBytes: uploadRuleMaxSizeBytes(rule),
		}, uploadprovider.Session{
			ObjectKey: session.ObjectKey,
		})
		if err != nil {
			return c.Error(err)
		}
		result["direct"] = direct
	}

	return c.JSON(result)
}

func UploadPart(c *server.Context) error {
	sessionID := util.ToUint64(c.Input("session_id", "required", "上传会话"))
	partNumber, err := strconv.Atoi(strings.TrimSpace(c.Input("part_number", "required", "分片序号")))
	if err != nil || partNumber <= 0 {
		return c.Error("分片序号无效")
	}

	session, err := uploadrepo.FindUploadSession(c.Context(), sessionID)
	if err != nil {
		return c.Error(err)
	}
	if err := ensureUploadSessionToken(session, c.Input("token")); err != nil {
		return c.Error(err)
	}
	rule, err := uploadrepo.FindUploadRule(c.Context(), session.RuleID)
	if err != nil {
		return c.Error(err)
	}
	if strings.EqualFold(rule.Transport, "direct") {
		return c.Error("当前上传规则不接收后端分片")
	}
	if partNumber > session.ChunkTotal {
		return c.Error("分片序号超出范围")
	}

	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前上传环境不支持分片")
	}
	fileHeader, err := raw.FormFile("file")
	if err != nil {
		return c.Error("上传文件不能为空")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return c.Error("读取上传分片失败")
	}
	defer file.Close()

	if err := saveUploadPart(sessionID, partNumber, file, uploadPartMaxBytes(session, partNumber)); err != nil {
		return c.Error(err)
	}

	parts := appendUploadPart(session.UploadedParts, partNumber)
	if err := uploadrepo.UpdateUploadSession(c.Context(), sessionID, map[string]any{
		"uploaded_parts": encodeUploadParts(parts),
		"status":         uploadSessionUploading,
	}); err != nil {
		return c.Error(err)
	}

	return c.JSON(map[string]any{
		"session_id":  sessionID,
		"part_number": partNumber,
	})
}

func CompleteUpload(c *server.Context) error {
	var input uploadCompleteInput
	if err := c.BindJSON(&input); err != nil {
		return c.Error("请求体格式错误")
	}

	session, err := uploadrepo.FindUploadSession(c.Context(), input.SessionID)
	if err != nil {
		return c.Error(err)
	}
	if err := ensureUploadSessionToken(session, input.Token); err != nil {
		return c.Error(err)
	}
	rule, err := uploadrepo.FindUploadRule(c.Context(), session.RuleID)
	if err != nil {
		return c.Error(err)
	}

	if existing := uploadrepo.FindUploadFileByPath(c.Context(), session.ObjectKey); existing != nil && strings.EqualFold(rule.Transport, "direct") {
		_ = updateUploadFileRelationMetaIfNeeded(c.Context(), *existing, session.BizID, session.CategoryID)
		_ = cleanupUploadSession(input.SessionID)
		refreshed, err := uploadrepo.FindUploadFile(c.Context(), existing.ID)
		if err == nil {
			logUploadFile(c, refreshed.ID, input)
			return c.JSON(uploadrepo.BuildUploadFilePayload(refreshed))
		}
		logUploadFile(c, existing.ID, input)
		return c.JSON(uploadrepo.BuildUploadFilePayload(*existing))
	}

	fileRecord, err := completeUploadSession(c.Context(), rule, session)
	if err != nil {
		return c.Error(err)
	}
	_ = cleanupUploadSession(input.SessionID)
	logUploadFile(c, fileRecord.ID, input)
	return c.JSON(uploadrepo.BuildUploadFilePayload(fileRecord))
}

func logUploadFile(c *server.Context, fileID uint64, payload any) {
	operationlog.Record(c, operationlog.Entry{
		Action:      "upload",
		TargetModel: "front.NewUploadFileModel",
		TargetID:    fmt.Sprint(fileID),
		Message:     "上传资源文件",
		Payload:     uploadLogPayload(payload),
	})
}

func uploadLogPayload(payload any) any {
	switch input := payload.(type) {
	case uploadCompleteInput:
		return map[string]any{"session_id": input.SessionID}
	case *uploadCompleteInput:
		if input == nil {
			return nil
		}
		return map[string]any{"session_id": input.SessionID}
	default:
		return payload
	}
}

func uploadPartMaxBytes(session resolvedUploadSession, partNumber int) int64 {
	chunkSize := session.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 2 * uploadSizeMBUnit
	}
	if session.Size <= 0 || partNumber < session.ChunkTotal {
		return chunkSize
	}
	used := int64(partNumber-1) * chunkSize
	remaining := session.Size - used
	if remaining <= 0 || remaining > chunkSize {
		return chunkSize
	}
	return remaining
}

func newUploadSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成上传会话令牌失败")
	}
	return hex.EncodeToString(buf), nil
}

func ensureUploadSessionToken(session resolvedUploadSession, token string) error {
	expected := strings.TrimSpace(session.Token)
	if expected == "" {
		return fmt.Errorf("上传会话令牌无效")
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(expected)) != 1 {
		return fmt.Errorf("上传会话令牌无效")
	}
	return nil
}

func OpenUpload(c *server.Context) error {
	fileID := util.ToUint64(c.Input("id", "required", "文件ID"))
	fileRecord, err := uploadrepo.FindUploadFile(c.Context(), fileID)
	if err != nil {
		return c.Error(err)
	}

	provider, err := uploadprovider.Resolve(resolveUploadStorageProvider(fileRecord.Storage))
	if err != nil {
		return c.Error(err)
	}
	target, err := provider.ResolveOpen(c.Context(), uploadprovider.File{
		Path:    fileRecord.Path,
		Storage: fileRecord.Storage,
	})
	if err != nil {
		return c.Error(err)
	}

	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前上传环境不支持文件输出")
	}
	if target == nil {
		return c.Error("文件不存在")
	}
	if strings.TrimSpace(target.Redirect) != "" {
		return raw.Redirect(strings.TrimSpace(target.Redirect), http.StatusFound)
	}
	if strings.TrimSpace(target.LocalPath) == "" {
		return c.Error("文件不存在")
	}
	return raw.SendFile(strings.TrimSpace(target.LocalPath))
}

func ensureUploadSessionDir(sessionID uint64) error {
	if err := saveUploadSessionMarker(sessionID); err != nil {
		return fmt.Errorf("创建上传会话目录失败: %w", err)
	}
	return nil
}
