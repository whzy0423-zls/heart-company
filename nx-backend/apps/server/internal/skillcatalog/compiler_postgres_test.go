package skillcatalog

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/testdb"
)

func TestRetireCatalogVersionAfterRollbackPostgres(t *testing.T) {
	database, ctx, catalog, skillID, releases, versions := catalogVersionStateFixture(t)
	store := NewStore(database)
	if _, err := store.applyCatalogVersionState(ctx, catalog, CatalogCommand{Action: "rollback", Version: "1.0.0"}); err != nil {
		t.Fatalf("rollback catalog: %v", err)
	}
	if _, err := store.applyCatalogVersionState(ctx, catalog, CatalogCommand{Action: "retire", Version: "1.0.1"}); err != nil {
		t.Fatalf("retire catalog after rollback: %v", err)
	}
	assertCatalogVersionState(t, ctx, database, skillID, releases, versions)
}

func TestRetireCurrentCatalogVersionRestoresImmutableSnapshotPostgres(t *testing.T) {
	database, ctx, catalog, skillID, releases, versions := catalogVersionStateFixture(t)
	var before string
	if err := database.QueryRowContext(ctx, `SELECT (to_jsonb(release)-'status'-'update_time')::text FROM theory_library_releases release WHERE id=$1`, releases[0]).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(database).applyCatalogVersionState(ctx, catalog, CatalogCommand{Action: "retire", Version: "1.0.1"}); err != nil {
		t.Fatalf("retire current catalog: %v", err)
	}
	assertCatalogVersionState(t, ctx, database, skillID, releases, versions)
	var after string
	if err := database.QueryRowContext(ctx, `SELECT (to_jsonb(release)-'status'-'update_time')::text FROM theory_library_releases release WHERE id=$1`, releases[0]).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("fallback changed immutable snapshot: before=%s after=%s", before, after)
	}
	if _, err := database.ExecContext(ctx, `UPDATE theory_library_releases SET chunk_count=chunk_count+1 WHERE id=$1`, releases[0]); err == nil {
		t.Fatal("fallback weakened immutable snapshot protection")
	}
	if _, err := database.ExecContext(ctx, `UPDATE theory_library_releases SET status='active' WHERE id=$1`, releases[1]); err == nil {
		t.Fatal("transaction-local rollback permission leaked")
	}
}

func catalogVersionStateFixture(t *testing.T) (*sql.DB, context.Context, BuiltinCatalog, int64, [2]int64, [2]int64) {
	t.Helper()
	database, _ := testdb.OpenEnvIsolatedSchema(t, "skill_catalog_state")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	schema, err := os.ReadFile(filepath.Join("..", "db", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	catalog := BuiltinCatalog{Key: "catalog-state", Categories: []BuiltinCategory{{Key: "category", Skills: []BuiltinSkill{{Key: "skill"}}}}}
	var libraryID, categoryID, skillID, theoryLibraryID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_skill_libraries(key,name,status) VALUES($1,$1,'enabled') RETURNING id`, catalog.Key).Scan(&libraryID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_skill_categories(library_id,key,name,status) VALUES($1,'category','category','enabled') RETURNING id`, libraryID).Scan(&categoryID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_skills(category_id,key,name,status) VALUES($1,'skill','skill','enabled') RETURNING id`, categoryID).Scan(&skillID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO theory_libraries(key,name,status,current_version) VALUES('catalog-state','catalog-state','enabled',2) RETURNING id`).Scan(&theoryLibraryID); err != nil {
		t.Fatal(err)
	}
	var releases, versions [2]int64
	for index, version := range []string{"1.0.0", "1.0.1"} {
		// Direct publication leaves activated_at NULL; fallback must preserve it.
		if err := database.QueryRowContext(ctx, `INSERT INTO theory_library_releases(library_id,version,status) VALUES($1,$2,'active') RETURNING id`, theoryLibraryID, index+1).Scan(&releases[index]); err != nil {
			t.Fatal(err)
		}
		if err := database.QueryRowContext(ctx, `INSERT INTO app_skill_versions(skill_id,version,instructions,theory_release_id,content_hash,status,published_at) VALUES($1,$2,'rules',$3,$2,'published',now()) RETURNING id`, skillID, version, releases[index]).Scan(&versions[index]); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			if _, err := database.ExecContext(ctx, `UPDATE theory_library_releases SET status='retired' WHERE id=$1`, releases[index]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_skills SET latest_published_version_id=$2 WHERE id=$1`, skillID, versions[1]); err != nil {
		t.Fatal(err)
	}
	return database, ctx, catalog, skillID, releases, versions
}

func assertCatalogVersionState(t *testing.T, ctx context.Context, database *sql.DB, skillID int64, releases, versions [2]int64) {
	t.Helper()
	var latest, current int64
	var skillStatus, libraryStatus string
	if err := database.QueryRowContext(ctx, `SELECT skill.latest_published_version_id,skill.status,library.current_version,library.status FROM app_skills skill JOIN app_skill_versions version ON version.id=skill.latest_published_version_id JOIN theory_library_releases release ON release.id=version.theory_release_id JOIN theory_libraries library ON library.id=release.library_id WHERE skill.id=$1`, skillID).Scan(&latest, &skillStatus, &current, &libraryStatus); err != nil {
		t.Fatal(err)
	}
	if latest != versions[0] || current != 1 || skillStatus != "enabled" || libraryStatus != "enabled" {
		t.Fatalf("fallback latest=%d current=%d skill=%s library=%s", latest, current, skillStatus, libraryStatus)
	}
	for index, want := range []string{"active", "retired"} {
		var releaseStatus, versionStatus string
		if err := database.QueryRowContext(ctx, `SELECT release.status,version.status FROM theory_library_releases release JOIN app_skill_versions version ON version.theory_release_id=release.id WHERE release.id=$1`, releases[index]).Scan(&releaseStatus, &versionStatus); err != nil {
			t.Fatal(err)
		}
		wantVersion := "published"
		if index == 1 {
			wantVersion = "retired"
		}
		if releaseStatus != want || versionStatus != wantVersion {
			t.Fatalf("version %d release=%s version=%s", index, releaseStatus, versionStatus)
		}
	}
}
