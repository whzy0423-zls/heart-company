package db

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func openAppKnowledgeSchemaTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, _ := testdb.OpenEnvIsolatedSchema(t, "app_knowledge")
	raw, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(raw)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return database
}

func TestAppChatKnowledgeBindingConcurrentEnableIsUnique(t *testing.T) {
	database := openAppKnowledgeSchemaTestDB(t)
	ctx := context.Background()
	var libraryID int64
	if err := database.QueryRowContext(ctx, `
		INSERT INTO theory_libraries(key,name,status)
		VALUES ('binding-concurrency','Binding concurrency','enabled') RETURNING id
	`).Scan(&libraryID); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errorsByWorker := make(chan error, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := database.ExecContext(ctx, `
				INSERT INTO app_chat_knowledge_bindings(layer_kind,enneagram_type,theory_library_id,status)
				VALUES ('enneagram_type',3,$1,'enabled')
			`, libraryID)
			errorsByWorker <- err
		}()
	}
	close(start)
	workers.Wait()
	close(errorsByWorker)

	successes := 0
	failures := 0
	for err := range errorsByWorker {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("expected one enabled binding and one conflict, got success=%d failure=%d", successes, failures)
	}
}

func TestAppChatKnowledgeTraceIsUniquePerAssistantMessage(t *testing.T) {
	database := openAppKnowledgeSchemaTestDB(t)
	ctx := context.Background()
	var userID, cardID, sessionID, messageID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES ('knowledge-trace') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_user_cards(app_user_id,enneagram) VALUES ($1,3) RETURNING id`, userID).Scan(&cardID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_chat_sessions(app_user_id,card_id) VALUES ($1,$2) RETURNING id`, userID, cardID).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_chat_messages(session_id,role,content) VALUES ($1,'assistant','answer') RETURNING id`, sessionID).Scan(&messageID); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO app_chat_knowledge_traces(session_id,assistant_message_id,card_id,enneagram_type,card_revision,layer_hits) VALUES ($1,$2,$3,3,1,'{}')`
	if _, err := database.ExecContext(ctx, insert, sessionID, messageID, cardID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, insert, sessionID, messageID, cardID); err == nil {
		t.Fatal("expected duplicate assistant trace to be rejected")
	}
}

func TestReleasedTheorySnapshotRejectsMappingChunkAndReleaseMutation(t *testing.T) {
	database := openAppKnowledgeSchemaTestDB(t)
	ctx := context.Background()
	var libraryID, releaseID, cardID, chunkID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO theory_libraries(key,name,status,current_version) VALUES ('immutable-release','Immutable','enabled',1) RETURNING id`).Scan(&libraryID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `
		INSERT INTO theory_cards(library_id,canonical_key,canonical_name,card_kind,epistemic_status,evidence_level,clinical_safety,authority_level,status)
		VALUES ($1,'card','Card','concept','source_text','moderate','general',3,'published') RETURNING id
	`, libraryID).Scan(&cardID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `
		INSERT INTO theory_chunks(library_id,card_id,chunk_key,chunk_kind,title,content,authority_level,evidence_level,clinical_safety,content_hash)
		VALUES ($1,$2,'chunk','card','Chunk','Body',3,'moderate','general',$3) RETURNING id
	`, libraryID, cardID, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").Scan(&chunkID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `
		INSERT INTO theory_library_releases(library_id,version,status,card_count,chunk_count)
		VALUES ($1,1,'ready',1,1) RETURNING id
	`, libraryID).Scan(&releaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO theory_release_cards(release_id,card_id,chunk_id) VALUES ($1,$2,$3)`, releaseID, cardID, chunkID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE theory_library_releases SET status='active' WHERE id=$1`, releaseID); err != nil {
		t.Fatal(err)
	}

	for name, query := range map[string]string{
		"mapping": `DELETE FROM theory_release_cards WHERE release_id=$1`,
		"chunk":   `UPDATE theory_chunks SET content='changed' WHERE id=$1`,
		"release": `DELETE FROM theory_library_releases WHERE id=$1`,
	} {
		t.Run(name, func(t *testing.T) {
			argument := releaseID
			if name == "chunk" {
				argument = chunkID
			}
			if _, err := database.ExecContext(ctx, query, argument); err == nil {
				t.Fatal("expected released snapshot mutation to fail")
			}
		})
	}
	if _, err := database.ExecContext(ctx, `UPDATE theory_library_releases SET status='retired' WHERE id=$1`, releaseID); err != nil {
		t.Fatalf("active release must be allowed to retire: %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE theory_library_releases SET status='active' WHERE id=$1`, releaseID); err == nil {
		t.Fatal("retired release must not be reactivated")
	}
}
