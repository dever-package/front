package openurl

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	deverjwt "github.com/shemic/dever/auth/jwt"
	"github.com/shemic/dever/config"
	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	"github.com/dever-package/front/service/siteconfig"
)

const (
	SignatureParam = "sign"
	ExpiresParam   = "expires"
	DefaultTTL     = 5 * time.Minute
	MaxTTL         = 24 * time.Hour
)

var (
	signSecretOnce  sync.Once
	signSecretCache []byte
	signSecretErr   error
)

func IsSignedRequest(c *server.Context) bool {
	if c == nil || !siteconfig.IsFrontRuntimeAPIEndpoint(c.Path(), "upload/open") {
		return false
	}
	return ValidateRequest(c) == nil
}

func ValidateRequest(c *server.Context) error {
	fileID := util.ToUint64(c.Query("id"))
	expires, err := strconv.ParseInt(strings.TrimSpace(c.Query(ExpiresParam)), 10, 64)
	if fileID == 0 || err != nil || expires <= 0 {
		return fmt.Errorf("文件签名参数无效")
	}
	if time.Now().Unix() > expires {
		return fmt.Errorf("文件签名已过期")
	}

	actual := strings.TrimSpace(c.Query(SignatureParam))
	if actual == "" {
		actual = strings.TrimSpace(c.Query("signature"))
	}
	if actual == "" {
		return fmt.Errorf("文件签名不能为空")
	}

	expected, err := sign(fileID, expires)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(strings.ToLower(actual)), []byte(expected)) {
		return fmt.Errorf("文件签名无效")
	}
	return nil
}

func BuildSigned(fileID uint64, ttl time.Duration) (string, time.Time, error) {
	if fileID == 0 {
		return "", time.Time{}, fmt.Errorf("文件ID不能为空")
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if ttl > MaxTTL {
		ttl = MaxTTL
	}

	expiresAt := time.Now().Add(ttl).Truncate(time.Second)
	expires := expiresAt.Unix()
	signature, err := sign(fileID, expires)
	if err != nil {
		return "", time.Time{}, err
	}

	query := url.Values{}
	query.Set("id", strconv.FormatUint(fileID, 10))
	query.Set(ExpiresParam, strconv.FormatInt(expires, 10))
	query.Set(SignatureParam, signature)
	return siteconfig.FrontRuntimeAPIURL("upload/open", query), expiresAt, nil
}

func sign(fileID uint64, expires int64) (string, error) {
	secret, err := signSecret()
	if err != nil {
		return "", err
	}
	message := fmt.Sprintf("front-upload-open:v1:%d:%d", fileID, expires)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func signSecret() ([]byte, error) {
	signSecretOnce.Do(func() {
		signSecretCache, signSecretErr = loadSignSecret()
	})
	return signSecretCache, signSecretErr
}

func loadSignSecret() ([]byte, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("读取认证配置失败: %w", err)
	}
	signer, err := deverjwt.ResolveSigner(cfg.Auth, "front", "user", "default")
	if err != nil {
		return nil, fmt.Errorf("读取上传签名密钥失败: %w", err)
	}
	secret := strings.TrimSpace(signer.Secret)
	if secret == "" {
		return nil, fmt.Errorf("上传签名密钥未配置")
	}
	sum := sha256.Sum256([]byte(secret + ":front-upload-open"))
	return sum[:], nil
}
