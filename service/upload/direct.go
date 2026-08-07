package upload

import (
	"context"
	"fmt"
	"strings"

	uploadprovider "github.com/dever-package/front/service/upload/provider"
)

func prepareDirectUpload(
	ctx context.Context,
	rule resolvedUploadRule,
	session resolvedUploadSession,
) (resolvedUploadSession, *uploadprovider.DirectInitResult, func(), error) {
	resolvedName, unlockName, err := lockAvailableDirectUploadName(
		ctx,
		rule.ID,
		session.BizID,
		session.Name,
		session.Ext,
		session.Hash,
	)
	if err != nil {
		return session, nil, nil, err
	}
	fail := func(err error) (resolvedUploadSession, *uploadprovider.DirectInitResult, func(), error) {
		unlockName()
		return session, nil, nil, err
	}

	_, sourceName := resolveUploadSource(rule, session)
	providerTarget := resolveUploadProviderTarget(
		rule.Storage,
		session.ObjectKey,
		sourceName,
		resolvedName.Candidates,
	)
	if providerTarget.PrimaryObjectKey() == "" {
		return fail(fmt.Errorf("上传文件缺少存储路径"))
	}
	driver, err := uploadprovider.Resolve(resolveUploadStorageProvider(rule.Storage))
	if err != nil {
		return fail(err)
	}
	direct, err := driver.InitDirect(ctx, uploadprovider.Rule{
		Storage:      rule.Storage,
		MimeLimit:    resolveUploadProviderMimeLimit(rule.Accept),
		MaxSizeBytes: uploadRuleMaxSizeBytes(rule),
	}, uploadprovider.Session{
		ObjectKey:           providerTarget.PrimaryObjectKey(),
		ObjectKeyCandidates: append([]string(nil), providerTarget.ObjectKeys...),
		NameCandidates:      append([]string(nil), resolvedName.Candidates...),
		PathMode:            providerTarget.PathMode,
	})
	if err != nil {
		return fail(err)
	}
	if direct == nil {
		return fail(fmt.Errorf("存储方式未返回直传配置"))
	}

	storedName, err := resolveStoredUploadName(resolvedName, direct.StoredName)
	if err != nil {
		return fail(err)
	}
	providerKey, err := resolveDirectProviderKey(direct.ProviderKey, providerTarget.ObjectKeys)
	if err != nil {
		return fail(err)
	}
	session.Name = storedName
	session.ProviderKey = providerKey
	return session, direct, unlockName, nil
}

func resolveDirectProviderKey(providerKey string, candidates []string) (string, error) {
	providerKey = strings.TrimSpace(providerKey)
	if providerKey == "" && len(candidates) > 0 {
		providerKey = strings.TrimSpace(candidates[0])
	}
	for _, candidate := range candidates {
		if providerKey == strings.TrimSpace(candidate) {
			return providerKey, nil
		}
	}
	return "", fmt.Errorf("存储方式返回了无效的文件路径")
}
