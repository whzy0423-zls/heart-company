package appuser

import (
	"context"
	"os"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/db"
)

func TestRotateRefreshTokenRevokesOldTokenAtomically(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run appuser store integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := db.Open(ctx, dsn, "admin", "123456")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	store := NewStore(database)
	phone := "13800000014"
	user, err := store.FindOrCreateByPhone(ctx, phone)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	suffix := time.Now().UnixNano()
	oldHash := HashToken("old-refresh-token-" + time.Unix(0, suffix).String())
	newHash := HashToken("new-refresh-token-" + time.Unix(0, suffix).String())
	if err := store.CreateRefreshToken(ctx, user.ID, oldHash, "test-device", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	rotated, err := store.RotateRefreshToken(ctx, oldHash, newHash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("rotate refresh token: %v", err)
	}
	if rotated.AppUserID != user.ID || rotated.DeviceInfo != "test-device" {
		t.Fatalf("unexpected rotated token metadata: %+v", rotated)
	}

	oldToken, err := store.FindRefreshToken(ctx, oldHash)
	if err != nil {
		t.Fatalf("find old token: %v", err)
	}
	if !oldToken.Revoked {
		t.Fatal("expected old token to be revoked")
	}

	if _, err := store.RotateRefreshToken(ctx, oldHash, HashToken("another-refresh-token-"+time.Unix(0, suffix).String()), time.Now().Add(time.Hour)); err == nil {
		t.Fatal("expected second rotation of the same token to fail")
	}
}
