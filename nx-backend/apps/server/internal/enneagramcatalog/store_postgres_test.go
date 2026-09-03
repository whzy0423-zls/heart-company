package enneagramcatalog

import (
	"context"
	"os"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestCatalogImportPublishIsolationAndForwardRollbackPostgres(t *testing.T) {
	database, _ := testdb.OpenEnvIsolatedSchema(t, "enneagram_catalog")
	raw, err := os.ReadFile("../db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(raw)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	ctx := context.Background()
	var actorID int64
	if err := database.QueryRow(`INSERT INTO users(username,password_hash) VALUES ('enneagram-reviewer','test') RETURNING id`).Scan(&actorID); err != nil {
		t.Fatal(err)
	}

	store := NewStore(database)
	catalog := validCatalog(t)
	first, err := store.ImportCatalog(ctx, catalog, actorID)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 10 {
		t.Fatalf("expected ten imports, got %d", len(first))
	}
	for _, result := range first {
		if !result.Created {
			t.Fatalf("first import for %s was not created", result.LibraryKey)
		}
	}
	second, err := store.ImportCatalog(ctx, catalog, actorID)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range second {
		if result.Created {
			t.Fatalf("duplicate import created a draft for %s", result.LibraryKey)
		}
	}

	for _, index := range []int{0, 2, 3} {
		if err := store.SubmitReview(ctx, first[index].ImportID, actorID); err != nil {
			t.Fatal(err)
		}
		if err := store.Approve(ctx, first[index].ImportID, actorID, "approved"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Publish(ctx, first[index].ImportID, actorID); err != nil {
			t.Fatalf("publish %s: %v", first[index].LibraryKey, err)
		}
	}

	changed := validCatalog(t)
	changed.Packages[3].Dimensions[RequiredDimensions[0]][0].Text = "changed type three content"
	changed.Packages[3].ContentDigest = mustDigest(t, changed.Packages[3])
	changed.Manifest.Packages[3].ContentDigest = changed.Packages[3].ContentDigest
	imports, err := store.ImportCatalog(ctx, changed, actorID)
	if err != nil {
		t.Fatal(err)
	}
	if !imports[3].Created {
		t.Fatal("changed type three content must create a new draft")
	}
	if err := store.SubmitReview(ctx, imports[3].ImportID, actorID); err != nil {
		t.Fatal(err)
	}
	if err := store.Approve(ctx, imports[3].ImportID, actorID, "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(ctx, imports[3].ImportID, actorID); err != nil {
		t.Fatal(err)
	}

	versions := map[string]int{}
	rows, err := database.Query(`SELECT key,current_version FROM theory_libraries WHERE key IN ('enneagram-core','enneagram-type-02','enneagram-type-03')`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var key string
		var version int
		if err := rows.Scan(&key, &version); err != nil {
			t.Fatal(err)
		}
		versions[key] = version
	}
	rows.Close()
	if versions["enneagram-core"] != 1 || versions["enneagram-type-02"] != 1 || versions["enneagram-type-03"] != 2 {
		t.Fatalf("type publication changed unrelated versions: %#v", versions)
	}

	rollback, err := store.Rollback(ctx, "enneagram-type-03", 1, int(actorID))
	if err != nil {
		t.Fatal(err)
	}
	if rollback.Version != 3 {
		t.Fatalf("rollback must create forward version 3, got %d", rollback.Version)
	}
	var differences int
	if err := database.QueryRow(`
		SELECT count(*) FROM (
			(SELECT card_id,chunk_id FROM theory_release_cards WHERE release_id=(SELECT id FROM theory_library_releases WHERE library_id=$1 AND version=1)
			 EXCEPT
			 SELECT card_id,chunk_id FROM theory_release_cards WHERE release_id=$2)
			UNION ALL
			(SELECT card_id,chunk_id FROM theory_release_cards WHERE release_id=$2
			 EXCEPT
			 SELECT card_id,chunk_id FROM theory_release_cards WHERE release_id=(SELECT id FROM theory_library_releases WHERE library_id=$1 AND version=1))
		) difference
	`, rollback.LibraryID, rollback.ReleaseID).Scan(&differences); err != nil {
		t.Fatal(err)
	}
	if differences != 0 {
		t.Fatalf("forward rollback did not copy the old snapshot: %d differences", differences)
	}
}
