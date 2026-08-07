package upload

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	uploadprovider "github.com/dever-package/front/service/upload/provider"
	uploadrepo "github.com/dever-package/front/service/upload/repository"
)

type storeUploadInput struct {
	Rule      resolvedUploadRule
	Session   resolvedUploadSession
	LocalPath string
	Hash      string
	Progress  func(loaded int64, total int64)
}

type uploadStoreMutex struct {
	mutex sync.Mutex
	refs  int
}

var uploadStoreMutexes = struct {
	sync.Mutex
	items map[string]*uploadStoreMutex
}{
	items: make(map[string]*uploadStoreMutex),
}

func storeNewUpload(ctx context.Context, input storeUploadInput) (resolvedUploadFile, error) {
	session := input.Session
	session.Hash = normalizeUploadHash(input.Hash)
	if strings.TrimSpace(session.ObjectKey) == "" {
		return resolvedUploadFile{}, fmt.Errorf("上传文件缺少对象路径")
	}

	unlockObject := lockUploadStoreKey("object:" + session.ObjectKey)
	defer unlockObject()
	if existing := uploadrepo.FindUploadFileByPath(ctx, session.ObjectKey); existing != nil {
		return reuseExistingUploadFile(ctx, *existing, session.BizID, session.CategoryID), nil
	}

	resolvedName, unlockName, err := lockAvailableUploadName(
		ctx,
		input.Rule.ID,
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

	driver, err := uploadprovider.Resolve(resolveUploadStorageProvider(input.Rule.Storage))
	if err != nil {
		return resolvedUploadFile{}, err
	}
	sourceKey, sourceName := resolveUploadSource(input.Rule, session)
	providerTarget := resolveUploadProviderTarget(
		input.Rule.Storage,
		session.ObjectKey,
		sourceName,
		resolvedName.Candidates,
	)
	if providerTarget.PrimaryObjectKey() == "" {
		return resolvedUploadFile{}, fmt.Errorf("上传文件缺少存储路径")
	}
	saveResult, err := uploadprovider.Save(ctx, driver, uploadprovider.SaveInput{
		Rule: uploadprovider.Rule{
			Storage:      input.Rule.Storage,
			MimeLimit:    resolveUploadProviderMimeLimit(input.Rule.Accept),
			MaxSizeBytes: uploadRuleMaxSizeBytes(input.Rule),
		},
		Session: uploadprovider.Session{
			ObjectKey: session.ObjectKey,
		},
		LocalPath:           input.LocalPath,
		ObjectKey:           providerTarget.PrimaryObjectKey(),
		ObjectKeyCandidates: append([]string(nil), providerTarget.ObjectKeys...),
		Name:                session.Name,
		NameCandidates:      append([]string(nil), resolvedName.Candidates...),
		SourceKey:           sourceKey,
		SourceName:          sourceName,
		Mime:                session.Mime,
		Size:                session.Size,
		Hash:                session.Hash,
		Ext:                 session.Ext,
		PathMode:            providerTarget.PathMode,
		Progress:            input.Progress,
	})
	if err != nil {
		return resolvedUploadFile{}, err
	}

	session.Name, err = resolveStoredUploadName(resolvedName, saveResult.StoredName)
	if err != nil {
		return resolvedUploadFile{}, err
	}
	return persistUploadFile(
		ctx,
		input.Rule,
		session,
		session.Hash,
		session.ObjectKey,
		resolveSavedProviderKey(saveResult, providerTarget.PrimaryObjectKey()),
	)
}

func resolveStoredUploadName(resolved resolvedUploadName, storedName string) (string, error) {
	storedName = strings.TrimSpace(storedName)
	if storedName == "" {
		return resolved.Name, nil
	}
	for _, candidate := range resolved.Candidates {
		if storedName == candidate {
			return storedName, nil
		}
	}
	return "", fmt.Errorf("存储返回了无效的文件名称")
}

func resolveUploadSource(rule resolvedUploadRule, session resolvedUploadSession) (string, string) {
	if session.BizID != 0 {
		key := strings.TrimSpace(session.BizKey)
		if key == "" {
			key = strconv.FormatUint(session.BizID, 10)
		}
		name := strings.TrimSpace(session.BizName)
		if name == "" {
			name = key
		}
		return "biz:" + key, name
	}

	key := "rule:" + strconv.FormatUint(rule.ID, 10)
	name := strings.TrimSpace(rule.Name)
	if name == "" {
		name = "上传规则" + strconv.FormatUint(rule.ID, 10)
	}
	return key, name
}

func reuseExistingUploadFile(ctx context.Context, existing resolvedUploadFile, bizID, categoryID uint64) resolvedUploadFile {
	if err := updateUploadFileRelationMetaIfNeeded(ctx, existing, bizID, categoryID); err == nil {
		if refreshed, refreshErr := uploadrepo.FindUploadFile(ctx, existing.ID); refreshErr == nil {
			return refreshed
		}
	}
	return existing
}

func uploadNameLockKey(ruleID, bizID uint64, name string) string {
	if bizID != 0 {
		return "name:biz:" + strconv.FormatUint(bizID, 10) + ":" + strings.ToLower(name)
	}
	return "name:rule:" + strconv.FormatUint(ruleID, 10) + ":" + strings.ToLower(name)
}

func lockUploadStoreKey(key string) func() {
	uploadStoreMutexes.Lock()
	entry := uploadStoreMutexes.items[key]
	if entry == nil {
		entry = &uploadStoreMutex{}
		uploadStoreMutexes.items[key] = entry
	}
	entry.refs++
	uploadStoreMutexes.Unlock()

	entry.mutex.Lock()
	return func() {
		entry.mutex.Unlock()
		uploadStoreMutexes.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(uploadStoreMutexes.items, key)
		}
		uploadStoreMutexes.Unlock()
	}
}
