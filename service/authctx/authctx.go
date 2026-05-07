package authctx

import (
	"context"

	deverjwt "github.com/shemic/dever/auth/jwt"
)

func OptionalUID(ctx context.Context) int64 {
	if uid, ok := deverjwt.ActiveInt64(ctx); ok && uid > 0 {
		return uid
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
