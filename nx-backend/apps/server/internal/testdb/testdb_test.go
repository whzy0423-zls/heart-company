package testdb

import "testing"

func TestParseSafeConfigRejectsUnsafeRoutesAndNames(t *testing.T) {
	for _, dsn := range []string{
		"postgres://postgres:secret@127.0.0.1/nx_test?host=remote.example",
		"postgres://postgres:secret@127.0.0.1/nx_test?hostaddr=203.0.113.10",
		"postgres://postgres:secret@127.0.0.1/nx_test?service=production",
		"host=127.0.0.1 dbname=nx_test service=production",
		"postgres://postgres:secret@127.0.0.1:5432,remote.example:5432/nx_test",
		"host=127.0.0.1,remote.example port=5432,5432 user=postgres password=secret dbname=nx_test",
		"postgres://postgres:secret@127.0.0.1/latest",
		"postgres://postgres:secret@127.0.0.1/contest",
	} {
		if _, err := ParseSafeConfig(dsn); err == nil {
			t.Errorf("unsafe TEST_DATABASE_URL accepted: %q", dsn)
		}
	}
}

func TestParseSafeConfigAcceptsIsolatedLoopbackTarget(t *testing.T) {
	config, err := ParseSafeConfig("postgres://postgres:secret@127.0.0.1:5432/nx_enterprise_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if config.Host != "127.0.0.1" || config.Database != "nx_enterprise_test" {
		t.Fatalf("final host=%q database=%q", config.Host, config.Database)
	}
}

func TestSchemaNameRejectsUnsafePrefix(t *testing.T) {
	if _, err := schemaName("enterprise-promotion", 1); err == nil {
		t.Fatal("unsafe schema prefix accepted")
	}
	if got, err := schemaName("enterprise_promotion", 42); err != nil || got != "enterprise_promotion_42" {
		t.Fatalf("schema name=%q err=%v", got, err)
	}
}
