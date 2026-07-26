package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode"
)

type feishuFileListResponse struct {
	feishuBaseResponse
	Data struct {
		Files         []feishuFolderFile `json:"files"`
		NextPageToken string             `json:"next_page_token"`
		HasMore       bool               `json:"has_more"`
	} `json:"data"`
}

type feishuFolderFile struct {
	Token string `json:"token"`
	Name  string `json:"name"`
	Type  string `json:"type"`
}

type feishuFolderPage struct {
	Files         []feishuFolderFile
	NextPageToken string
	HasMore       bool
}

type feishuCreateFolderResponse struct {
	feishuBaseResponse
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
}

func (client *feishuClient) ensureSourceFolder(
	ctx context.Context,
	accessToken string,
	sourceKey string,
	sourceName string,
) (string, error) {
	sourceName = normalizeFeishuFolderName(sourceName)
	sourceKey = strings.TrimSpace(sourceKey)
	if sourceKey == "" {
		sourceKey = sourceName
	}
	return client.ensureChildFolder(
		ctx,
		accessToken,
		client.config.RootFolderToken,
		"source:"+sourceKey,
		sourceName,
	)
}

func (client *feishuClient) ensureHashFolder(
	ctx context.Context,
	accessToken string,
	objectKey string,
) (string, string, error) {
	fileName := path.Base(strings.ReplaceAll(strings.TrimSpace(objectKey), "\\", "/"))
	if !isValidFeishuEntryName(fileName) {
		return "", "", fmt.Errorf("飞书云盘哈希文件名称无效")
	}
	stem := strings.TrimSuffix(fileName, path.Ext(fileName))
	if len(stem) < 2 {
		return "", "", fmt.Errorf("飞书云盘哈希文件标识无效")
	}

	hashRootToken, err := client.ensureChildFolder(
		ctx,
		accessToken,
		client.config.RootFolderToken,
		"layout:hash",
		"_hash",
	)
	if err != nil {
		return "", "", err
	}
	shard := strings.ToLower(stem[:2])
	folderToken, err := client.ensureChildFolder(
		ctx,
		accessToken,
		hashRootToken,
		"layout:hash:"+shard,
		shard,
	)
	if err != nil {
		return "", "", err
	}
	return folderToken, fileName, nil
}

func (client *feishuClient) ensureChildFolder(
	ctx context.Context,
	accessToken string,
	parentToken string,
	stableKey string,
	displayName string,
) (string, error) {
	displayName = normalizeFeishuFolderName(displayName)
	cacheKey := strings.TrimSpace(parentToken) + "\x00" + strings.TrimSpace(stableKey) + "\x00" + displayName

	client.folderState.folderMu.Lock()
	defer client.folderState.folderMu.Unlock()

	if token := strings.TrimSpace(client.folderState.folderTokens[cacheKey]); token != "" {
		return token, nil
	}
	if token, err := client.findFolderToken(ctx, accessToken, parentToken, displayName); err != nil {
		return "", err
	} else if token != "" {
		client.folderState.folderTokens[cacheKey] = token
		return token, nil
	}

	token, err := client.createFolder(ctx, accessToken, parentToken, displayName)
	if err != nil {
		if recoveredToken, recoveryErr := client.findFolderToken(ctx, accessToken, parentToken, displayName); recoveryErr == nil && recoveredToken != "" {
			client.folderState.folderTokens[cacheKey] = recoveredToken
			return recoveredToken, nil
		}
		return "", err
	}
	client.folderState.folderTokens[cacheKey] = token
	return token, nil
}

func (client *feishuClient) findFolderToken(ctx context.Context, accessToken, parentToken, name string) (string, error) {
	return client.findNamedChildToken(ctx, accessToken, parentToken, name, func(file feishuFolderFile) bool {
		return strings.EqualFold(strings.TrimSpace(file.Type), "folder")
	})
}

func (client *feishuClient) findFileToken(ctx context.Context, accessToken, parentToken, name string) (string, error) {
	return client.findNamedChildToken(ctx, accessToken, parentToken, name, func(file feishuFolderFile) bool {
		return !strings.EqualFold(strings.TrimSpace(file.Type), "folder")
	})
}

func (client *feishuClient) findAvailableFileName(
	ctx context.Context,
	accessToken string,
	parentToken string,
	candidates []string,
) (string, error) {
	occupied := make(map[string]struct{})
	err := client.visitFolderFiles(ctx, accessToken, parentToken, func(file feishuFolderFile) bool {
		if !strings.EqualFold(strings.TrimSpace(file.Type), "folder") {
			occupied[strings.TrimSpace(file.Name)] = struct{}{}
		}
		return true
	})
	if err != nil {
		return "", err
	}
	for _, candidate := range candidates {
		if _, exists := occupied[candidate]; !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("飞书云盘目标目录中没有可用文件名")
}

func (client *feishuClient) findNamedChildToken(
	ctx context.Context,
	accessToken string,
	parentToken string,
	name string,
	matchType func(feishuFolderFile) bool,
) (string, error) {
	name = strings.TrimSpace(name)
	found := ""
	err := client.visitFolderFiles(ctx, accessToken, parentToken, func(file feishuFolderFile) bool {
		if strings.TrimSpace(file.Name) != name || !matchType(file) {
			return true
		}
		found = strings.TrimSpace(file.Token)
		return found == ""
	})
	return found, err
}

func (client *feishuClient) visitFolderFiles(
	ctx context.Context,
	accessToken string,
	folderToken string,
	visit func(feishuFolderFile) bool,
) error {
	pageToken := ""
	for {
		page, err := client.listFolderPage(ctx, accessToken, folderToken, pageToken)
		if err != nil {
			return err
		}
		for _, file := range page.Files {
			if !visit(file) {
				return nil
			}
		}
		if !page.HasMore {
			return nil
		}
		nextPageToken := strings.TrimSpace(page.NextPageToken)
		if nextPageToken == "" || nextPageToken == pageToken {
			return fmt.Errorf("飞书云盘文件夹分页标记无效")
		}
		pageToken = nextPageToken
	}
}

func (client *feishuClient) listFolderPage(ctx context.Context, accessToken, folderToken, pageToken string) (feishuFolderPage, error) {
	query := url.Values{}
	query.Set("folder_token", strings.TrimSpace(folderToken))
	query.Set("page_size", "200")
	if pageToken = strings.TrimSpace(pageToken); pageToken != "" {
		query.Set("page_token", pageToken)
	}
	var response feishuFileListResponse
	if err := client.doDriveJSON(ctx, http.MethodGet, "/drive/v1/files?"+query.Encode(), accessToken, nil, &response, "读取目标文件夹"); err != nil {
		return feishuFolderPage{}, err
	}
	return feishuFolderPage{
		Files:         response.Data.Files,
		NextPageToken: strings.TrimSpace(response.Data.NextPageToken),
		HasMore:       response.Data.HasMore,
	}, nil
}

func (client *feishuClient) createFolder(ctx context.Context, accessToken, parentToken, name string) (string, error) {
	var response feishuCreateFolderResponse
	if err := client.doDriveJSON(ctx, http.MethodPost, "/drive/v1/files/create_folder", accessToken, map[string]any{
		"name":         strings.TrimSpace(name),
		"folder_token": strings.TrimSpace(parentToken),
	}, &response, "创建目录"); err != nil {
		return "", err
	}
	token := strings.TrimSpace(response.Data.Token)
	if token == "" {
		return "", fmt.Errorf("飞书云盘创建目录失败：未返回文件夹标识")
	}
	return token, nil
}

func normalizeFeishuFolderName(value string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(value) {
		switch {
		case unicode.IsControl(char):
			continue
		case strings.ContainsRune(`\\/:*?"<>|`, char):
			builder.WriteRune('_')
		default:
			builder.WriteRune(char)
		}
	}
	name := strings.TrimSpace(builder.String())
	if name == "" || name == "." || name == ".." {
		name = "未分类"
	}
	runes := []rune(name)
	if len(runes) > 240 {
		name = string(runes[:240])
	}
	return name
}
