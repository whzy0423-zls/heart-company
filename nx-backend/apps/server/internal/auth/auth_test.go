package auth

import "testing"

func TestTokenKindRoundTrips(t *testing.T) {
	token, err := Sign(UserInfo{
		ID:        1,
		TokenKind: TokenKindBackend,
		Username:  "admin",
	}, "secret")
	if err != nil {
		t.Fatal(err)
	}

	user, ok := Verify(token, "secret")
	if !ok {
		t.Fatal("expected token to verify")
	}
	if user.TokenKind != TokenKindBackend {
		t.Fatalf("expected backend token kind, got %q", user.TokenKind)
	}
}

func TestBearerUserWithKindRejectsWrongKind(t *testing.T) {
	token, err := Sign(UserInfo{
		ID:        1,
		TokenKind: TokenKindApp,
		Username:  "app",
	}, "secret")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := BearerUserWithKind("Bearer "+token, "secret", TokenKindBackend); err == nil {
		t.Fatal("expected backend verifier to reject app token")
	}
}

func TestBearerUserWithKindAllowsLegacyBackendToken(t *testing.T) {
	token, err := Sign(UserInfo{
		ID:       1,
		Username: "admin",
	}, "secret")
	if err != nil {
		t.Fatal(err)
	}

	user, err := BearerUserWithKind("Bearer "+token, "secret", TokenKindBackend)
	if err != nil {
		t.Fatalf("expected legacy backend token to be accepted, got %v", err)
	}
	if user.ID != 1 {
		t.Fatalf("unexpected user id %d", user.ID)
	}
}

func TestBearerUserWithKindRejectsLegacyMiniappRoleAsBackend(t *testing.T) {
	token, err := Sign(UserInfo{
		ID:       1,
		Roles:    []string{"miniapp"},
		Username: "openid-1",
	}, "secret")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := BearerUserWithKind("Bearer "+token, "secret", TokenKindBackend); err == nil {
		t.Fatal("expected legacy miniapp token to be rejected by backend verifier")
	}
}
