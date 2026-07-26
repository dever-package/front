package upload

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	uploadprovider "github.com/dever-package/front/service/upload/provider"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

func completeUploadSession(ctx context.Context, rule resolvedUploadRule, session resolvedUploadSession) (resolvedUploadFile, error) {
	if rule.Transport == "direct" || strings.EqualFold(rule.Transport, "direct") {
		return completeDirectUploadSession(ctx, rule, session)
	}
	return completeRelayUploadSession(ctx, rule, session)
}

func completeDirectUploadSession(ctx context.Context, rule resolvedUploadRule, session resolvedUploadSession) (resolvedUploadFile, error) {
	if session.ObjectKey == "" {
		return resolvedUploadFile{}, fmt.Errorf("直传文件缺少对象路径")
	}
	if err := validateUploadStoredFile(rule, session.Name, session.Mime); err != nil {
		return resolvedUploadFile{}, err
	}

	unlockObject := lockUploadStoreKey("object:" + session.ObjectKey)
	defer unlockObject()

	var fileRecord resolvedUploadFile
	if existing := uploadrepo.FindUploadFileByPath(ctx, session.ObjectKey); existing != nil {
		fileRecord = reuseExistingUploadFile(ctx, *existing, session.BizID, session.CategoryID)
	} else {
		providerKey := strings.TrimSpace(session.ProviderKey)
		if providerKey == "" {
			resolvedName, unlockName, err := lockAvailableUploadName(
				ctx,
				rule.ID,
				session.BizID,
				session.ID,
				session.Name,
				session.Ext,
				session.Hash,
			)
			if err != nil {
				return resolvedUploadFile{}, err
			}
			defer unlockName()
			session.Name = resolvedName.Name
			providerKey = session.ObjectKey
		}

		persisted, err := persistUploadFile(ctx, rule, session, session.Hash, session.ObjectKey, providerKey)
		if err != nil {
			return resolvedUploadFile{}, err
		}
		fileRecord = persisted
	}
	session.Name = fileRecord.Name
	session.ProviderKey = fileRecord.ProviderKey
	return finalizeUploadResult(ctx, session, fileRecord)
}

func completeRelayUploadSession(ctx context.Context, rule resolvedUploadRule, session resolvedUploadSession) (resolvedUploadFile, error) {
	uploadedParts := decodeUploadParts(session.UploadedParts)
	if len(uploadedParts) < session.ChunkTotal {
		return resolvedUploadFile{}, fmt.Errorf("上传尚未完成")
	}

	mergedPath, hash, size, err := mergeUploadSessionParts(session)
	if err != nil {
		return resolvedUploadFile{}, err
	}
	defer os.Remove(mergedPath)

	if session.Hash != "" && session.Hash != hash {
		return resolvedUploadFile{}, fmt.Errorf("文件校验失败，请重新上传")
	}
	detectedMime, err := detectUploadFileMime(mergedPath, session.Name, session.Mime)
	if err != nil {
		return resolvedUploadFile{}, err
	}
	if err = validateUploadStoredFile(rule, session.Name, detectedMime); err != nil {
		return resolvedUploadFile{}, err
	}
	session.Size = size
	session.Mime = detectedMime
	session.Ext = resolveUploadExt(session.Name, detectedMime)
	session.Kind = resolveUploadKind(session.Kind, session.Name, detectedMime)

	objectKey := buildUploadObjectKey(rule.ID, hash, session.Ext, session.BizKey)
	session.Hash = hash
	session.ObjectKey = objectKey
	fileRecord, err := storeNewUpload(ctx, storeUploadInput{
		Rule:      rule,
		Session:   session,
		LocalPath: mergedPath,
		Hash:      hash,
	})
	if err != nil {
		return resolvedUploadFile{}, err
	}
	session.Name = fileRecord.Name
	session.ProviderKey = fileRecord.ProviderKey
	return finalizeUploadResult(ctx, session, fileRecord)
}

func finalizeUploadResult(
	ctx context.Context,
	session resolvedUploadSession,
	fileRecord resolvedUploadFile,
) (resolvedUploadFile, error) {
	if err := uploadrepo.UpdateUploadSession(ctx, session.ID, map[string]any{
		"hash":         session.Hash,
		"kind":         session.Kind,
		"name":         session.Name,
		"ext":          session.Ext,
		"mime":         session.Mime,
		"size":         session.Size,
		"object_key":   session.ObjectKey,
		"provider_key": session.ProviderKey,
		"status":       uploadSessionComplete,
	}); err != nil {
		return resolvedUploadFile{}, err
	}
	return fileRecord, nil
}

func persistUploadFile(ctx context.Context, rule resolvedUploadRule, session resolvedUploadSession, hash, objectKey, providerKey string) (resolvedUploadFile, error) {
	fileModel, err := uploadrepo.ResolveFileModel()
	if err != nil {
		return resolvedUploadFile{}, err
	}
	if existing := uploadrepo.FindUploadFileByPath(ctx, objectKey); existing != nil {
		if err := updateUploadFileRelationMetaIfNeeded(ctx, *existing, session.BizID, session.CategoryID); err == nil {
			if refreshed, refreshErr := uploadrepo.FindUploadFile(ctx, existing.ID); refreshErr == nil {
				return refreshed, nil
			}
		}
		return *existing, nil
	}

	storageID := session.StorageID
	if storageID == 0 {
		storageID = rule.StorageID
	}

	fileID := uint64(fileModel.Insert(ctx, map[string]any{
		"rule_id":      rule.ID,
		"storage_id":   storageID,
		"kind":         session.Kind,
		"biz_id":       session.BizID,
		"category_id":  session.CategoryID,
		"name":         session.Name,
		"ext":          session.Ext,
		"mime":         session.Mime,
		"size":         session.Size,
		"hash":         hash,
		"path":         objectKey,
		"provider_key": strings.TrimSpace(providerKey),
		"created_at":   time.Now(),
	}))
	if fileID == 0 {
		if existing := uploadrepo.FindUploadFileByPath(ctx, objectKey); existing != nil {
			return reuseExistingUploadFile(ctx, *existing, session.BizID, session.CategoryID), nil
		}
		return resolvedUploadFile{}, fmt.Errorf("保存上传文件失败")
	}
	return uploadrepo.FindUploadFile(ctx, fileID)
}

func resolveSavedProviderKey(result uploadprovider.SaveResult, objectKey string) string {
	if providerKey := strings.TrimSpace(result.ProviderKey); providerKey != "" {
		return providerKey
	}
	return strings.TrimSpace(objectKey)
}

func updateUploadFileRelationMetaIfNeeded(ctx context.Context, file resolvedUploadFile, bizID, categoryID uint64) error {
	updates := map[string]any{}
	if bizID != 0 && file.BizID == 0 {
		updates["biz_id"] = bizID
	}
	if categoryID != 0 && file.CategoryID == 0 {
		updates["category_id"] = categoryID
	}
	if len(updates) == 0 {
		return nil
	}

	fileModel, err := uploadrepo.ResolveFileModel()
	if err != nil {
		return err
	}
	fileModel.Update(ctx, map[string]any{"id": file.ID}, updates)
	return nil
}
