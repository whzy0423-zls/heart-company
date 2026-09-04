package db

import (
	"os"
	"strings"
	"testing"
)

func socialSchema(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(string(raw)), " ")
}

func TestSchemaIncludesAppSocialProfileFields(t *testing.T) {
	schema := socialSchema(t)
	for _, fragment := range []string{
		"ALTER TABLE app_users ADD COLUMN IF NOT EXISTS user_code TEXT",
		"ALTER TABLE app_users ADD COLUMN IF NOT EXISTS invite_code TEXT",
		"ALTER TABLE app_users ADD COLUMN IF NOT EXISTS personality_visibility TEXT NOT NULL DEFAULT 'friends'",
		"ALTER TABLE app_users ADD COLUMN IF NOT EXISTS personality_visibility_version BIGINT NOT NULL DEFAULT 1",
		"idx_app_users_user_code_unique",
		"idx_app_users_invite_code_unique",
		"CHECK (personality_visibility IN ('private', 'friends'))",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema is missing social profile contract %q", fragment)
		}
	}
}

func TestSchemaIncludesFriendRelationships(t *testing.T) {
	schema := socialSchema(t)
	for _, table := range []string{"friend_requests", "friendships", "user_blocks", "user_reports"} {
		if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("schema is missing %s", table)
		}
	}
	for _, fragment := range []string{
		"requester_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE",
		"addressee_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE",
		"user_low_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE",
		"user_high_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE",
		"CHECK (user_low_id < user_high_id)",
		"CHECK (requester_id <> addressee_id)",
		"status TEXT NOT NULL CHECK (status IN ('pending', 'accepted', 'rejected', 'cancelled'))",
		"status TEXT NOT NULL CHECK (status IN ('active', 'deleted'))",
		"idx_friend_requests_incoming",
		"idx_friend_requests_outgoing",
		"idx_friendships_user_low",
		"idx_friendships_user_high",
		"idx_user_blocks_lookup",
		"idx_user_reports_reported",
		"WHERE status = 'pending'",
		"WHERE status = 'active'",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("schema is missing friendship contract %q", fragment)
		}
	}
}

func TestSchemaPreservesDeletedFriendshipHistory(t *testing.T) {
	schema := socialSchema(t)
	if !strings.Contains(schema, "deleted_at TIMESTAMPTZ") {
		t.Fatal("friendships must retain deletion timestamp")
	}
	if !strings.Contains(schema, "idx_friendships_active_pair") {
		t.Fatal("friendships must have an active pair uniqueness index")
	}
	if !strings.Contains(schema, "UNIQUE (requester_id, addressee_id, status)") {
		t.Fatal("friend requests must support state transitions without duplicate rows")
	}
}
