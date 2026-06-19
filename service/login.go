package service

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	deverjwt "github.com/shemic/dever/auth/jwt"
	"github.com/shemic/dever/config"
	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	operationlog "github.com/dever-package/front/service/operationlog"
	permissionservice "github.com/dever-package/front/service/permission"
	frontrecord "github.com/dever-package/front/service/record"
	"github.com/dever-package/front/service/siteconfig"
)

func Login(c *server.Context) error {
	payload := map[string]any{}
	if err := c.BindJSON(&payload); err != nil {
		return c.Error("请求体格式错误")
	}

	account := util.ToStringTrimmed(payload["account"])
	password := util.ToStringTrimmed(payload["password"])
	if account == "" || password == "" {
		return c.Error("账户和密码不能为空")
	}

	accountModel := frontrecord.Resolve("front.NewAccountModel")
	if accountModel == nil {
		return c.Error("账户模型未注册")
	}

	if err := permissionservice.EnsureDefaultAccount(c.Context(), account, password, HashPassword); err != nil {
		return c.Error(err)
	}

	accountRow := accountModel.FindMap(c.Context(), map[string]any{"account": account})
	if len(accountRow) == 0 || !verifyAccountPassword(c.Context(), accountModel, accountRow, password) {
		return c.Error("账户或密码错误")
	}

	accountID := util.ToUint64(accountRow["id"])
	if accountID == 0 {
		return c.Error("账户信息异常")
	}
	roleNames := resolveAccountRoleNames(
		c.Context(),
		permissionservice.ResolveAccountRoleIDs(c.Context(), accountID),
	)
	site, ok := siteconfig.FromContext(c.Context())
	if !ok {
		site, _ = siteconfig.MustLoad().FindBySiteKey(siteconfig.DefaultSiteKey)
	}
	expiredAt := time.Now().Add(7 * 24 * time.Hour)
	token, err := createLoginToken(accountID, expiredAt, site)
	if err != nil {
		return c.Error(err)
	}
	operationlog.RecordForAccount(c, accountRow, operationlog.Entry{
		Action:   "login",
		PagePath: "front/auth/login",
		TargetID: fmt.Sprint(accountRow["id"]),
		Message:  "管理员登录",
	})

	return c.JSON(map[string]any{
		"token": token,
		"user": map[string]any{
			"id":      accountID,
			"name":    fmt.Sprint(accountRow["name"]),
			"account": fmt.Sprint(accountRow["account"]),
			"role":    roleNames,
			"exp":     expiredAt.UnixMilli(),
		},
	})
}

func createLoginToken(uid uint64, expiredAt time.Time, site siteconfig.Site) (string, error) {
	cfg, err := config.Load("")
	if err != nil {
		return "", fmt.Errorf("读取配置失败")
	}

	signer, err := deverjwt.ResolveSigner(cfg.Auth, loginTokenSignerPreferences(site)...)
	if err != nil {
		return "", fmt.Errorf("JWT密钥未配置")
	}

	uidText := fmt.Sprintf("%d", uid)
	claims := jwt.MapClaims{
		"uid":   uidText,
		"sub":   uidText,
		"site":  site.Key,
		"scope": site.Access.AuthProvider,
		"exp":   expiredAt.Unix(),
		"iat":   time.Now().Unix(),
	}
	applyActorClaims(claims, signer.ClaimKeys, uidText)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(signer.Secret))
}

func applyActorClaims(claims jwt.MapClaims, claimKeys []string, value string) {
	for _, key := range claimKeys {
		key = util.ToStringTrimmed(key)
		if key == "" || isReservedLoginClaim(key) {
			continue
		}
		claims[key] = value
	}
}

func isReservedLoginClaim(key string) bool {
	switch key {
	case "exp", "iat", "nbf", "site", "scope":
		return true
	default:
		return false
	}
}

func loginTokenSignerPreferences(site siteconfig.Site) []string {
	preferred := make([]string, 0, 4)
	if provider := util.ToStringTrimmed(site.Access.AuthProvider); provider != "" {
		preferred = append(preferred, provider)
	}
	if siteKey := util.ToStringTrimmed(site.Key); siteKey != "" {
		preferred = append(preferred, siteKey)
	}
	return append(preferred, "front", "default")
}

func resolveAccountRoleNames(ctx context.Context, roleIDs []uint64) []string {
	if len(roleIDs) == 0 {
		return []string{}
	}

	roleModel := frontrecord.Resolve("front.NewRoleModel")
	if roleModel == nil {
		return []string{}
	}

	rows := roleModel.SelectMap(ctx, map[string]any{"id": roleIDs}, map[string]any{
		"field": "main.id, main.name",
	})
	nameByID := make(map[uint64]string, len(rows))
	for _, row := range rows {
		roleID := util.ToUint64(row["id"])
		if roleID == 0 {
			continue
		}
		nameByID[roleID] = util.ToStringTrimmed(row["name"])
	}

	result := make([]string, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		if name := nameByID[roleID]; name != "" {
			result = append(result, name)
		}
	}
	return result
}
