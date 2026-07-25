package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestParseCLIRequiresExplicitWriteGates(t *testing.T) {
	if _, err := parseCLI([]string{"stage", "--package", "/tmp/package", "--actor", "1"}); err == nil {
		t.Fatal("stage without --apply must fail")
	}
	command, err := parseCLI([]string{"stage", "--package", "/tmp/package", "--apply", "--actor", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if command.name != "stage" || command.packagePath != "/tmp/package" || command.actorID != 1 || !command.apply {
		t.Fatalf("command = %+v", command)
	}
}

func TestParseCLIRejectsDSNArgumentsAndRequiresDryRun(t *testing.T) {
	if _, err := parseCLI([]string{"plan", "--package", "/tmp/package"}); err == nil {
		t.Fatal("plan without --dry-run must fail")
	}
	_, err := parseCLI([]string{"plan", "--package", "/tmp/package", "--dry-run", "--database-url", "postgres://user:super-secret@db/name"})
	if err == nil {
		t.Fatal("DSN CLI argument must be rejected")
	}
	if strings.Contains(err.Error(), "super-secret") || strings.Contains(err.Error(), "postgres://") {
		t.Fatalf("parse error leaked DSN: %v", err)
	}
}

func TestRunRedactsEnvironmentDSNFailures(t *testing.T) {
	secret := "postgres://user:environment-secret@%/broken"
	err := run(context.Background(), []string{"plan", "--package", "/tmp/missing", "--dry-run"}, func(string) string { return secret }, &bytes.Buffer{})
	if err == nil {
		t.Fatal("invalid DSN must fail")
	}
	if strings.Contains(err.Error(), "environment-secret") || strings.Contains(err.Error(), "postgres://") {
		t.Fatalf("runtime error leaked DSN: %v", err)
	}
}

func TestParseCLIReviewAndPromoteUseDatabaseUserIDs(t *testing.T) {
	review, err := parseCLI([]string{"review", "--package-id", "xinzhili-round-001", "--type", "safety-review", "--reviewer", "8"})
	if err != nil || review.reviewerID != 8 {
		t.Fatalf("review=%+v err=%v", review, err)
	}
	promote, err := parseCLI([]string{"promote", "--package-id", "xinzhili-round-001", "--actor", "9"})
	if err != nil || promote.actorID != 9 {
		t.Fatalf("promote=%+v err=%v", promote, err)
	}
}
