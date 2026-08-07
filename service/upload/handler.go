package upload

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	operationlog "github.com/dever-package/front/service/operationlog"
	uploadaccess "github.com/dever-package/front/service/upload/access"
	uploadprovider "github.com/dever-package/front/service/upload/provider"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

type uploadSessionMutex struct {
	mutex sync.Mutex
	refs  int
}

var uploadSessionMutexes = struct {
	sync.Mutex
	items map[uint64]*uploadSessionMutex
}{
	items: make(map[uint64]*uploadSessionMutex),
}

func InitUpload(c *server.Context) error {
	var input uploadInitInput
	if err := c.BindJSON(&input); err != nil {
		return c.Error("请求体格式错误")
	}
	input.Mime = resolveUploadMimeType(input.Name, input.Mime)

	kind := resolveUploadKind(input.Kind, input.Name, input.Mime)
	if err := requireUploadCreateAccess(c, uploadCreateAccessInput{
		BizKey:     input.BizKey,
		Kind:       kind,
		CategoryID: input.CategoryID,
	}); err != nil {
		return err
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
	if objectKey != "" {
		if existing := uploadrepo.FindUploadFileByPath(c.Context(), objectKey); existing != nil {
			if err := uploadaccess.EnsureFile(c, uploadaccess.OperationRead, *existing); err != nil {
				return c.Error(err, uploadaccess.Status(err))
			}
			reused := reuseExistingUploadFile(c.Context(), *existing, bizRecord.ID, categoryID)
			return c.JSON(map[string]any{
				"transport": rule.Transport,
				"reused":    uploadrepo.BuildUploadFilePayload(reused),
			})
		}
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
	now := time.Now()
	expiredAt := now.Add(6 * time.Hour)
	session := resolvedUploadSession{
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
		ExpiredAt:  expiredAt,
	}

	var direct *uploadprovider.DirectInitResult
	if strings.EqualFold(rule.Transport, "direct") {
		var unlockName func()
		session, direct, unlockName, err = prepareDirectUpload(c.Context(), rule, session)
		if err != nil {
			return c.Error(err)
		}
		defer unlockName()
	}

	sessionID := util.ToUint64(sessionModel.Insert(c.Context(), map[string]any{
		"rule_id":            rule.ID,
		"storage_id":         rule.StorageID,
		"kind":               kind,
		"biz_id":             bizRecord.ID,
		"category_id":        categoryID,
		"name":               session.Name,
		"ext":                ext,
		"mime":               strings.TrimSpace(input.Mime),
		"size":               input.Size,
		"hash":               hash,
		"token":              sessionToken,
		"object_key":         objectKey,
		"provider_key":       session.ProviderKey,
		"chunk_size":         chunkSize,
		"chunk_total":        chunkTotal,
		"uploaded_parts":     "[]",
		"provider_upload_id": "",
		"status":             uploadSessionPending,
		"created_at":         now,
		"expired_at":         expiredAt,
	}))
	if sessionID == 0 {
		return c.Error("创建上传会话失败")
	}

	if err := ensureUploadSessionDir(sessionID); err != nil {
		return c.Error(err)
	}
	session.ID = sessionID

	result := map[string]any{
		"session_id":  sessionID,
		"token":       sessionToken,
		"transport":   rule.Transport,
		"chunk_size":  chunkSize,
		"chunk_total": chunkTotal,
		"mime":        session.Mime,
	}

	if direct != nil {
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
	unlock := lockUploadSession(sessionID)
	defer unlock()

	session, err = uploadrepo.FindUploadSession(c.Context(), sessionID)
	if err != nil {
		return c.Error(err)
	}
	if err := ensureUploadSessionToken(session, c.Input("token")); err != nil {
		return c.Error(err)
	}
	if err := ensureUploadSessionActive(session); err != nil {
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

	if err := recordUploadPartLocked(c.Context(), sessionID, partNumber); err != nil {
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
	unlock := lockUploadSession(input.SessionID)
	defer unlock()

	session, err = uploadrepo.FindUploadSession(c.Context(), input.SessionID)
	if err != nil {
		return c.Error(err)
	}
	if err := ensureUploadSessionToken(session, input.Token); err != nil {
		return c.Error(err)
	}
	if strings.EqualFold(session.Status, uploadSessionComplete) {
		if existing := uploadrepo.FindUploadFileByPath(c.Context(), session.ObjectKey); existing != nil {
			_ = cleanupUploadSession(input.SessionID)
			return c.JSON(uploadrepo.BuildUploadFilePayload(*existing))
		}
		return c.Error("上传会话已完成")
	}
	if err := ensureUploadSessionActive(session); err != nil {
		return c.Error(err)
	}
	rule, err := uploadrepo.FindUploadRule(c.Context(), session.RuleID)
	if err != nil {
		return c.Error(err)
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

func ensureUploadSessionActive(session resolvedUploadSession) error {
	if !session.ExpiredAt.IsZero() && !time.Now().Before(session.ExpiredAt) {
		return fmt.Errorf("上传会话已过期，请重新上传")
	}
	if strings.EqualFold(session.Status, uploadSessionComplete) {
		return fmt.Errorf("上传会话已完成")
	}
	return nil
}

func recordUploadPartLocked(ctx context.Context, sessionID uint64, partNumber int) error {
	session, err := uploadrepo.FindUploadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := ensureUploadSessionActive(session); err != nil {
		return err
	}
	parts := appendUploadPart(session.UploadedParts, partNumber)
	return uploadrepo.UpdateUploadSession(ctx, sessionID, map[string]any{
		"uploaded_parts": encodeUploadParts(parts),
		"status":         uploadSessionUploading,
	})
}

func lockUploadSession(sessionID uint64) func() {
	uploadSessionMutexes.Lock()
	entry := uploadSessionMutexes.items[sessionID]
	if entry == nil {
		entry = &uploadSessionMutex{}
		uploadSessionMutexes.items[sessionID] = entry
	}
	entry.refs++
	uploadSessionMutexes.Unlock()

	entry.mutex.Lock()
	return func() {
		entry.mutex.Unlock()
		uploadSessionMutexes.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(uploadSessionMutexes.items, sessionID)
		}
		uploadSessionMutexes.Unlock()
	}
}

func OpenUpload(c *server.Context) error {
	fileID := util.ToUint64(c.Input("id", "required", "文件ID"))
	fileRecord, err := uploadrepo.FindUploadFile(c.Context(), fileID)
	if err != nil {
		return c.Error(err)
	}
	if err := ensureUploadOpenAccess(c, fileRecord); err != nil {
		return c.Error(err, uploadaccess.Status(err))
	}
	raw, ok := c.Raw.(*fiber.Ctx)
	if !ok {
		return c.Error("当前上传环境不支持文件输出")
	}

	provider, err := uploadprovider.Resolve(resolveUploadStorageProvider(fileRecord.Storage))
	if err != nil {
		return c.Error(err)
	}
	target, err := uploadprovider.Open(c.Context(), provider, uploadprovider.OpenInput{
		File: uploadprovider.File{
			Path:        fileRecord.Path,
			ProviderKey: fileRecord.ProviderKey,
			Storage:     fileRecord.Storage,
		},
		ProviderKey: fileRecord.ProviderKey,
		Name:        fileRecord.Name,
		Mime:        fileRecord.Mime,
		Range:       strings.TrimSpace(raw.Get("Range")),
	})
	if err != nil {
		if statusCode, header, ok := uploadprovider.ResolveOpenError(err); ok {
			setUploadOpenErrorHeaders(raw, header)
			return c.Error(err, statusCode)
		}
		return c.Error(err)
	}

	if target == nil {
		return c.Error("文件不存在")
	}
	setUploadResponseHeaders(raw)
	if strings.TrimSpace(target.Redirect) != "" {
		return raw.Redirect(strings.TrimSpace(target.Redirect), http.StatusFound)
	}
	if strings.TrimSpace(target.LocalPath) == "" {
		if target.Stream != nil {
			return sendUploadStream(raw, target)
		}
		return c.Error("文件不存在")
	}
	return raw.SendFile(strings.TrimSpace(target.LocalPath))
}

func setUploadOpenErrorHeaders(raw *fiber.Ctx, header http.Header) {
	for _, name := range []string{"Content-Range", "Retry-After"} {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			raw.Set(name, value)
		}
	}
}

func sendUploadStream(raw *fiber.Ctx, target *uploadprovider.OpenResult) error {
	for _, header := range []string{
		"Content-Type",
		"Content-Disposition",
		"Content-Range",
		"Accept-Ranges",
		"ETag",
		"Last-Modified",
		"Cache-Control",
	} {
		if value := strings.TrimSpace(target.Header.Get(header)); value != "" {
			raw.Set(header, value)
		}
	}
	statusCode := target.StatusCode
	if statusCode <= 0 {
		statusCode = http.StatusOK
	}
	raw.Status(statusCode)
	maxInt := int64(^uint(0) >> 1)
	if target.ContentLength >= 0 && target.ContentLength <= maxInt {
		return raw.SendStream(target.Stream, int(target.ContentLength))
	}
	return raw.SendStream(target.Stream)
}

func setUploadResponseHeaders(raw *fiber.Ctx) {
	raw.Set("X-Content-Type-Options", "nosniff")
}

func ensureUploadSessionDir(sessionID uint64) error {
	if err := saveUploadSessionMarker(sessionID); err != nil {
		return fmt.Errorf("创建上传会话目录失败: %w", err)
	}
	return nil
}
