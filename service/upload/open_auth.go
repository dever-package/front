package upload

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	authctx "my/package/front/service/internal/authctx"
	uploadaccess "my/package/front/service/upload/access"
	"my/package/front/service/upload/openurl"
	uploadrepo "my/package/front/service/upload/repository"
)

const (
	uploadOpenDefaultTTL = openurl.DefaultTTL
	uploadOpenMaxTTL     = openurl.MaxTTL
)

type uploadOpenSignInput struct {
	ID        uint64 `json:"id"`
	TTL       int64  `json:"ttl"`
	ExpiresIn int64  `json:"expires_in"`
}

func SignUploadOpen(c *server.Context) error {
	if !hasUploadOpenActor(c) {
		return c.Error("请先登录", http.StatusUnauthorized)
	}

	input, err := parseUploadOpenSignInput(c)
	if err != nil {
		return c.Error(err)
	}
	fileRecord, err := uploadrepo.FindUploadFile(c.Context(), input.ID)
	if err != nil {
		return c.Error(err)
	}
	if err := uploadaccess.EnsureFile(c, uploadaccess.OperationSign, fileRecord); err != nil {
		return c.Error(err, uploadaccess.Status(err))
	}

	openURL, expiresAt, err := openurl.BuildSigned(input.ID, uploadOpenTTL(input))
	if err != nil {
		return c.Error(err)
	}
	return c.JSON(map[string]any{
		"id":         input.ID,
		"url":        openURL,
		"expires_at": expiresAt.Unix(),
		"ttl":        int64(time.Until(expiresAt).Seconds()),
	})
}

func ensureUploadOpenAccess(c *server.Context, file uploadrepo.UploadFile) error {
	if openurl.ValidateRequest(c) == nil {
		return nil
	}
	return uploadaccess.EnsureFile(c, uploadaccess.OperationRead, file)
}

func hasUploadOpenActor(c *server.Context) bool {
	return c != nil && authctx.OptionalUID(c.Context()) > 0
}

func parseUploadOpenSignInput(c *server.Context) (uploadOpenSignInput, error) {
	var input uploadOpenSignInput
	contentType := strings.ToLower(strings.TrimSpace(c.Header("Content-Type")))
	if strings.Contains(contentType, "application/json") {
		if err := c.BindJSON(&input); err != nil {
			return input, fmt.Errorf("请求体格式错误")
		}
	}
	if input.ID == 0 {
		input.ID = util.ToUint64(c.Input("id"))
	}
	if input.TTL <= 0 {
		input.TTL = util.ToInt64(c.Input("ttl"))
	}
	if input.ExpiresIn <= 0 {
		input.ExpiresIn = util.ToInt64(c.Input("expires_in"))
	}
	if input.ID == 0 {
		return input, fmt.Errorf("文件ID不能为空")
	}
	return input, nil
}

func uploadOpenTTL(input uploadOpenSignInput) time.Duration {
	seconds := input.ExpiresIn
	if seconds <= 0 {
		seconds = input.TTL
	}
	if seconds <= 0 {
		return uploadOpenDefaultTTL
	}
	ttl := time.Duration(seconds) * time.Second
	if ttl > uploadOpenMaxTTL {
		return uploadOpenMaxTTL
	}
	return ttl
}
