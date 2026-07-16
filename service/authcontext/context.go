package authcontext

import (
	"context"
	"strings"
)

const ActorTypeAdmin = "admin"

type Actor struct {
	Type string
	ID   uint64
}

type actorContextKey struct{}

// WithActor carries a trusted process-local actor into durable background
// services. It deliberately accepts an ID, never a bearer token or cookie.
func WithActor(ctx context.Context, actor Actor) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	actor.Type = strings.TrimSpace(actor.Type)
	if actor.ID == 0 {
		return ctx
	}
	return context.WithValue(ctx, actorContextKey{}, actor)
}

func FromContext(ctx context.Context) (Actor, bool) {
	if ctx == nil {
		return Actor{}, false
	}
	actor, ok := ctx.Value(actorContextKey{}).(Actor)
	return actor, ok && actor.ID > 0
}

func AdminID(ctx context.Context) (uint64, bool) {
	actor, ok := FromContext(ctx)
	return actor.ID, ok && strings.EqualFold(actor.Type, ActorTypeAdmin)
}
