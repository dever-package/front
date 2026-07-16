package authctx

import (
	"context"

	frontauthcontext "github.com/dever-package/front/service/authcontext"
	deverjwt "github.com/shemic/dever/auth/jwt"
)

func OptionalUID(ctx context.Context) int64 {
	if uid, ok := deverjwt.ActiveInt64(ctx); ok && uid > 0 {
		return uid
	}
	if actorID, ok := frontauthcontext.AdminID(ctx); ok {
		return int64(actorID)
	}
	return 0
}

func RequireUID(ctx context.Context) int64 {
	uid := OptionalUID(ctx)
	if uid <= 0 {
		panic("用户信息不正确")
	}
	return uid
}
