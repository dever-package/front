package upload

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	uploadprovider "my/package/front/service/upload/provider"
	uploadrepo "my/package/front/service/upload/repository"
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
	if existing := uploadrepo.FindUploadFileByPath(ctx, session.ObjectKey); existing != nil {
		return *existing, nil
	}
	if err := uploadrepo.UpdateUploadSession(ctx, session.ID, map[string]any{"status": uploadSessionComplete}); err != nil {
		return resolvedUploadFile{}, err
	}
	session.Status = uploadSessionComplete
	return persistUploadFile(ctx, rule, session, session.Hash, session.ObjectKey)
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
	session.Size = size

	objectKey := buildUploadObjectKey(rule.ID, hash, session.Ext, session.BizKey)
	if existing := uploadrepo.FindUploadFileByPath(ctx, objectKey); existing != nil {
		_ = updateUploadFileRelationMetaIfNeeded(ctx, *existing, session.BizID, session.CategoryID)
		if refreshed, err := uploadrepo.FindUploadFile(ctx, existing.ID); err == nil {
			return refreshed, nil
		}
		return *existing, nil
	}

	provider, err := uploadprovider.Resolve(resolveUploadStorageProvider(rule.Storage))
	if err != nil {
		return resolvedUploadFile{}, err
	}
	if err = provider.Save(ctx, uploadprovider.SaveInput{
		Rule: uploadprovider.Rule{
			Storage:      rule.Storage,
			Accept:       rule.Accept,
			MaxSizeBytes: uploadRuleMaxSizeBytes(rule),
		},
		Session: uploadprovider.Session{
			ObjectKey: session.ObjectKey,
		},
		LocalPath: mergedPath,
		ObjectKey: objectKey,
		Name:      session.Name,
		Mime:      session.Mime,
		Size:      size,
		Hash:      hash,
		Ext:       session.Ext,
	}); err != nil {
		return resolvedUploadFile{}, err
	}

	if err = uploadrepo.UpdateUploadSession(ctx, session.ID, map[string]any{
		"hash":       hash,
		"object_key": objectKey,
		"status":     uploadSessionComplete,
	}); err != nil {
		return resolvedUploadFile{}, err
	}

	session.Hash = hash
	session.ObjectKey = objectKey
	session.Status = uploadSessionComplete
	return persistUploadFile(ctx, rule, session, hash, objectKey)
}

func persistUploadFile(ctx context.Context, rule resolvedUploadRule, session resolvedUploadSession, hash, objectKey string) (resolvedUploadFile, error) {
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
		"rule_id":     rule.ID,
		"storage_id":  storageID,
		"kind":        session.Kind,
		"biz_id":      session.BizID,
		"category_id": session.CategoryID,
		"name":        session.Name,
		"ext":         session.Ext,
		"mime":        session.Mime,
		"size":        session.Size,
		"hash":        hash,
		"path":        objectKey,
		"created_at":  time.Now(),
	}))
	if fileID == 0 {
		return resolvedUploadFile{}, fmt.Errorf("保存上传文件失败")
	}
	return uploadrepo.FindUploadFile(ctx, fileID)
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
