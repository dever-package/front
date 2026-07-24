package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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

func (client *feishuClient) ensureShardFolder(ctx context.Context, accessToken, shard string) (string, error) {
	client.folderState.folderMu.Lock()
	defer client.folderState.folderMu.Unlock()

	if !client.folderState.foldersLoaded {
		folders, err := client.loadShardFolders(ctx, accessToken)
		if err != nil {
			return "", err
		}
		client.folderState.folderTokens = folders
		client.folderState.foldersLoaded = true
	}
	if token := strings.TrimSpace(client.folderState.folderTokens[shard]); token != "" {
		return token, nil
	}

	token, err := client.createFolder(ctx, accessToken, client.config.RootFolderToken, shard)
	if err != nil {
		if recoveredToken, recoveryErr := client.findFolderToken(ctx, accessToken, client.config.RootFolderToken, shard); recoveryErr == nil && recoveredToken != "" {
			client.folderState.folderTokens[shard] = recoveredToken
			return recoveredToken, nil
		}
		return "", err
	}
	client.folderState.folderTokens[shard] = token
	return token, nil
}

func (client *feishuClient) loadShardFolders(ctx context.Context, accessToken string) (map[string]string, error) {
	result := make(map[string]string, 256)
	err := client.visitFolderFiles(ctx, accessToken, client.config.RootFolderToken, func(file feishuFolderFile) bool {
		name := strings.ToLower(strings.TrimSpace(file.Name))
		if strings.EqualFold(strings.TrimSpace(file.Type), "folder") && isFeishuShardName(name) {
			result[name] = strings.TrimSpace(file.Token)
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return result, nil
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
	}, &response, "创建存储分片目录"); err != nil {
		return "", err
	}
	token := strings.TrimSpace(response.Data.Token)
	if token == "" {
		return "", fmt.Errorf("飞书云盘创建存储分片目录失败：未返回文件夹标识")
	}
	return token, nil
}

func isFeishuShardName(value string) bool {
	if len(value) != 2 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
