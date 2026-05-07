package service

import (
	"github.com/shemic/dever/server"
	"github.com/shemic/dever/util"

	authctx "my/package/front/service/authctx"
	frontrecord "my/package/front/service/record"
)

type ProfileService struct{}

func (ProfileService) ProviderLoadCurrentAccountProfile(c *server.Context, _ []any) any {
	accountModel := frontrecord.Resolve("front.NewAccountModel")
	if accountModel == nil {
		panic("账户模型未注册")
	}

	accountID := uint64(authctx.RequireUID(c.Context()))
	accountRow := accountModel.FindMap(c.Context(), map[string]any{"id": accountID})
	if len(accountRow) == 0 {
		panic("当前账户不存在")
	}

	return map[string]any{
		"name":     util.ToStringTrimmed(accountRow["name"]),
		"password": "",
	}
}

func (ProfileService) ProviderBeforeSaveAccountProfile(c *server.Context, params []any) any {
	record := profilePayload(params)
	name := util.ToStringTrimmed(record["name"])
	if name == "" {
		panic("姓名不能为空")
	}

	sanitized := map[string]any{
		"id":   uint64(authctx.RequireUID(c.Context())),
		"name": name,
	}

	password := util.ToStringTrimmed(record["password"])
	if password != "" {
		if len([]rune(password)) < 6 {
			panic("密码至少 6 位")
		}
		sanitized["password"] = password
	}

	return sanitized
}

func profilePayload(params []any) map[string]any {
	if len(params) == 0 {
		return map[string]any{}
	}

	record, _ := params[0].(map[string]any)
	if record == nil {
		return map[string]any{}
	}

	return util.CloneMap(record)
}
