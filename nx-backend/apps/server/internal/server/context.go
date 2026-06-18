package server

import (
	"context"
	"net/http"

	"nine-xing/nx-backend/apps/server/internal/auth"
)

type userContextKey struct{}

func withUser(ctx context.Context, user auth.UserInfo) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

func userFromRequest(r *http.Request) auth.UserInfo {
	user, _ := r.Context().Value(userContextKey{}).(auth.UserInfo)
	return user
}
