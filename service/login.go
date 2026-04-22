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

	permissionservice "github.com/dever-package/front/service/permission"
	frontrecord "github.com/dever-package/front/service/record"
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

	if err := permissionservice.EnsureDefaultAccount(c.Context(), account, password, hashPlainPassword); err != nil {
		return c.Error(err)
	}

	accountRow := accountModel.FindMap(c.Context(), map[string]any{"account": account})
	if len(accountRow) == 0 || !verifyAccountPassword(c.Context(), accountModel, accountRow, password) {
		return c.Error("账户或密码错误")
	}

	roleNames := resolveAccountRoleNames(
		c.Context(),
		permissionservice.ResolveAccountRoleIDs(c.Context(), util.ToUint64(accountRow["id"])),
	)
	expiredAt := time.Now().Add(7 * 24 * time.Hour)
	token, err := createLoginToken(util.ToUint64(accountRow["id"]), expiredAt)
	if err != nil {
		return c.Error(err)
	}

	return c.JSON(map[string]any{
		"token": token,
		"user": map[string]any{
			"id":      util.ToUint64(accountRow["id"]),
			"name":    fmt.Sprint(accountRow["name"]),
			"account": fmt.Sprint(accountRow["account"]),
			"role":    roleNames,
			"exp":     expiredAt.UnixMilli(),
		},
	})
}

func createLoginToken(uid uint64, expiredAt time.Time) (string, error) {
	cfg, err := config.Load("")
	if err != nil {
		return "", fmt.Errorf("读取配置失败")
	}

	signer, err := deverjwt.ResolveSigner(cfg.Auth, "user", "default")
	if err != nil {
		return "", fmt.Errorf("JWT密钥未配置")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid": fmt.Sprintf("%d", uid),
		"exp": expiredAt.Unix(),
		"iat": time.Now().Unix(),
	})
	return token.SignedString([]byte(signer.Secret))
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
